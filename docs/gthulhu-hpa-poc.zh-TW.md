# WOMS Gthulhu HPA PoC 設計

## 來源基準

本文件依據目前 WOMS repository 狀態，以及本機 Gthulhu repository `/home/ubuntu/Gthulhu` 的 `develop` 分支、commit `00fc41f` 更新。本次更新前，Gthulhu source 已用下列指令刷新：

```bash
cd /home/ubuntu/Gthulhu
git pull origin develop
```

Git 回覆 `Already up to date.`

2026-05-15 更新：當初 WOMS Phase 0-3 實測基準是 `d11nn/feat/woms-poc` 的 `f71f78a feat: add monitor pod index refresh`。此 branch 相對 upstream `main` 是 `0 behind / 2 ahead`；若改和最新 upstream `develop` 比，則是 `3 behind / 1 ahead`，因為 upstream `develop` 已前進到 `6984d82`。若要改用最新 upstream `develop`，請先依照 `docs/gthulhu-woms-deployment.zh-TW.md` 重新對齊 branch/image 並重跑驗證；不要把 floating `develop` tag 直接視為等同於已驗證環境。

目前 WOMS 架構已經不是單純 deployment skeleton。Helm chart 會部署：

- Go API deployment `woms-woms-api`，預設 `2` replicas，包含 JWT/RBAC、PostgreSQL store、Redis address、Kafka publisher、readiness/liveness probes，以及 `minAvailable: 1` 的 PDB `woms-woms-api`。
- Static web deployment `woms-woms-web`，預設 `2` replicas，由 NGINX proxy API，使用 non-root 與 read-only filesystem 設定，並有 `minAvailable: 1` 的 PDB `woms-woms-web`。
- Go scheduler worker deployment `woms-woms-worker`，預設 `1` replica，使用 Kafka consumer group `woms-scheduler-workers`，連接 PostgreSQL，並有 retry 與 worker resources 設定。
- PostgreSQL、Redis、Kafka Helm dependencies，預設供本機或 clean-VM demo 使用。
- Kafka topic hook job 會建立 `woms.schedule.jobs`；當 `kafkaTopic.partitions` 為 `0` 時會使用 `keda.maxReplicaCount`，確保 topic partitions 足以支援擴出的 workers。
- KEDA `ScaledObject` `woms-woms-worker`，會建立指向 `Deployment/woms-woms-worker` 的 HPA `woms-woms-worker-hpa`。

目前 KEDA/HPA baseline：

- Kafka trigger 預設啟用。
- CPU trigger 預設啟用。
- `minReplicaCount: 1`。
- `maxReplicaCount: 10`。
- `pollingInterval: 30`。
- `cooldownPeriod: 120`。
- scale-up：每 30 秒可增加 100 percent，沒有 stabilization window。
- scale-down：每 60 秒可減少 50 percent，stabilization window 為 120 秒。

## 信心狀態

這份 plan 目前不是 100 percent deployment guarantee。它是重新檢查目前 WOMS chart 與 Gthulhu `develop` source 後，最安全的 code-based 策略。真正的 100 percent 需要在 live cluster 證明 Gthulhu 會輸出 pod-labeled Prometheus metrics、Prometheus 能 scrape、KEDA 能讀 query，而且 WOMS 既有 worker HPA 行為沒有被破壞。

相較前一版 proposal，這裡最重要的修正是：

- Gthulhu manager/API code 確實可以列出 pod labels，也可以在 manager-side pod inventory path 套用 Kubernetes label selectors。
- 但 `PodSchedulingMetrics` 使用的 Gthulhu monitor/eBPF path 是另一條路徑。在 commit `00fc41f` 上，WOMS 依賴它進 HPA 前，仍需要 live verification。
- 如果 Gthulhu 已經提供完整且部署驗證過、符合這個 use case 的 scaler path，WOMS 不應在 application code 重複實作。WOMS 仍可能需要一個很小的 Helm 修改，讓 Kafka、CPU、Gthulhu metrics 保持在同一個既有 `ScaledObject` 內。

