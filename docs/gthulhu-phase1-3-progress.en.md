# Gthulhu Phase 1-3 Progress Log

## 2026-05-14 Status

### User Goal

- Run Phase 1 observe-only.
- If Phase 1 fully passes, continue to Phase 2.
- If Phase 2 fully passes, continue to Phase 3.
- Push the Gthulhu development work to the user's fork: `https://github.com/d11nn/Gthulhu.git`, branch `feat/woms-poc`.

### Completed: Gthulhu Fork Branch

Created the branch in `/home/ubuntu/Gthulhu` from the latest `origin/develop`:

```text
feat/woms-poc
```

Base commit:

```text
00fc41feeb2a23481b12aa27898cc8e4cd215b42
```

Committed and pushed to the user's fork:

```text
remote: https://github.com/d11nn/Gthulhu.git
branch: feat/woms-poc
commit: f71f78a feat: add monitor pod index refresh
```

GitHub suggested PR URL:

```text
https://github.com/d11nn/Gthulhu/pull/new/feat/woms-poc
```

This Gthulhu branch includes:

- `monitor/monitor.go`
  - Creates an in-cluster Kubernetes client when the monitor starts.
  - Lists local-node pods every 30 seconds using `spec.nodeName=<NODE_NAME>`.
  - Writes `pod.UID -> collector.PodRef` into `PodMapper.SetPodIndex()`.
  - Logs `pod index refreshed` on success.
- `Dockerfile`
  - Copies `/build/sched_monitor.bpf.o` into `/gthulhu/sched_monitor.bpf.o` in the runtime image.
- `go.mod` / `go.sum`
  - Updated by `go mod tidy`.

Validation:

- `git diff --check` passed.
- The same patch was used successfully in the Phase 0 Docker build:
  - image: `localhost:32000/gthulhu-monitor:phase0`
  - digest: `sha256:a8bbc6578ed79dc5947fd5b02aaa30fb76b500139eabb01b89b1e28260518c10`
- The same image passed Phase 0 on MicroK8s:
  - `pod index refreshed`
  - `sched_monitor BPF program loaded`
  - `attached BPF program handle_sched_switch`
  - `attached BPF program handle_sched_process_exit`
  - `count(gthulhu_pod_process_count{pod_name!="",namespace!=""}) = 21`

Limitation:

- Local `go test ./monitor/...` failed because the host lacks libbpf headers:

```text
fatal error: bpf/bpf.h: No such file or directory
```

This is a host dependency issue, not a syntax or module issue in the submitted patch; the Docker build path passed end to end.

### Phase 1 Observe-only Result

Phase 1 gates from `docs/gthulhu-hpa-poc.en.md`:

- Create the WOMS `PodSchedulingMetrics`.
- Run the existing WOMS HPA peak demo from the web UI.
- Confirm Kafka lag and Gthulhu worker metrics move during the same workload window.

Applied the observe-only manifest:

```text
/tmp/woms-podschedulingmetrics.yaml
```

The manifest is a namespace-scoped observe-only PSM and does not include `spec.scaling`, so it does not create another scaler and does not modify the WOMS `ScaledObject` or HPA.

```bash
kubectl apply -f /tmp/woms-podschedulingmetrics.yaml
```

Result:

- `PodSchedulingMetrics/woms-scheduler-worker` exists in namespace `woms`.
- `spec.enabled: true`, `collectionIntervalSeconds: 10`.
- `k8sNamespaces: ["woms"]`.
- `labelSelectors` target `app.kubernetes.io/instance=woms` and `app.kubernetes.io/component=scheduler-worker`.
- No `spec.scaling`.

After running the WOMS HPA peak demo, the API reported:

```text
lineCount: 200
orderCount: 1000
jobCount: 200
statuses: queued=200
createdAt: 2026-05-14T17:50:15Z
```

During the same workload window:

