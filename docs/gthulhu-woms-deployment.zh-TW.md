# Gthulhu 與 WOMS 部署對齊指南

## 目前基準狀態

截至 2026-05-15，本機檢查到的 Gthulhu refs：

```text
upstream main:    Gthulhu/Gthulhu origin/main 5e4a72f
upstream develop: Gthulhu/Gthulhu origin/develop 6984d82
WOMS PoC branch:  d11nn/Gthulhu feat/woms-poc f71f78a
```

WOMS 先前通過 Phase 0-3 的實測基準是 `d11nn/feat/woms-poc`。

`feat/woms-poc` 相對 upstream `main` 是 `0 behind / 2 ahead`。這兩個 ahead commits 是：

```text
00fc41f feat: support per-node runtime scheduler selection (#110)
f71f78a feat: add monitor pod index refresh
```

如果拿 `feat/woms-poc` 去和最新 upstream `develop` 比，則是 `3 behind / 1 ahead`。唯一仍在 fork branch 上、尚未進 upstream `develop` 的必要 WOMS PoC commit 是：

```text
f71f78a feat: add monitor pod index refresh
```

此 commit 讓 Gthulhu monitor 在 Kubernetes 內定期用 `spec.nodeName=<NODE_NAME>` refresh pod UID index，並把 `sched_monitor.bpf.o` 放進 runtime image。沒有這個修正時，Gthulhu monitor 可能無法穩定把 eBPF per-PID metrics 對應回 `woms-woms-worker-*` pod，因此不能假設 WOMS 的 KEDA Prometheus trigger 會和先前實測環境一樣可用。

## 結論

目前可以把 `d11nn/feat/woms-poc` 視為 WOMS/Gthulhu PoC 的已驗證基準；但不能直接保證「改用其他 Gthulhu branch 或 floating image tag」仍等同於當初與 WOMS Helm chart 配合驗證成功的環境。

可接受策略只有兩種：

1. **重現已驗證環境**：使用 `d11nn/feat/woms-poc` 的 `f71f78a` 或同等 image，並依本文件驗證 Gthulhu metrics、Prometheus scrape、KEDA scaler health 與 WOMS HPA events。
2. **升級到最新 upstream develop**：先把 `f71f78a` 的 monitor pod index refresh 重新套到最新 `origin/develop`，重建並部署 Gthulhu image/chart，再完整重跑本文件的驗證。驗證通過前，不要聲稱最新 upstream `develop` 已等同於 WOMS PoC 環境。

## 版本對齊檢查

在 Gthulhu repo 先確認 upstream 與 fork 狀態：

```bash
cd /home/ubuntu/Gthulhu
git fetch origin develop
git fetch d11nn develop feat/woms-poc
git rev-list --left-right --count origin/main...d11nn/feat/woms-poc
git rev-list --left-right --count origin/develop...d11nn/feat/woms-poc
git log --oneline --left-right --cherry-pick origin/main...d11nn/feat/woms-poc
git log --oneline --left-right --cherry-pick origin/develop...d11nn/feat/woms-poc
```

期望你能清楚看到 `feat/woms-poc` 相對 upstream `main` 是已驗證 PoC branch，且相對 upstream `develop` 是否仍有尚未進 upstream 的 WOMS 必要 commit。如果 upstream `develop` 已經包含同等修正，才可以把 upstream `develop` 當作新的候選基準。

## 建議的升級流程

若要用最新 upstream `develop`，先建立新的整合分支：

```bash
cd /home/ubuntu/Gthulhu
git fetch origin develop
git switch -c feat/woms-poc-refresh origin/develop
git cherry-pick f71f78a
```

如果 cherry-pick 衝突，必須保留兩件事：

- monitor startup path 需要定期 refresh Kubernetes pod index，讓 `PodMapper.SetPodIndex()` 有 production 資料來源。
- runtime image 需要包含 monitor eBPF object，例如 `/gthulhu/sched_monitor.bpf.o`。

完成後重建 Gthulhu images，並把 Helm values 指到你剛 build/push 的 image tags。不要只使用 floating `develop` tag 來判斷是否和本次 PoC 一樣，因為 `develop` tag 會隨 upstream 推進而改變。

## Gthulhu 部署前置條件

WOMS PoC 使用的邊界如下：

- WOMS Helm 只負責 WOMS API、web、worker、PostgreSQL、Redis、Kafka 與 worker `ScaledObject`。
- Gthulhu Helm 負責 Gthulhu CRDs、scheduler/monitor DaemonSet、manager、ServiceMonitor 與 Gthulhu runtime components。
- Prometheus/Grafana 由 platform monitoring stack 提供。本次實測使用的 Prometheus service address 是：

```text
http://monitoring-kube-prometheus-prometheus.monitoring:9090
```

