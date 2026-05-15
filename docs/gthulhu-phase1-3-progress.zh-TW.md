# Gthulhu Phase 1-3 進度紀錄

## 2026-05-14 狀態

### 使用者目標

- 執行 Phase 1 observe-only。
- 如果 Phase 1 完全符合，就繼續 Phase 2。
- 如果 Phase 2 完全符合，就繼續 Phase 3。
- 將 Gthulhu 開發推到使用者 fork：`https://github.com/d11nn/Gthulhu.git`，branch `feat/woms-poc`。

### 已完成：Gthulhu fork branch

已在 `/home/ubuntu/Gthulhu` 從最新 `origin/develop` 建立 branch：

```text
feat/woms-poc
```

基準 commit：

```text
00fc41feeb2a23481b12aa27898cc8e4cd215b42
```

已提交並推到使用者 fork：

```text
remote: https://github.com/d11nn/Gthulhu.git
branch: feat/woms-poc
commit: f71f78a feat: add monitor pod index refresh
```

GitHub 提示可建立 PR：

```text
https://github.com/d11nn/Gthulhu/pull/new/feat/woms-poc
```

此次 Gthulhu branch 包含：

- `monitor/monitor.go`
  - monitor 啟動時建立 in-cluster Kubernetes client。
  - 每 30 秒用 `spec.nodeName=<NODE_NAME>` list 本 node pods。
  - 將 `pod.UID -> collector.PodRef` 寫入 `PodMapper.SetPodIndex()`。
  - 成功時 log：`pod index refreshed`。
- `Dockerfile`
  - runtime image 額外 copy `/build/sched_monitor.bpf.o` 到 `/gthulhu/sched_monitor.bpf.o`。
- `go.mod` / `go.sum`
  - `go mod tidy` 後更新 module metadata。

驗證：

- `git diff --check` 通過。
- 同一份 patch 已在 Phase 0 使用 Docker build 成功：
  - image：`localhost:32000/gthulhu-monitor:phase0`
  - digest：`sha256:a8bbc6578ed79dc5947fd5b02aaa30fb76b500139eabb01b89b1e28260518c10`
- 同一份 image 已在 MicroK8s 跑通 Phase 0：
  - `pod index refreshed`
  - `sched_monitor BPF program loaded`
  - `attached BPF program handle_sched_switch`
  - `attached BPF program handle_sched_process_exit`
  - `count(gthulhu_pod_process_count{pod_name!="",namespace!=""}) = 21`

限制：

- 本機 `go test ./monitor/...` 因 host 缺少 libbpf headers 失敗：

```text
fatal error: bpf/bpf.h: No such file or directory
```

此失敗是 host dependency 問題，不是已提交 patch 的語法或 module 問題；Docker build path 已完整通過。

### Phase 1 observe-only 結果

Phase 1 gate 來自 `docs/gthulhu-hpa-poc.zh-TW.md`：

- 建立 WOMS `PodSchedulingMetrics`。
- 從 web UI 執行現有 WOMS HPA peak demo。
- 確認 Kafka lag 與 Gthulhu worker metrics 在同一段 workload 中變化。

已套用 observe-only manifest：

```text
/tmp/woms-podschedulingmetrics.yaml
```

內容為 namespace-scoped observe-only PSM，不含 `spec.scaling`，因此不會建立另一個 scaler，也不會修改 WOMS `ScaledObject` 或 HPA。

```bash
kubectl apply -f /tmp/woms-podschedulingmetrics.yaml
```

結果：

- `PodSchedulingMetrics/woms-scheduler-worker` 已建立在 namespace `woms`。
- `spec.enabled: true`，`collectionIntervalSeconds: 10`。
- `k8sNamespaces: ["woms"]`。
- `labelSelectors` 指向 `app.kubernetes.io/instance=woms` 與 `app.kubernetes.io/component=scheduler-worker`。
- 無 `spec.scaling`。

執行 WOMS HPA peak demo 後，API 回報：

```text
lineCount: 200
orderCount: 1000
jobCount: 200
statuses: queued=200
createdAt: 2026-05-14T17:50:15Z
```

同一段 workload 中觀察到：