- The HPA external metric rose from about `9` to `14500m`, then about `10334m/10 (avg)`.
- The worker deployment scaled from 1 replica to 3 replicas.
- Demo jobs completed: `statuses completed=200`.
- Gthulhu `/metrics` exposed pod-labeled `gthulhu_pod_*` metrics for `woms-woms-worker-*`.
- After Prometheus scrape, Gthulhu's raw `namespace="woms"` label is preserved as `exported_namespace="woms"` because the scrape target already has `namespace="gthulhu-system"`.
- Prometheus queries:
  - `avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) = 0.6296506179835625`
  - `avg(rate(gthulhu_pod_wait_time_nanoseconds_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) / 1000000000 = 5.244143697741777`

Phase 1 gate conclusion: passed.

### Phase 2 Result

Phase 2 goals:

- Add optional `keda.gthulhu` values and trigger template to WOMS.
- Render the chart with `keda.gthulhu.enabled=true`.
- Confirm the rendered `ScaledObject` still targets only `woms-woms-worker`.
- Confirm the triggers are Kafka, CPU, and one Gthulhu prometheus trigger.

Changed on branch `feat/gthulhu-keda-trigger`:

- `deploy/helm/woms/values.yaml`
  - Adds `keda.gthulhu.enabled: false`.
  - Defaults Prometheus server to `http://monitoring-kube-prometheus-prometheus.monitoring:9090`.
  - Uses `exported_namespace="woms"` in the query.
- `deploy/helm/woms/templates/keda-scaledobject.yaml`
  - Adds one `type: prometheus` trigger only when `keda.gthulhu.enabled=true`.
- `deploy/helm/woms/chart-static.test.mjs`
  - Verifies values and template wiring.
- `scripts/verify-hpa-render.sh`
  - Verifies Kafka+CPU by default.
  - Verifies Kafka+CPU+Gthulhu when `GTHULHU_ENABLED=true`.
- `README.md`, `README.en.md`, `README.zh-TW.md`
  - Document the optional Gthulhu trigger, single `ScaledObject` ownership, and `exported_namespace`.
- `docs/gthulhu-hpa-poc.zh-TW.md`, `docs/gthulhu-hpa-poc.en.md`
  - Update PromQL and Phase 2 implementation status.

Render evidence:

- Default render does not include the Gthulhu trigger.
- `GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh` passed.
- `helm template ... --set keda.gthulhu.enabled=true` renders a `ScaledObject` with:
  - `scaleTargetRef.name: woms-woms-worker`
  - triggers: `kafka`, `cpu`, `prometheus`
  - prometheus `metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate`
  - prometheus query using `gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}`

Tests:

- `node --test deploy/helm/woms/chart-static.test.mjs` passed.
- `./scripts/verify-hpa-render.sh` passed.
- `GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh` passed.
- `npm run test:web` passed.
- `GOCACHE=/tmp/woms-go-build-cache go test ./...` passed.

Live cluster safety boundary:

- Did not run `helm upgrade` to enable the Gthulhu trigger.
- `kubectl get scaledobject woms-woms-worker -n woms -o yaml` confirms the live `ScaledObject` still has only Kafka and CPU triggers.

Phase 2 gate conclusion: passed for chart/code/render validation; live WOMS HPA trigger was not changed.

### Phase 3 Result

Phase 3 originally required explicit approval because it changes the live WOMS HPA trigger set. The user later approved it, so the live Gthulhu trigger was enabled and calibrated.

Live upgrade:

```text
release: woms
namespace: woms
revision: 8
time: 2026-05-14T18:02:30Z
```

Live `ScaledObject/woms-woms-worker` after enabling:

- `scaleTargetRef.name: woms-woms-worker`
- triggers: Kafka, CPU, Gthulhu Prometheus
- Kafka trigger unchanged: `lagThreshold: "10"`
- CPU trigger unchanged: `value: "70"`
- Gthulhu trigger:
  - `metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate`
  - `threshold: "20"`
  - query: `avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))`
- KEDA status:
  - `s0-kafka-woms-schedule-jobs: Happy`
  - `s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate: Happy`

