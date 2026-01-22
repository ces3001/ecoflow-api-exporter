# EcoFlow + Weather Monitoring System on Railway

## Overview

This system collects and visualizes real-time data from:
1. **EcoFlow portable power station** (battery metrics via official API)
2. **National Weather Service** (weather data for Maui, HI)

Data flows through Prometheus for collection and Grafana for visualization, all hosted on Railway's free tier.

**Live Dashboard**: https://grafana-production-1edc.up.railway.app  
**GitHub Repository**: https://github.com/ces3001/ecoflow-api-exporter  
**Railway Project**: talented-celebration

---

## Architecture

```
┌─────────────────┐       ┌─────────────────┐       ┌─────────────────┐
│ EcoFlow Device  │──API──│ ecoflow_exporter│──9090─│                 │
└─────────────────┘       └─────────────────┘       │                 │
                                                     │   Prometheus    │
┌─────────────────┐       ┌─────────────────┐       │   (scrapes      │
│ NWS API         │──API──│  nws_exporter   │──8080─│    metrics)     │
└─────────────────┘       └─────────────────┘       │                 │
                                                     └────────┬────────┘
                                                              │
                                                              │9090
                                                              │
                                                     ┌────────▼────────┐
                                                     │    Grafana      │
                                                     │  (visualization)│
                                                     └─────────────────┘
                                                            :3000
                                                      (public access)
```

All services run on Railway and communicate via private networking.

---

## Railway Services

### 1. **ecoflow_exporter** (Python)
- **Purpose**: Connects to EcoFlow API, subscribes to MQTT stream, exports metrics
- **Dockerfile**: `Dockerfile.api`
- **Port**: 9090
- **Environment Variables**:
  - `DEVICE_SN`: EcoFlow device serial number
  - `ECOFLOW_ACCESS_KEY`: API key from EcoFlow Developer Portal
  - `ECOFLOW_SECRET_KEY`: API secret from EcoFlow Developer Portal
  - `ECOFLOW_API_HOST`: `api.ecoflow.com`
  - `EXPORTER_PORT`: `9090`
  - `LOG_LEVEL`: `INFO`
- **Metrics**: `ecoflow_*` (battery voltage, current, SOC, power, etc.)

### 2. **nws_exporter** (Go)
- **Purpose**: Polls NWS API for weather observations, exports metrics
- **Dockerfile**: `nws_exporter/Dockerfile.railway`
- **Root Directory**: `nws_exporter`
- **Port**: 8080
- **Command**: `-station PHOG -backofftime 1800 -verbose`
- **Metrics**: 
  - `nws_*` (temperature, humidity, wind, pressure, etc.)
  - `sun_*` (altitude, azimuth, sunrise/sunset times)

### 3. **prometheus**
- **Purpose**: Scrapes metrics from exporters, stores time-series data
- **Dockerfile**: `Dockerfile.prometheus`
- **Port**: 9090
- **Config**: `prometheus.yml`
- **Scrape Targets**:
  - `ecoflow_exporter:9090` (every 10s)
  - `nws_exporter:8080` (every 60s)
  - `localhost:9090` (self-monitoring)
- **Public URL**: https://prometheus-production-52cc.up.railway.app

### 4. **grafana**
- **Purpose**: Dashboard and visualization interface
- **Image**: `grafana/grafana` (Docker Hub)
- **Port**: 3000
- **Environment Variables**:
  - `GF_SECURITY_ADMIN_USER`: `admin`
  - `GF_SECURITY_ADMIN_PASSWORD`: `grafana`
  - `GF_SERVER_ROOT_URL`: https://grafana-production-1edc.up.railway.app
- **Data Source**: Prometheus at `http://prometheus:9090`
- **Public URL**: https://grafana-production-1edc.up.railway.app

---

## Essential Files (Deployed to Railway)

### Root Directory
- **`Dockerfile.api`**: Builds EcoFlow exporter (Python)
- **`Dockerfile.prometheus`**: Custom Prometheus with config and flags
- **`prometheus.yml`**: Prometheus scrape configuration
- **`ecoflow_exporter_api.py`**: Main exporter code (uses API keys, not username/password)
- **`requirements.txt`**: Python dependencies

### nws_exporter/ Directory
- **`Dockerfile.railway`**: Builds NWS exporter (Go) with command args
- **`main.go`**: Main exporter code
- **`observation.go`**: NWS API client
- **`sun.go`**: Sun position calculations for Maui
- **`go.mod`**, **`go.sum`**: Go dependencies

---

## Local Development Files (Not Used by Railway)

