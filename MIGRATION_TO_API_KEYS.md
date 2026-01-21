# Migration Guide: Username/Password to AccessKey/SecretKey

This guide explains how to switch from username/password authentication to EcoFlow Developer API authentication using AccessKey and SecretKey.

## Why Switch?

The EcoFlow Developer API provides:
- More stable authentication
- Access to the official public API
- Better long-term support
- Access to additional device types (like Smart Plugs)

## Prerequisites

1. **Register for EcoFlow Developer Account**
   - Go to https://developer.ecoflow.com
   - Register and apply for developer access
   - **Wait approximately 1 week** for approval
   - You'll receive an email: "Approval notice from EcoFlow Developer Platform"

2. **Generate API Credentials**
   - After approval, log into https://developer.ecoflow.com
   - Navigate to Security section
   - Generate an **AccessKey** and **SecretKey** pair
   - **Save these credentials securely** - the secret key won't be shown again

3. **Determine Your API Region**
   - **Americas**: `api.ecoflow.com` (default)
   - **Europe**: `api-e.ecoflow.com`
   - **Asia/Australia**: `api-a.ecoflow.com`

## Changes Made to ecoflow_exporter.py

### 1. Added New Dependencies
```python
import hmac
import hashlib
import random
```

### 2. Updated Authentication Class
The `EcoflowAuthentication` class now:
- Accepts `ecoflow_access_key` and `ecoflow_secret_key` instead of username/password
- Generates HMAC-SHA256 signatures for API requests
- Calls the EcoFlow Developer API endpoint: `/iot-open/sign/certification`
- Uses proper authentication headers (accessKey, nonce, timestamp, sign)

### 3. Modified Environment Variables
**Old variables (NO LONGER USED):**
- `ECOFLOW_USERNAME`
- `ECOFLOW_PASSWORD`

**New variables (REQUIRED):**
- `ECOFLOW_ACCESS_KEY`
- `ECOFLOW_SECRET_KEY`

**Optional:**
- `ECOFLOW_API_HOST` - defaults to `api.ecoflow.com`

## Migration Steps

### Step 1: Update .env File

Edit `/Users/ces/Documents/Dev/EcoFlow/ecoflow_exporter-2.1.0/docker-compose/.env`:

```bash
# Serial number of your device shown in the mobile application
DEVICE_SN="DCABZ5SG7080038"

# Access Key from EcoFlow Developer Portal
ECOFLOW_ACCESS_KEY="your_access_key_here"

# Secret Key from EcoFlow Developer Portal
ECOFLOW_SECRET_KEY="your_secret_key_here"

# API Host (uncomment and modify if not in Americas)
# ECOFLOW_API_HOST="api-e.ecoflow.com"  # For Europe
# ECOFLOW_API_HOST="api-a.ecoflow.com"  # For Asia/Australia

# Grafana credentials (unchanged)
GRAFANA_USERNAME="admin"
GRAFANA_PASSWORD="grafana"
```

### Step 2: Replace Your Credentials

Replace `your_access_key_here` and `your_secret_key_here` with the actual credentials from the EcoFlow Developer Portal.

### Step 3: Test the Modified Script

Before deploying with Docker, test the script directly:

```bash
cd /Users/ces/Documents/Dev/EcoFlow/ecoflow_exporter-2.1.0

# Set environment variables
export DEVICE_SN="DCABZ5SG7080038"
export ECOFLOW_ACCESS_KEY="your_access_key_here"
export ECOFLOW_SECRET_KEY="your_secret_key_here"
export ECOFLOW_API_HOST="api.ecoflow.com"
export LOG_LEVEL="DEBUG"

# Run the exporter
python3 ecoflow_exporter.py
```

You should see output like:
```
INFO Authenticating with EcoFlow Developer API at api.ecoflow.com
INFO Successfully obtained MQTT credentials
INFO MQTT URL: mqtt-xxxx.ecoflow.com:8883
INFO MQTT Username: open-xxxxxxxxxx
INFO Connecting to MQTT Broker...
```

### Step 4: Restart Docker Compose (if using Docker)

```bash
cd /Users/ces/Documents/Dev/EcoFlow/ecoflow_exporter-2.1.0/docker-compose
docker-compose down
docker-compose up -d --build
```

### Step 5: Verify Operation

Check the logs:
```bash
docker-compose logs -f
```

Check metrics endpoint:
```bash
curl http://localhost:9090/metrics
```

## Troubleshooting

### "accessKey is invalid" Error
- Verify you copied the correct AccessKey from the developer portal
- Make sure you're using the correct API region (api.ecoflow.com vs api-e.ecoflow.com)
- Ensure your developer account has been approved

### "signature is wrong" Error
- Check that your SecretKey is correct
- Verify there are no extra spaces or quotes in your .env file
- The signature algorithm uses HMAC-SHA256 - ensure the implementation matches

### "KeyError: 'data'" or Connection Errors
- Confirm your devices are bound to your EcoFlow account
- Wait a few minutes after generating new keys
- Try regenerating your AccessKey/SecretKey pair

### MQTT Connection Fails
- The MQTT credentials are obtained via the API - if API auth works, MQTT should work
- Check firewall rules allow outbound connections to port 8883
- Verify the MQTT URL and port from the API response

## Testing the Signature Generation

You can test your signature generation using the test values provided by EcoFlow:

```python
import hmac
import hashlib

# Test values from EcoFlow documentation
access_key = "Fp4SvIprYSDPXtYJidEtUAd1o"
secret_key = "WIbFEKre0s6sLnh4ei7SPUeYnptHG6V"
nonce = "345164"
timestamp = "1671171709428"

# Empty params for this test
message = f"nonce={nonce}&timestamp={timestamp}"
signature = hmac.new(
    secret_key.encode('utf-8'),
    message.encode('utf-8'),
    hashlib.sha256
).hexdigest()

print(f"Generated signature: {signature}")
print(f"Expected signature:  07c13b65e037faf3b153d51613638fa80003c4c38d2407379a7f52851af1473e")
```

The signatures should match if the implementation is correct.

## Rollback (if needed)

If you need to revert to username/password authentication:

1. Restore the original `ecoflow_exporter.py` from backup or git
2. Update `.env` with:
   ```bash
   ECOFLOW_USERNAME="your_email"
   ECOFLOW_PASSWORD="your_password"
   ```
3. Restart the service

## Additional Notes

- The developer API has rate limits - keep the default COLLECTING_INTERVAL of 10 seconds
- Each AccessKey/SecretKey pair has a limit on concurrent MQTT connections (~5-10)
- You can only have one AccessKey/SecretKey pair active at a time
- The MQTT client ID format changed from `ANDROID_*` to `python-mqtt-*`
- The old username/password API may be deprecated in the future

## Support

For issues with:
- **EcoFlow Developer Portal**: Contact EcoFlow support
- **This exporter**: Check the original project at https://github.com/varet80/ecoflow-mqtt-prometheus-exporter
- **API Documentation**: https://developer.ecoflow.com/us/document/introduction