如果你的 kube-prometheus-stack release name 不同，WOMS 的 `keda.gthulhu.prometheusServerAddress` 必須改成實際 service DNS。

確認 Prometheus service：

```bash
kubectl get svc -n monitoring
kubectl get servicemonitor -A | grep -i gthulhu
```

## WOMS observe-only PSM

先建立 observe-only `PodSchedulingMetrics`，不要啟用 `spec.scaling`，避免 Gthulhu 建立第二個 scaler 控制 `woms-woms-worker`：

```yaml
apiVersion: gthulhu.io/v1alpha1
kind: PodSchedulingMetrics
metadata:
  name: woms-scheduler-worker
  namespace: woms
spec:
  enabled: true
  collectionIntervalSeconds: 10
  k8sNamespaces:
    - woms
  labelSelectors:
    - key: app.kubernetes.io/instance
      operator: In
      values:
        - woms
    - key: app.kubernetes.io/component
      operator: In
      values:
        - scheduler-worker
```

套用後確認：

```bash
kubectl get podschedulingmetrics -n woms
kubectl describe podschedulingmetrics woms-scheduler-worker -n woms
```

目前 Gthulhu monitor path 仍應視為 namespace-scoped，worker 篩選必須靠 Prometheus query 的 `pod_name=~"woms-woms-worker-.*"`。

## Prometheus 驗證

先確認 Gthulhu raw metrics 有 WOMS worker pods：

```bash
kubectl get pod -n gthulhu-system
kubectl port-forward -n gthulhu-system svc/gthulhu-scheduler-sidecar 9091:9091
curl -s http://127.0.0.1:9091/metrics | grep 'woms-woms-worker'
```

再到 Prometheus 查：

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_wait_time_nanoseconds_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) / 1000000000
```

在 kube-prometheus-stack 中，Gthulhu raw endpoint 的 `namespace="woms"` 可能會被 Prometheus 保存成 `exported_namespace="woms"`，因為 scrape target 自己已有 `namespace="gthulhu-system"`。WOMS 預設 query 因此使用 `exported_namespace`。

## 啟用 WOMS Gthulhu Trigger

只有在 Prometheus query 能回傳 worker scalar 後，才啟用 WOMS trigger：

```bash
helm upgrade --install woms ./deploy/helm/woms --dependency-update \
  --namespace woms \
  --set keda.gthulhu.enabled=true \
  --set keda.gthulhu.prometheusServerAddress=http://monitoring-kube-prometheus-prometheus.monitoring:9090 \
  --set keda.gthulhu.threshold=20
```

確認同一個 `ScaledObject` 內有 Kafka、CPU、Prometheus 三個 triggers：

```bash
kubectl get scaledobject woms-woms-worker -n woms -o yaml
kubectl describe hpa woms-woms-worker-hpa -n woms
```

不要啟用 Gthulhu chart 的 example `ScaledObject` 來控制 `woms-woms-worker`，也不要在 `PodSchedulingMetrics.spec.scaling` 內建立另一個 scaler。WOMS worker 必須只由 WOMS chart 的 `ScaledObject/woms-woms-worker` 擁有。

## WOMS 驗證

執行 render 與 Kubernetes 檢查：

```bash
./scripts/verify-hpa-render.sh
GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh
NAMESPACE=woms ./scripts/verify-k8s.sh
```

跑 web 的「多產線排程尖峰」demo 後確認：

```bash
kubectl get hpa,deploy,pod -n woms -w
kubectl describe hpa woms-woms-worker-hpa -n woms
kubectl logs deploy/woms-woms-worker -n woms -f
```

若要證明 Gthulhu trigger 真的能主導 HPA，可暫時把 threshold 降到 `1` 做 proof demo：

```bash
helm upgrade --install woms ./deploy/helm/woms --dependency-update \
  --namespace woms \
  --set keda.gthulhu.enabled=true \
  --set keda.gthulhu.threshold=1
```

成功時 HPA events 應出現類似：

```text
New size: 4; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target
New size: 8; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target
```

demo 後把 threshold 恢復保守值，例如 `20`。`threshold=1` 是 proof/demo calibration，不是 production 建議。

## Merge 前必須滿足

- Gthulhu branch 明確記錄基準 commit，不使用模糊的 floating `develop` tag 當唯一依據。
- 最新候選 Gthulhu image 已證明會輸出 `woms-woms-worker-*` 的 `gthulhu_pod_*` metrics。
- Prometheus query 使用實際 label 名稱；如果不是 kube-prometheus-stack 的 `exported_namespace` 行為，必須同步調整 WOMS values。
- WOMS `ScaledObject/woms-woms-worker` 只有一個，且同時包含 Kafka、CPU、Gthulhu Prometheus triggers。
- `./scripts/verify-hpa-render.sh`、`GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh`、`NAMESPACE=woms ./scripts/verify-k8s.sh` 通過。
