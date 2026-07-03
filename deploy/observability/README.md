# Local Observability Stack

Prometheus + Grafana for local development and demos.
Scrapes the cost-event-consumer metrics endpoint on `:9000`.

## Quick Start

```bash
# 1. Start the cost-event-consumer (must be running on localhost:9000)
cd inventory-watcher
INGEST_LISTEN_ADDR=localhost:8020 ./inventory-watcher

# 2. Start Prometheus + Grafana
cd deploy/observability
docker compose up -d

# 3. Open Grafana
open http://localhost:3000
# Login: admin / admin (or anonymous — auto-login enabled)
# Dashboard: "Cost Event Consumer" is auto-provisioned
```

## What You Get

- **Prometheus** at `http://localhost:9090` — scrapes `:9000/metrics` every 5s
- **Grafana** at `http://localhost:3000` — pre-built dashboard with:
  - Event throughput (rate/s by type and status)
  - HTTP request rate and p99 latency
  - Live resource gauges (VMs, clusters, models)
  - Metering and cost entry creation rates
  - Sweep duration percentiles (metering, rating, reconcile)
  - Reconcile drift (created/deleted resources)
  - Alert fire counts by threshold
  - Go runtime (goroutines, RSS)

## Teardown

```bash
docker compose down
```
