# WOMS Gthulhu HPA PoC 方案

## 目標

本方案把 WOMS 現有的 `scheduler-worker` HPA 情境升級成 Gthulhu-backed autoscaling PoC。WOMS 仍保留目前以 Kafka lag 為主的 KEDA/HPA 模型，但新增 Gthulhu 回傳的 pod-level scheduling information 作為第二組 scaling signal，讓 HPA 不只看 backlog 或平均 CPU，也能看 pod 在 node 上實際遇到的 kernel scheduling pressure。

這個 PoC 的定位不是取代 WOMS 現有 HPA，而是建立一個可複製到 SD-Core SMF/UPF 的 autoscaling pattern：

```text
workload pod information
  -> Gthulhu eBPF pod scheduling metrics
  -> Prometheus
  -> Grafana dashboard / operational observation
  -> KEDA prometheus trigger
  -> Kubernetes HPA
  -> target workload replicas
```

## 為什麼使用 Gthulhu

WOMS 目前的 HPA 壓力來源主要是 `woms.schedule.jobs` Kafka lag。這能回答「排程任務是否堆積」，但不能回答「worker pod 是否已經在 node 上被 CPU scheduling 延遲、頻繁 preempt、migration 過多，或受到其他 pod 干擾」。Gthulhu 的價值在於補上這個缺口。

使用 Gthulhu 的主要原因如下：

1. **看見 pod-level kernel scheduling pressure**
   Gthulhu 透過 eBPF 收集 per-process scheduling events，並聚合成 pod-level metrics，例如 `wait_time_ns`、`run_count`、`involuntary_ctx_switches`、`cpu_migrations`、`numa_migrations` 與 `process_count`。這比 Kubernetes CPU utilization 更接近「pod 是否真的被 scheduler 卡住」。

2. **不需要改 WOMS application code**
   WOMS 不必在 Go API 或 worker 內新增自訂 metric 才能得到 scheduling pressure。Gthulhu 從 node/kernel 層觀察 pod，適合作為監管型部署能力。

3. **保留 KEDA/HPA 的 Kubernetes-native 操作模式**
   Gthulhu 不是直接控制 replica 數，而是把 metrics 暴露給 Prometheus，再由 KEDA prometheus trigger 餵給 HPA。這讓 WOMS 現有 Helm/KEDA/HPA 架構可以小幅延伸，而不是改成另一套 autoscaler。

4. **補齊可觀測性與決策校準**
   Prometheus 是 Gthulhu-backed HPA 的必要資料面，KEDA 需要透過 prometheus trigger 查詢 Gthulhu metrics。Grafana 雖然不是 HPA 必要元件，但應納入 PoC，用來觀察 Kafka lag、worker replicas、Gthulhu scheduling pressure 與 HPA events 是否同步，避免只看到 replica 變化卻無法解釋 scale-out 原因。

5. **能把 WOMS 當成 SD-Core SMF/UPF 前導 PoC**
   SMF/UPF 對 CPU scheduling jitter、preemption、migration、NUMA locality 與 packet-processing latency 更敏感。先用 WOMS `scheduler-worker` 驗證「Gthulhu pod information -> KEDA -> HPA」後，未來可把同一套 pattern 套到 SMF control-plane pod 或 UPF data-plane pod。

6. **比單純 CPU trigger 更適合高壓力判斷**
   CPU utilization 高不一定代表 pod 需要 scale out；CPU utilization 不高也可能因為 run queue wait 或 preemption 導致處理延遲。Gthulhu metrics 可作為 Kafka lag 與 CPU 之間的補強訊號。

## WOMS 與 Gthulhu 的關係

WOMS 的 `scheduler-worker` 是 Kafka consumer。當月底排程、急單重排或大量 demo jobs 進入 `woms.schedule.jobs` 時，Kafka lag 上升，KEDA 目前會建立並驅動 `woms-woms-worker-hpa`，調整 `Deployment/woms-woms-worker` replicas。

Gthulhu 在這個場景中的角色是監管與回饋層：

- Gthulhu 監看 WOMS worker pods。
- eBPF collector 收集 worker pod 的 scheduling metrics。
- Prometheus scrape Gthulhu `/metrics`。
- WOMS 的 KEDA `ScaledObject` 新增 prometheus trigger。
- HPA 由 Kafka lag、CPU utilization 與 Gthulhu scheduling pressure 共同決定 replicas。

建議 PoC 只先納入 `scheduler-worker`，不要同時納入 API/web。原因是 worker 的 workload 壓力來源清楚、已有 Kafka lag baseline，而且 scale-out 行為不會直接改變 request path 的 availability model。

## 建議架構

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

## Prometheus / Grafana 規劃

WOMS 現況沒有部署 Prometheus 或 Grafana；目前 HPA 依賴 KEDA Kafka trigger、CPU trigger 與 `metrics-server`。導入 Gthulhu 後，Prometheus 與 Grafana 的定位如下：

