import logging as log
import sys
import os
import signal
import ssl
import time
import json
import re
import base64
import uuid
import hmac
import hashlib
import random
from queue import Queue
from threading import Timer
from multiprocessing import Process
import requests
import paho.mqtt.client as mqtt
from prometheus_client import start_http_server, REGISTRY, Gauge, Counter


class RepeatTimer(Timer):
    def run(self):
        while not self.finished.wait(self.interval):
            self.function(*self.args, **self.kwargs)


class EcoflowMetricException(Exception):
    pass


class EcoflowAuthentication:
    def __init__(self, ecoflow_access_key, ecoflow_secret_key, ecoflow_api_host):
        self.ecoflow_access_key = ecoflow_access_key
        self.ecoflow_secret_key = ecoflow_secret_key
        self.ecoflow_api_host = ecoflow_api_host
        self.mqtt_url = None
        self.mqtt_port = None
        self.mqtt_username = None
        self.mqtt_password = None
        self.mqtt_client_id = None
        self.authorize()

    def generate_signature(self, params, nonce, timestamp):
        """Generate HMAC-SHA256 signature for EcoFlow API requests."""
        # Sort parameters by key
        sorted_params = sorted(params.items())
        # Create query string
        param_str = '&'.join([f"{k}={v}" for k, v in sorted_params])
        # Create message to sign: params&accessKey=value&nonce=value&timestamp=value
        if param_str:
            message = f"{param_str}&accessKey={self.ecoflow_access_key}&nonce={nonce}&timestamp={timestamp}"
        else:
            message = f"accessKey={self.ecoflow_access_key}&nonce={nonce}&timestamp={timestamp}"
        # Generate HMAC-SHA256 signature
        signature = hmac.new(
            self.ecoflow_secret_key.encode('utf-8'),
            message.encode('utf-8'),
            hashlib.sha256
        ).hexdigest()
        return signature

    def call_api(self, endpoint, params=None):
        """Make authenticated API call to EcoFlow Developer API."""
        if params is None:
            params = {}
        
        url = f"https://{self.ecoflow_api_host}{endpoint}"
        nonce = str(random.randint(100000, 999999))
        timestamp = str(int(time.time() * 1000))
        
        # Generate signature
        sign = self.generate_signature(params, nonce, timestamp)
        
        # Prepare headers
        headers = {
            "accessKey": self.ecoflow_access_key,
            "nonce": nonce,
            "timestamp": timestamp,
            "sign": sign,
            "Content-Type": "application/json;charset=UTF-8"
        }
        
        # Make request (GET for now, can be extended for POST)
        if params:
            response = requests.get(url, params=params, headers=headers)
        else:
            response = requests.get(url, headers=headers)
        
        return self.get_json_response(response)

    def authorize(self):
        """Get MQTT credentials using EcoFlow Developer API."""
        log.info(f"Authenticating with EcoFlow Developer API at {self.ecoflow_api_host}")
        
        # Get MQTT credentials from the certification endpoint
        endpoint = "/iot-open/sign/certification"
        
        try:
            response = self.call_api(endpoint)
            
            # Extract MQTT credentials
            self.mqtt_url = response["data"]["url"]
            self.mqtt_port = int(response["data"]["port"])
            self.mqtt_username = response["data"]["certificateAccount"]
            self.mqtt_password = response["data"]["certificatePassword"]
            # Generate a unique client ID
            self.mqtt_client_id = f"python-mqtt-{str(uuid.uuid4())[:8]}"
            
            log.info(f"Successfully obtained MQTT credentials")
            log.info(f"MQTT URL: {self.mqtt_url}:{self.mqtt_port}")
            log.info(f"MQTT Username: {self.mqtt_username}")
            
        except KeyError as key:
            raise Exception(f"Failed to extract key {key} from API response")
        except Exception as error:
            raise Exception(f"Failed to get MQTT credentials: {error}")

    def get_json_response(self, request):
        if request.status_code != 200:
            raise Exception(f"Got HTTP status code {request.status_code}: {request.text}")

        try:
            response = json.loads(request.text)
            response_message = response["message"]
        except KeyError as key:
            raise Exception(f"Failed to extract key {key} from {response}")
        except Exception as error:
            raise Exception(f"Failed to parse response: {request.text} Error: {error}")

        if response_message.lower() != "success":
            raise Exception(f"{response_message}")

        return response


