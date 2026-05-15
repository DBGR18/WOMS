# Gthulhu Phase 0 Deployment Resume Log

Date: 2026-05-14

## Goal

Start from Phase 0 in `/home/ubuntu/WOMS/docs/gthulhu-hpa-poc.en.md`:

- Check Kubernetes, Helm, Prometheus, Grafana, KEDA, WOMS, and VM resources.
- Install or connect Prometheus/Grafana.
- Deploy the Gthulhu monitor.
- Verify Prometheus can query `gthulhu_pod_*` metrics with `pod_name` / `namespace` labels.
- Do not continue to Phase 1 observe-only until Phase 0 passes.
- Do not modify WOMS HPA or enable a Gthulhu trigger.

## Completed

- Refreshed `/home/ubuntu/Gthulhu` and confirmed `develop` equals `origin/develop`:
  - commit: `00fc41feeb2a23481b12aa27898cc8e4cd215b42`
  - subject: `feat: support per-node runtime scheduler selection (#110)`
- Refreshed `/home/ubuntu/WOMS` tags. Latest tag is `v0.1.33`.
- Updated WOMS Kubernetes workloads to the latest tag:
  - `woms-woms-api`: `docker.io/d11nn/woms-api:v0.1.33`
  - `woms-woms-web`: `docker.io/d11nn/woms-web:v0.1.33`
  - `woms-woms-worker`: `docker.io/d11nn/woms-scheduler-worker:v0.1.33`
- WOMS rollouts completed. No `ScaledObject` or HPA changes were made.
- Installed Prometheus/Grafana:
  - Helm release: `monitoring`
  - namespace: `monitoring`
  - chart: `/home/ubuntu/Gthulhu/chart/kube-prometheus-stack`
  - Prometheus service: `monitoring-kube-prometheus-prometheus.monitoring:9090`
  - Grafana service: `monitoring-grafana.monitoring:80`
- Monitoring pods are Running:
  - Grafana, Prometheus, Prometheus Operator, kube-state-metrics, and node-exporter.
- VM resource state:
  - Latest audit: node `vm1`: about 13% CPU and 33% memory.
  - No need to stop WOMS or resize resources at this point.
- WOMS HPA/ScaledObject audit:
  - `ScaledObject/woms-woms-worker` still targets `Deployment/woms-woms-worker`.
  - triggers are still only `kafka` and `cpu`.
  - no Prometheus/Gthulhu trigger is present.
  - HPA `woms-woms-worker-hpa` remains `min=1`, `max=10`.

## Findings

- `/snap/bin/kubectl` and `/snap/bin/helm` fail inside the sandbox because of `snap-confine` / AppArmor. Cluster checks used escalated `kubectl` / `helm`.
- `Gthulhu/README.md` `## Usage` / `Setting Up Dependencies` must be part of the gate:
  - The documented build flow needs `make dep`, `git submodule init/sync/update`, `cd scx && cargo build --release -p scx_rustland`, and `make build`.
  - The VM currently has `go1.25.5`, `gcc 14.2.0`, `make 4.4.1`, `pkg-config 1.8.1`, `bpftool v7.6.0`, and readable `/sys/kernel/btf/vmlinux`.
  - `clang`, `cargo`, and `rustc` are not currently found in PATH. If building the Gthulhu image locally from the README flow, these dependencies must be installed first.
  - If using official images / chart deployment instead, still verify that the image contains the monitor-required `sched_monitor.bpf.o` and that `/metrics` service wiring is correct.
- The WOMS Helm release still shows `failed` because a previous Helm upgrade conflicted with HPA/scale subresource ownership of `Deployment.spec.replicas`.
  - Running workloads are healthy.
  - To avoid changing HPA behavior, this session only used rolling image updates to switch to `v0.1.33`.
- Gthulhu CRDs are not installed yet:
  - `podschedulingmetrics.gthulhu.io` does not exist yet.