Phase 3 baseline demo result:

- demo created at `2026-05-14T18:03:32Z`
- workers scaled from 3 replicas to 8 replicas.
- HPA events showed Kafka as the scale-out reason:
  - `New size: 6; reason: external metric s0-kafka-woms-schedule-jobs above target`
  - `New size: 8; reason: external metric s0-kafka-woms-schedule-jobs above target`
- The Gthulhu trigger was calculated by HPA, but the conservative threshold `20` did not drive scale-out:
  - example HPA metric: `2245m/20`
  - Prometheus scalar: `avg(rate(...)) = 6.7333333333333325`
- Scale-down behavior was not broken by the Gthulhu trigger:
  - Kafka, Gthulhu, and CPU all fell below target after the workload.
  - HPA began scaling down according to the 120-second stabilization and 50% policy.

Conclusion: Phase 3 live wiring passed; conservative threshold `20` does not over-trigger. This baseline is not proof that Gthulhu drove scale-out.

### 2026-05-15 Gthulhu-trigger Proof Scenario

Goal: prove that the WOMS HPA can be driven by the Gthulhu Prometheus trigger, not merely that the metric is wired into HPA.

Actions:

- Temporarily lowered `keda.gthulhu.threshold` from `20` to `1`.
- Kept Kafka trigger `lagThreshold: "10"` unchanged.
- Kept CPU trigger `value: "70"` unchanged.
- Re-ran the WOMS HPA peak demo.

Baseline:

- Kafka: `7/10`
- Gthulhu: `999m/1`
- worker replicas: `1`
- KEDA scaler health: Kafka and Gthulhu were both `Happy`

demo:

```text
createdAt: 2026-05-15T05:02:03Z
lineCount: 200
orderCount: 1000
jobCount: 200
```

Key observations:

- After demo jobs completed, Kafka was below target while Gthulhu was above target:
  - Kafka: `8125m/10`
  - Gthulhu: `3243m/1`
  - workers: `10/10`