## 目標

在不取代既有 Kafka lag 與 CPU triggers 的前提下，把 Gthulhu-backed scheduling-pressure signal 加到 WOMS scheduler-worker autoscaling path。

WOMS 中類似 NF workload 的目標元件是 `scheduler-worker`：它是非同步排程執行者。API 接收排程請求並 publish job 到 Kafka；worker 消費 `woms.schedule.jobs`，針對每條 production line 執行 scheduling lock，將 allocation 寫入 PostgreSQL，並記錄 audit 結果。HPA demo 期間，API 會建立 200 條產線、1,000 張待排程訂單與 200 個 queued jobs，再由 workers 透過 consumer group `woms-scheduler-workers` 消化 backlog。

PoC 要驗證這條 loop：

```text
WOMS scheduling workload
  -> Kafka backlog on woms.schedule.jobs
  -> scheduler-worker pods consume jobs
  -> Gthulhu eBPF monitor observes pod scheduling events
  -> Prometheus stores Gthulhu pod metrics
  -> WOMS KEDA ScaledObject adds one prometheus trigger
  -> KEDA-created HPA adjusts Deployment/woms-woms-worker replicas
```

## 為什麼這個 PoC 需要 Gthulhu

Kafka lag 能回答「排程任務是否等待消化」。CPU utilization 能回答「worker containers 是否忙碌」。但這兩個訊號都不能證明 worker pod 是否因 kernel scheduling pressure、preemption、CPU migration、NUMA migration 或 node 上的 noisy neighbors 而被延遲。

Gthulhu 補的是這個缺口：它從 node/kernel 層收集 pod-level scheduling metrics。目前 `develop` 分支的 Prometheus collector 會用 `pod_name`、`pod_uid`、`namespace`、`node_name` labels 暴露下列 metrics：

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

對 WOMS 來說，第一階段最有價值的是 `involuntary_ctx_switches`、`wait_time`、`cpu_migrations` 與 `numa_migrations`。這些比平均 CPU utilization 更接近 runtime scheduling pressure。

## 目前整合邊界

WOMS 目前沒有把 Gthulhu、Prometheus 或 Grafana vendor 到 `deploy/helm/woms`。第一階段 PoC 建議保留這個邊界：

- WOMS Helm 負責 WOMS workloads、PostgreSQL、Redis、Kafka、worker `ScaledObject`、PDBs 與 optional Ingress。
- Gthulhu Helm 負責 Gthulhu CRDs、monitor、eBPF collector、ServiceMonitor 與 Gthulhu runtime components。
- Platform 負責 Prometheus/Grafana，通常使用 `kube-prometheus-stack` 或既有 monitoring stack。

KEDA 不應該同時收到兩個獨立 scaling controllers 去控制同一個 WOMS worker deployment。KEDA 官方 `ScaledObject` model 是把同一個 target workload 的 triggers 放在同一個 `ScaledObject` 內；該 `ScaledObject` 會擁有 target workload 的 generated HPA。WOMS 目前已經有 worker `ScaledObject`，其中結合 Kafka 與 CPU triggers。

Gthulhu 有 chart-level example `ScaledObject`，`PodSchedulingMetrics.spec.scaling` 也存在於 CRD/API model。但目前 code review 沒有找到一條 production-ready controller path，能把 live `PodSchedulingMetrics.spec.scaling` 自動轉成完全符合 WOMS worker 的 scaler，並保留 Kafka trigger、CPU trigger、HPA behavior 與命名。因此預設 PoC 策略是：

- 由 WOMS 繼續擁有 `ScaledObject/woms-woms-worker`；
- 把 Gthulhu 作為一個 optional prometheus trigger 加進同一個 object；
- 除非 live deployment 證明 Gthulhu scaler 能安全擁有完整 combined scaler，否則不要對 `woms-woms-worker` 啟用 Gthulhu example scaler 或 PSM scaling hints。