- HPA external metric 從先前約 `9` 上升到 `14500m`，後續約 `10334m/10 (avg)`。
- Worker deployment 從 1 replica 擴到 3 replicas。
- Demo jobs 完成：`statuses completed=200`。
- Gthulhu `/metrics` 有 `woms-woms-worker-*` pod-labeled `gthulhu_pod_*` metrics。
- Prometheus scrape 後，Gthulhu 原始 `namespace="woms"` label 被保存為 `exported_namespace="woms"`，因為 scrape target 自己已有 `namespace="gthulhu-system"`。
- Prometheus query：
  - `avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) = 0.6296506179835625`
  - `avg(rate(gthulhu_pod_wait_time_nanoseconds_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) / 1000000000 = 5.244143697741777`

Phase 1 gate 結論：通過。

### Phase 2 結果

Phase 2 目標：

- 在 WOMS 新增 optional `keda.gthulhu` values 與 trigger template。
- 用 `keda.gthulhu.enabled=true` render chart。
- 確認 rendered `ScaledObject` 仍只 target `woms-woms-worker`。
- 確認 triggers 是 Kafka、CPU 與一個 Gthulhu prometheus trigger。

已在 branch `feat/gthulhu-keda-trigger` 修改：

- `deploy/helm/woms/values.yaml`
  - 新增 `keda.gthulhu.enabled: false`。
  - Prometheus server 預設為 `http://monitoring-kube-prometheus-prometheus.monitoring:9090`。
  - query 使用 `exported_namespace="woms"`。
- `deploy/helm/woms/templates/keda-scaledobject.yaml`
  - 只有 `keda.gthulhu.enabled=true` 時才新增一個 `type: prometheus` trigger。
- `deploy/helm/woms/chart-static.test.mjs`
  - 驗證 values 與 template wiring。
- `scripts/verify-hpa-render.sh`
  - 預設驗證 Kafka+CPU。
  - `GTHULHU_ENABLED=true` 時驗證 Kafka+CPU+Gthulhu。
- `README.md`、`README.en.md`、`README.zh-TW.md`
  - 說明 optional Gthulhu trigger、同一個 `ScaledObject` ownership，以及 `exported_namespace`。
- `docs/gthulhu-hpa-poc.zh-TW.md`、`docs/gthulhu-hpa-poc.en.md`
  - 更新 PromQL 與 Phase 2 實作狀態。

Render 實證：

- 預設 render 不包含 Gthulhu trigger。
- `GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh` 通過。
- `helm template ... --set keda.gthulhu.enabled=true` 產生的 `ScaledObject`：
  - `scaleTargetRef.name: woms-woms-worker`
  - triggers：`kafka`、`cpu`、`prometheus`
  - prometheus `metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate`
  - prometheus query 使用 `gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}`

測試：

- `node --test deploy/helm/woms/chart-static.test.mjs` 通過。
- `./scripts/verify-hpa-render.sh` 通過。
- `GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh` 通過。
- `npm run test:web` 通過。
- `GOCACHE=/tmp/woms-go-build-cache go test ./...` 通過。

Live cluster 安全界線：

- 沒有執行 `helm upgrade` 啟用 Gthulhu trigger。
- `kubectl get scaledobject woms-woms-worker -n woms -o yaml` 確認 live `ScaledObject` 仍只有 Kafka 與 CPU triggers。

Phase 2 gate 結論：以 chart/code/render 驗證通過；live WOMS HPA trigger 未修改。

### Phase 3 結果

Phase 3 原本需要明確批准 live WOMS HPA trigger。使用者後續已批准，因此已啟用 live Gthulhu trigger 並執行 calibration。

Live upgrade：

```text
release: woms
namespace: woms
revision: 8
time: 2026-05-14T18:02:30Z
```

啟用後的 live `ScaledObject/woms-woms-worker`：

- `scaleTargetRef.name: woms-woms-worker`
- triggers：Kafka、CPU、Gthulhu Prometheus
- Kafka trigger 未修改：`lagThreshold: "10"`
- CPU trigger 未修改：`value: "70"`
- Gthulhu trigger：
  - `metricName: woms_worker_gthulhu_involuntary_ctx_switches_rate`
  - `threshold: "20"`
  - query：`avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))`
- KEDA status：
  - `s0-kafka-woms-schedule-jobs: Happy`
  - `s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate: Happy`

Phase 3 baseline demo 結果：

