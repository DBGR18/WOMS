# Gthulhu Phase 0 部署接續紀錄

日期：2026-05-14

## 目標

依 `/home/ubuntu/WOMS/docs/gthulhu-hpa-poc.en.md` 從 Phase 0 開始部署 Gthulhu：

- 確認 Kubernetes、Helm、Prometheus、Grafana、KEDA、WOMS 與 VM 資源現況。
- 安裝或接入 Prometheus/Grafana。
- 部署 Gthulhu monitor。
- 驗證 Prometheus 可查到 `gthulhu_pod_*` metrics，且包含 `pod_name` / `namespace` labels。
- Phase 0 通過前，不進 Phase 1 observe-only。
- 不修改 WOMS HPA，不啟用 Gthulhu trigger。

## 已完成

- `/home/ubuntu/Gthulhu` 已刷新並確認 `develop` 等於 `origin/develop`：
  - commit：`00fc41feeb2a23481b12aa27898cc8e4cd215b42`
  - subject：`feat: support per-node runtime scheduler selection (#110)`
- `/home/ubuntu/WOMS` 已刷新 tags，最新 tag 為 `v0.1.33`。
- WOMS Kubernetes workloads 已更新到最新 tag：
  - `woms-woms-api`: `docker.io/d11nn/woms-api:v0.1.33`
  - `woms-woms-web`: `docker.io/d11nn/woms-web:v0.1.33`
  - `woms-woms-worker`: `docker.io/d11nn/woms-scheduler-worker:v0.1.33`
- WOMS rollout 已完成，未修改 `ScaledObject` 或 HPA。
- 已安裝 Prometheus/Grafana：
  - Helm release：`monitoring`
  - namespace：`monitoring`
  - chart：`/home/ubuntu/Gthulhu/chart/kube-prometheus-stack`
  - Prometheus service：`monitoring-kube-prometheus-prometheus.monitoring:9090`
  - Grafana service：`monitoring-grafana.monitoring:80`
- Monitoring pods 狀態：
  - Grafana、Prometheus、Prometheus Operator、kube-state-metrics、node-exporter 皆 Running。
- VM 資源狀態：
  - 最新 audit：node `vm1`: CPU 約 13%，memory 約 33%。
  - 不需要先停 WOMS 或調整資源。
- WOMS HPA/ScaledObject audit：
  - `ScaledObject/woms-woms-worker` 仍指向 `Deployment/woms-woms-worker`。
  - triggers 仍只有 `kafka` 與 `cpu`。
  - 沒有 Prometheus/Gthulhu trigger。
  - HPA `woms-woms-worker-hpa` 仍為 `min=1`、`max=10`。

## 目前發現

- 原本 `/snap/bin/kubectl` 與 `/snap/bin/helm` 在 sandbox 內會被 `snap-confine` / AppArmor 擋住；查叢集時使用 escalated `kubectl` / `helm`。
- `Gthulhu/README.md` 的 `## Usage` / `Setting Up Dependencies` 要納入 gate：
  - 官方 build flow 需要 `make dep`、`git submodule init/sync/update`、`cd scx && cargo build --release -p scx_rustland`、`make build`。
  - 目前 VM 有 `go1.25.5`、`gcc 14.2.0`、`make 4.4.1`、`pkg-config 1.8.1`、`bpftool v7.6.0`，且 `/sys/kernel/btf/vmlinux` 可讀。
  - 目前 PATH 找不到 `clang`、`cargo`、`rustc`，若要依 README 在本機 build Gthulhu image，必須先補這些 dependencies。
  - 若採用官方 image / chart 部署，仍需確認 image 內包含 monitor 需要的 `sched_monitor.bpf.o`，以及 `/metrics` service wiring 是否正確。
- WOMS Helm release 目前仍顯示 `failed`，原因是先前 Helm upgrade 與 HPA/scale subresource 對 `Deployment.spec.replicas` 發生 field ownership conflict。
  - 這不影響目前 running workloads。
  - 為避免改 HPA 行為，本次只用 rolling image update 切到 `v0.1.33`。
- 目前尚未安裝 Gthulhu CRD：
  - `podschedulingmetrics.gthulhu.io` 尚不存在。