## Gthulhu develop 分支的重要限制

在 Gthulhu `develop` commit `00fc41f` 上，`PodSchedulingMetrics` 需要 `spec.labelSelectors`，CRD 也提供 `k8sNamespaces`、`commandRegex`、metric flags 與 optional scaling hints。

### Label Selector 路徑分裂

Gthulhu manager/API 與 Gthulhu monitor/eBPF 目前不能視為有相同的 selector guarantee。

- Manager-side Kubernetes adapter 會 copy `pod.Labels`，也會用 `selector.Matches(labels.Set(pod.Labels))`，所以 manager UI/API 知道 labels。
- Monitor path 使用 `monitor/collector.PodRef`，目前只有 `PodName`、`PodUID`、`Namespace`、`NodeName`，沒有 labels。
- `monitor/crdwatcher/watcher.go` 會檢查 `k8sNamespaces`，接著註解說明 `PodRef` 不攜帶 labels，然後直接 `return true`。因此在 monitor path 裡，`labelSelectors` 與 `commandRegex` 目前不能被視為 worker-only 硬篩選。

因此短期 WOMS PoC 可以用 namespace `woms` 限縮 Gthulhu collection，但 KEDA 與 Grafana 的 Prometheus query 必須再用 `pod_name=~"woms-woms-worker-.*"` 過濾 worker metrics。正式重用前，Gthulhu 應先擴充 `PodRef` 與 informer path，加入 labels，並真正執行 `labelSelectors` 與必要的 command matching。

### Pod Index 與 Scrape Path 必須先驗證

Monitor collector 會透過 `PodMapper.GetPodForPID` 把 eBPF per-PID metrics 聚合成 pod metrics。這個函式需要透過 `SetPodIndex` 建立 pod UID index。在目前檢查到的 `develop` source 裡，`SetPodIndex` 有出現在 tests，但 monitor startup path 沒有清楚看到 Kubernetes informer 在 production 中填入它。

Chart 也需要 live scrape check。目前 chart 有 manager/sidecar 的 ServiceMonitor，也有 monitor-specific ServiceMonitor，但 monitor `/metrics` endpoint 與 service port wiring 必須在實際 cluster 中驗證後，KEDA 才能依賴 `gthulhu_pod_*` metrics。

這些是 Gthulhu-side verification/fix items，不是 WOMS application-code items。如果 live deployment 沒有暴露 `gthulhu_pod_*{pod_name=...,namespace=...}`，要先修 Gthulhu monitor pod mapping 或 metrics service discovery。

## 目標架構

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

Grafana 不參與 HPA 決策，但在此 PoC 中是必要的 operational calibration 工具。沒有同時看到 Kafka lag、worker replicas、HPA desired replicas、Gthulhu metrics 與 HPA events，就無法安全決定 threshold。

## Scaling Signal Policy

### 主訊號：Kafka lag

Kafka lag 維持為 WOMS worker autoscaling 主訊號：

- topic：`woms.schedule.jobs`
- consumer group：`woms-scheduler-workers`
- threshold：`keda.kafka.lagThreshold`，目前為 `"10"`
- 原因：backlog 直接代表尚未被消化的排程工作

### 輔助訊號：CPU utilization

CPU utilization 維持為輔助訊號：

- trigger type：`cpu`
- metric type：`Utilization`
- target：`keda.cpu.targetUtilization`，目前為 `"70"`
- 原因：排程計算可能在 Kafka lag 變大前就出現 compute-heavy burst

### 補強訊號：Gthulhu scheduling pressure

Gthulhu 應作為補強訊號，不取代 Kafka lag。

建議第一個 trigger query：

```promql
avg(
  rate(gthulhu_pod_involuntary_ctx_switches_total{
    exported_namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
)
```

