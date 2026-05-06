# WOMS Gthulhu HPA PoC Proposal

## Goal

This proposal upgrades the existing WOMS `scheduler-worker` HPA scenario into a Gthulhu-backed autoscaling PoC. WOMS should keep its current KEDA/HPA model with Kafka lag as the primary signal, then add pod-level scheduling information from Gthulhu as a second scaling signal. The HPA will then react not only to backlog or average CPU utilization, but also to the kernel scheduling pressure actually observed by worker pods on Kubernetes nodes.

The PoC does not replace the current WOMS HPA design. It establishes a reusable autoscaling pattern for future SD-Core SMF/UPF integration:

```text
workload pod information
  -> Gthulhu eBPF pod scheduling metrics
  -> Prometheus
  -> Grafana dashboard / operational observation
  -> KEDA prometheus trigger
  -> Kubernetes HPA
  -> target workload replicas
```

## Why Use Gthulhu

The current WOMS HPA pressure source is mainly Kafka lag on `woms.schedule.jobs`. That answers whether scheduling jobs are piling up, but it does not answer whether the worker pods are being delayed by CPU scheduling, frequent preemption, excessive migration, or noisy neighbors on the node. Gthulhu fills that gap.

The main reasons to use Gthulhu are:

1. **Pod-level kernel scheduling pressure**
   Gthulhu uses eBPF to collect per-process scheduling events and aggregate them into pod-level metrics such as `wait_time_ns`, `run_count`, `involuntary_ctx_switches`, `cpu_migrations`, `numa_migrations`, and `process_count`. These metrics are closer to "is the pod being delayed by scheduling pressure?" than Kubernetes CPU utilization.

2. **No WOMS application code changes**
   WOMS does not need custom instrumentation in the Go API or worker to expose scheduling pressure. Gthulhu observes pods from the node/kernel layer, which makes it suitable as a supervisory deployment capability.

3. **Kubernetes-native operation through KEDA/HPA**
   Gthulhu does not need to directly control replica counts. It exposes metrics to Prometheus, and KEDA's prometheus trigger feeds them into HPA. This extends the current WOMS Helm/KEDA/HPA architecture instead of replacing it with a separate autoscaler.

4. **Observability and threshold calibration**
   Prometheus is required for the Gthulhu-backed HPA data path because KEDA needs the prometheus trigger to query Gthulhu metrics. Grafana is not required for HPA itself, but it should be part of the PoC so operators can correlate Kafka lag, worker replicas, Gthulhu scheduling pressure, and HPA events instead of seeing replica changes without knowing why they happened.

5. **A practical bridge to SD-Core SMF/UPF**
   SMF and UPF are more sensitive to CPU scheduling jitter, preemption, migration, NUMA locality, and packet-processing latency. Validating the "Gthulhu pod information -> KEDA -> HPA" loop with WOMS first makes it easier to reuse the same pattern for SMF control-plane pods or UPF data-plane pods.

6. **Better high-pressure diagnosis than CPU alone**
   High CPU utilization does not always mean scale-out is needed, and low CPU utilization can still hide run-queue wait or preemption problems. Gthulhu metrics complement Kafka lag and CPU utilization with runtime scheduling evidence.

## Relationship Between WOMS And Gthulhu

The WOMS `scheduler-worker` is a Kafka consumer. During end-of-month scheduling, rush-order recovery, or demo peak generation, many jobs are published to `woms.schedule.jobs`. Kafka lag rises, and KEDA currently creates and drives `woms-woms-worker-hpa` for `Deployment/woms-woms-worker`.

Gthulhu becomes the supervisory feedback layer in this scenario:

- Gthulhu monitors WOMS worker pods.
- The eBPF collector gathers worker pod scheduling metrics.
- Prometheus scrapes Gthulhu `/metrics`.
- The WOMS KEDA `ScaledObject` adds a prometheus trigger.
- HPA uses Kafka lag, CPU utilization, and Gthulhu scheduling pressure together to decide worker replicas.

The PoC should target only `scheduler-worker` first, not API/web. The worker pressure source is clear, the Kafka lag baseline already exists, and scaling worker pods does not directly change the request-path availability model.

## Proposed Architecture

```text
WOMS API
  -> Kafka topic woms.schedule.jobs
  -> WOMS scheduler-worker pods
        ^
        |
        | pod/process scheduling observation
        |
Gthulhu DaemonSet / monitor
  -> eBPF scheduling metrics
  -> Prometheus metrics:
       gthulhu_pod_wait_time_nanoseconds_total
       gthulhu_pod_involuntary_ctx_switches_total
       gthulhu_pod_cpu_migrations_total
       gthulhu_pod_numa_migrations_total
  -> Grafana dashboards:
       worker backlog / replicas / scheduling pressure
  -> KEDA prometheus trigger
  -> HPA woms-woms-worker-hpa
  -> Deployment woms-woms-worker replicas
```

## Prometheus / Grafana Plan