- HPA events explicitly showed the Gthulhu Prometheus trigger driving scale-out:
  - `New size: 4; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
  - `New size: 8; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
  - `New size: 10; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
- A later Prometheus query still returned a scalar:
  - `avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) = 1.8000600020000668`

Proof scenario conclusion: passed. Gthulhu is not only scraped; through the KEDA Prometheus scaler it can become the source of an HPA scale decision.

### 2026-05-15 Realistic Pressure Scenario

Goal: use a higher threshold, add more realistic scheduling pressure, and see whether Gthulhu can still drive or hold a scale decision after Kafka lag falls below target.

Attempt 1: node-level CPU pressure

- Set `keda.gthulhu.threshold` to `5`.
- Created a short-lived CPU pressure Job:
  - namespace: `woms`
  - job: `woms-node-cpu-pressure`
  - parallelism: `4`
  - image: `docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`
  - each pod ran several `yes > /dev/null` loops.
- Observed about 3.4 cores of pressure.

Result:

- demo created at `2026-05-15T05:10:23Z`
- demo jobs completed.
- HPA scale-out reason was still Kafka:
  - `New size: 10; reason: external metric s0-kafka-woms-schedule-jobs above target`
- Gthulhu stayed below threshold:
  - HPA: `125m/5`
  - Prometheus scalar: `1.2421856569945742`
  - wait-time rate: `2.172601850609666`

Attempt 2: ephemeral CPU pressure inside a real worker pod, without restarting Gthulhu

- Restored worker runtime to the production/demo default:
  - `WORKER_MIN_JOB_DURATION_MS=0`
- Waited until Kafka and Gthulhu were below target:
  - Kafka: `2/10`
  - Gthulhu: `0/5`
  - worker: `1` replica
- Injected an ephemeral container into a real `woms-woms-worker-*` pod:
  - container: `gthulhu-worker-pressure`
  - image: `docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`
  - workload: 12 `yes > /dev/null` loops for 180 seconds.

Result:

- The ephemeral container did run in the worker pod; `ps` showed multiple `yes` processes.
- Gthulhu did not expose new worker-pod `gthulhu_pod_*` series:
  - Prometheus `{__name__=~"gthulhu_pod_.*",pod_name=~"woms-woms-worker-.*"}` returned empty.
  - Direct `gthulhu-scheduler-sidecar` `/metrics` also did not show new `woms-woms-worker-*` series.
- HPA did not show a Gthulhu reason; it briefly showed the CPU resource trigger:
  - `New size: 2; reason: cpu resource utilization above target`

Attempt 3: restart Gthulhu monitor, then retry worker-pod ephemeral CPU pressure

- Temporarily kept `keda.gthulhu.threshold: "5"`.
- Ran `kubectl rollout restart daemonset/gthulhu-scheduler -n gthulhu-system`.
- Waited for Gthulhu to reload BPF and refresh the pod index:
  - `sched_monitor BPF program loaded`
  - `pod index refreshed`
- Injected ephemeral CPU pressure into a real `woms-woms-worker-*` pod again:
  - container: `gthulhu-worker-pressure2`
  - 16 `yes > /dev/null` loops for 180 seconds.

Result:

- Gthulhu worker pod series recovered:
  - `gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name="woms-woms-worker-6f96996ddd-vpn6w"}`
  - `gthulhu_pod_process_count{namespace="woms",pod_name="woms-woms-worker-6f96996ddd-vpn6w"} 23`
- Kafka was below target:
  - `500m/10`
- Gthulhu was above threshold:
  - HPA: `5197m/5`
  - Prometheus scalar: `21.13681462962963`
- CPU was also above target:
  - CPU: `462%/70%`
- Therefore HPA events still attributed scale-out to CPU:
  - `New size: 2/4/8/10; reason: cpu resource utilization above target`
- After pressure ended:
  - Kafka: `200m/10`
  - CPU: `1%/70%`
  - Gthulhu: `109m/5`
  - Prometheus scalar: `1.0844444444444443`
  - high replicas were due to scale-down stabilization, not continued Gthulhu pressure.

Realistic scenario conclusion: partially passed, but not fully. After restarting the Gthulhu monitor, real worker-pod pressure can push Gthulhu above threshold `5` while Kafka is below target. However, the same pressure also triggers the CPU scaler, so HPA attribution did not switch to the Gthulhu Prometheus metric. The proof scenario fully demonstrates that Gthulhu can drive HPA; the realistic scenario still needs a workload that raises scheduling pressure without letting CPU utilization dominate, or a test setup that temporarily isolates the CPU trigger for attribution.

Live state was restored after the experiment:

- `keda.gthulhu.threshold: "20"`
- `WORKER_MIN_JOB_DURATION_MS=0`
- Kafka scaler health: `Happy`
- Gthulhu scaler health: `Happy`

### Demo Reproduction Guidance

The currently reliable demo is the proof scenario:

1. Confirm the live trigger is enabled and KEDA scaler health is `Happy`.
2. Temporarily set `keda.gthulhu.threshold` to `1`.
3. Run the WOMS HPA peak demo.
4. Observe:
   - `kubectl describe hpa woms-woms-worker-hpa -n woms`
   - HPA events show `s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
   - the Prometheus query returns a scalar.
5. Restore the threshold to conservative value `20` after the demo.

What Gthulhu demonstrated:

- It produced WOMS worker pod scheduling metrics through the eBPF / monitor path.
- Prometheus scraped those metrics and made them readable by the KEDA Prometheus scaler.
- It joined the existing WOMS `ScaledObject` as a third trigger without replacing Kafka or CPU.
- At proof threshold, it directly caused HPA scale-out.

Not yet claimed:

- Threshold `5` or `20` is a stable production scheduling-pressure threshold on this single-node MicroK8s VM.
- The monitor-only Gthulhu path reliably tracks every new worker pod after worker rollouts.
- Realistic pressure produces a pure Gthulhu HPA reason without changing the CPU trigger.