這個 query 回傳 worker pod 平均 involuntary context-switch rate。對 HPA trigger 來說，per-pod average 比 raw cluster-wide sum 更安全，因為 sum 可能隨 replicas 增加而上升，造成正回饋。

在本次 MicroK8s/kube-prometheus 實測中，Gthulhu `/metrics` endpoint 原始 label 是 `namespace="woms"`，但 Prometheus scrape target 本身已有 `namespace="gthulhu-system"`，因此 Prometheus 會把原始 pod namespace label 改成 `exported_namespace="woms"`。KEDA 查 Prometheus 時必須使用 `exported_namespace`。

run-queue wait 建議先放 dashboard 校準：

```promql
avg(
  rate(gthulhu_pod_wait_time_nanoseconds_total{
    exported_namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
) / 1000000000
```

在變成 trigger 前，Grafana 也應先畫出：

```promql
avg(rate(gthulhu_pod_cpu_migrations_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_numa_migrations_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
max by (pod_name) (rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Threshold 必須在實際 WOMS cluster 校準。

## Gthulhu Preflight 通過後的 WOMS Helm 必要修改

Phase 0/Phase 1 實測已證明 Prometheus query 會回傳穩定 scalar；WOMS chart 現在提供預設關閉的 `keda.gthulhu` optional values block：

```yaml
keda:
  gthulhu:
    enabled: false
    prometheusServerAddress: http://monitoring-kube-prometheus-prometheus.monitoring:9090
    metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate
    threshold: "20"
    query: |
      avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

`deploy/helm/woms/templates/keda-scaledobject.yaml` 會在既有 `triggers:` list 裡條件式 render：

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

`deploy/helm/woms/chart-static.test.mjs` 與 `scripts/verify-hpa-render.sh` 已同步驗證 optional prometheus trigger 只在啟用時 render。

如果未來 Gthulhu 提供已驗證的 controller，能完整擁有 WOMS worker scaler，包含 Kafka lag、CPU utilization、Gthulhu Prometheus metrics、HPA name、min/max replicas 與 scale behavior，那這個 Helm 修改可以改成把整個 worker `ScaledObject` 委派給該 controller。不要讓同一個 worker deployment 被兩個 charts 分別控制。

## Gthulhu PodSchedulingMetrics 草案

第一階段使用 namespace-scoped PSM。`labelSelectors` 表達目標意圖，但在 Gthulhu develop 修好 label matching 前，行為上仍應視為 namespace-only。

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

此 PoC 不要針對 WOMS target 啟用 `spec.scaling`。Scaling 應留在 WOMS 的 `ScaledObject`。

## PoC 階段

### Phase 0：Gthulhu Preflight

- 部署或接入 Prometheus/Grafana。
- 從 `develop` 分支或對應 image/chart 部署 Gthulhu。
- 確認 monitor 實際有啟動，不只是 manager UI。
- 確認 Prometheus 能 scrape monitor `/metrics` endpoint。
- 確認至少有一個 query 回傳 pod-labeled data：

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
```

- 如果 query 沒有資料，先停止 WOMS HPA integration，修 Gthulhu pod index population 或 ServiceMonitor/service wiring。

### Phase 1：只觀測

- 建立 WOMS `PodSchedulingMetrics`。
- 從 web UI 執行現有 WOMS HPA peak demo。
- 確認 Kafka lag 與 Gthulhu worker metrics 在同一段 workload 中變化。

### Phase 2：新增 WOMS Prometheus Trigger

- 在 WOMS 新增 optional `keda.gthulhu` values 與 trigger template。
- 用 `keda.gthulhu.enabled=true` render chart。
- 確認 rendered `ScaledObject` 仍只 target `woms-woms-worker`。
- 確認 triggers 是 Kafka、CPU 與一個 Gthulhu prometheus trigger。
- 從保守 threshold 開始。

### Phase 3：校準

- 比較三種情境：只有 Kafka+CPU、Kafka+CPU+Gthulhu observe-only、Kafka+CPU+Gthulhu trigger enabled。
- 測試不同 `WORKER_MIN_JOB_DURATION_MS`、order volumes 與 worker resource requests。
- 確認 Gthulhu 只在 worker pods 真的出現 scheduling pressure 時提早 scale-out。
- 確認 scale-down 仍遵守既有 120 秒 cooldown 與 stabilization behavior。

## 驗證方式

WOMS static 與 unit checks：

```bash
./scripts/verify-hpa-render.sh
go test ./...
npm run test:web
```

新增 `keda.gthulhu` 後的 render check：

```bash
helm template woms ./deploy/helm/woms --dependency-update \
  --namespace woms \
  --set keda.gthulhu.enabled=true \
  --set keda.gthulhu.prometheusServerAddress=http://monitoring-kube-prometheus-prometheus.monitoring:9090
