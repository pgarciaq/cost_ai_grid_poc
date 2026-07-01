# CloudEvents Catalog

> All CloudEvents formats consumed or produced by the cost-event-consumer.
> Sources linked for each schema.

## VMaaS — Compute Instance Lifecycle

**Type:** `osac.compute_instance.lifecycle`
**Source:** OSAC metering collector ([schema](https://github.com/masayag/osac-metering-discover-poc/blob/main/collector/README.md#cloudevents-schema))
**Our handler:** `internal/ingest/handler.go` → `handleComputeInstanceEvent`

```json
{
  "specversion": "1.0",
  "type": "osac.compute_instance.lifecycle",
  "source": "osac.metering.collector",
  "id": "<uuid>",
  "time": "<ISO8601>",
  "subject": "<tenant_id>",
  "data": {
    "duration_seconds": 60,
    "cpu_core_seconds": 120,
    "memory_gib_seconds": 240,
    "tenant_id": "osac-e2e-ci",
    "instance_id": "019eb257-8108-773f-99c4-5d7642e9e7d8",
    "template": "osac.templates.ocp_virt_vm",
    "catalog_item": "",
    "state": "RUNNING",
    "cores": 2,
    "memory_gib": 4
  }
}
```

**Fields we meter:** `duration_seconds` → `vm_uptime_seconds`,
`cpu_core_seconds` → `vm_cpu_core_seconds`,
`memory_gib_seconds` → `vm_memory_gib_seconds`

**Note:** `catalog_item` field present but empty in current collector.
`project_id` is missing — Moti noted it should be added alongside `tenant_id`.

---

## CaaS — Cluster Lifecycle

**Type:** `osac.cluster.lifecycle`
**Source:** OSAC metering collector ([schema](https://github.com/masayag/osac-metering-discover-poc/blob/main/collector/README-caas.md#cloudevents-schema))
**Our handler:** `internal/ingest/handler.go` → `handleClusterEvent`

### Control Plane Event

```json
{
  "specversion": "1.0",
  "type": "osac.cluster.lifecycle",
  "source": "osac.metering.collector",
  "id": "<uuid>",
  "time": "<ISO8601>",
  "subject": "<tenant_id>",
  "data": {
    "duration_seconds": 60,
    "worker_node_seconds": 0,
    "node_count": 0,
    "tenant_id": "shared",
    "cluster_id": "<uuid>",
    "template": "osac.templates.ocp_ci_small",
    "state": "READY",
    "host_type": "_control_plane"
  }
}
```

### Worker Node Set Event

```json
{
  "specversion": "1.0",
  "type": "osac.cluster.lifecycle",
  "source": "osac.metering.collector",
  "id": "<uuid>",
  "time": "<ISO8601>",
  "subject": "<tenant_id>",
  "data": {
    "duration_seconds": 60,
    "worker_node_seconds": 60,
    "node_count": 1,
    "tenant_id": "shared",
    "cluster_id": "<uuid>",
    "template": "osac.templates.ocp_ci_small",
    "state": "READY",
    "host_type": "ci-worker"
  }
}
```

**Fields we meter:** Control plane → `cluster_uptime_seconds`.
Worker → `cluster_worker_node_seconds` (node_count × duration).
`host_type == "_control_plane"` determines which meters fire.

---

## MaaS — Inference Token Usage (IPP Plugin)

**Type:** `inference.tokens.used`
**Source:** IPP external-metering plugin ([PR #320](https://github.com/opendatahub-io/ai-gateway-payload-processing/pull/320), [client.go](https://github.com/opendatahub-io/ai-gateway-payload-processing/blob/61b6160e8b3c3172353d4c2740f11eb782fb5717/pkg/plugins/external-metering/client.go))
**Our handler:** `internal/ingest/handler.go` → `handleModelEvent`

```json
{
  "specversion": "1.0",
  "type": "inference.tokens.used",
  "source": "maas-gateway",
  "id": "<uuid>",
  "time": "<ISO8601>",
  "subject": "<username>",
  "data": {
    "user": "<username>",
    "group": "<k8s-group>",
    "subscription": "<maas-subscription-name>",
    "provider": "anthropic",
    "model": "claude-sonnet-4-20250514",
    "prompt_tokens": 1500,
    "completion_tokens": 800,
    "total_tokens": 2300,
    "cached_input_tokens": 200,
    "cache_creation_tokens": 0,
    "reasoning_tokens": 150,
    "duration_ms": 3200
  }
}
```

**Gap:** Our current handler expects `tokens_in`/`tokens_out`. The real
IPP sends `prompt_tokens`/`completion_tokens` + `cached_input_tokens`/
`cache_creation_tokens`/`reasoning_tokens`. We need to either:
- Accept both naming conventions (backwards compat)
- Or map: prompt→tokens_in, completion→tokens_out, add new meters for
  cached/reasoning

**Note:** `subject` is the username (not tenant_id like VMaaS/CaaS events).
Tenant attribution may need Keycloak lookup or event augmentation.

---

## Balance Check API (IPP Plugin → Cost Consumer)

**Not a CloudEvent** — this is a synchronous REST call.

**Source:** IPP external-metering plugin ([client.go](https://github.com/opendatahub-io/ai-gateway-payload-processing/blob/61b6160e8b3c3172353d4c2740f11eb782fb5717/pkg/plugins/external-metering/client.go))

### Request

```
GET /api/v1/customers/{customerID}/entitlements/{featureKey}/value?model={model}
```

- `customerID` — resolved from `x-maas-username` or `x-maas-subscription` headers
- `featureKey` — configured, default `"inference-tokens"`
- `model` — the model being requested

### Response

```json
{
  "has_access": true,
  "balance": 45000.0,
  "usage": 5000.0,
  "overage": 0.0
}
```

- `has_access: false` → IPP returns `ResourceExhausted` (blocks inference)
- `has_access: true` → request proceeds
- On error + `failOpen=true` → request proceeds anyway

### What We Need to Implement

`GET /api/v1/customers/{id}/entitlements/{key}/value` that:
1. Maps `customerID` to a tenant (via Keycloak username → tenant lookup,
   or direct if customerID = tenant_id)
2. Queries current consumption: `MeteringSum(tenant, meter, periodStart, periodEnd)`
3. Queries quota limit: `QuotasForTenant(tenant)`
4. Returns `has_access` = (consumed < limit), `balance` = (limit - consumed),
   `usage` = consumed, `overage` = max(0, consumed - limit)

**Effort:** ~40 lines in handler.go — reuses existing store methods.

---

## MaaS — Our Current Mock Format

**Type:** `osac.model.lifecycle`
**Source:** Our `maas-simulator` (`cmd/maas-simulator/main.go`)
**Our handler:** `internal/ingest/handler.go` → `handleModelEvent`

```json
{
  "specversion": "1.0",
  "type": "osac.model.lifecycle",
  "source": "osac.maas.simulator",
  "id": "<uuid>",
  "time": "<ISO8601>",
  "subject": "<tenant_id>",
  "data": {
    "model_name": "llama-3-70b",
    "model_id": "<uuid>",
    "tenant_id": "tenant-acme",
    "tokens_in": 1500,
    "tokens_out": 800,
    "request_count": 1,
    "duration_seconds": 3,
    "state": "MODEL_STATE_RUNNING"
  }
}
```

**Differences from real IPP format:**
| Field | Our mock | Real IPP |
|-------|----------|----------|
| Event type | `osac.model.lifecycle` | `inference.tokens.used` |
| Subject | `tenant_id` | `username` |
| Token fields | `tokens_in`, `tokens_out` | `prompt_tokens`, `completion_tokens`, `cached_input_tokens`, `cache_creation_tokens`, `reasoning_tokens` |
| Identity | `tenant_id` only | `user`, `group`, `subscription` |
| Duration | `duration_seconds` | `duration_ms` |
| Model | `model_name` | `model` |

These differences need to be reconciled when integrating with the real
IPP plugin. Our handler should accept both formats during transition.