- demo 建立時間：`2026-05-14T18:03:32Z`
- worker 從 3 replicas 擴到 8 replicas。
- HPA events 顯示 scale-out reason 仍是 Kafka：
  - `New size: 6; reason: external metric s0-kafka-woms-schedule-jobs above target`
  - `New size: 8; reason: external metric s0-kafka-woms-schedule-jobs above target`
- Gthulhu trigger 在 HPA 中正常計算，但保守 threshold `20` 下沒有主導擴容：
  - 範例 HPA metric：`2245m/20`
  - Prometheus scalar：`avg(rate(...)) = 6.7333333333333325`
- scale-down 行為未被 Gthulhu 破壞：
  - workload 完成後 Kafka/Gthulhu/CPU 皆低於 target。
  - HPA 依 120 秒 stabilization 與 50% policy 開始降 replicas。

結論：Phase 3 live 接線成功；保守 threshold `20` 不會誤導擴容，但此 baseline 不是 Gthulhu 主導 scale-out 的證明。

### 2026-05-15 Gthulhu-trigger proof scenario

目的：證明 WOMS HPA 可以被 Gthulhu Prometheus trigger 主導，而不只是把 metric 接進 HPA。

操作：

- 暫時把 `keda.gthulhu.threshold` 從 `20` 降到 `1`。
- 保留 Kafka trigger `lagThreshold: "10"`，不改 CPU trigger `value: "70"`。
- 重跑 WOMS HPA peak demo。

基準：

- Kafka：`7/10`
- Gthulhu：`999m/1`
- worker replicas：`1`
- KEDA scaler health：Kafka 與 Gthulhu 都是 `Happy`

demo：

```text
createdAt: 2026-05-15T05:02:03Z
lineCount: 200
orderCount: 1000
jobCount: 200
```

關鍵觀測：

- demo jobs 完成後，曾觀察到 Kafka 已低於 target，但 Gthulhu 高於 target：
  - Kafka：`8125m/10`
  - Gthulhu：`3243m/1`
  - worker：`10/10`