class EcoflowMQTT():

    def __init__(self, message_queue, device_sn, username, password, addr, port, client_id, timeout_seconds):
        self.message_queue = message_queue
        self.addr = addr
        self.port = port
        self.username = username
        self.password = password
        self.client_id = client_id
        self.device_sn = device_sn
        # Use the Developer API MQTT topic format
        self.topic = f"/open/{username}/{device_sn}/quota"
        self.timeout_seconds = timeout_seconds
        self.last_message_time = None
        self.client = None

        self.connect()

        self.idle_timer = RepeatTimer(10, self.idle_reconnect)
        self.idle_timer.daemon = True
        self.idle_timer.start()

    def connect(self):
        if self.client:
            self.client.loop_stop()
            self.client.disconnect()

        self.client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, self.client_id)
        self.client.username_pw_set(self.username, self.password)
        self.client.tls_set(certfile=None, keyfile=None, cert_reqs=ssl.CERT_REQUIRED)
        self.client.tls_insecure_set(False)
        self.client.on_connect = self.on_connect
        self.client.on_disconnect = self.on_disconnect
        self.client.on_message = self.on_message

        log.info(f"Connecting to MQTT Broker {self.addr}:{self.port} using client id {self.client_id}")
        self.client.connect(self.addr, self.port)
        self.client.loop_start()

    def idle_reconnect(self):
        if self.last_message_time and time.time() - self.last_message_time > self.timeout_seconds:
            log.error(f"No messages received for {self.timeout_seconds} seconds. Reconnecting to MQTT")
            # We pull the following into a separate process because there are actually quite a few things that can go
            # wrong inside the connection code, including it just timing out and never returning. So this gives us a
            # measure of safety around reconnection
            while True:
                connect_process = Process(target=self.connect)
                connect_process.start()
                connect_process.join(timeout=60)
                connect_process.terminate()
                if connect_process.exitcode == 0:
                    log.info("Reconnection successful, continuing")
                    # Reset last_message_time here to avoid a race condition between idle_reconnect getting called again
                    # before on_connect() or on_message() are called
                    self.last_message_time = None
                    break
                else:
                    log.error("Reconnection errored out, or timed out, attempted to reconnect...")

    def on_connect(self, client, userdata, flags, reason_code, properties):
        # Initialize the time of last message at least once upon connection so that other things that rely on that to be
        # set (like idle_reconnect) work
        self.last_message_time = time.time()
        if reason_code == "Success":
            self.client.subscribe(self.topic)
            log.info(f"Subscribed to MQTT topic {self.topic}")
        elif reason_code == "Keep alive timeout":
            log.error("Failed to connect to MQTT: connection timed out")
        elif reason_code == "Unsupported protocol version":
            log.error("Failed to connect to MQTT: unsupported protocol version")
        elif reason_code == "Client identifier not valid":
            log.error("Failed to connect to MQTT: invalid client identifier")
        elif reason_code == "Server unavailable":
            log.error("Failed to connect to MQTT: server unavailable")
        elif reason_code == "Bad user name or password":
            log.error("Failed to connect to MQTT: bad username or password")
        elif reason_code == "Not authorized":
            log.error("Failed to connect to MQTT: not authorised")
        else:
            log.error(f"Failed to connect to MQTT: another error occured: {reason_code}")

        return client

    def on_disconnect(self, client, userdata, flags, reason_code, properties):
        if reason_code > 0:
            log.error(f"Unexpected MQTT disconnection: {reason_code}. Will auto-reconnect")
            time.sleep(5)

    def on_message(self, client, userdata, message):
        self.message_queue.put(message.payload.decode("utf-8"))
        self.last_message_time = time.time()


class EcoflowMetric:
    def __init__(self, ecoflow_payload_key, device_name):
        self.ecoflow_payload_key = ecoflow_payload_key
        self.device_name = device_name
        self.name = f"ecoflow_{self.convert_ecoflow_key_to_prometheus_name()}"
        self.metric = Gauge(self.name, f"value from MQTT object key {ecoflow_payload_key}", labelnames=["device"])

    def convert_ecoflow_key_to_prometheus_name(self):
        # bms_bmsStatus.maxCellTemp -> bms_bms_status_max_cell_temp
        # pd.ext4p8Port -> pd_ext4p8_port
        key = self.ecoflow_payload_key.replace('.', '_')
        new = key[0].lower()
        for character in key[1:]:
            if character.isupper() and not new[-1] == '_':
                new += '_'
            new += character.lower()
        # Check that metric name complies with the data model for valid characters
        # https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels
        if not re.match("[a-zA-Z_:][a-zA-Z0-9_:]*", new):
            raise EcoflowMetricException(f"Cannot convert payload key {self.ecoflow_payload_key} to comply with the Prometheus data model. Please, raise an issue!")
        return new

    def set(self, value):
        # According to best practices for naming metrics and labels, the voltage should be in volts and the current in amperes
        # WARNING! This will ruin all Prometheus historical data and backward compatibility of Grafana dashboard
        # value = value / 1000 if value.endswith("_vol") or value.endswith("_amp") else value
        log.debug(f"Set {self.name} = {value}")
        self.metric.labels(device=self.device_name).set(value)

    def clear(self):
        log.debug(f"Clear {self.name}")
        self.metric.clear()


