# WOMS Gthulhu HPA PoC Design

## Source Baseline

This document is based on the current WOMS repository state and the local Gthulhu repository at `/home/ubuntu/Gthulhu` on branch `develop`, commit `00fc41f`. Before this update, the Gthulhu source was refreshed with:

```bash
cd /home/ubuntu/Gthulhu
git pull origin develop
```

Git reported `Already up to date.`

The current WOMS architecture is no longer only a deployment skeleton. The Helm chart deploys:

- Go API deployment `woms-woms-api`, default `2` replicas, JWT/RBAC, PostgreSQL store, Redis address, Kafka publisher, readiness/liveness probes, and a PDB `woms-woms-api` with `minAvailable: 1`.
- Static web deployment `woms-woms-web`, default `2` replicas, NGINX proxy to the API, non-root read-only filesystem settings, and a PDB `woms-woms-web` with `minAvailable: 1`.
- Go scheduler worker deployment `woms-woms-worker`, default `1` replica, Kafka consumer group `woms-scheduler-workers`, PostgreSQL access, retry settings, and worker resources.
- PostgreSQL, Redis, and Kafka Helm dependencies enabled by default for local or clean-VM demos.
- A Kafka topic hook job that creates `woms.schedule.jobs`; when `kafkaTopic.partitions` is `0`, it uses `keda.maxReplicaCount`, so the topic has enough partitions for scaled workers.
- KEDA `ScaledObject` `woms-woms-worker`, which creates HPA `woms-woms-worker-hpa` targeting `Deployment/woms-woms-worker`.

The current KEDA/HPA baseline is:

- Kafka trigger enabled by default.
- CPU trigger enabled by default.
- `minReplicaCount: 1`.
- `maxReplicaCount: 10`.
- `pollingInterval: 30`.
- `cooldownPeriod: 120`.
- scale-up: 100 percent every 30 seconds, no stabilization window.
- scale-down: 50 percent every 60 seconds, 120-second stabilization window.

## Confidence Position

This plan is not a 100 percent deployment guarantee yet. It is the safest code-based strategy after reviewing the current WOMS chart and Gthulhu `develop` source. A 100 percent claim requires a live cluster run that proves Gthulhu emits pod-labeled Prometheus metrics, Prometheus can scrape them, KEDA can read the query, and the existing WOMS worker HPA still behaves correctly.

The main correction from the earlier proposal is this:

- Gthulhu manager/API code can list pods with labels and can apply Kubernetes label selectors in its manager-side pod inventory path.
- The Gthulhu monitor/eBPF path used by `PodSchedulingMetrics` is a different path. On commit `00fc41f`, this path still needs live verification before WOMS can depend on it for HPA.
- If Gthulhu already provides a fully working, deployment-tested scaler path for this exact use case, WOMS should not duplicate it in application code. WOMS may still need a small Helm change if it wants Kafka, CPU, and Gthulhu metrics in the same existing `ScaledObject`.

## Goal

Add a Gthulhu-backed scheduling-pressure signal to the existing WOMS scheduler-worker autoscaling path without replacing the current Kafka lag and CPU triggers.

The target NF-like component in WOMS is `scheduler-worker`: it is the asynchronous scheduling executor. The API accepts schedule requests and publishes jobs to Kafka; the worker consumes `woms.schedule.jobs`, locks each production line while scheduling, persists allocations to PostgreSQL, and records audit results. During the HPA demo, the API creates 200 lines, 1,000 pending orders, and 200 queued jobs, then workers drain the backlog through consumer group `woms-scheduler-workers`.

The PoC should prove this loop:

```text
WOMS scheduling workload
  -> Kafka backlog on woms.schedule.jobs
  -> scheduler-worker pods consume jobs
  -> Gthulhu eBPF monitor observes pod scheduling events
  -> Prometheus stores Gthulhu pod metrics
  -> WOMS KEDA ScaledObject adds one prometheus trigger
  -> KEDA-created HPA adjusts Deployment/woms-woms-worker replicas
```

## Why Gthulhu Belongs In This PoC

Kafka lag answers whether scheduling jobs are waiting. CPU utilization answers whether worker containers are busy. Neither signal proves that a worker pod is delayed by kernel scheduling pressure, preemption, CPU migration, NUMA migration, or noisy neighbors on the node.

Gthulhu fills that gap by collecting pod-level scheduling metrics from the node/kernel layer. On the current `develop` branch, the Prometheus collector exposes these metric names with labels `pod_name`, `pod_uid`, `namespace`, and `node_name`:

- `gthulhu_pod_voluntary_ctx_switches_total`
- `gthulhu_pod_involuntary_ctx_switches_total`
- `gthulhu_pod_cpu_time_nanoseconds_total`
- `gthulhu_pod_wait_time_nanoseconds_total`
- `gthulhu_pod_run_count_total`
- `gthulhu_pod_cpu_migrations_total`
- `gthulhu_pod_smt_migrations_total`
- `gthulhu_pod_l3_migrations_total`
- `gthulhu_pod_numa_migrations_total`
- `gthulhu_pod_process_count`

For WOMS, `involuntary_ctx_switches`, `wait_time`, `cpu_migrations`, and `numa_migrations` are the most useful first metrics. They are closer to runtime scheduling pressure than average CPU utilization.

## Current Integration Boundary

WOMS does not currently vendor Gthulhu, Prometheus, or Grafana into `deploy/helm/woms`. That boundary should remain for the first PoC:

- WOMS Helm owns WOMS workloads, PostgreSQL, Redis, Kafka, the worker `ScaledObject`, PDBs, and optional Ingress.
- Gthulhu Helm owns Gthulhu CRDs, monitor, eBPF collector, ServiceMonitor, and any Gthulhu-specific runtime components.
- The platform owns Prometheus/Grafana, typically through `kube-prometheus-stack` or an existing monitoring stack.

KEDA should not receive two independent scaling controllers for the same WOMS worker deployment. KEDA's official `ScaledObject` model puts all triggers for one target workload inside one `ScaledObject`; that `ScaledObject` owns the generated HPA for the target workload. WOMS already has the worker `ScaledObject` that combines Kafka and CPU triggers.

Gthulhu has a chart-level example `ScaledObject`, and `PodSchedulingMetrics.spec.scaling` exists in the CRD/API model. However, the current code review did not find a production-ready controller path that automatically converts a live `PodSchedulingMetrics.spec.scaling` resource into the exact WOMS worker scaler while preserving WOMS' Kafka trigger, CPU trigger, HPA behavior, and naming. Therefore the default PoC strategy is:

- keep WOMS as the owner of `ScaledObject/woms-woms-worker`;
- add Gthulhu as one optional prometheus trigger inside that existing object;
- do not enable Gthulhu's example scaler or PSM scaling hints for `woms-woms-worker` unless a live deployment proves it can own the full combined scaler safely.

## Important Gthulhu Develop-Branch Limitations

On Gthulhu `develop` commit `00fc41f`, `PodSchedulingMetrics` requires `spec.labelSelectors`, and the CRD also exposes `k8sNamespaces`, `commandRegex`, metric flags, and optional scaling hints.

### Label Selector Split

Gthulhu manager/API and Gthulhu monitor/eBPF do not currently expose the same selector guarantees.

- The manager-side Kubernetes adapter copies `pod.Labels` and uses `selector.Matches(labels.Set(pod.Labels))`, so the manager UI/API can know labels.
- The monitor path uses `monitor/collector.PodRef`, which currently contains `PodName`, `PodUID`, `Namespace`, and `NodeName`, but no labels.
- `monitor/crdwatcher/watcher.go` checks `k8sNamespaces`, then documents that `PodRef` does not carry labels and returns `true`. Therefore `labelSelectors` and `commandRegex` must not be treated as hard worker-only selection in the monitor path yet.

That means a short-term WOMS PoC can scope Gthulhu collection to namespace `woms`, but it should use Prometheus label filtering such as `pod_name=~"woms-woms-worker-.*"` when feeding KEDA and Grafana. Before production reuse, Gthulhu should extend `PodRef` and the informer path with labels, then enforce `labelSelectors` and, if needed, command matching.

### Pod Index And Scrape Path Must Be Proven

The monitor collector aggregates eBPF per-PID metrics into pod metrics through `PodMapper.GetPodForPID`. That function needs a pod UID index populated through `SetPodIndex`. In the reviewed `develop` source, `SetPodIndex` is visible in tests, but the monitor startup path does not clearly show a Kubernetes informer populating it in production.

The chart also needs a live scrape check. The current chart has ServiceMonitor templates for manager/sidecar and a monitor-specific ServiceMonitor, but the monitor `/metrics` endpoint and service port wiring must be verified in the running cluster before KEDA depends on `gthulhu_pod_*` metrics.

These are Gthulhu-side verification/fix items, not WOMS application-code items. If the live deployment does not expose `gthulhu_pod_*{pod_name=...,namespace=...}`, fix Gthulhu monitor pod mapping and metrics service discovery first.

## Target Architecture

