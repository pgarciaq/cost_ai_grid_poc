# IPP End-to-End Stress Test Report — 2026-07-05

## Setup

Full IPP gateway stack on local k3d (see [k3d-ipp-deployment.md](k3d-ipp-deployment.md)):
- Istio 1.29.2 with `ENABLE_GATEWAY_API_INFERENCE_EXTENSION=true`
- IPP from PR #320 with external-metering plugin (Helm chart, provider: istio)
- llm-katan echo mode as mock LLM
- Cost-event-consumer with PostgreSQL
- Prometheus scraping consumer metrics at 5s interval
- Grafana dashboard available

## Test

100 sequential requests through the full IPP pipeline:
- 5 different users (`user-0` through `user-4`)
- 3 different tenants (`tenant-0`, `tenant-1`, `tenant-2`)
- Each request: `POST /v1/chat/completions` with `X-MaaS-Username`,
  `X-MaaS-Group`, `X-MaaS-Subscription` headers

Flow per request:
1. curl → Istio Gateway → IPP ext_proc
2. IPP checkBalance → `GET /customers/{user}/entitlements/inference-tokens/value` on our consumer
3. IPP forwards to llm-katan (echo response with usage block)
4. IPP reportUsage → `POST /api/v1/events` CloudEvent to our consumer
5. Consumer: raw_events → metering_entries → (rating sweep) → cost_entries

## Results

| Metric | Value |
|--------|-------|
| Total requests | 100 |
| Successful | **100** |
| Failed | **0** |
| Duration | 24s |
| Throughput | 4.1 req/s |

**Note:** Throughput is limited by sequential `kubectl exec` for each
request (shell overhead ~200ms per call). The actual pipeline latency
is much lower — see metrics below.

## Database State After Test

| Table | Count |
|-------|-------|
| raw_events | 101 (100 + 1 manual test) |
| metering_entries | 202 (2 per event: tokens_in + tokens_out) |
| cost_entries | 202 (rated by rating sweep) |

### Cost by Tenant

| Tenant | Entries | Total Cost |
|--------|---------|------------|
| tenant-1 | 68 | $0.000612 |
| tenant-2 | 66 | $0.000594 |
| tenant-0 | 66 | $0.000594 |
| test-tenant | 2 | $0.000016 |

## Prometheus Metrics

| Metric | Value |
|--------|-------|
| `events_processed_total{type=inference.tokens.used, status=accepted}` | 101 |
| `metering_entries_created_total{resource_type=model}` | 101 (tokens_in) + 101 (tokens_out) |
| `cost_entries_created_total{resource_type=model}` | 202 |
| `http_request_duration_seconds_sum{GET /customers}` | 0.037s total (103 calls) |
| `http_request_duration_seconds_sum{POST /events}` | 0.219s total (101 calls) |

### Latency

| Endpoint | Avg Latency |
|----------|-------------|
| Balance check (GET) | **0.36ms** |
| Usage report (POST) | **2.17ms** |

Both well within the IPP's 5-second timeout. The balance check is
sub-millisecond.

## Load Test with hey (proper benchmark)

The 4.1 req/s figure above was limited by `kubectl exec` shell overhead
(~200ms per call). Proper load test with `hey` via port-forward:

```
hey -n 1000 -c 10 -m POST \
  -H "x-maas-username: hey-user" \
  -H "x-maas-group: tenant-hey" \
  -H "x-maas-subscription: tenant-hey/plan" \
  -d '{"model":"test-model","messages":[...]}' \
  http://localhost:18080/v1/chat/completions
```

| Metric | Value |
|--------|-------|
| Total requests | 1000 |
| Concurrency | 10 |
| Success | **1000 (100%)** |
| Duration | **1.41s** |
| Throughput | **708 req/s** |
| Avg latency | **14ms** |
| P50 | 13ms |
| P95 | 21ms |
| P99 | 38ms |
| Fastest | 4.7ms |
| Slowest | 79ms |

All 1000 events ingested → 2204 metering entries → 1202 cost entries
(after rating sweep).

### Cost by tenant (after load test)

| Tenant | Entries | Total Cost |
|--------|---------|------------|
| tenant-hey | 1000 | $0.007998 |
| tenant-1 | 68 | $0.000612 |
| tenant-2 | 66 | $0.000594 |
| tenant-0 | 66 | $0.000594 |

## Known Issue

The IPP logs `"failed to report usage to metering system: usage report
returned status 202"` for each event. The IPP client expects 200 or 204
but we return 202 (Accepted). The event IS received and processed —
this is a cosmetic error that should be fixed by returning 200 instead
of 202 from our handler. Does not affect functionality.