- HPA events 明確顯示 Gthulhu Prometheus trigger 主導 scale-out：
  - `New size: 4; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
  - `New size: 8; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
  - `New size: 10; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
- 後續 Prometheus query 仍可查到 scalar：
  - `avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) = 1.8000600020000668`

Proof scenario 結論：通過。Gthulhu 不只是被 scrape，也能透過 KEDA Prometheus scaler 成為 HPA scale decision 的來源。

### 2026-05-15 realistic pressure scenario

目的：使用較高 threshold，加入更接近真實的 scheduling pressure，觀察 Kafka lag 低於門檻時 Gthulhu 是否仍能主導或維持 scale decision。

嘗試 1：node-level CPU pressure

- 將 `keda.gthulhu.threshold` 設為 `5`。
- 建立短時 CPU pressure Job：
  - namespace：`woms`
  - job：`woms-node-cpu-pressure`
  - parallelism：`4`
  - image：`docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`
  - 每個 pod 跑多個 `yes > /dev/null` loop。
- 實測 pressure 約 3.4 cores。

結果：

- demo 建立時間：`2026-05-15T05:10:23Z`
- demo jobs 完成。
- HPA scale-out reason 仍是 Kafka：
  - `New size: 10; reason: external metric s0-kafka-woms-schedule-jobs above target`
- Gthulhu 未達 threshold：
  - HPA：`125m/5`
  - Prometheus scalar：`1.2421856569945742`
  - wait-time rate：`2.172601850609666`

嘗試 2：worker pod 內 ephemeral CPU pressure，未重啟 Gthulhu

- 先把 worker runtime 回到 production/demo 預設：
  - `WORKER_MIN_JOB_DURATION_MS=0`
- 等 Kafka 與 Gthulhu metrics 低於 target：
  - Kafka：`2/10`
  - Gthulhu：`0/5`
  - worker：`1` replica
- 在實際 `woms-woms-worker-*` pod 注入 ephemeral container：
  - container：`gthulhu-worker-pressure`
  - image：`docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`
  - 內容：12 個 `yes > /dev/null` loop，持續 180 秒。

結果：

- Ephemeral container 確實在 worker pod 中執行，`ps` 顯示多個 `yes` processes。
- 但 Gthulhu endpoint 沒有輸出新 worker pod 的 `gthulhu_pod_*` series：
  - Prometheus `{__name__=~"gthulhu_pod_.*",pod_name=~"woms-woms-worker-.*"}` 回 empty。
  - 直接讀 `gthulhu-scheduler-sidecar` `/metrics` 也沒有新的 `woms-woms-worker-*` series。
- HPA 沒有出現 Gthulhu reason；反而短暫出現 CPU resource trigger：
  - `New size: 2; reason: cpu resource utilization above target`

嘗試 3：重啟 Gthulhu monitor 後再做 worker pod ephemeral CPU pressure

- 暫時維持 `keda.gthulhu.threshold: "5"`。
- `kubectl rollout restart daemonset/gthulhu-scheduler -n gthulhu-system`。
- 等待 Gthulhu 重新載入 BPF 並 refresh pod index：
  - `sched_monitor BPF program loaded`
  - `pod index refreshed`
- 再次在實際 `woms-woms-worker-*` pod 注入 ephemeral CPU pressure：
  - container：`gthulhu-worker-pressure2`
  - 16 個 `yes > /dev/null` loop，持續 180 秒。

結果：

- Gthulhu worker pod series 恢復：
  - `gthulhu_pod_involuntary_ctx_switches_total{namespace="woms",pod_name="woms-woms-worker-6f96996ddd-vpn6w"}`
  - `gthulhu_pod_process_count{namespace="woms",pod_name="woms-woms-worker-6f96996ddd-vpn6w"} 23`
- Kafka 低於 target：
  - `500m/10`
- Gthulhu 高於 threshold：
  - HPA：`5197m/5`
  - Prometheus scalar：`21.13681462962963`
- 但 CPU trigger 也同時高於 target：
  - CPU：`462%/70%`
- HPA events 因此仍歸因 CPU：
  - `New size: 2/4/8/10; reason: cpu resource utilization above target`
- pressure 結束後：
  - Kafka：`200m/10`
  - CPU：`1%/70%`
  - Gthulhu：`109m/5`
  - Prometheus scalar：`1.0844444444444443`
  - replicas 保持高位是 scale-down stabilization，不是 Gthulhu 繼續維持。

Realistic scenario 結論：部分通過但未完整通過。重啟 Gthulhu monitor 後，實際 worker pod pressure 可以讓 Gthulhu metric 在 Kafka 低於門檻時超過 threshold `5`；但同一個 pressure 同時觸發 CPU scaler，因此 HPA reason 沒有轉成 Gthulhu Prometheus metric。Proof scenario 已完整證明 Gthulhu 可主導 HPA；realistic scenario 還需要一個能提高 scheduling pressure、但不讓 CPU utilization 同時主導的 workload，或需要暫時隔離 CPU trigger 才能做純 Gthulhu attribution。

實驗結束後 live 狀態已恢復：

- `keda.gthulhu.threshold: "20"`
- `WORKER_MIN_JOB_DURATION_MS=0`
- Kafka scaler health：`Happy`
- Gthulhu scaler health：`Happy`

### Demo 復現建議

目前可可靠展示的 demo 是 proof scenario：

1. 確認 live trigger 已啟用且 KEDA scaler health 是 `Happy`。
2. 暫時把 `keda.gthulhu.threshold` 設為 `1`。
3. 執行 WOMS HPA peak demo。
4. 觀察：
   - `kubectl describe hpa woms-woms-worker-hpa -n woms`
   - HPA events 出現 `s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target`
   - Prometheus query 有值。
5. demo 後把 threshold 恢復保守值 `20`。

Gthulhu 在此 demo 辦到的事：

- 用 eBPF / monitor path 產生 WOMS worker pod scheduling metrics。
- 透過 Prometheus scrape 讓 KEDA Prometheus scaler 可讀取。
- 作為第三個 trigger 加入 WOMS 既有 `ScaledObject`，不取代 Kafka 或 CPU。
- 在 proof threshold 下，讓 HPA 實際因 Gthulhu metric scale out。

尚未宣稱的事：

- threshold `5` 或 `20` 在這台單節點 MicroK8s VM 上已能穩定代表 production scheduling pressure。
- worker rollout 後 Gthulhu monitor-only path 能穩定追蹤所有新的 worker pods。
- 在不改 CPU trigger 的情況下，realistic pressure 能產生純 Gthulhu HPA reason。