- 最新 audit 顯示尚無 `gthulhu-system` namespace，且沒有 Gthulhu pods/services/ServiceMonitor 殘留。
- Gthulhu 官方 K8s 文件：
  - `https://gthulhu.org/k8s/`
  - 文件流程是完整 Gthulhu scheduler/API server deployment，包含 privileged DaemonSet，不是單純 Phase 0 monitor-only。
  - 官方 prerequisites：MicroK8s、可用的 `kubectl`、MicroK8s built-in registry、`microk8s enable rbac`。
  - 官方本機 image 流程：`make image`、`cd api && make image`，再 push `127.0.0.1:32000/gthulhu-api:latest` 與 `127.0.0.1:32000/gthulhu:latest`。
  - 官方 chart 流程：`cd chart && helm install gthulhu gthulhu`；使用官方 images 時改用 `helm install gthulhu gthulhu -f ./gthulhu/values-production.yaml`。
  - 目前沒有看到 `container-registry` namespace/service；若走本機 build + local registry，需要先啟用 MicroK8s registry。
  - 2026-05-14 更新：`registry` addon 已成功啟用，最新 audit 仍 Running。
  - `registry` namespace/service 狀態：
    - namespace：`container-registry`
    - pod：`registry-6c9fcc695f-tcrd5`，`1/1 Running`
    - service：`registry`，`NodePort 5000:32000/TCP`
    - PVC：`registry-claim`，`20Gi Bound`
  - 2026-05-14 更新：使用者已在 VM shell 執行 `sudo microk8s enable rbac`，最新 `microk8s status --wait-ready` 顯示 `rbac` addon enabled。
  - Kubernetes RBAC API 可用，且目前 credentials 有建立 RBAC/DaemonSet/CRD 的權限。
  - `kubectl auth can-i` 顯示目前 credentials 可建立 `clusterrole`、`daemonset`、`customresourcedefinition`。
  - `values-production.yaml` 使用 `ghcr.io/gthulhu/gthulhu:latest` 與 `ghcr.io/gthulhu/gthulhu-api:latest`，但仍設定 `privileged: true`、`hostPID: true` 與 node-level capabilities。
  - `values-production.yaml` 會設定 `scheduler.mode: gthulhu` 與 `kernel_mode: true`，並部署 manager/MongoDB；這超出 Phase 0 monitor-only 範圍。
- Gthulhu chart 預設 config 會啟用 scheduler mode。
  - Phase 0 只需要 monitor，不能改 host scheduler 或 WOMS HPA。
  - 因此已在 `/tmp/gthulhu-phase0-chart` 準備 monitor-only chart copy。

## 已準備但尚未部署

`/tmp/gthulhu-phase0-chart` 是從 `/home/ubuntu/Gthulhu/chart/gthulhu` 複製後修改的部署用 chart：

- `templates/configmap.yaml`
  - 新增 `monitor.enabled: true`
  - `monitor.monitor_all: true`
  - `monitor.prometheus_port: 9090`
  - `monitor.enable_crd_watcher: false`
  - 設定 `scheduler.mode: none`
  - 設定 `api.enabled: false`
- `templates/deployment.yaml`
  - 在 `gthulhu-scheduler` container 補上 `monitor-metrics` port `9090`。
- `templates/service.yaml`
  - 在 scheduler sidecar service 補上 `monitor-metrics` service port。
- `templates/rbac.yaml`
  - 補上 `podschedulingmetrics` read/watch 權限。

## Blocker

目前 blocker：

部署 Gthulhu monitor-only DaemonSet 的 Helm command 被安全審核拒絕，原因是它會部署：

- `privileged: true`
- `hostPID: true`
- node-level capabilities，例如 `SYS_ADMIN`、`SYS_RESOURCE`、`SYS_PTRACE`
- host paths，例如 `/proc`、`/sys/kernel/debug`、`/var/run`

這些權限是 eBPF scheduling monitor 所需，但也有節點安全與穩定性風險。需要使用者明確批准後才能部署。

## 下一步

等待使用者明確批准是否部署 privileged Gthulhu monitor-only DaemonSet。

若批准，先選定部署來源：

1. 使用官方 image / chart：依 `https://gthulhu.org/k8s/` 部署，但要避免啟用會改 host scheduler 的設定，並驗證 monitor `/metrics`。
2. 本機 build image：先依 `README.md` 補齊 `clang`、Rust/Cargo dependencies，執行 `make dep`、submodule sync/update、`cargo build --release -p scx_rustland`、`make build` / `make image`。
3. 不建議直接使用原始 `values-production.yaml` 做 Phase 0，因為它會啟用完整 scheduler mode，不是 monitor-only。

目前準備好的 monitor-only chart command：

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

部署後必須驗證：