class Worker:
    def __init__(self, message_queue, device_name, collecting_interval_seconds=10):
        self.message_queue = message_queue
        self.device_name = device_name
        self.collecting_interval_seconds = collecting_interval_seconds
        self.metrics_collector = []
        self.online = Gauge("ecoflow_online", "1 if device is online", labelnames=["device"])
        self.mqtt_messages_receive_total = Counter("ecoflow_mqtt_messages_receive_total", "total MQTT messages", labelnames=["device"])

    def loop(self):
        time.sleep(self.collecting_interval_seconds)
        while True:
            queue_size = self.message_queue.qsize()
            if queue_size > 0:
                log.info(f"Processing {queue_size} event(s) from the message queue")
                self.online.labels(device=self.device_name).set(1)
                self.mqtt_messages_receive_total.labels(device=self.device_name).inc(queue_size)
            else:
                log.info("Message queue is empty. Assuming that the device is offline")
                self.online.labels(device=self.device_name).set(0)
                # Clear metrics for NaN (No data) instead of last value
                for metric in self.metrics_collector:
                    metric.clear()

            while not self.message_queue.empty():
                payload = self.message_queue.get()
                log.debug(f"Recived payload: {payload}")
                if payload is None:
                    continue

                try:
                    payload = json.loads(payload)
                    params = payload['params']
                except KeyError as key:
                    log.error(f"Failed to extract key {key} from payload: {payload}")
                except Exception as error:
                    log.error(f"Failed to parse MQTT payload: {payload} Error: {error}")
                    continue
                self.process_payload(params)

            time.sleep(self.collecting_interval_seconds)

    def get_metric_by_ecoflow_payload_key(self, ecoflow_payload_key):
        for metric in self.metrics_collector:
            if metric.ecoflow_payload_key == ecoflow_payload_key:
                log.debug(f"Found metric {metric.name} linked to {ecoflow_payload_key}")
                return metric
        log.debug(f"Cannot find metric linked to {ecoflow_payload_key}")
        return False

    def process_payload(self, params):
        log.debug(f"Processing params: {params}")
        for ecoflow_payload_key in params.keys():
            ecoflow_payload_value = params[ecoflow_payload_key]
            if not isinstance(ecoflow_payload_value, (int, float)):
                log.warning(f"Skipping unsupported metric {ecoflow_payload_key}: {ecoflow_payload_value}")
                continue

            metric = self.get_metric_by_ecoflow_payload_key(ecoflow_payload_key)
            if not metric:
                try:
                    metric = EcoflowMetric(ecoflow_payload_key, self.device_name)
                except EcoflowMetricException as error:
                    log.error(error)
                    continue
                log.info(f"Created new metric from payload key {metric.ecoflow_payload_key} -> {metric.name}")
                self.metrics_collector.append(metric)

            metric.set(ecoflow_payload_value)

            if ecoflow_payload_key == 'inv.acInVol' and ecoflow_payload_value == 0:
                ac_in_current = self.get_metric_by_ecoflow_payload_key('inv.acInAmp')
                if ac_in_current:
                    log.debug("Set AC inverter input current to zero because of zero inverter voltage")
                    ac_in_current.set(0)


def signal_handler(signum, frame):
    log.info(f"Received signal {signum}. Exiting...")
    sys.exit(0)


def main():
    # Register the signal handler for SIGTERM
    signal.signal(signal.SIGTERM, signal_handler)

    # Disable Process and Platform collectors
    for coll in list(REGISTRY._collector_to_names.keys()):
        REGISTRY.unregister(coll)

    log_level = os.getenv("LOG_LEVEL", "INFO")

    if log_level == "DEBUG":
        log_level = log.DEBUG
    elif log_level == "INFO":
        log_level = log.INFO
    elif log_level == "WARNING":
        log_level = log.WARNING
    elif log_level == "ERROR":
        log_level = log.ERROR
    else:
        log_level = log.INFO

    log.basicConfig(stream=sys.stdout, level=log_level, format='%(asctime)s %(levelname)-7s %(message)s')

    device_sn = os.getenv("DEVICE_SN")
    device_name = os.getenv("DEVICE_NAME") or device_sn
    ecoflow_access_key = os.getenv("ECOFLOW_ACCESS_KEY")
    ecoflow_secret_key = os.getenv("ECOFLOW_SECRET_KEY")
    ecoflow_api_host = os.getenv("ECOFLOW_API_HOST", "api.ecoflow.com")
    exporter_port = int(os.getenv("EXPORTER_PORT", "9090"))
    collecting_interval_seconds = int(os.getenv("COLLECTING_INTERVAL", "10"))
    timeout_seconds = int(os.getenv("MQTT_TIMEOUT", "60"))

    if (not device_sn or not ecoflow_access_key or not ecoflow_secret_key):
        log.error("Please, provide all required environment variables: DEVICE_SN, ECOFLOW_ACCESS_KEY, ECOFLOW_SECRET_KEY")
        sys.exit(1)

    try:
        auth = EcoflowAuthentication(ecoflow_access_key, ecoflow_secret_key, ecoflow_api_host)
    except Exception as error:
        log.error(error)
        sys.exit(1)

    message_queue = Queue()

    EcoflowMQTT(message_queue, device_sn, auth.mqtt_username, auth.mqtt_password, auth.mqtt_url, auth.mqtt_port, auth.mqtt_client_id, timeout_seconds)

    metrics = Worker(message_queue, device_name, collecting_interval_seconds)

    start_http_server(exporter_port)

    try:
        metrics.loop()

    except KeyboardInterrupt:
        log.info("Received KeyboardInterrupt. Exiting...")
        sys.exit(0)


if __name__ == '__main__':
    main()