```

Kubernetes checks：

```bash
kubectl get deploy,pod,scaledobject,hpa -n woms
kubectl get pods -n woms -l app.kubernetes.io/component=scheduler-worker
kubectl describe scaledobject woms-woms-worker -n woms
kubectl describe hpa woms-woms-worker-hpa -n woms
kubectl get podschedulingmetrics -n woms
```

Prometheus checks：

```promql
avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_wait_time_nanoseconds_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) / 1000000000
avg(rate(gthulhu_pod_cpu_migrations_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_numa_migrations_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Grafana dashboard 最少應包含：

- `woms.schedule.jobs` / `woms-scheduler-workers` 的 Kafka lag。
- Worker current replicas 與 desired replicas。
- HPA events 或 desired replica changes。
- Worker CPU utilization。
- Gthulhu worker involuntary context-switch rate。
- Gthulhu worker wait-time rate。
- Gthulhu worker CPU migration 與 NUMA migration rates。

## 成功標準

- Gthulhu 未啟用時，WOMS 仍可透過 Kafka lag 擴展 workers。
- Gthulhu worker pod metrics 可被 Prometheus 查詢。
- Optional Gthulhu prometheus trigger 進入同一個 WOMS `ScaledObject`，而不是另一個獨立 scaler。
- Gthulhu/Prometheus data missing 不會移除 Kafka lag scaling path。
- HPA scale-down 不比目前 WOMS 行為更激進。
- Grafana 能解釋 scale-out 是來自 backlog、CPU，還是 Gthulhu scheduling pressure。

## 風險與控制

1. **目前 Gthulhu label matching 不精準**
   在 develop commit `00fc41f` 上，先把 `PodSchedulingMetrics` 視為 namespace-scoped；在 Gthulhu 加入 label-aware `PodRef` matching 前，WOMS worker metrics 需靠 PromQL `pod_name` 過濾。

2. **同一個 worker deployment 不能有兩個 HPA 控制來源**
   不要針對 `woms-woms-worker` 啟用 Gthulhu chart example scaling 或 `PodSchedulingMetrics.spec.scaling`。Gthulhu 應作為 WOMS 既有 `ScaledObject` 內的一個 trigger。

3. **Threshold 依環境而定**
   Gthulhu metrics 會受 kernel version、CPU topology、node pressure、worker resource requests 與 co-located workloads 影響。

4. **Gthulhu 不是 backlog signal**
   Kafka lag 仍是主要 queue-depth source。Gthulhu 用來提供 runtime scheduling evidence。

## 建議結論

以目前 WOMS Kafka+CPU KEDA/HPA path 作為穩定 baseline。Gthulhu 應作為一個 optional prometheus trigger 加進既有 WOMS `ScaledObject`，Grafana 作為 threshold calibration 必要條件，而且不要對 `woms-woms-worker` 啟用任何獨立的 Gthulhu-managed scaler。第一個可用 trigger 建議使用 worker pod 平均 involuntary context-switch rate；`wait_time` 與 migration metrics 先留在 dashboard，等實際 cluster threshold 明確後再考慮納入 scaling。