- Latest audit shows there is still no `gthulhu-system` namespace and no leftover Gthulhu pods/services/ServiceMonitor.
- Official Gthulhu K8s docs:
  - `https://gthulhu.org/k8s/`
  - The documented flow deploys the full Gthulhu scheduler/API server, including a privileged DaemonSet, not only a Phase 0 monitor.
  - Official prerequisites: MicroK8s, working `kubectl`, MicroK8s built-in registry, and `microk8s enable rbac`.
  - Official local image flow: `make image`, `cd api && make image`, then push `127.0.0.1:32000/gthulhu-api:latest` and `127.0.0.1:32000/gthulhu:latest`.
  - Official chart flow: `cd chart && helm install gthulhu gthulhu`; when using official images, use `helm install gthulhu gthulhu -f ./gthulhu/values-production.yaml`.
  - No `container-registry` namespace/service is currently present; if using the local build + local registry route, MicroK8s registry must be enabled first.
  - 2026-05-14 update: the `registry` addon has been enabled successfully and is still Running in the latest audit.
  - Registry namespace/service state:
    - namespace: `container-registry`
    - pod: `registry-6c9fcc695f-tcrd5`, `1/1 Running`
    - service: `registry`, `NodePort 5000:32000/TCP`
    - PVC: `registry-claim`, `20Gi Bound`
  - 2026-05-14 update: the user ran `sudo microk8s enable rbac` in the VM shell, and the latest `microk8s status --wait-ready` shows the `rbac` addon enabled.
  - The Kubernetes RBAC API is usable, and the current credentials can create RBAC/DaemonSet/CRD resources.
  - `kubectl auth can-i` shows the current credentials can create `clusterrole`, `daemonset`, and `customresourcedefinition`.
  - `values-production.yaml` uses `ghcr.io/gthulhu/gthulhu:latest` and `ghcr.io/gthulhu/gthulhu-api:latest`, but still sets `privileged: true`, `hostPID: true`, and node-level capabilities.
  - `values-production.yaml` sets `scheduler.mode: gthulhu` and `kernel_mode: true`, and deploys manager/MongoDB; this is beyond Phase 0 monitor-only scope.
- The default Gthulhu chart config enables scheduler mode.
  - Phase 0 only needs the monitor and must not change the host scheduler or WOMS HPA.
  - A monitor-only chart copy has been prepared at `/tmp/gthulhu-phase0-chart`.

## Prepared But Not Deployed

`/tmp/gthulhu-phase0-chart` was copied from `/home/ubuntu/Gthulhu/chart/gthulhu` and patched for deployment:

- `templates/configmap.yaml`
  - Added `monitor.enabled: true`
  - Set `monitor.monitor_all: true`
  - Set `monitor.prometheus_port: 9090`
  - Set `monitor.enable_crd_watcher: false`
  - Set `scheduler.mode: none`
  - Set `api.enabled: false`
- `templates/deployment.yaml`
  - Added `monitor-metrics` container port `9090` to the `gthulhu-scheduler` container.
- `templates/service.yaml`
  - Added a `monitor-metrics` service port to the scheduler sidecar service.
- `templates/rbac.yaml`
  - Added read/watch permissions for `podschedulingmetrics`.

## Blocker

Current blocker:

The Helm command to deploy the Gthulhu monitor-only DaemonSet was rejected by safety review because it would deploy:

- `privileged: true`
- `hostPID: true`
- node-level capabilities such as `SYS_ADMIN`, `SYS_RESOURCE`, and `SYS_PTRACE`
- host paths such as `/proc`, `/sys/kernel/debug`, and `/var/run`

These permissions are required for the eBPF scheduling monitor, but they also carry node security and stability risk. Explicit user approval is required before deployment.

## Next Step

Wait for explicit user approval to deploy the privileged Gthulhu monitor-only DaemonSet.

If approved, first choose the deployment source:

1. Use official image / chart: follow `https://gthulhu.org/k8s/`, but avoid settings that change the host scheduler and verify monitor `/metrics`.
2. Build locally: install missing `clang` and Rust/Cargo dependencies, then follow `README.md`: `make dep`, submodule sync/update, `cargo build --release -p scx_rustland`, and `make build` / `make image`.
3. Do not use the unmodified `values-production.yaml` directly for Phase 0 because it enables full scheduler mode rather than monitor-only behavior.

Prepared monitor-only chart command:

```bash
helm upgrade --install gthulhu /tmp/gthulhu-phase0-chart \
  --namespace gthulhu-system \
  --create-namespace \
  --wait \
  --timeout 5m \
  --set manager.enabled=false \
  --set mongodb.enabled=false \
  --set scheduler.enabled=true \
  --set scheduler.sidecar.enabled=true \
  --set monitoring.enabled=true \
  --set monitoring.serviceMonitor.enabled=true \
  --set monitoring.serviceMonitor.labels.release=monitoring \
  --set keda.enabled=false \
  --set prometheusAdapter.enabled=false
```

After deployment, verify:

```bash
kubectl get pods,svc,servicemonitor -n gthulhu-system
kubectl logs -n gthulhu-system -l app.kubernetes.io/component=scheduler -c gthulhu-scheduler
```