WOMS does not currently deploy Prometheus or Grafana. The current HPA path uses KEDA's Kafka trigger, the CPU trigger, and `metrics-server`. After Gthulhu is introduced, Prometheus and Grafana should be positioned as follows:

- **Prometheus: required**
  Gthulhu pod scheduling metrics must be scraped by Prometheus, and KEDA's prometheus trigger needs Prometheus queries to convert Gthulhu metrics into HPA external metrics. Without Prometheus, the Gthulhu -> KEDA -> HPA loop cannot be completed.

- **Grafana: included in the PoC as an observation and calibration tool**
  Grafana does not directly participate in HPA decisions, but the PoC should include dashboards for Kafka lag, worker replicas, HPA desired replicas, Gthulhu `involuntary_ctx_switches`, `wait_time`, `cpu_migrations`, and `numa_migrations`. This helps calibrate thresholds and identify whether scale-out is caused by backlog, CPU, or kernel scheduling pressure.

- **metrics-server: keep it**
  `metrics-server` still provides the resource metrics needed by the current CPU trigger. Prometheus/Grafana do not replace it.

The PoC monitoring stack should use `kube-prometheus-stack` or an existing platform Prometheus/Grafana. The WOMS Helm chart does not need to vendor the monitoring stack directly, but deployment and verification docs should list Prometheus/Grafana as prerequisites for the Gthulhu HPA PoC.

## Scaling Signal Design

### Keep The Existing Primary Signal: Kafka Lag

Kafka lag remains the primary autoscaling signal for WOMS workers:

- topic: `woms.schedule.jobs`
- consumer group: `woms-scheduler-workers`
- threshold: `keda.kafka.lagThreshold`
- purpose: represent scheduling jobs that workers have not consumed yet

### Keep The Existing Secondary Signal: CPU Utilization

The CPU trigger remains a secondary signal for compute-heavy scheduling bursts:

- target utilization: `keda.cpu.targetUtilization`
- purpose: support scale-out during CPU-heavy scheduling computation

### Add Gthulhu Signal: Pod Scheduling Pressure

The first phase should use `involuntary_ctx_switches` and `wait_time`, instead of adding too many metrics at once.

Recommended Prometheus query:

```promql
sum(
  rate(gthulhu_pod_involuntary_ctx_switches_total{
    namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
)
```

If the Gthulhu environment is already collecting `wait_time` reliably, add:

```promql
sum(
  rate(gthulhu_pod_wait_time_nanoseconds_total{
    namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
) / 1000000000
```

For the first implementation, use `involuntary_ctx_switches` as the KEDA prometheus trigger because it is easier to observe as a preemption pressure signal. Keep `wait_time` in dashboards and use it for later threshold calibration.

## Helm / Kubernetes Design Draft

### Gthulhu PodSchedulingMetrics

The current WOMS worker pod has these labels:

- `app.kubernetes.io/instance: <release>`
- `app.kubernetes.io/component: scheduler-worker`

PoC resource draft:

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

Implementation note: `/home/ubuntu/Gthulhu/monitor/crdwatcher/watcher.go` currently notes that `PodRef` does not carry pod labels, and `psmMatchesPod` does not yet enforce label selectors precisely. WOMS has two options:

1. **Short-term PoC**: isolate the test in namespace `woms`, then filter worker pods precisely in Prometheus with `pod_name=~"woms-woms-worker-.*"`.
2. **Before production integration**: extend Gthulhu `PodRef` and the informer path to include pod labels, then enforce `labelSelectors` accurately.

### Add A Prometheus Trigger To The WOMS KEDA ScaledObject

Add an optional trigger to `deploy/helm/woms/templates/keda-scaledobject.yaml`, with a new `keda.gthulhu` block in `values.yaml`.

Example values:

```yaml
keda:
  gthulhu:
    enabled: true
    prometheusServerAddress: http://prometheus-kube-prometheus-prometheus.monitoring:9090
    metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate
    threshold: "20"
    query: |
      sum(rate(gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Example trigger:

```yaml
- type: prometheus
  metadata:
    serverAddress: {{ .Values.keda.gthulhu.prometheusServerAddress | quote }}
    metricName: {{ .Values.keda.gthulhu.metricName | quote }}
    query: {{ .Values.keda.gthulhu.query | quote }}
    threshold: {{ .Values.keda.gthulhu.threshold | quote }}