```bash
kubectl get pods,svc,servicemonitor -n gthulhu-system
kubectl logs -n gthulhu-system -l app.kubernetes.io/component=scheduler -c gthulhu-scheduler
```

Prometheus query：

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

只有這個 query 回傳 pod-labeled data 後，才可回報 Phase 0 成功並等待使用者確認是否進入 Phase 1。

## 2026-05-14 交接更新：Rate-limit 停止點

本段覆蓋前面「尚未部署 / 等待批准」的舊狀態。使用者已明確批准 privileged monitor-only DaemonSet，且部署已繼續進行。

### 已完成

- MicroK8s 已完成安裝/啟動驗證。
- `kubectl` 與 `microk8s kubectl` 均可正常查詢 cluster。
- MicroK8s built-in registry 已啟用：
  - namespace：`container-registry`
  - service：`registry`，`NodePort 5000:32000/TCP`
  - PVC：`registry-claim`，`20Gi Bound`
- 使用者已執行 `sudo microk8s enable rbac`，`rbac` addon 已啟用。
- WOMS 已確認使用最新 tag `v0.1.33`：
  - `woms-woms-api`: `docker.io/d11nn/woms-api:v0.1.33`
  - `woms-woms-web`: `docker.io/d11nn/woms-web:v0.1.33`
  - `woms-woms-worker`: `docker.io/d11nn/woms-scheduler-worker:v0.1.33`
- WOMS HPA / KEDA 未改動：
  - `ScaledObject/woms-woms-worker` triggers 仍只有 `kafka` 與 `cpu`
  - 沒有加入 Prometheus/Gthulhu trigger
- Monitoring stack 已部署：
  - Helm release：`monitoring`
  - namespace：`monitoring`
  - Prometheus service：`monitoring-kube-prometheus-prometheus.monitoring:9090`
  - Grafana service：`monitoring-grafana.monitoring:80`
- `/tmp/gthulhu-phase0-chart` 已可部署 monitor-only Gthulhu：
  - manager/MongoDB disabled
  - scheduler sidecar disabled
  - scheduler mode disabled
  - monitor enabled
  - ServiceMonitor labels 設為 `release=monitoring`
  - chart 另外加入 initContainer，在 pod 內用 host BTF 編出 `sched_monitor.bpf.o`
  - chart 已補 `NODE_NAME` Downward API，供 monitor 只 list 本 node pods
- 使用 stock `ghcr.io/gthulhu/gthulhu-scx:develop` 部署後，Gthulhu monitor 能啟動且 BPF attach 成功：
  - log 出現 `running in monitor-only mode`
  - log 出現 `sched_monitor BPF program loaded`
  - log 出現 `attached BPF program handle_sched_switch`
  - log 出現 `attached BPF program handle_sched_process_exit`
- Prometheus target 可 scrape Gthulhu endpoint：
  - `up{namespace="gthulhu-system"}` 回傳 `1`

### 重要診斷

目前不能直接說「Gthulhu developer demo 失敗」或「Gthulhu 有整體缺失」。README 描述的 demo / Web GUI / Manager path 會透過 Kubernetes API / informer 查 pods，這與本次 Phase 0 採用的 monitor-only DaemonSet path 不是完全同一條資料流。

本次實測的問題限定在 `develop` branch 的 monitor-only Prometheus path：

- `monitor.StartMonitor()` 建立 `collector.NewPodMapper()`，並啟動 `/proc` PID scan。
- `collector.PodMapper` 的註解明確指出 `podIndex` 要由 caller 透過 `SetPodIndex()` 填入。
- 原始 `monitor.StartMonitor()` 沒有 Kubernetes informer/list path 去呼叫 `SetPodIndex()`。
- `PodMapper.SetPodIndex()` 目前只在 tests 中被呼叫。
- 因此即使 BPF map 有 scheduling event，`resolvePIDtoPod()` 也只能從 `/proc/<pid>/cgroup` 解析 pod UID，卻找不到 UID 對應的 `pod_name` / `namespace`。
- 結果是 Prometheus target up，但查不到帶 pod label 的 `gthulhu_pod_*` series。

已驗證失敗的 Phase 0 query：

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

結果為 empty vector，Phase 0 尚未通過。

### `/tmp` 最小修補狀態

已在 `/tmp/gthulhu-build` 建立 Gthulhu build copy，沒有改 `/home/ubuntu/Gthulhu` 原始 worktree。

已修改 `/tmp/gthulhu-build/monitor/monitor.go`：

