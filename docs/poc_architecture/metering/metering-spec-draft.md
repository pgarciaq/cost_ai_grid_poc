# Capacity-Based Metering Specification

> **Status:** PoC draft  
> **Requirements:** POC-ARCH, REQ-1b, REQ-2, REQ-1a  
> **Related:** [architecture.md](../architecture.md) · [event-types.md](../event-types.md) · [data-model.md](../data-model.md) · [cost_model_metric_feasibility.md](cost_model_metric_feasibility.md) · [cost-reports-feasibility.md](cost-reports-feasibility.md)

---

## 1. Purpose

This spec defines how Cost Management meters OSAC-provisioned infrastructure for the AI Grid PoC. Metering answers one question:

**How much capacity was provisioned, for how long, and for whom?**

The PoC charges on **provisioned capacity × duration**, not on actual CPU/memory consumption. No Prometheus scraping, no pod-level metrics, and no changes to the existing CSV ingestion pipeline.

---

## 2. Scope

### In Scope (PoC)

| Resource | Service | Billing model | Meters |
|---|---|---|---|
| Compute Instance (VM) | VMaaS | Capacity-based | `vm_uptime_seconds`, `vm_cpu_core_seconds`, `vm_memory_gib_seconds` |
| Cluster | CaaS | Capacity-based | `cluster_uptime_seconds`, `cluster_worker_node_seconds`, `cluster_worker_node_count` |
| Bare Metal | BMaaS | Capacity-based (TBD) | Schema pending OSAC — see §8 |

### Out of Scope (PoC)

| Item | Reason |
|---|---|
| Usage-based metering (`*_usage_*` metrics) | Requires Prometheus inside VMs — see [cost_model_metric_feasibility.md](cost_model_metric_feasibility.md) |
| Token / MaaS consumption metering | Separate post-PoC workstream (REQ-4) |
| Storage / PVC / GPU metering | OSAC does not expose these today |
| Network data transfer | Requires flow monitoring |
| Rework of hourly CSV pipeline | New HTTP/event data path only |

---

## 3. Metering Principles

### 3.1 Allocation = Request

OSAC declares capacity via HostType / template specs (cores, memory, node count). For infrastructure billing there is no distinction between "requested" and "allocated" — the declared spec **is** the billable quantity. This makes all allocation-based Koku metrics computable without live telemetry.

### 3.2 Two Inputs, One Output

Metering requires two kinds of input:

| Input | Source | Provides |
|---|---|---|
| **State** | OSAC Watch stream (lifecycle events) + reconciler | Which resources exist, their specs, billable state, tenant/project |
| **Duration** | 60-second sweep (PoC) or OSAC heartbeat CloudEvents (target) | How long each billable resource has been active |

The Watch stream alone is insufficient: it emits on state transitions (CREATED/UPDATED/DELETED), not periodically. A VM in `RUNNING` with no subsequent events would produce zero metering without a time-based mechanism.

### 3.3 Immutable Audit Trail

Every metering increment is stored as a row in `metering_entries`, linked to the source event (when applicable) or sweep timestamp. Raw events are never modified after insert. See [data-model.md](../data-model.md).

---

## 4. Event Sources

### 4.1 PoC Implementation (current)

```mermaid
flowchart TB
    subgraph osac [OSAC Region Management Cluster]
        watch["Watch stream\n(NDJSON)"]
        list["List APIs\n(reconciler)"]
    end

    subgraph watcher [inventory-watcher]
        ingest["Event ingestion\n+ inventory upsert"]
        sweep["60s metering sweep"]
        final["Final metering\non DELETE"]
    end

    subgraph db [POC PostgreSQL]
        raw["raw_events"]
        inv["inventory tables"]
        meters["metering_entries"]
    end

    watch --> ingest
    list --> ingest
    ingest --> raw
    ingest --> inv
    sweep -->|"billable resources"| meters
    final --> meters
    inv --> sweep
    inv --> final
```

| Component | Interval | Role |
|---|---|---|
| Watch stream | Real-time | State transitions → inventory upsert, raw event log |
| Reconciler | Configurable (default 5m) | Catch missed Watch events via List API diff |
| Metering sweep | **60s** ([ADR-001](../../decisions/001-metering-sweep-interval.md)) | Produce time-based metering for all billable resources |
| Final metering | On DELETE | Capture usage from `last_metered_at` to deletion timestamp |