- **Prometheus：必要**
  Gthulhu pod scheduling metrics 需要被 Prometheus scrape，KEDA prometheus trigger 也需要 Prometheus query 才能把 Gthulhu metrics 轉成 HPA external metric。沒有 Prometheus，就無法完成 Gthulhu -> KEDA -> HPA 的閉環。

- **Grafana：納入 PoC，作為觀測與校準工具**
  Grafana 不直接參與 HPA 決策，但 PoC 應部署 dashboard，顯示 Kafka lag、worker replicas、HPA desired replicas、Gthulhu `involuntary_ctx_switches`、`wait_time`、`cpu_migrations` 與 `numa_migrations`。這能幫助校準 threshold，並判斷 scale-out 是 backlog、CPU 還是 kernel scheduling pressure 造成。

- **metrics-server：保留**
  `metrics-server` 仍負責目前 CPU trigger 所需的 resource metrics，不被 Prometheus/Grafana 取代。

建議 PoC monitoring stack 使用 `kube-prometheus-stack` 或既有平台 Prometheus/Grafana。WOMS Helm chart 不必直接 vendor 監控 stack，但部署文件與驗證文件應把 Prometheus/Grafana 列為 Gthulhu HPA PoC prerequisites。

## Scaling Signal 設計

### 保留既有主訊號：Kafka lag

Kafka lag 仍是 WOMS worker autoscaling 的主訊號：

- topic：`woms.schedule.jobs`
- consumer group：`woms-scheduler-workers`
- threshold：`keda.kafka.lagThreshold`
- 目的：反映「尚未被 worker 消化的 scheduling jobs」

### 保留既有輔助訊號：CPU utilization

CPU trigger 保留為 compute burst 的輔助訊號：

- target utilization：`keda.cpu.targetUtilization`
- 目的：在 worker 排程計算密集時補強 scale-out

### 新增 Gthulhu 訊號：pod scheduling pressure

建議第一階段使用 `involuntary_ctx_switches` 與 `wait_time`，不要一開始納入太多 metrics。

建議 Prometheus query：

```promql
sum(
  rate(gthulhu_pod_involuntary_ctx_switches_total{
    namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
)
```

若 Gthulhu 環境已穩定收集 `wait_time`，可再加入：

```promql
sum(
  rate(gthulhu_pod_wait_time_nanoseconds_total{
    namespace="woms",
    pod_name=~"woms-woms-worker-.*"
  }[2m])
) / 1000000000
```

第一階段建議先用 `involuntary_ctx_switches` 作 KEDA prometheus trigger，因為它較容易觀察到 preemption pressure；`wait_time` 可作 dashboard 與後續 threshold 校準。

## Helm / Kubernetes 設計草案

### Gthulhu PodSchedulingMetrics

WOMS worker pod 目前有以下 labels：

- `app.kubernetes.io/instance: <release>`
- `app.kubernetes.io/component: scheduler-worker`

PoC resource 草案：

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

落地注意：目前 `/home/ubuntu/Gthulhu` 的 `monitor/crdwatcher/watcher.go` 註解指出 `PodRef` 尚未攜帶 labels，`psmMatchesPod` 目前主要以 namespace 篩選，label selector 尚未精準生效。因此 WOMS PoC 有兩個選擇：

1. **短期 PoC**：用 namespace `woms` 隔離測試環境，並在 Prometheus query 用 `pod_name=~"woms-woms-worker-.*"` 精準選 worker。
2. **正式整合前**：先在 Gthulhu 補齊 PodRef labels / informer label matching，再依 `labelSelectors` 精準監控 worker pods。

### WOMS KEDA ScaledObject 新增 prometheus trigger

建議在 `deploy/helm/woms/templates/keda-scaledobject.yaml` 增加可選 trigger，並在 `values.yaml` 新增 `keda.gthulhu` 區塊。

範例值：

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

範例 trigger：

```yaml
- type: prometheus
  metadata:
    serverAddress: {{ .Values.keda.gthulhu.prometheusServerAddress | quote }}
    metricName: {{ .Values.keda.gthulhu.metricName | quote }}
    query: {{ .Values.keda.gthulhu.query | quote }}
    threshold: {{ .Values.keda.gthulhu.threshold | quote }}
```

## SD-Core SMF/UPF 延伸路徑

WOMS PoC 成功後，建議拆成兩條 5GC 延伸線：

1. **SMF HPA**
   SMF 是 control-plane NF，壓力通常來自 PDU session 建立、修改、釋放，以及 PFCP/N4 control-plane 互動。Gthulhu 可觀察 SMF pod 是否出現 scheduling latency 或 preemption，再與 request rate、session count 或 queue depth 一起驅動 HPA。

2. **UPF HPA**
   UPF 是 data-plane NF，對 CPU locality、preemption、migration、NUMA placement 與 packet-processing jitter 更敏感。Gthulhu 的 `cpu_migrations`、`numa_migrations`、`wait_time` 與 `involuntary_ctx_switches` 比一般 CPU utilization 更接近 UPF 實際資料面壓力。不過 UPF scale-out 會牽涉流量導向、PFCP session state、PDR/FAR/QER 安裝與 datapath consistency，不能只看 HPA replica 數。