```text
User / web UI
  -> Go API
  -> PostgreSQL schedule_jobs row
  -> Kafka topic woms.schedule.jobs
  -> scheduler-worker pods
       -> PostgreSQL schedule_allocations
       -> audit_logs

KEDA baseline:
  Kafka lag trigger + CPU trigger
  -> ScaledObject woms-woms-worker
  -> HPA woms-woms-worker-hpa
  -> Deployment woms-woms-worker

Gthulhu PoC extension:
  Gthulhu monitor / eBPF collector
  -> Prometheus scrape of /metrics
  -> PromQL query filtered to worker pods
  -> WOMS ScaledObject prometheus trigger
  -> same HPA woms-woms-worker-hpa
```

Grafana is not part of HPA decision-making. It is required operationally for this PoC because thresholds cannot be chosen safely without seeing Kafka lag, worker replicas, HPA desired replicas, Gthulhu metrics, and HPA events together.

## Scaling Signal Policy

### Primary Signal: Kafka Lag

Keep Kafka lag as the primary WOMS worker autoscaling signal:

- topic: `woms.schedule.jobs`
- consumer group: `woms-scheduler-workers`
- threshold: `keda.kafka.lagThreshold`, currently `"10"`
- reason: backlog directly represents scheduling work not yet consumed

### Secondary Signal: CPU Utilization

Keep CPU utilization as a secondary signal:

- trigger type: `cpu`
- metric type: `Utilization`
- target: `keda.cpu.targetUtilization`, currently `"70"`
- reason: scheduling can be compute-heavy even before Kafka lag becomes large

### Complementary Signal: Gthulhu Scheduling Pressure

Use Gthulhu as a complementary signal, not a replacement for Kafka lag.

Recommended first trigger query:

```promql
avg(
  rate(gthulhu_pod_involuntary_ctx_switches_total{
    namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
)
```

This query returns a scalar average involuntary context-switch rate per worker pod. For an HPA trigger, an average per pod is safer than a raw cluster-wide sum because a sum can rise as replicas increase and can create positive feedback.

Dashboard-only calibration query for run-queue wait:

```promql
avg(
  rate(gthulhu_pod_wait_time_nanoseconds_total{
    namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
) / 1000000000
```

Also chart these in Grafana before turning them into triggers:

```promql
avg(rate(gthulhu_pod_cpu_migrations_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_numa_migrations_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
max by (pod_name) (rate(gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Thresholds must be calibrated on the actual WOMS cluster.

## Required WOMS Helm Change After Gthulhu Preflight

The current WOMS chart does not yet have `keda.gthulhu`. Add this only after Gthulhu preflight proves the Prometheus query returns a stable scalar. The optional values block should be:

```yaml
keda:
  gthulhu:
    enabled: false
    prometheusServerAddress: http://prometheus-kube-prometheus-prometheus.monitoring:9090
    metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate
    threshold: "20"
    query: |
      avg(rate(gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Then extend `deploy/helm/woms/templates/keda-scaledobject.yaml` inside the existing `triggers:` list:

```yaml
{{- if .Values.keda.gthulhu.enabled }}
- type: prometheus
  metadata:
    serverAddress: {{ .Values.keda.gthulhu.prometheusServerAddress | quote }}
    metricName: {{ .Values.keda.gthulhu.metricName | quote }}
    query: {{ tpl .Values.keda.gthulhu.query . | quote }}
    threshold: {{ .Values.keda.gthulhu.threshold | quote }}
{{- end }}
```

After that change, update `deploy/helm/woms/chart-static.test.mjs` and `scripts/verify-hpa-render.sh` so CI can prove the optional prometheus trigger renders only when enabled.

If Gthulhu later provides a verified controller that can own the complete WOMS worker scaler, including Kafka lag, CPU utilization, Gthulhu Prometheus metrics, HPA name, min/max replicas, and scale behavior, then this Helm change can be replaced by delegating the whole worker `ScaledObject` to that controller. Do not split ownership between two charts for the same worker deployment.

## Gthulhu PodSchedulingMetrics Draft

Use a namespace-scoped PSM for the first PoC. The label selectors document intent, but current Gthulhu develop behavior should be assumed namespace-only until label matching is fixed.

```yaml
apiVersion: gthulhu.io/v1alpha1
kind: PodSchedulingMetrics
metadata:
  name: woms-scheduler-worker
  namespace: woms
spec:
  enabled: true
  k8sNamespaces:
    - woms
  labelSelectors:
    - key: app.kubernetes.io/instance
      value: woms
    - key: app.kubernetes.io/component
      value: scheduler-worker
  collectionIntervalSeconds: 10
  metrics:
    voluntaryCtxSwitches: true
    involuntaryCtxSwitches: true
    cpuTimeNs: true
    waitTimeNs: true
    runCount: true
    cpuMigrations: true
```

Do not enable `spec.scaling` for this WOMS target during the PoC. Keep scaling in the WOMS `ScaledObject`.

## PoC Phases

### Phase 0: Gthulhu Preflight

- Deploy or connect Prometheus/Grafana.
- Deploy Gthulhu from the `develop` branch or the matching image/chart produced from it.
- Confirm the monitor is actually running, not only the manager UI.
- Confirm Prometheus can scrape the monitor `/metrics` endpoint.
- Confirm at least one query returns pod-labeled data:

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

- If the query returns no data, stop the WOMS HPA integration and fix Gthulhu pod index population or ServiceMonitor/service wiring first.

### Phase 1: Observe Only

- Create the WOMS `PodSchedulingMetrics`.
- Run the existing WOMS HPA peak demo from the web UI.
- Confirm Kafka lag and Gthulhu worker metrics move during the same workload.

### Phase 2: Add The WOMS Prometheus Trigger

- Add the optional `keda.gthulhu` values and trigger template to WOMS.
- Render the chart with `keda.gthulhu.enabled=true`.
- Confirm the rendered `ScaledObject` still targets only `woms-woms-worker`.
- Confirm the triggers are Kafka, CPU, and one Gthulhu prometheus trigger.
- Start with a conservative threshold.

### Phase 3: Calibrate

- Compare three cases: Kafka+CPU only, Kafka+CPU+Gthulhu observe-only, and Kafka+CPU+Gthulhu trigger enabled.
- Test different `WORKER_MIN_JOB_DURATION_MS`, order volumes, and worker resource requests.
- Check whether Gthulhu causes earlier scale-out only when worker pods show real scheduling pressure.
- Confirm scale-down still follows the existing 120-second cooldown and stabilization behavior.

## Verification

WOMS static and unit checks:

```bash
./scripts/verify-hpa-render.sh
go test ./...
npm run test:web
```

Render check after adding `keda.gthulhu`:

```bash
helm template woms ./deploy/helm/woms --dependency-update \
  --namespace woms \
  --set keda.gthulhu.enabled=true \
  --set keda.gthulhu.prometheusServerAddress=http://prometheus-kube-prometheus-prometheus.monitoring:9090
```

Kubernetes checks:

```bash
kubectl get deploy,pod,scaledobject,hpa -n woms
kubectl get pods -n woms -l app.kubernetes.io/component=scheduler-worker
kubectl describe scaledobject woms-woms-worker -n woms
kubectl describe hpa woms-woms-worker-hpa -n woms
kubectl get podschedulingmetrics -n woms
```

Prometheus checks:

```promql
avg(rate(gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_wait_time_nanoseconds_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) / 1000000000
avg(rate(gthulhu_pod_cpu_migrations_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_numa_migrations_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Grafana dashboard minimum panels:

- Kafka lag for `woms.schedule.jobs` / `woms-scheduler-workers`.
- Worker current replicas and desired replicas.
- HPA events or desired replica changes.
- Worker CPU utilization.
- Gthulhu worker involuntary context-switch rate.
- Gthulhu worker wait-time rate.
- Gthulhu worker CPU migration and NUMA migration rates.

## Success Criteria

- WOMS still scales workers from Kafka lag without Gthulhu enabled.
- Gthulhu can observe WOMS worker pod metrics through Prometheus.
- The optional Gthulhu prometheus trigger is part of the same WOMS `ScaledObject`, not a second scaler for the same deployment.
- Missing Gthulhu/Prometheus data does not remove the Kafka lag path.
- HPA scale-down remains no more aggressive than the current WOMS behavior.
- Grafana can explain whether a scale-out came from backlog, CPU, or Gthulhu scheduling pressure.

## Risks And Controls

1. **Current Gthulhu label matching is not precise**
   Treat `PodSchedulingMetrics` as namespace-scoped on develop commit `00fc41f`; use PromQL `pod_name` filtering for WOMS worker metrics until Gthulhu adds label-aware `PodRef` matching.

2. **Two HPAs for one worker deployment would conflict**
   Do not enable Gthulhu chart example scaling or `PodSchedulingMetrics.spec.scaling` for `woms-woms-worker`. Add Gthulhu as a trigger inside WOMS' existing `ScaledObject`.

3. **Thresholds are environment-specific**
   Gthulhu metric values depend on kernel version, CPU topology, node pressure, worker resource requests, and co-located workloads.

4. **Gthulhu is not a backlog signal**
   Kafka lag remains the main queue-depth source. Gthulhu is for runtime scheduling evidence.

## Recommendation

Use the current WOMS Kafka+CPU KEDA/HPA path as the stable baseline. Add Gthulhu as one optional prometheus trigger in the existing WOMS `ScaledObject`, keep Grafana as a calibration requirement, and do not enable any separate Gthulhu-managed scaler for `woms-woms-worker`. The first usable trigger should be average worker-pod involuntary context-switch rate, with wait time and migration metrics kept in dashboards until real cluster thresholds are known.