```

## SD-Core SMF/UPF Extension Path

After the WOMS PoC succeeds, split the 5GC extension into two paths:

1. **SMF HPA**
   SMF is a control-plane NF. Its pressure usually comes from PDU session establishment, modification, release, and PFCP/N4 control-plane interaction. Gthulhu can observe whether SMF pods suffer scheduling latency or preemption, then combine that with request rate, session count, or queue depth to drive HPA.

2. **UPF HPA**
   UPF is a data-plane NF and is more sensitive to CPU locality, preemption, migration, NUMA placement, and packet-processing jitter. Gthulhu metrics such as `cpu_migrations`, `numa_migrations`, `wait_time`, and `involuntary_ctx_switches` are closer to UPF data-plane pressure than generic CPU utilization. However, UPF scale-out also requires traffic steering, PFCP session state, PDR/FAR/QER installation, and datapath consistency. It cannot be solved by replica count alone.

WOMS is valuable because it validates the autoscaling feedback loop before the 5GC stateful datapath problem is introduced.

## PoC Phases

### Phase 1: Observe Without Changing HPA Decisions

- Deploy Gthulhu.
- Deploy or connect Prometheus, and confirm it can scrape Gthulhu `/metrics`.
- Deploy or connect Grafana, and create a WOMS worker / Gthulhu scheduling dashboard.
- Create the WOMS `PodSchedulingMetrics`.
- Scrape Gthulhu metrics with Prometheus.
- Verify worker pod metrics with Grafana or Prometheus queries.
- Run the WOMS HPA demo with 200 lines, 1,000 orders, and 200 jobs, then check whether Kafka lag and Gthulhu metrics rise together.

### Phase 2: Add The Gthulhu Prometheus Trigger To KEDA

- Add `keda.gthulhu.enabled` to the WOMS Helm chart.
- Keep the Kafka lag and CPU triggers.
- Add the prometheus trigger.
- Start with a conservative threshold so Gthulhu does not over-scale too early.
- Verify HPA events show scale-up from the external metric.

### Phase 3: Calibrate Thresholds And Failure Policy

- Test multiple job volumes.
- Compare scale-up timing between "Kafka lag only" and "Kafka lag + Gthulhu".
- Define behavior when Gthulhu or Prometheus is unavailable. Missing Gthulhu metrics must not break Kafka lag scaling.
- Decide whether `wait_time` should become a trigger or remain dashboard-only.

### Phase 4: Generalize For SD-Core

- Turn metric query, threshold, target workload, namespace, and label selector into values.
- Create SMF/UPF variants of `PodSchedulingMetrics` and KEDA prometheus trigger templates.
- Add NUMA / CPU migration dashboards and alerts for UPF.

## Verification

WOMS-side checks:

```bash
./scripts/verify-hpa-render.sh
go test ./...
npm test
```

Kubernetes-side checks:

```bash
kubectl get pods -n woms -l app.kubernetes.io/component=scheduler-worker
kubectl get podschedulingmetrics -n woms
kubectl get hpa -n woms
kubectl describe hpa woms-woms-worker-hpa -n woms
kubectl get scaledobject -n woms
```

Prometheus queries:

```promql
sum(rate(gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
sum(rate(gthulhu_pod_wait_time_nanoseconds_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

The Grafana dashboard should include at least:

- Kafka lag: `woms.schedule.jobs` / `woms-scheduler-workers`
- worker replicas: current / desired
- HPA events or desired replica changes
- Gthulhu worker pod `involuntary_ctx_switches` rate
- Gthulhu worker pod `wait_time` rate
- Gthulhu worker pod `cpu_migrations` / `numa_migrations` rate

Success criteria:

- Gthulhu can observe WOMS worker pod metrics.
- Prometheus can scrape and query Gthulhu worker pod metrics.
- Grafana can show the worker backlog, replicas, and Gthulhu scheduling pressure dashboard.
- Kafka lag rises during the WOMS HPA demo.
- Gthulhu scheduling pressure metrics change while workers are busy.
- The KEDA `ScaledObject` contains Kafka, CPU, and Gthulhu prometheus triggers.
- HPA can scale out `woms-woms-worker`, and scale-down is not more aggressive than the current cooldown behavior.

## Risks And Limits

1. **Gthulhu label selectors may not be precise yet**
   The current Gthulhu watcher does not store pod labels in `PodRef`, so production integration should either add label matching or filter precisely with `pod_name` in Prometheus.

2. **Thresholds require real calibration**
   Absolute values for `involuntary_ctx_switches` and `wait_time` depend on kernel version, CPU topology, neighboring workloads, and worker resource requests. They cannot be copied directly to SD-Core.

3. **Do not let the Gthulhu trigger override Kafka lag**
   Kafka lag remains the primary WOMS backlog signal. Gthulhu should complement it with pod scheduling pressure, not become the only scaling source.

4. **UPF cannot be solved by HPA alone**
   UPF includes flow/session state and datapath routing. Gthulhu + KEDA can provide better pressure signals, but scale-out must still coordinate with the 5GC control plane and traffic steering.

## Recommendation

WOMS should adopt a "Kafka lag primary, CPU secondary, Gthulhu scheduling pressure complementary" HPA design. The reason to use Gthulhu is that it exposes pod-level kernel scheduling information that Kubernetes/HPA cannot see natively, while still integrating through Prometheus and KEDA into the existing HPA loop. This makes WOMS a low-risk PoC and creates a shared architecture for future Gthulhu-backed SD-Core SMF/UPF autoscaling.