- 新增 in-cluster Kubernetes client。
- 啟動 monitor 時呼叫 `startPodIndexRefresher()`。
- 每 30 秒 list all namespaces pods，field selector 使用 `spec.nodeName=<NODE_NAME>`。
- 建立 `pod.UID -> collector.PodRef{PodName, PodUID, Namespace, NodeName}` map。
- 呼叫 `podMapper.SetPodIndex(index)`。
- 成功時 log：`pod index refreshed`。

已修改 `/tmp/gthulhu-build/Dockerfile`：

- runtime image 額外 copy `/build/sched_monitor.bpf.o` 到 `/gthulhu/sched_monitor.bpf.o`。

已修改 `/tmp/gthulhu-phase0-chart/templates/deployment.yaml`：

```yaml
env:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
```

### 停止點 / Blocker

Docker build 已跑到最後 Go build 階段，但失敗：

```text
go: updates to go.mod needed; to update it:
        go mod tidy
make: *** [Makefile:95: build] Error 1
```

原因：最小修補新增 `k8s.io/client-go/kubernetes` / `k8s.io/apimachinery/pkg/apis/meta/v1` import 後，`/tmp/gthulhu-build/go.mod` / `go.sum` 需要 tidy。

嘗試執行 `go mod tidy` 時，sandbox 內 Go cache 指向 `/home/ubuntu/.cache/go-build`，因唯讀限制失敗。依規則改用 escalated `go mod tidy`，但工具回覆已達 usage limit：

```text
You've hit your usage limit ... try again at 8:14 PM.
```

依使用者要求，已在 rate-limit 接近/達到時停止，不再嘗試繞路執行同等操作。

### 下一輪建議步驟

1. 在 `/tmp/gthulhu-build` 執行 `go mod tidy`，完成 `go.mod` / `go.sum` 更新。
2. 重新 build image：

```bash
docker build -t localhost:32000/gthulhu-monitor:phase0 /tmp/gthulhu-build
```

3. push 到 MicroK8s registry：

```bash
docker push localhost:32000/gthulhu-monitor:phase0
```

4. 用 patched chart 升級 release：

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

若 immutable configmap 擋住 upgrade，只刪除 `gthulhu-scheduler-config` 後重試：

```bash
kubectl delete configmap gthulhu-scheduler-config -n gthulhu-system
```

5. 驗證 logs：

```bash
kubectl logs -n gthulhu-system -l app.kubernetes.io/component=scheduler -c gthulhu-scheduler --tail=300
```

必須看到：

- `pod index refreshed`
- `sched_monitor BPF program loaded`
- `attached BPF program handle_sched_switch`
- `attached BPF program handle_sched_process_exit`

6. 驗證 Prometheus target 與 Phase 0 query：

```bash
kubectl exec -n monitoring prometheus-monitoring-kube-prometheus-prometheus-0 -c prometheus -- \
  sh -c "wget -qO- 'http://127.0.0.1:9090/api/v1/query?query=up%7Bnamespace%3D%22gthulhu-system%22%7D'"

kubectl exec -n monitoring prometheus-monitoring-kube-prometheus-prometheus-0 -c prometheus -- \
  sh -c "wget -qO- 'http://127.0.0.1:9090/api/v1/query?query=count(gthulhu_pod_process_count%7Bpod_name!%3D%22%22%2Cnamespace!%3D%22%22%7D)'"
```

只有第二個 query 回傳非 0 scalar，Phase 0 才算通過。

### 下一個 Goal Prompt

```text
接續 /home/ubuntu/WOMS/docs/gthulhu-phase0-deploy-log.zh-TW.md 的 2026-05-14 交接更新。

目標：完成 Gthulhu Phase 0 monitor-only 驗證，不進入 Phase 1，不改 WOMS HPA/ScaledObject trigger。

已知狀態：
- MicroK8s、kubectl、registry、rbac 都已完成。
- WOMS 已更新到 latest tag v0.1.33。
- Prometheus/Grafana monitoring stack 已部署。
- stock Gthulhu develop image 可啟動 monitor 且 BPF attach 成功，但 Prometheus 查不到 pod-labeled gthulhu_pod_*。
- 診斷：monitor-only path 沒有填 PodMapper.SetPodIndex；SetPodIndex 只在 tests 使用，Manager/Web GUI demo path 可能不同，不能把 demo 成功直接等同於 monitor-only Prometheus path 已完整。
- /tmp/gthulhu-build 已有最小修補：monitor 啟動時用 in-cluster client 每 30 秒 list 本 node pods，呼叫 podMapper.SetPodIndex。
- /tmp/gthulhu-phase0-chart 已補 NODE_NAME env。
- 上次停止點：docker build 失敗於 go.mod/go.sum 需要 go mod tidy；因 usage limit 停止。

請從 /tmp/gthulhu-build 執行 go mod tidy，重新 docker build/push localhost:32000/gthulhu-monitor:phase0，helm upgrade /tmp/gthulhu-phase0-chart，然後驗證 Prometheus query：
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})

若 query 非 0，回報 Phase 0 成功並等待我確認是否進入 Phase 1。若仍為 empty/0，停止並更新 docs/gthulhu-phase0-deploy-log.zh-TW.md 與 .en.md。
```