WOMS 的價值是先驗證 autoscaling feedback loop，而不是直接處理 5GC stateful datapath 問題。

## PoC 階段規劃

### Phase 1：觀測，不改 HPA 決策

- 部署 Gthulhu。
- 部署或接入 Prometheus，並確認可 scrape Gthulhu `/metrics`。
- 部署或接入 Grafana，建立 WOMS worker / Gthulhu scheduling dashboard。
- 建立 WOMS `PodSchedulingMetrics`。
- Prometheus scrape Gthulhu metrics。
- Grafana 或 Prometheus query 驗證 worker pod 有 metrics。
- 用 WOMS HPA demo 建立 200 lines、1,000 orders、200 jobs，觀察 Kafka lag 與 Gthulhu metrics 是否同步上升。

### Phase 2：KEDA 新增 Gthulhu prometheus trigger

- 在 WOMS Helm chart 加入 `keda.gthulhu.enabled`。
- 保留 Kafka lag 與 CPU trigger。
- 新增 prometheus trigger。
- 設定保守 threshold，避免 Gthulhu signal 一開始過度 scale out。
- 驗證 HPA events 可看到 external metric 觸發 scale-up。

### Phase 3：校準 threshold 與 failure policy

- 用不同 job volume 測試 threshold。
- 比較「只有 Kafka lag」與「Kafka lag + Gthulhu」的 scale-up timing。
- 設定 Gthulhu/Prometheus 不可用時的行為：HPA 不應因 Gthulhu metrics missing 而影響 Kafka lag scaling。
- 決定是否把 `wait_time` 納入 trigger 或只留在 dashboard。

### Phase 4：抽象成 SD-Core 可重用模式

- 把 metric query、threshold、target workload、namespace、label selector 做成 values。
- 建立 SMF/UPF 版本的 PodSchedulingMetrics 與 KEDA prometheus trigger 範本。
- 對 UPF 加入 NUMA / CPU migration dashboard 與告警。

## 驗證方式

WOMS 端：

```bash
./scripts/verify-hpa-render.sh
go test ./...
npm test
```

Kubernetes 端：

```bash
kubectl get pods -n woms -l app.kubernetes.io/component=scheduler-worker
kubectl get podschedulingmetrics -n woms
kubectl get hpa -n woms
kubectl describe hpa woms-woms-worker-hpa -n woms
kubectl get scaledobject -n woms
```

Prometheus query：

```promql
sum(rate(gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
sum(rate(gthulhu_pod_wait_time_nanoseconds_total{namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Grafana dashboard 至少應包含：

- Kafka lag：`woms.schedule.jobs` / `woms-scheduler-workers`
- worker replicas：current / desired
- HPA events 或 desired replicas 變化
- Gthulhu worker pod `involuntary_ctx_switches` rate
- Gthulhu worker pod `wait_time` rate
- Gthulhu worker pod `cpu_migrations` / `numa_migrations` rate

成功標準：

- Gthulhu 能觀察到 WOMS worker pod metrics。
- Prometheus 能 scrape 並查詢 Gthulhu worker pod metrics。
- Grafana 能顯示 worker backlog、replicas 與 Gthulhu scheduling pressure dashboard。
- WOMS HPA demo 期間 Kafka lag 上升。
- Gthulhu scheduling pressure metrics 在 worker 忙碌時有可觀察變化。
- KEDA `ScaledObject` 同時包含 Kafka、CPU 與 Gthulhu prometheus triggers。
- HPA 能 scale out `woms-woms-worker`，且 scale down 不會比現有 cooldown 更激進。

## 風險與限制

1. **Gthulhu label selector 目前可能未精準生效**
   目前 Gthulhu watcher 的 `PodRef` 尚未保存 pod labels，因此正式整合前應補齊 label matching，或在 Prometheus query 層用 `pod_name` 精準過濾。

2. **threshold 需要實測校準**
   `involuntary_ctx_switches` 與 `wait_time` 的絕對值會受 node kernel、CPU topology、其他 workload 與 worker resource request 影響，不能直接沿用到 SD-Core。

3. **不要讓 Gthulhu trigger 蓋過 Kafka lag**
   WOMS 的主要排程 backlog 還是 Kafka lag。Gthulhu trigger 應作為「pod scheduling pressure 補強」，不是唯一 scaling source。

4. **UPF 不能只靠 HPA 解決**
   UPF 涉及 flow/session state 與 datapath routing，Gthulhu + KEDA 可以提供更好的 pressure signal，但 scale-out 邏輯仍需搭配 5GC control-plane 與 traffic steering。

## 建議結論

WOMS 應先採用「Kafka lag 主導、CPU 輔助、Gthulhu scheduling pressure 補強」的 HPA 方案。Gthulhu 的使用理由是它能提供 Kubernetes/HPA 原生訊號看不到的 pod-level kernel scheduling information，並且可透過 Prometheus 與 KEDA 進入現有 HPA loop。這讓 WOMS 成為低風險 PoC，也為後續 SD-Core SMF/UPF 的 Gthulhu-backed autoscaling 建立共同架構。