Implementation: `inventory-watcher/internal/metering/`.

### 4.2 Target Implementation (OSAC heartbeat collector)

When OSAC's metering collector is available, it will emit CloudEvents every ~60s with pre-calculated `duration_seconds` and derived quantities. Cost Management will:

1. Consume heartbeat CloudEvents via the same Watch stream (or HTTP/Kafka — transport TBD)
2. Insert raw event → extract meters → insert `metering_entries` directly from event payload
3. **Retire the local sweep** — the collector becomes the duration source

The `metering_entries` schema and meter names stay the same; only the producer changes. See [event-types.md](../event-types.md) for CloudEvent schemas.

### 4.3 Transport Options

| Option | Status | Notes |
|---|---|---|
| A — Watch stream + sweep | **PoC default** | [ADR-002](../../decisions/002-arguments-against-kafka.md) |
| B — REST polling | Fallback | 60s snapshot granularity; misses inter-poll events |
| C — Kafka | Deferred | Only if multi-consumer fan-out is required |

Requirements reference "heartbeat events via HTTP or Kafka" (REQ-1b). The PoC satisfies this functionally via the sweep until OSAC delivers native heartbeat CloudEvents.

---

## 5. Meters

Six discrete meters drive all feasible Koku cost model metrics for CaaS/VMaaS.

### 5.1 VMaaS Meters

| Meter | Unit | Formula | Stored per sweep/event |
|---|---|---|---|
| `vm_uptime_seconds` | seconds | `duration_seconds` | 1 row per VM |
| `vm_cpu_core_seconds` | core-seconds | `cores × duration_seconds` | 1 row per VM |
| `vm_memory_gib_seconds` | GiB-seconds | `memory_gib × duration_seconds` | 1 row per VM |

**Billable states:** `COMPUTE_INSTANCE_STATE_RUNNING`  
**Non-billable:** `STOPPED`, `DELETED`, and all other states

### 5.2 CaaS Meters

| Meter | Unit | Formula | Stored per sweep/event |
|---|---|---|---|
| `cluster_uptime_seconds` | seconds | `duration_seconds` (control plane) | 1 row per cluster |
| `cluster_worker_node_seconds` | node-seconds | `SUM(node_count × duration_seconds)` per node set | 1 row per cluster (aggregated) |
| `cluster_worker_node_count` | count | `MAX(node_count)` per node set | Snapshot from inventory |

**Billable states:** `CLUSTER_STATE_READY`, `CLUSTER_STATE_PROGRESSING`  
**Non-billable:** `FAILED`, `UNSPECIFIED`, and all other states

Control plane uptime and worker node time are metered separately. Worker node seconds are accumulated across all node sets in the cluster's `node_sets` spec.

### 5.3 Unit Conversions (for cost calculation)

| Meter unit | Koku input unit | Conversion |
|---|---|---|
| seconds | hours | `÷ 3600` |
| core-seconds | core-hours | `÷ 3600` |
| GiB-seconds | GiB-hours | `÷ 3600` |
| node-seconds | node-hours | `÷ 3600` |

---

## 6. Metering Pipeline

### 6.1 End-to-End Flow

```
OSAC event or 60s sweep
  │
  ├─► Validate & deduplicate (ce_id for events)
  │
  ├─► UPSERT inventory (clusters, compute_instances)
  │     └── Auto-register tenant on first event (REQ-1b)
  │
  ├─► If billable state:
  │     INSERT metering_entries (one row per meter)
  │     UPDATE last_metered_at on inventory record
  │
  ├─► On DELETE (if previously billable):
  │     INSERT final metering_entries (last_metered_at → deleted_at)
  │
  └─► [planned] Rate lookup → INSERT cost_entries
        └─► [planned] Quota evaluation → alerts → OSAC
```

### 6.2 Metering Entry Schema

Each row in `metering_entries` represents one meter increment for one resource over one time window:

| Field | Source |
|---|---|
| `resource_type` | `cluster` or `compute_instance` |
| `resource_id` | OSAC resource UUID |
| `tenant_id` | From event subject / inventory |
| `meter_name` | One of the six meters above |
| `value` | Calculated quantity |
| `unit` | `seconds`, `core_seconds`, `gib_seconds`, `node_seconds` |
| `period_start` | `last_metered_at` or `created_at` (sweep) / `ce_time - duration_seconds` (event) |
| `period_end` | Sweep timestamp or `ce_time` |

### 6.3 Restart Recovery

`last_metered_at` on each inventory record is the reconciliation point. After a restart, the first sweep covers exactly the gap since shutdown — no usage is lost.

### 6.4 Volume Estimates

At 60s intervals, one billable VM produces 3 metering rows per minute (~4,320/day). A cluster produces 2–3 rows per minute. For 100 VMs: ~432,000 rows/day — manageable with indexing and periodic aggregation into summary tables.

---

## 7. Cost Calculation

Metering produces quantities; cost calculation applies rates.

### 7.1 Rate Lookup

Rates are stored in the `rates` table, keyed by `resource_type`, `meter_name`, and optionally `tenant_id`. For PoC, rates may be seeded manually from the OSAC service catalog (REQ-3b).

| Rate type | Example | Applies to meter |
|---|---|---|
| Flat hourly | `$0.10 / VM-hour` | `vm_uptime_seconds` |
| Per-core hourly | `$0.05 / core-hour` | `vm_cpu_core_seconds` |
| Per-GiB hourly | `$0.01 / GiB-hour` | `vm_memory_gib_seconds` |
| Flat monthly | `$500 / node-month` | `cluster_worker_node_seconds` (amortized) |

### 7.2 Cost Formula

```
cost_amount = metered_value × price_per_unit
```

For monthly rates applied to sub-monthly metering windows:

```
daily_cost = (monthly_rate / days_in_month) × (metered_seconds / 86400)
```

### 7.3 Koku Metric Mapping

Full mapping of meters → Koku cost model metrics is documented in [cost_model_metric_feasibility.md](cost_model_metric_feasibility.md). Summary:

| Feasible (allocation-based) | Not feasible (usage-based) |
|---|---|
| `vm_cost_per_hour`, `vm_cost_per_month` | `cpu_core_usage_per_hour` |
| `vm_core_cost_per_hour`, `vm_core_cost_per_month` | `cpu_core_effective_usage_per_hour` |
| `cpu_core_request_per_hour`, `memory_gb_request_per_hour` | `memory_gb_usage_per_hour` |
| `cluster_cost_per_hour`, `cluster_cost_per_month` | `storage_gb_usage_per_month` |
| `node_cost_per_hour`, `node_core_cost_per_hour` | `gpu_cost_per_month` |
| `project_per_month` | |

Existing Koku SQL cost model queries can be adapted for the feasible metrics. The PoC may implement a simplified subset focused on demo value.

### 7.4 Report Outputs

Reports derived from metering + rates are documented in [cost-reports-feasibility.md](cost-reports-feasibility.md). PoC priority:

| Phase | Report | Key metrics |
|---|---|---|
| **1** | Compute cost by tenant/project | `cpu_core_hours`, `memory_gb_hours`, `cost_total` |
| **2** | Cluster cost, instance type breakdown | `node_count`, `total_cores`, `cost_total` |
| **3** | Network resource billing, cost distribution | `network_cost_total`, `distributed_cost` |

Response format follows Koku's hierarchical JSON structure (`meta` / `data` / `total`).

---

## 8. Bare Metal (BMaaS)

Bare metal metering is scoped (REQ-8) but the OSAC CloudEvent schema is not yet defined. Expected shape:

| Meter (proposed) | Unit | Formula |
|---|---|---|
| `bm_uptime_seconds` | seconds | `duration_seconds` |
| `bm_cpu_core_seconds` | core-seconds | `cpu_cores × duration_seconds` |
| `bm_memory_gib_seconds` | GiB-seconds | `memory_gib × duration_seconds` |

The metering pipeline will treat bare metal identically to VMs once the event schema is confirmed — same sweep pattern, same `metering_entries` table, same cost calculation path.

---

## 9. SLA and Timing

