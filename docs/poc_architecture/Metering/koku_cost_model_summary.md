# Koku OCP Cost Model Metrics Reference

This document describes every metric available in the OpenShift cost model, including:
- How cost is calculated
- Whether the metric is usage-, request-, or capacity-based
- Which Prometheus queries the koku-metrics-operator uses to collect the underlying data

> **Source code references:**
> - Metric definitions: [`koku/api/metrics/constants.py`](https://github.com/project-koku/koku/blob/main/koku/api/metrics/constants.py)
> - Usage cost SQL: [`koku/masu/database/sql/openshift/cost_model/usage_costs.sql`](https://github.com/project-koku/koku/blob/main/koku/masu/database/sql/openshift/cost_model/usage_costs.sql)
> - Node/Cluster monthly SQL: [`koku/masu/database/sql/openshift/cost_model/monthly_cost_cluster_and_node.sql`](https://github.com/project-koku/koku/blob/main/koku/masu/database/sql/openshift/cost_model/monthly_cost_cluster_and_node.sql)
> - PVC monthly SQL: [`koku/masu/database/sql/openshift/cost_model/monthly_cost_persistentvolumeclaim.sql`](https://github.com/project-koku/koku/blob/main/koku/masu/database/sql/openshift/cost_model/monthly_cost_persistentvolumeclaim.sql)
> - VM monthly SQL: [`koku/masu/database/sql/openshift/cost_model/monthly_cost_virtual_machine.sql`](https://github.com/project-koku/koku/blob/main/koku/masu/database/sql/openshift/cost_model/monthly_cost_virtual_machine.sql)
> - Prometheus queries: [`internal/collector/queries.go`](https://github.com/project-koku/koku-metrics-operator/blob/main/internal/collector/queries.go) (koku-metrics-operator)

---

## Table of Contents

1. [How Cost Calculation Works](#how-cost-calculation-works)
2. [CPU Metrics](#cpu-metrics)
3. [Memory Metrics](#memory-metrics)
4. [Storage Metrics](#storage-metrics)
5. [Node Metrics](#node-metrics)
6. [Cluster Metrics](#cluster-metrics)
7. [Persistent Volume Claim (PVC) Metrics](#persistent-volume-claim-pvc-metrics)
8. [Virtual Machine (VM) Metrics](#virtual-machine-vm-metrics)
9. [GPU Metrics](#gpu-metrics)
10. [Project Metrics](#project-metrics)
11. [Metric Summary Table](#metric-summary-table)
12. [Billing Model Classification](#billing-model-classification)

---

## How Cost Calculation Works

### Rate Types

OCP cost model metrics fall into two billing categories:

| Category | Description | Applied To |
|---|---|---|
| **Infrastructure** | Costs attributed to infrastructure (nodes, clusters, PVCs) | Usually operator/admin charges |
| **Supplementary** | Costs attributed to workload usage (CPU, memory) | Usually end-user workload charges |

### Calculation Patterns

There are three fundamental calculation patterns:

**1. Usage × Rate** (applies to CPU usage/request, memory usage/request, storage)

```
cost = measured_quantity_per_hour_or_month × rate_per_unit
```

**2. Amortized Monthly Rate** (applies to Node, Cluster, PVC)

```
cost = (pod_effective_usage / node_or_cluster_capacity) × monthly_rate
```

The monthly rate is divided evenly across all days in the billing period (amortized), so each day receives an equal share. This distributes node/cluster fixed costs proportionally to the pods running on that node.

**3. Flat Monthly Rate** (applies to VM, PVC-count-based)

```
cost = flat_rate  (one row per entity per day, amortized across days)
```

### Effective Usage Defined

`pod_effective_usage` = `min(pod_usage, pod_request)` — the lesser of actual consumption or what was requested. This prevents a pod that used more than requested (due to bursting) from being charged for usage it did not explicitly request.

---

## CPU Metrics

### `cpu_core_usage_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based |
| **Unit** | core-hours |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** Actual CPU cores consumed by running containers, summed over time.

**Prometheus queries used by the operator:**

```promql
# Actual CPU usage rate per pod
sum by (pod, namespace, node) (
  rate(container_cpu_usage_seconds_total{
    container!='', container!='POD', pod!='', namespace!='', node!=''
  }[5m])
)
```

The operator integrates this rate over each hourly interval to produce `pod-usage-cpu-core-seconds`, which koku converts to `pod_usage_cpu_core_hours`.

**Cost formula:**

```
cost_model_cpu_cost += pod_usage_cpu_core_hours × rate
```

---

### `cpu_core_request_per_hour`

| Property | Value |
|---|---|
| **Category** | Request-based |
| **Unit** | core-hours |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** CPU cores reserved by pod resource requests, for running pods only.

**Prometheus queries used by the operator:**

```promql
# CPU resource requests per running pod
sum by (pod, namespace, node) (
  kube_pod_container_resource_requests{
    pod!='', namespace!='', node!='', resource='cpu'
  }
  * on(pod, namespace) group_left
  max by (pod, namespace) (kube_pod_status_phase{phase='Running'})
)
```

The operator integrates this over each hourly interval to produce `pod-request-cpu-core-seconds`, which koku converts to `pod_request_cpu_core_hours`.

**Cost formula:**

```
cost_model_cpu_cost += pod_request_cpu_core_hours × rate
```

---

### `cpu_core_effective_usage_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based (capped at request) |
| **Unit** | core-hours |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** The minimum of actual CPU usage and CPU request. This charges for real consumption but never more than what the pod requested, preventing burst overcharge.

```
pod_effective_usage_cpu_core_hours = min(pod_usage_cpu_core_hours, pod_request_cpu_core_hours)
```

**Prometheus queries used by the operator:** Same two queries as `cpu_core_usage_per_hour` and `cpu_core_request_per_hour`. Koku applies the `min()` during CSV processing.

**Cost formula:**

```
cost_model_cpu_cost += pod_effective_usage_cpu_core_hours × rate
```

---

## Memory Metrics

### `memory_gb_usage_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based |
| **Unit** | GiB-hours |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_memory_cost` |

**What it measures:** Actual memory bytes consumed by running containers, summed over time.

**Prometheus queries used by the operator:**

```promql
# Actual memory usage per running pod
sum by (pod, namespace, node) (
  container_memory_usage_bytes{
    container!='', container!='POD', pod!='', namespace!='', node!=''
  }
)
```

The operator integrates this over each hourly interval to produce `pod-usage-memory-byte-seconds`, which koku converts to `pod_usage_memory_gigabyte_hours` (dividing by 1024³ × 3600).

**Cost formula:**

```
cost_model_memory_cost += pod_usage_memory_gigabyte_hours × rate
```

---

### `memory_gb_request_per_hour`

| Property | Value |
|---|---|
| **Category** | Request-based |
| **Unit** | GiB-hours |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_memory_cost` |

**What it measures:** Memory bytes reserved by pod resource requests for running pods.

**Prometheus queries used by the operator:**

```promql
# Memory resource requests per running pod
sum by (pod, namespace, node) (
  kube_pod_container_resource_requests{
    pod!='', namespace!='', node!='', resource='memory'
  }
  * on(pod, namespace) group_left
  max by (pod, namespace) (kube_pod_status_phase{phase='Running'})
)
```

The operator integrates this over each hourly interval to produce `pod-request-memory-byte-seconds`, which koku converts to `pod_request_memory_gigabyte_hours`.

**Cost formula:**

```
cost_model_memory_cost += pod_request_memory_gigabyte_hours × rate
```

---

### `memory_gb_effective_usage_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based (capped at request) |
| **Unit** | GiB-hours |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_memory_cost` |

**What it measures:** The minimum of actual memory usage and memory request. Same logic as `cpu_core_effective_usage_per_hour` but for memory.

```
pod_effective_usage_memory_gigabyte_hours = min(pod_usage_memory_gigabyte_hours, pod_request_memory_gigabyte_hours)
```

**Prometheus queries used by the operator:** Same two queries as `memory_gb_usage_per_hour` and `memory_gb_request_per_hour`. Koku applies `min()` during CSV processing.

**Cost formula:**

```
cost_model_memory_cost += pod_effective_usage_memory_gigabyte_hours × rate
```

---

## Storage Metrics

### `storage_gb_usage_per_month`

| Property | Value |
|---|---|
| **Category** | Usage-based |
| **Unit** | GiB-month |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_volume_cost` |

**What it measures:** Actual bytes written to persistent volumes, as reported by kubelet.

**Prometheus queries used by the operator:**

```promql
# Actual bytes used in each PVC
kubelet_volume_stats_used_bytes
  * on(persistentvolumeclaim, namespace) group_left(volumename)
  max by(namespace, persistentvolumeclaim, volumename) (
    kube_persistentvolumeclaim_info{volumename != ''}
  )
```

The operator integrates this into `persistentvolumeclaim-usage-byte-seconds`, which koku converts to `persistentvolumeclaim_usage_gigabyte_months`.

**Cost formula:**

```
cost_model_volume_cost += persistentvolumeclaim_usage_gigabyte_months × rate
```

---

### `storage_gb_request_per_month`

| Property | Value |
|---|---|
| **Category** | Request-based |
| **Unit** | GiB-month |
| **Default Cost Type** | Supplementary |
| **DB Column Populated** | `cost_model_volume_cost` |

**What it measures:** Storage capacity requested in the PVC spec, regardless of how much is actually used.

**Prometheus queries used by the operator:**

```promql
# Storage requested in PVC spec
kube_persistentvolumeclaim_resource_requests_storage_bytes
  * on(persistentvolumeclaim, namespace) group_left(volumename)
  max by(namespace, persistentvolumeclaim, volumename) (
    kube_persistentvolumeclaim_info{volumename != ''}
  )
```

The operator integrates this into `persistentvolumeclaim-request-byte-seconds`, which koku converts to `volume_request_storage_gigabyte_months`.

**Cost formula:**

```
cost_model_volume_cost += volume_request_storage_gigabyte_months × rate
```

---

## Node Metrics

Node metrics are **amortized monthly rates**. The monthly rate is spread evenly across all days in the billing period, and within each day it is distributed to pods proportionally by their usage share of the node.

**Prometheus queries used by the operator (node capacity):**

```promql
# Node CPU capacity
kube_node_status_capacity{resource='cpu'}
  * on(node) group_left(provider_id)
  max by (node, provider_id) (kube_node_info)

# Node memory capacity
kube_node_status_capacity{resource='memory'}
  * on(node) group_left(provider_id)
  max by (node, provider_id) (kube_node_info)
```

---

### `node_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Capacity-based (amortized monthly) |
| **Unit** | node-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` or `cost_model_memory_cost` (by `monthly_cost_type='Node'`) |

**What it measures:** A flat monthly cost per node, distributed proportionally to pods running on that node.

**Cost formula (CPU distribution mode):**

```
pod_cost = (pod_effective_usage_cpu_core_hours / node_capacity_cpu_core_hours) × monthly_rate
```

**Cost formula (memory distribution mode):**

```
pod_cost = (pod_effective_usage_memory_gigabyte_hours / node_capacity_memory_gigabyte_hours) × monthly_rate
```

Each pod on the node receives a share proportional to how much of the node's capacity it consumed that day.

---

### `node_core_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Capacity-based (amortized monthly, per core) |
| **Unit** | core-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` (by `monthly_cost_type='Node_Core_Month'`) |

**What it measures:** A monthly cost per CPU core on the node, distributed proportionally to pods.

**Cost formula (CPU distribution mode):**

```
pod_cost = (pod_effective_usage_cpu_core_hours / node_capacity_cpu_core_hours)
           × node_capacity_cpu_cores × rate_per_core
```

**Cost formula (memory distribution mode):**

```
pod_cost = (pod_effective_usage_memory_gigabyte_hours / node_capacity_memory_gigabyte_hours)
           × node_capacity_cpu_cores × rate_per_core
```

---

### `node_core_cost_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based (per core-hour on node) |
| **Unit** | core-hour |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** A per-core, per-hour rate applied to the node. The cost is charged based on the effective CPU usage of each pod relative to the node's CPU.

**Cost formula (CPU distribution mode):**

```
# Applied inline in usage_costs.sql
cost_model_cpu_cost += pod_effective_usage_cpu_core_hours × rate
```

**Cost formula (memory distribution mode):**

```
cost_model_cpu_cost += (pod_effective_usage_memory_gigabyte_hours
                        / node_capacity_memory_gigabyte_hours)
                       × node_capacity_cpu_core_hours × rate
```

---

## Cluster Metrics

Cluster metrics are also amortized monthly rates, distributed across all pods in the cluster proportionally.

**Prometheus queries used by the operator:** Same node capacity queries as above. Cluster capacity is derived by summing node capacities within the cluster during koku CSV processing.

---

### `cluster_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Capacity-based (amortized monthly) |
| **Unit** | cluster-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` or `cost_model_memory_cost` (by `monthly_cost_type='Cluster'`) |

**What it measures:** A flat monthly cost for the entire cluster, distributed proportionally to all pods in the cluster.

**Cost formula (CPU distribution mode):**

```
pod_cost = (pod_effective_usage_cpu_core_hours / cluster_capacity_cpu_core_hours) × monthly_rate
```

**Cost formula (memory distribution mode):**

```
pod_cost = (pod_effective_usage_memory_gigabyte_hours / cluster_capacity_memory_gigabyte_hours) × monthly_rate
```

---

### `cluster_cost_per_hour`

| Property | Value |
|---|---|
| **Category** | Capacity-based (per node-size, per hour) |
| **Unit** | cluster-hour |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` or `cost_model_memory_cost` |

**What it measures:** An hourly cost for the cluster where each node is charged proportionally by how large it is relative to the whole cluster, then distributed to pods on that node proportionally.

**Cost formula (CPU distribution mode):**

```
node_cluster_hour_cost_per_day =
  (node_capacity_cpu_core_hours / cluster_capacity_cpu_core_hours)   # node size fraction
  × (node_capacity_cpu_core_hours / node_capacity_cpu_cores)         # hours node was running
  × rate

pod_cost = (pod_effective_usage_cpu_core_hours / node_total_pod_cpu_usage)
           × node_cluster_hour_cost_per_day
```

**Cost formula (memory distribution mode):** Same pattern using memory capacity fractions.

---

### `cluster_core_cost_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based (per core-hour in cluster) |
| **Unit** | core-hour |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** A per-core, per-hour rate applied at the cluster level. Charged based on each pod's effective CPU core hours.

**Cost formula (CPU distribution mode):**

```
cost_model_cpu_cost += pod_effective_usage_cpu_core_hours × rate
```

**Cost formula (memory distribution mode):**

```
cost_model_cpu_cost += (pod_effective_usage_memory_gigabyte_hours
                        / node_capacity_memory_gigabyte_hours)
                       × node_capacity_cpu_core_hours × rate
```

---

## Persistent Volume Claim (PVC) Metrics

### `pvc_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Capacity-based (flat monthly, divided by PVC count) |
| **Unit** | pvc-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_volume_cost` (by `monthly_cost_type='PVC'`) |

**What it measures:** A flat monthly cost split evenly across all PVCs in a namespace on each day.

**Prometheus queries used by the operator:**

```promql
# PVC capacity (from the underlying persistent volume)
kube_persistentvolume_capacity_bytes{persistentvolume != ''}

# PVC storage requests
kube_persistentvolumeclaim_resource_requests_storage_bytes
  * on(persistentvolumeclaim, namespace) group_left(volumename)
  max by(namespace, persistentvolumeclaim, volumename) (
    kube_persistentvolumeclaim_info{volumename != ''}
  )
```

**Cost formula:**

```
pvc_cost = rate / count_of_distinct_pvcs_in_namespace_that_day
```

Each PVC in the namespace on that day receives an equal share of the flat rate.

---

## Virtual Machine (VM) Metrics

VM metrics apply costs to pods that are KubeVirt virtual machines (identified by the `vm_kubevirt_io_name` label). They are flat or per-core monthly/hourly rates.

**Prometheus queries used by the operator:**

```promql
# VM CPU request (cores, sockets, threads)
sum by (name, namespace) (
  kubevirt_vm_resource_requests{name!='', namespace!='', resource='cpu', unit='cores'}
) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})

# VM CPU usage
sum by (name, namespace) (
  rate(kubevirt_vmi_cpu_usage_seconds_total{name!='', namespace!=''}[5m])
) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})

# VM memory request
sum by (name, namespace) (
  kubevirt_vm_resource_requests{name!='', namespace!='', resource='memory'}
) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})

# VM memory usage
sum by (name, namespace) (
  sum_over_time(kubevirt_vmi_memory_used_bytes{name!='', namespace!=''}[5m])
) * on (name, namespace) group_left max by (name, namespace) (kubevirt_vmi_info{phase='running'})

# VM uptime (used for hourly rates)
sum by (name, namespace, node, os, instance_type, ...) (
  kubevirt_vmi_info{phase='running'}
) * on(node) group_left(provider_id) max by (node, provider_id) (kube_node_info)
```

---

### `vm_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Flat monthly rate |
| **Unit** | vm-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` (by `monthly_cost_type='OCP_VM'`) |

**What it measures:** A flat monthly cost per virtual machine, amortized across days of the month. One row is inserted per VM per day.

**Cost formula:**

```
cost = rate / days_in_month   (per day, per VM)
```

Supports tag-based rates: a different rate can be applied per tag value (e.g., different rates for different VM sizes or environments).

---

### `vm_cost_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based (per running VM-hour) |
| **Unit** | vm-hour |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** An hourly rate for each virtual machine while it is running. The operator collects `kubevirt_vmi_info` to track VM uptime, which drives this metric.

**Cost formula:**

```
cost = vm_uptime_hours × rate
```

---

### `vm_core_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Capacity-based (flat monthly, per core) |
| **Unit** | core-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` (by `monthly_cost_type='OCP_VM'`) |

**What it measures:** A monthly cost per virtual CPU core allocated to the VM, amortized across days.

**Prometheus queries:** `kubevirt_vm_resource_requests{resource='cpu'}` for CPU cores/sockets/threads.

**Cost formula:**

```
cost = vm_requested_cores × rate / days_in_month   (per day, per VM)
```

---

### `vm_core_cost_per_hour`

| Property | Value |
|---|---|
| **Category** | Usage-based (per vCPU-hour) |
| **Unit** | core-hour |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` |

**What it measures:** An hourly rate per virtual CPU core while the VM is running.

**Prometheus queries:** `kubevirt_vm_resource_requests{resource='cpu', unit='cores'}` for core count, `kubevirt_vmi_info` for uptime.

**Cost formula:**

```
cost = vm_requested_cores × vm_uptime_hours × rate
```

---

## GPU Metrics

> **Feature flag:** GPU cost model metrics are gated behind the `ocp-gpu-cost-model` Unleash feature flag.

### `gpu_cost_per_month`

| Property | Value |
|---|---|
| **Category** | Capacity-based (flat monthly) |
| **Unit** | gpu-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` (by `monthly_cost_type='GPU'`) |

**What it measures:** A flat monthly cost per GPU assigned to a pod, amortized across days of the month.

**Prometheus queries used by the operator:**

```promql
# Non-MIG GPU memory capacity per pod (via kube scheduler resource requests)
sum by (pod, namespace, node, label_nvidia_com_gpu_memory) (
  (kube_pod_container_resource_requests{resource='nvidia_com_gpu'} * on(pod, namespace) group_left
   max by (pod, namespace) (kube_pod_status_phase{phase='Running'}))
  * on(node) group_left(label_nvidia_com_gpu_memory)
  (max by (node, label_nvidia_com_gpu_memory) (kube_node_labels))
)

# MIG GPU utilization (for GPU slices)
sum by (exported_pod, exported_namespace, Hostname, UUID, modelName, GPU_I_ID, GPU_I_PROFILE, device) (
  DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!=''}
) * on(exported_pod, exported_namespace) group_left(pod, namespace)
  max by (exported_pod, exported_namespace) (label_replace(
    label_replace(kube_pod_status_phase{phase='Running'}, 'exported_pod', '$1', 'pod', '(.*)'),
    'exported_namespace', '$1', 'namespace', '(.*)'
  ))

# GPU pod uptime (for GPU-hours)
sum by (...) (clamp_max(DCGM_FI_PROF_GR_ENGINE_ACTIVE{UUID!=''} + 1, 1))
  * on(exported_pod, exported_namespace) group_left(pod, namespace) ...

# MIG max slices per GPU
sum by (...) (DCGM_FI_DEV_MIG_MAX_SLICES{UUID!=''})
  * on(exported_pod, exported_namespace) group_left(pod, namespace) ...
```

**Cost formula:**

```
cost = gpu_count × rate / days_in_month   (per day)
```

For MIG (Multi-Instance GPU) slices, the operator also collects `DCGM_FI_DEV_MIG_MAX_SLICES` to determine how many slices the GPU is divided into, enabling proportional cost allocation at the slice level.

---

## Project Metrics

### `project_per_month`

| Property | Value |
|---|---|
| **Category** | Flat monthly rate |
| **Unit** | project-month |
| **Default Cost Type** | Infrastructure |
| **DB Column Populated** | `cost_model_cpu_cost` (by `monthly_cost_type='Project'`) |

**What it measures:** A flat monthly cost per OpenShift namespace/project.

**Prometheus queries used by the operator:**

```promql
# Namespace labels (used to identify and enumerate projects)
kube_namespace_labels
```

**Cost formula:**

```
cost = rate / days_in_month   (per day, per namespace)
```

---

## Metric Summary Table

| Metric | Category | Unit | Default Cost Type | DB Column | Prometheus Metric(s) |
|---|---|---|---|---|---|
| `cpu_core_usage_per_hour` | Usage | core-hours | Supplementary | `cost_model_cpu_cost` | `container_cpu_usage_seconds_total` |
| `cpu_core_request_per_hour` | Request | core-hours | Supplementary | `cost_model_cpu_cost` | `kube_pod_container_resource_requests{resource='cpu'}` |
| `cpu_core_effective_usage_per_hour` | Usage (capped) | core-hours | Supplementary | `cost_model_cpu_cost` | `container_cpu_usage_seconds_total`, `kube_pod_container_resource_requests{resource='cpu'}` |
| `memory_gb_usage_per_hour` | Usage | GiB-hours | Supplementary | `cost_model_memory_cost` | `container_memory_usage_bytes` |
| `memory_gb_request_per_hour` | Request | GiB-hours | Supplementary | `cost_model_memory_cost` | `kube_pod_container_resource_requests{resource='memory'}` |
| `memory_gb_effective_usage_per_hour` | Usage (capped) | GiB-hours | Supplementary | `cost_model_memory_cost` | `container_memory_usage_bytes`, `kube_pod_container_resource_requests{resource='memory'}` |
| `storage_gb_usage_per_month` | Usage | GiB-month | Supplementary | `cost_model_volume_cost` | `kubelet_volume_stats_used_bytes` |
| `storage_gb_request_per_month` | Request | GiB-month | Supplementary | `cost_model_volume_cost` | `kube_persistentvolumeclaim_resource_requests_storage_bytes` |
| `node_cost_per_month` | Capacity (amortized) | node-month | Infrastructure | `cost_model_cpu/memory_cost` | `kube_node_status_capacity{resource='cpu/memory'}` |
| `node_core_cost_per_month` | Capacity (amortized) | core-month | Infrastructure | `cost_model_cpu_cost` | `kube_node_status_capacity{resource='cpu/memory'}` |
| `node_core_cost_per_hour` | Usage (per node core) | core-hour | Infrastructure | `cost_model_cpu_cost` | `kube_node_status_capacity{resource='cpu/memory'}` |
| `cluster_cost_per_month` | Capacity (amortized) | cluster-month | Infrastructure | `cost_model_cpu/memory_cost` | `kube_node_status_capacity{resource='cpu/memory'}` |
| `cluster_cost_per_hour` | Capacity (per node-size) | cluster-hour | Infrastructure | `cost_model_cpu/memory_cost` | `kube_node_status_capacity{resource='cpu/memory'}` |
| `cluster_core_cost_per_hour` | Usage (per cluster core) | core-hour | Infrastructure | `cost_model_cpu_cost` | `kube_node_status_capacity{resource='cpu/memory'}` |
| `pvc_cost_per_month` | Capacity (flat, split) | pvc-month | Infrastructure | `cost_model_volume_cost` | `kube_persistentvolume_capacity_bytes`, `kube_persistentvolumeclaim_resource_requests_storage_bytes` |
| `vm_cost_per_month` | Flat monthly | vm-month | Infrastructure | `cost_model_cpu_cost` | `kubevirt_vmi_info` |
| `vm_cost_per_hour` | Usage (per VM-hour) | vm-hour | Infrastructure | `cost_model_cpu_cost` | `kubevirt_vmi_info` |
| `vm_core_cost_per_month` | Capacity (flat, per core) | core-month | Infrastructure | `cost_model_cpu_cost` | `kubevirt_vm_resource_requests{resource='cpu'}`, `kubevirt_vmi_info` |
| `vm_core_cost_per_hour` | Usage (per vCPU-hour) | core-hour | Infrastructure | `cost_model_cpu_cost` | `kubevirt_vm_resource_requests{resource='cpu'}`, `kubevirt_vmi_cpu_usage_seconds_total` |
| `gpu_cost_per_month` ¹ | Capacity (flat) | gpu-month | Infrastructure | `cost_model_cpu_cost` | `DCGM_FI_PROF_GR_ENGINE_ACTIVE`, `kube_pod_container_resource_requests{resource='nvidia_com_gpu'}`, `DCGM_FI_DEV_MIG_MAX_SLICES` |
| `project_per_month` | Flat monthly | project-month | Infrastructure | `cost_model_cpu_cost` | `kube_namespace_labels` |

> ¹ `gpu_cost_per_month` is gated behind the `ocp-gpu-cost-model` Unleash feature flag.

---

## Key Relationships

```
Metric Category     → SQL File                                      → DB Column
─────────────────────────────────────────────────────────────────────────────────
CPU/Memory/Storage  → usage_costs.sql                              → cost_model_{cpu,memory,volume}_cost
Node/Cluster/Cores  → monthly_cost_cluster_and_node.sql            → cost_model_{cpu,memory}_cost
PVC                 → monthly_cost_persistentvolumeclaim.sql        → cost_model_volume_cost
VM                  → monthly_cost_virtual_machine.sql              → cost_model_cpu_cost
GPU                 → (GPU distribution SQL)                        → cost_model_cpu_cost
```

All cost model costs are separate from infrastructure raw costs (AWS/Azure/GCP charges). For OCP-on-Cloud deployments, both sources contribute to the final cost visible in the UI.

---

## Billing Model Classification

This section maps every OCP cost model metric to one of two billing paradigms — **usage-based** (requires live consumption telemetry) or **capacity-based** (requires only provisioned/inventory state). This classification is relevant for determining what data must be collected versus what can be derived purely from cluster inventory.

### Definitions

| Billing Model | Description | Analogy |
|---|---|---|
| **Capacity-Based** | Charge is based on what was provisioned or reserved — node existence, core counts, PVC size, VM allocation. No real-time consumption data required. | CaaS / VMaaS |
| **Usage-Based** | Charge is based on actual runtime consumption — CPU cycles used, memory bytes consumed, VM uptime hours. Requires live Prometheus telemetry from running workloads. | MaaS / consumption |

---

### Usage-Based Metrics (13 of 21)

These metrics **cannot be computed without Prometheus scraping active workloads**. Any metric that uses `pod_effective_usage` — even indirectly for pod-level cost distribution — falls here, because `pod_effective_usage = min(actual_usage, request)` requires live consumption telemetry. Substituting request alone would produce inaccurate results for pods that consumed less than they requested.

| Metric | Subtype | Unit | Cost Type | Data Required |
|---|---|---|---|---|
| `cpu_core_usage_per_hour` | Actual Usage | core-hours | Supplementary | Actual CPU cores consumed (`container_cpu_usage_seconds_total`) |
| `cpu_core_effective_usage_per_hour` | Actual Usage (capped at request) | core-hours | Supplementary | min(actual CPU, request) — requires both usage and request telemetry |
| `memory_gb_usage_per_hour` | Actual Usage | GiB-hours | Supplementary | Actual memory consumed (`container_memory_usage_bytes`) |
| `memory_gb_effective_usage_per_hour` | Actual Usage (capped at request) | GiB-hours | Supplementary | min(actual memory, request) — requires both usage and request telemetry |
| `storage_gb_usage_per_month` | Actual Usage | GiB-month | Supplementary | Actual bytes written to PVCs (`kubelet_volume_stats_used_bytes`) |
| `node_cost_per_month` | Capacity envelope, effective-usage distribution | node-month | Infrastructure | Billing envelope is provisioned node capacity; pod-level share requires `pod_effective_usage` |
| `node_core_cost_per_month` | Capacity envelope, effective-usage distribution | core-month | Infrastructure | Billing envelope is provisioned core count; pod-level share requires `pod_effective_usage` |
| `cluster_cost_per_month` | Capacity envelope, effective-usage distribution | cluster-month | Infrastructure | Billing envelope is provisioned cluster capacity; pod-level share requires `pod_effective_usage` |
| `cluster_cost_per_hour` | Capacity envelope, effective-usage distribution | cluster-hour | Infrastructure | Node-level charge based on provisioned size fraction; pod-level share requires `pod_effective_usage` |
| `node_core_cost_per_hour` | Actual Usage | core-hour | Infrastructure | Pod effective CPU core hours — `pod_effective_usage_cpu_core_hours × rate` |
| `cluster_core_cost_per_hour` | Actual Usage | core-hour | Infrastructure | Pod effective CPU core hours — `pod_effective_usage_cpu_core_hours × rate` |
| `vm_cost_per_hour` | Actual Usage (uptime) | vm-hour | Infrastructure | VM uptime hours — must track running state via `kubevirt_vmi_info{phase='running'}` |
| `vm_core_cost_per_hour` | Actual Usage (uptime × cores) | core-hour | Infrastructure | Allocated vCPU count × VM uptime hours |

---

### Capacity-Based Metrics (8 of 21)

These metrics are **computable from the Kubernetes API server alone** — resource declarations, PVC specs, VM allocations, namespace existence. No workload instrumentation or consumption telemetry required.

| Metric | Subtype | Unit | Cost Type | Data Required |
|---|---|---|---|---|
| `cpu_core_request_per_hour` | Request / Provisioned | core-hours | Supplementary | CPU cores reserved by pod resource requests |
| `memory_gb_request_per_hour` | Request / Provisioned | GiB-hours | Supplementary | Memory reserved by pod resource requests |
| `storage_gb_request_per_month` | Request / Provisioned | GiB-month | Supplementary | Storage capacity declared in PVC spec |
| `pvc_cost_per_month` | Flat Capacity (count-based) | pvc-month | Infrastructure | Count of distinct PVCs in namespace |
| `vm_cost_per_month` | Flat Capacity | vm-month | Infrastructure | VM existence (`kubevirt_vmi_info`) — provisioned presence only |
| `vm_core_cost_per_month` | Flat Capacity (per allocated core) | core-month | Infrastructure | vCPU cores allocated to the VM |
| `gpu_cost_per_month` ¹ | Flat Capacity | gpu-month | Infrastructure | GPU count assigned to pod (`kube_pod_container_resource_requests{resource='nvidia_com_gpu'}`) |
| `project_per_month` | Flat Capacity (admin) | project-month | Infrastructure | Namespace existence only (`kube_namespace_labels`) |

> ¹ `gpu_cost_per_month` is gated behind the `ocp-gpu-cost-model` Unleash feature flag.

---

### Key Observations

**Any metric using `pod_effective_usage` requires live telemetry.** `pod_effective_usage = min(actual_usage, request)`. This cannot be approximated with request data alone — a pod that consumed 0.3 cores against a 1-core request would be incorrectly billed at 1 core if only request data were available. This applies not just to direct usage metrics but also to `node_cost_per_month`, `node_core_cost_per_month`, `cluster_cost_per_month`, and `cluster_cost_per_hour`, which use `pod_effective_usage` to distribute a fixed monthly rate across pods.

**The node/cluster monthly metrics have a capacity billing envelope but a usage-driven distribution.** The *total* monthly charge is determined by what was provisioned (the flat node/cluster rate). However, computing each individual pod's share of that charge requires `pod_effective_usage`. Swapping in request data would produce inaccurate per-pod allocations. These metrics are therefore classified as usage-based from a data collection standpoint.

**Capacity-based metrics require only inventory-state Prometheus queries.** The key sources are:
- `kube_pod_container_resource_requests` — CPU/memory resource requests
- `kube_persistentvolumeclaim_resource_requests_storage_bytes` — PVC storage declarations
- `kubevirt_vm_resource_requests` — VM vCPU/memory allocations
- `kube_namespace_labels` — namespace/project existence

**Usage-based metrics require workload-instrumented Prometheus queries.** The key sources are:
- `container_cpu_usage_seconds_total` — real-time CPU consumption rate
- `container_memory_usage_bytes` — real-time memory consumption
- `kubelet_volume_stats_used_bytes` — actual bytes written to PVCs
- `kubevirt_vmi_info{phase='running'}` — VM running state for uptime tracking