Prometheus query:

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

Only if this query returns pod-labeled data should Phase 0 be reported as successful and the user asked whether to continue to Phase 1.

## 2026-05-14 Handoff Update: Rate-limit Stop Point

This section supersedes the older "not deployed / waiting for approval" state above. The user explicitly approved deploying the privileged monitor-only DaemonSet, and deployment work continued.

### Completed

- MicroK8s installation/startup has been verified.
- `kubectl` and `microk8s kubectl` can both query the cluster.
- The MicroK8s built-in registry is enabled:
  - namespace: `container-registry`
  - service: `registry`, `NodePort 5000:32000/TCP`
  - PVC: `registry-claim`, `20Gi Bound`
- The user ran `sudo microk8s enable rbac`, and the `rbac` addon is enabled.
- WOMS is on the latest tag `v0.1.33`:
  - `woms-woms-api`: `docker.io/d11nn/woms-api:v0.1.33`
  - `woms-woms-web`: `docker.io/d11nn/woms-web:v0.1.33`
  - `woms-woms-worker`: `docker.io/d11nn/woms-scheduler-worker:v0.1.33`
- WOMS HPA / KEDA was not changed:
  - `ScaledObject/woms-woms-worker` triggers are still only `kafka` and `cpu`
  - no Prometheus/Gthulhu trigger was added
- The monitoring stack is deployed:
  - Helm release: `monitoring`
  - namespace: `monitoring`
  - Prometheus service: `monitoring-kube-prometheus-prometheus.monitoring:9090`
  - Grafana service: `monitoring-grafana.monitoring:80`
- `/tmp/gthulhu-phase0-chart` is patched for monitor-only Gthulhu:
  - manager/MongoDB disabled
  - scheduler sidecar disabled
  - scheduler mode disabled
  - monitor enabled
  - ServiceMonitor labels set to `release=monitoring`
  - chart includes an initContainer that builds `sched_monitor.bpf.o` inside the pod using host BTF
  - chart now injects `NODE_NAME` through the Downward API so the monitor lists only pods on the local node
- With the stock `ghcr.io/gthulhu/gthulhu-scx:develop` image, the Gthulhu monitor starts and attaches BPF successfully:
  - logs show `running in monitor-only mode`
  - logs show `sched_monitor BPF program loaded`
  - logs show `attached BPF program handle_sched_switch`
  - logs show `attached BPF program handle_sched_process_exit`
- Prometheus can scrape the Gthulhu endpoint:
  - `up{namespace="gthulhu-system"}` returns `1`

### Key Diagnosis

This should not be described as "the Gthulhu developer demo failed" or as a general Gthulhu defect. The README demo / Web GUI / Manager path queries pods through the Kubernetes API / informer, while this Phase 0 monitor-only DaemonSet path is a different data flow.

The observed issue is limited to the `develop` branch monitor-only Prometheus path:

- `monitor.StartMonitor()` creates `collector.NewPodMapper()` and starts `/proc` PID scanning.
- `collector.PodMapper` documents that `podIndex` must be populated by the caller via `SetPodIndex()`.
- The original `monitor.StartMonitor()` has no Kubernetes informer/list path that calls `SetPodIndex()`.
- `PodMapper.SetPodIndex()` is currently only called from tests.
- Therefore, even when BPF maps contain scheduling events, `resolvePIDtoPod()` can extract a pod UID from `/proc/<pid>/cgroup` but cannot map that UID to `pod_name` / `namespace`.
- Result: the Prometheus target is up, but pod-labeled `gthulhu_pod_*` series are absent.

Failed Phase 0 query:

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

The result was an empty vector, so Phase 0 has not passed.

### `/tmp` Minimal Patch State

A Gthulhu build copy was created at `/tmp/gthulhu-build`. The original `/home/ubuntu/Gthulhu` worktree was not modified.

Patched `/tmp/gthulhu-build/monitor/monitor.go`:

- Adds an in-cluster Kubernetes client.
- Calls `startPodIndexRefresher()` when the monitor starts.
- Every 30 seconds, lists pods across all namespaces with field selector `spec.nodeName=<NODE_NAME>`.
- Builds a `pod.UID -> collector.PodRef{PodName, PodUID, Namespace, NodeName}` map.
- Calls `podMapper.SetPodIndex(index)`.
- Logs `pod index refreshed` on success.

Patched `/tmp/gthulhu-build/Dockerfile`:

- Copies `/build/sched_monitor.bpf.o` into `/gthulhu/sched_monitor.bpf.o` in the runtime image.

Patched `/tmp/gthulhu-phase0-chart/templates/deployment.yaml`:

```yaml
env:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
```

### Stop Point / Blocker

The Docker build reached the final Go build stage and failed:

```text
go: updates to go.mod needed; to update it:
        go mod tidy
make: *** [Makefile:95: build] Error 1
```

Reason: the minimal patch added `k8s.io/client-go/kubernetes` / `k8s.io/apimachinery/pkg/apis/meta/v1` imports, so `/tmp/gthulhu-build/go.mod` / `go.sum` need to be tidied.

Running `go mod tidy` inside the sandbox failed because the Go cache points to `/home/ubuntu/.cache/go-build`, which is read-only in the sandbox. The next compliant step was escalated `go mod tidy`, but the tool reported the usage limit:

```text
You've hit your usage limit ... try again at 8:14 PM.
```

Per the user's instruction, work stopped at the rate-limit point and no workaround was attempted.

### Recommended Next Steps

1. Run `go mod tidy` in `/tmp/gthulhu-build` to update `go.mod` / `go.sum`.
2. Rebuild the image:

```bash
docker build -t localhost:32000/gthulhu-monitor:phase0 /tmp/gthulhu-build
```

3. Push to the MicroK8s registry:

```bash
docker push localhost:32000/gthulhu-monitor:phase0
```

4. Upgrade the release with the patched chart:

```bash
helm upgrade gthulhu /tmp/gthulhu-phase0-chart \
  --namespace gthulhu-system \
  --wait \
  --timeout 5m \
  --set manager.enabled=false \
  --set mongodb.enabled=false \
  --set scheduler.enabled=true \
  --set scheduler.sidecar.enabled=false \
  --set scheduler.image.repository=localhost:32000/gthulhu-monitor \
  --set scheduler.image.tag=phase0 \
  --set scheduler.image.pullPolicy=Always \
  --set monitoring.enabled=true \
  --set monitoring.serviceMonitor.enabled=true \
  --set monitoring.serviceMonitor.labels.release=monitoring \
  --set keda.enabled=false \
  --set prometheusAdapter.enabled=false
```

If an immutable configmap blocks the upgrade, delete only `gthulhu-scheduler-config` and retry:

```bash
kubectl delete configmap gthulhu-scheduler-config -n gthulhu-system
```

5. Verify logs:

```bash
kubectl logs -n gthulhu-system -l app.kubernetes.io/component=scheduler -c gthulhu-scheduler --tail=300
```

Required log signals:

- `pod index refreshed`
- `sched_monitor BPF program loaded`
- `attached BPF program handle_sched_switch`
- `attached BPF program handle_sched_process_exit`

6. Verify the Prometheus target and Phase 0 query:

```bash
kubectl exec -n monitoring prometheus-monitoring-kube-prometheus-prometheus-0 -c prometheus -- \
  sh -c "wget -qO- 'http://127.0.0.1:9090/api/v1/query?query=up%7Bnamespace%3D%22gthulhu-system%22%7D'"

kubectl exec -n monitoring prometheus-monitoring-kube-prometheus-prometheus-0 -c prometheus -- \
  sh -c "wget -qO- 'http://127.0.0.1:9090/api/v1/query?query=count(gthulhu_pod_process_count%7Bpod_name!%3D%22%22%2Cnamespace!%3D%22%22%7D)'"
```

Phase 0 only passes if the second query returns a non-zero scalar.

### Next Goal Prompt

```text
Continue from /home/ubuntu/WOMS/docs/gthulhu-phase0-deploy-log.en.md, section "2026-05-14 Handoff Update: Rate-limit Stop Point".

Goal: complete Gthulhu Phase 0 monitor-only verification. Do not enter Phase 1. Do not change WOMS HPA/ScaledObject triggers.

Known state:
- MicroK8s, kubectl, registry, and RBAC are ready.
- WOMS is updated to latest tag v0.1.33.
- Prometheus/Grafana monitoring stack is deployed.
- The stock Gthulhu develop image starts the monitor and attaches BPF successfully, but Prometheus has no pod-labeled gthulhu_pod_* series.
- Diagnosis: the monitor-only path does not populate PodMapper.SetPodIndex; SetPodIndex is only used in tests. The Manager/Web GUI demo path may be different, so demo success should not be treated as proof that the monitor-only Prometheus path is complete.
- /tmp/gthulhu-build contains a minimal patch: monitor startup uses an in-cluster client every 30 seconds to list local-node pods and call podMapper.SetPodIndex.
- /tmp/gthulhu-phase0-chart injects NODE_NAME.
- Previous stop point: docker build failed because go.mod/go.sum need go mod tidy; work stopped due to usage limit.

From /tmp/gthulhu-build, run go mod tidy, rebuild/push localhost:32000/gthulhu-monitor:phase0, helm upgrade /tmp/gthulhu-phase0-chart, then verify:
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})

If the query is non-zero, report Phase 0 success and wait for confirmation before Phase 1. If it is still empty/0, stop and update docs/gthulhu-phase0-deploy-log.zh-TW.md and .en.md.
```