| Stage | Target | Requirement |
|---|---|---|
| Event ingestion | Real-time (Watch stream) | REQ-1 |
| Metering sweep / heartbeat processing | ≤ 60s | REQ-2, ADR-001 |
| Cost calculation | ≤ 60s after metering | REQ-2 |
| End-to-end (OSAC emit → cost available) | ≤ 90s | REQ-2 |
| Quota status API | Sub-second (cached aggregates) | REQ-9 |

The 60-second sweep interval directly satisfies the processing SLA: metering entries are available within one sweep cycle of any state change.

---

## 10. Tenant and Project Attribution

All metering entries carry `tenant_id`. Project attribution comes from the inventory record, populated from OSAC event data or reconciler List responses.

| Behavior | Detail |
|---|---|
| First event auto-registers tenant | REQ-1b — no pre-provisioning required |
| Project mapping | REQ-3a — costs drill down to project within tenant |
| Multi-tenant shared infra | Each resource's tenant/project from OSAC spec, not inferred |

---

## 11. Acceptance Criteria Mapping

| Requirement | How metering satisfies it |
|---|---|
| POC-ARCH: Costs from provisioned capacity | Meters use declared cores/memory/node count × duration |
| POC-ARCH: Heartbeat events drive cost | Sweep (PoC) / OSAC collector (target) provides duration |
| POC-ARCH: No workload cluster metrics | No Prometheus; allocation from OSAC specs only |
| POC-ARCH: Demo-ready within SLA | 60s sweep + planned cost calculation ≤ 90s E2E |
| REQ-1b: Parse tenant/project/resource/config | Inventory upsert from events; meters use spec fields |
| REQ-2: Process within 60s | Sweep interval = processing interval |
| REQ-1a: Cluster order lifecycle | Cluster state tracked; billable during READY/PROGRESSING |

---

## 12. PoC Phasing

| Phase | Deliverable | Status |
|---|---|---|
| **1a** | Inventory sync + metering sweep for VMs | Implemented |
| **1b** | Cluster metering (`cluster_uptime_seconds`, `cluster_worker_node_seconds`) | Implemented |
| **1c** | Final metering on DELETE | Implemented (VMs) |
| **2** | Cost calculation (`metering_entries` → `cost_entries`) | Planned |
| **3** | Cost reports API (tenant/project drill-down) | Planned |
| **4** | Switch to OSAC heartbeat CloudEvents (retire sweep) | Blocked on OSAC collector |
| **5** | Bare metal metering | Blocked on OSAC BMaaS schema |
| **6** | Quota evaluation + threshold alerts | Planned — see [alerting-spec-draft.md](../boundary_monitoring/alerting-spec-draft.md) |

---

## 13. Open Questions

| # | Question | Owner | Impact |
|---|---|---|---|
| 1 | When will OSAC metering collector emit heartbeat CloudEvents? | OSAC | Determines when to retire local sweep |
| 2 | Heartbeat transport: HTTP push vs Watch stream vs Kafka? | OSAC + Cost | REQ-1b — PoC uses sweep as interim |
| 3 | Where do rates live — OSAC catalog sync vs manual seed? | Cost | REQ-3b — manual acceptable for PoC |
| 4 | HostType catalog join for `cores_per_node` on clusters? | OSAC + Cost | Needed for `node_core_cost_per_*` metrics |
| 5 | BMaaS CloudEvent schema and billable states? | OSAC | REQ-8 |
| 6 | Network resource metering (VNets, subnets, IPs)? | OSAC + Cost | Phase 3 — not in initial PoC |
| 7 | Pre-aggregated summary tables for report queries? | Cost | Performance for dashboard SLA |

---

## 14. References

- [POC-ARCH requirements](../../requirements/csv_poc_requirements_summary.md#poc-arch--capacity-based-charging-model)
- [ADR-001: Metering sweep interval](../../decisions/001-metering-sweep-interval.md)
- [ADR-002: Watch stream instead of Kafka](../../decisions/002-arguments-against-kafka.md)
- [Cost model metric feasibility](cost_model_metric_feasibility.md)
- [Cost reports feasibility](cost-reports-feasibility.md)
- [Demo scenario](../../demo-scenario-1.md) — end-to-end walkthrough
- [Koku cost model constants](https://github.com/project-koku/koku/blob/main/koku/api/metrics/constants.py)
- [OSAC metering discover POC](https://github.com/masayag/osac-metering-discover-poc)