## 2026-05-14 Phase 0 完成驗證

### 執行結果

- 已在 `/tmp/gthulhu-build` 完成 `go mod tidy`。
  - 使用 `/tmp/gthulhu-go-build-cache` 與 `/tmp/gthulhu-go-mod-cache` 避免寫入 sandbox 內唯讀的 home cache。
  - 因 Go module download 需要 DNS/network，使用 escalated command 完成。
- 已成功 build image：

```bash
docker build -t localhost:32000/gthulhu-monitor:phase0 /tmp/gthulhu-build
```

- 已成功 push 到 MicroK8s registry：

```text
localhost:32000/gthulhu-monitor:phase0
digest: sha256:a8bbc6578ed79dc5947fd5b02aaa30fb76b500139eabb01b89b1e28260518c10
```

- 已使用 `/tmp/gthulhu-phase0-chart` 升級 Helm release：
  - release：`gthulhu`
  - namespace：`gthulhu-system`
  - revision：`6`
  - status：`deployed`
  - image：`localhost:32000/gthulhu-monitor:phase0`

### Gthulhu monitor-only 驗證

Kubernetes 狀態：

```text
pod/gthulhu-scheduler-x58bt  1/1 Running
daemonset.apps/gthulhu-scheduler  DESIRED=1 CURRENT=1 READY=1 AVAILABLE=1
service/gthulhu-scheduler-sidecar  ClusterIP None  9090/TCP
endpoints/gthulhu-scheduler-sidecar  10.1.225.25:9090
servicemonitor.monitoring.coreos.com/gthulhu-monitor
```

DaemonSet template 驗證：

```text
image: localhost:32000/gthulhu-monitor:phase0
env: NODE_NAME from fieldRef spec.nodeName
```

Gthulhu logs 已確認：

- `running in monitor-only mode (no scheduler mode configured)`
- `pod index refreshed`，node=`vm1`，pods=`23`
- `sched_monitor BPF program loaded`
- `attached BPF program handle_sched_switch`
- `attached BPF program handle_sched_process_exit`

Prometheus direct `/metrics` 已確認有 pod-labeled metrics，例如：

```text
gthulhu_pod_cpu_migrations_total{namespace="woms",node_name="vm1",pod_name="woms-woms-worker-7c6dc8ccc8-2l9q5",pod_uid="f5532df2-d27e-4494-8858-ad28a8e1c1fe"} 22
```

Prometheus target query：

```promql
up{namespace="gthulhu-system"}
```

結果：

```text
value: 1
```

Phase 0 gate query：

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

結果：

```text
value: 21
```

Phase 0 monitor-only 驗證通過。

### No Phase 1 / WOMS HPA 稽核

已確認沒有進入 Phase 1，且沒有修改 WOMS HPA / ScaledObject trigger。

WOMS workloads 仍使用 latest tag：

```text
woms-woms-api     docker.io/d11nn/woms-api:v0.1.33
woms-woms-web     docker.io/d11nn/woms-web:v0.1.33
woms-woms-worker  docker.io/d11nn/woms-scheduler-worker:v0.1.33
```

`ScaledObject/woms-woms-worker` triggers 仍只有：

- `kafka`
- `cpu`

`HPA/woms-woms-worker-hpa` metrics 仍只有：

- external metric `s0-kafka-woms-schedule-jobs`
- resource metric `cpu`

Gthulhu release values：

- `manager.enabled=false`
- `mongodb.enabled=false`
- `scheduler.sidecar.enabled=false`
- `keda.enabled=false`
- `prometheusAdapter.enabled=false`
- `monitoring.enabled=true`
- `monitoring.serviceMonitor.enabled=true`

### 後續

Phase 0 已完成。下一步必須等待使用者確認，才能討論或執行 Phase 1 observe-only；目前不應新增 Gthulhu Prometheus trigger 到 WOMS ScaledObject。