## 2026-05-14 Phase 0 Completion Verification

### Execution Result

- `go mod tidy` completed in `/tmp/gthulhu-build`.
  - Used `/tmp/gthulhu-go-build-cache` and `/tmp/gthulhu-go-mod-cache` to avoid writing to the sandbox read-only home cache.
  - Used an escalated command because Go module downloads require DNS/network access.
- Image build succeeded:

```bash
docker build -t localhost:32000/gthulhu-monitor:phase0 /tmp/gthulhu-build
```

- Image push to the MicroK8s registry succeeded:

```text
localhost:32000/gthulhu-monitor:phase0
digest: sha256:a8bbc6578ed79dc5947fd5b02aaa30fb76b500139eabb01b89b1e28260518c10
```

- Helm release was upgraded from `/tmp/gthulhu-phase0-chart`:
  - release: `gthulhu`
  - namespace: `gthulhu-system`
  - revision: `6`
  - status: `deployed`
  - image: `localhost:32000/gthulhu-monitor:phase0`

### Gthulhu Monitor-only Verification

Kubernetes state:

```text
pod/gthulhu-scheduler-x58bt  1/1 Running
daemonset.apps/gthulhu-scheduler  DESIRED=1 CURRENT=1 READY=1 AVAILABLE=1
service/gthulhu-scheduler-sidecar  ClusterIP None  9090/TCP
endpoints/gthulhu-scheduler-sidecar  10.1.225.25:9090
servicemonitor.monitoring.coreos.com/gthulhu-monitor
```

DaemonSet template verification:

```text
image: localhost:32000/gthulhu-monitor:phase0
env: NODE_NAME from fieldRef spec.nodeName
```

Gthulhu logs confirmed:

- `running in monitor-only mode (no scheduler mode configured)`
- `pod index refreshed`, node=`vm1`, pods=`23`
- `sched_monitor BPF program loaded`
- `attached BPF program handle_sched_switch`
- `attached BPF program handle_sched_process_exit`

Prometheus direct `/metrics` now exposes pod-labeled metrics, for example:

```text
gthulhu_pod_cpu_migrations_total{namespace="woms",node_name="vm1",pod_name="woms-woms-worker-7c6dc8ccc8-2l9q5",pod_uid="f5532df2-d27e-4494-8858-ad28a8e1c1fe"} 22
```

Prometheus target query:

```promql
up{namespace="gthulhu-system"}
```

Result:

```text
value: 1
```

Phase 0 gate query:

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

Result:

```text
value: 21
```

Phase 0 monitor-only verification passed.

### No Phase 1 / WOMS HPA Audit

Confirmed that Phase 1 was not entered and WOMS HPA / ScaledObject triggers were not changed.

WOMS workloads are still on the latest tag:

```text
woms-woms-api     docker.io/d11nn/woms-api:v0.1.33
woms-woms-web     docker.io/d11nn/woms-web:v0.1.33
woms-woms-worker  docker.io/d11nn/woms-scheduler-worker:v0.1.33
```

`ScaledObject/woms-woms-worker` triggers are still only:

- `kafka`
- `cpu`

`HPA/woms-woms-worker-hpa` metrics are still only:

- external metric `s0-kafka-woms-schedule-jobs`
- resource metric `cpu`

Gthulhu release values:

- `manager.enabled=false`
- `mongodb.enabled=false`
- `scheduler.sidecar.enabled=false`
- `keda.enabled=false`
- `prometheusAdapter.enabled=false`
- `monitoring.enabled=true`
- `monitoring.serviceMonitor.enabled=true`

### Next

Phase 0 is complete. The next step must wait for user confirmation before discussing or executing Phase 1 observe-only; do not add a Gthulhu Prometheus trigger to the WOMS ScaledObject yet.