### docker-compose/ Directory
**Purpose**: Local development/testing stack  
**Status**: Not deployed to Railway (Railway doesn't use docker-compose)

- `compose-api.yaml`: Local multi-service setup
- `compose.yaml`: Alternative local config
- `server-compose.yaml`: Server configuration
- `.env`: Local environment variables (NOT in git, contains secrets)
- `prometheus/`: Local Prometheus config
- `grafana/`: Local Grafana provisioning and dashboards

### Other Local Files
- `Dockerfile`: Old username/password version (not used - kept for reference)
- `ecoflow_exporter.py`: Old version using username/password (superseded by `ecoflow_exporter_api.py`)
- `nws_exporter/Dockerfile`: Original Dockerfile without Railway command args

---

## Deployment Process

### Initial Setup (Already Complete)

1. **GitHub Repository**
   - Repository: `ces3001/ecoflow-api-exporter`
   - All code pushed to main branch
   - `.env` files are gitignored (secrets not committed)

2. **Railway Project**
   - Project: `talented-celebration`
   - Workspace: `EcoFlow`
   - Connected to GitHub repo

3. **Service Configuration**
   - Each service connected to the same GitHub repo
   - Different root directories and Dockerfiles specified per service
   - Environment variables configured via Railway UI

### Redeploying

Railway auto-deploys on git push to main branch. Manual redeploy:
1. Railway Dashboard → Select service
2. Click **Deployments** → **Deploy**

---

## Key Configuration Details

### Private Networking
Services communicate using Railway's internal DNS:
- `ecoflow_exporter:9090` (not `ecoflowexporter`)
- `nws_exporter:8080`
- `prometheus:9090`
- `grafana:3000`

Use underscores, not hyphens, in service names for DNS resolution.

### Prometheus Configuration
File: `prometheus.yml`
```yaml
global:
  scrape_interval: 10s
  scrape_timeout: 10s
  evaluation_interval: 10s

scrape_configs:
  - job_name: prometheus
    static_configs:
      - targets:
        - localhost:9090

  - job_name: ecoflow
    static_configs:
      - targets:
          - ecoflow_exporter:9090

  - job_name: nws_exporter
    static_configs:
      - targets:
          - nws_exporter:8080
    scrape_interval: 60s
```

### Prometheus Flags
Dockerfile.prometheus includes `--web.enable-remote-write-receiver` to accept POST requests from Grafana.

---

## Monitoring & Troubleshooting

### Check Scrape Targets
https://prometheus-production-52cc.up.railway.app/targets

Should show all targets as **UP** (green):
- prometheus
- ecoflow (ecoflow_exporter:9090)
- nws_exporter (nws_exporter:8080)

### Check Raw Metrics
- EcoFlow: `http://ecoflow_exporter:9090/metrics` (internal)
- NWS: `http://nws_exporter:8080/metrics` (internal)
- Prometheus: https://prometheus-production-52cc.up.railway.app/metrics

### View Logs
Railway Dashboard → Select service → **Deployments** tab → Click latest deployment → View logs

### Common Issues

1. **"no such host" errors**: Service name mismatch in prometheus.yml
2. **"405 Method Not Allowed"**: Prometheus missing `--web.enable-remote-write-receiver` flag
3. **EcoFlow offline**: Device is actually offline OR missing environment variables
4. **No data in Grafana**: Check Prometheus targets, verify data source connection

---

## Grafana Dashboard

### Login
- URL: https://grafana-production-1edc.up.railway.app
- Username: `admin`
- Password: `grafana`

### Data Source
- Type: Prometheus
- URL: `http://prometheus:9090`
- Access: Server (default)

### Available Metrics

**EcoFlow Battery:**
- `ecoflow_online`: Device connection status
- `ecoflow_bms_soc`: State of charge (%)
- `ecoflow_pd_watts_out_sum`: Total power output
- `ecoflow_inv_ac_in_vol`: AC input voltage
- Many more...

**Weather (NWS):**
- `nws_temperature`: Temperature (°C)
- `nws_humidity`: Relative humidity (%)
- `nws_wind_speed`: Wind speed (km/h)
- `nws_barometric_pressure`: Pressure (Pa)

**Sun Position:**
- `sun_altitude`: Sun angle above horizon (degrees)
- `sun_azimuth`: Sun direction (0=N, 90=E, 180=S, 270=W)
- `sun_is_daylight`: 1 if day, 0 if night
- `sun_sunrise_time`: Unix timestamp
- `sun_sunset_time`: Unix timestamp

---

## Cost & Free Tier

**Railway Free Tier:**
- $5 credit per month
- 500 hours execution time
- 100 GB bandwidth
- GitHub verification required

**Current Usage:**
- ~2-5 GB/month bandwidth (well within limits)
- All services run 24/7 on free tier

---

## Credentials & Access

### EcoFlow API
- Access Key: Stored in Railway environment variables
- Secret Key: Stored in Railway environment variables
- Portal: https://developer.ecoflow.com

### Railway
- Account: c@wakeupstudio.com
- Project: talented-celebration
- CLI: `railway` command (installed via Homebrew)

### GitHub
- Repository: ces3001/ecoflow-api-exporter
- Branch: main
- Auto-deploy on push

---

## Future Enhancements

- Add alerting via Prometheus Alertmanager
- Create custom Grafana dashboards
- Add more EcoFlow devices
- Deploy to multiple regions for redundancy
- Add authentication to Grafana (beyond basic admin/password)

---

## Quick Reference Commands

### Local Development
```bash
# Run local stack
cd docker-compose
docker-compose -f compose-api.yaml up

# View local Grafana
open http://localhost:3000

# View local Prometheus
open http://localhost:9090
```

### Git Operations
```bash
# Commit and push changes (triggers Railway deployment)
git add .
git commit -m "Update configuration"
git push origin main
```

### Railway CLI
```bash
# Login
railway login

# Link to project
railway link

# View logs
railway logs

# Set environment variables
railway variables set KEY=value

# Deploy manually
railway up
```

---

**Last Updated**: January 22, 2026  
**Maintained by**: ces3001
