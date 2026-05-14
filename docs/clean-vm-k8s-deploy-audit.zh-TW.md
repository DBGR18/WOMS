# WOMS 乾淨 VM Kubernetes 部署稽核

更新時間：2026-05-14 UTC

## 結論

本次已在 `origin/main` / `v0.1.28` 基底上完成 WOMS clean VM Kubernetes 部署驗證。沒有 push。

最終狀態：通過。

- `helm template woms ./deploy/helm/woms --dependency-update --namespace woms`：通過。
- `helm upgrade --install woms ./deploy/helm/woms --dependency-update --namespace woms --create-namespace --timeout 15m`：通過，revision 5 `deployed`。
- `microk8s kubectl get pod,deploy,statefulset,job,pvc,scaledobject,hpa,pdb -n woms`：所有 WOMS workloads Running / ready，PVC Bound，ScaledObject Ready/Active。
- `KUBECTL=microk8s.kubectl HELM=microk8s.helm3 NAMESPACE=woms ./scripts/verify-k8s.sh`：通過。

最後驗證到的不只是 pod Running，也包含：

- worker `KAFKA_BROKERS` render 成 `kafka.woms.svc.cluster.local:9092`。
- Kafka topic `woms.schedule.jobs` 存在。
- Kafka consumer group `woms-scheduler-workers` 存在。
- KEDA external metric 可用。
- ScaledObject `Health` 為 `Happy`，failures 為 `0`。
- HPA 顯示 Kafka lag metric：`10/10 (avg)`。

## 從 0 部署流程

乾淨 VM 部署 WOMS 應分成兩層：先建 Kubernetes platform，再用 Helm 部署 WOMS。

### 1. 安裝並啟用 MicroK8s platform

```bash
sudo snap install microk8s --classic --channel=1.35/stable
sudo usermod -aG microk8s "$USER"
newgrp microk8s
microk8s status --wait-ready
microk8s enable dns hostpath-storage metrics-server
microk8s enable community
microk8s enable keda
microk8s kubectl get node
microk8s kubectl get pods -A
```

平台 pods 必須健康後才能部署 WOMS。若 namespace events 出現 `MissingClusterDNS`，不要繼續部署；先確認 cluster DNS Service IP 與 kubelet args：

```bash
microk8s kubectl -n kube-system get svc kube-dns
grep -E 'cluster-dns|cluster-domain' /var/snap/microk8s/current/args/kubelet
```

kubelet 的 `--cluster-dns` 值必須與 `kube-dns` Service `CLUSTER-IP` 一致；以這份 chart 預設的 service 名稱而言，domain 應為 `cluster.local`。本次驗證的 MicroK8s VM 使用：

```text
--cluster-dns=10.152.183.10
--cluster-domain=cluster.local
```

不要把 `10.152.183.10` 直接照抄到其他叢集；應使用該叢集自己的 CoreDNS Service IP。

### 2. 確認沒有殘留 app namespace

初次乾淨部署前，除了 `default`、`kube-*` 與平台 namespace 外，不應有舊的 app namespace。這次手動清理 stuck namespaces 後曾確認只剩：

```text
default
kube-node-lease
kube-public
kube-system
```

部署完成後，目前 namespace 狀態為：

```text
default           Active
keda              Active
kube-node-lease   Active
kube-public       Active
kube-system       Active
woms              Active
```

### 3. Render、部署與驗證 WOMS

若使用 MicroK8s wrapper，可先在 shell 設定 alias，或在驗證 script 使用 env override：

```bash
helm template woms ./deploy/helm/woms --dependency-update --namespace woms
helm upgrade --install woms ./deploy/helm/woms --dependency-update \
  --namespace woms --create-namespace --timeout 15m
microk8s kubectl get pod,deploy,statefulset,job,pvc,scaledobject,hpa,pdb -n woms
KUBECTL=microk8s.kubectl HELM=microk8s.helm3 NAMESPACE=woms ./scripts/verify-k8s.sh
```

## 先前失敗與修正

### 1. Namespace Terminating / finalizer

初始狀態：`woms`、`free5gc`、`keda` stuck in `Terminating`。

原因：

```text
NamespaceDeletionDiscoveryFailure=True
external.metrics.k8s.io/v1beta1: stale GroupVersion discovery
Some resources are remaining: scaledobjects.keda.sh has 1 resource instances
Some content in the namespace has finalizers remaining: finalizer.keda.sh in 1 resource instances
```

安全修正方式：

```bash
microk8s kubectl patch scaledobject woms-woms-worker -n woms --type=merge -p '{"metadata":{"finalizers":[]}}'
microk8s kubectl delete apiservice v1beta1.external.metrics.k8s.io --ignore-not-found --request-timeout=20s
microk8s kubectl wait --for=delete namespace/woms namespace/free5gc namespace/keda --timeout=120s
```

### 2. ClusterDNS 未設定

症狀：

```text
Warning MissingClusterDNS
kubelet does not have ClusterDNS IP configured and cannot create Pod using "ClusterFirst" policy.
```

pod 內 DNS 曾落到外部 resolver：

```text
lookup postgres on 8.8.8.8:53: no such host
```

這台 MicroK8s VM 修正後的 kubelet args：

```text
--cluster-dns=10.152.183.10
--cluster-domain=cluster.local
```

修正後 WOMS pod `/etc/resolv.conf`：

```text
search woms.svc.cluster.local svc.cluster.local cluster.local
nameserver 10.152.183.10
options ndots:5
```

### 3. Bitnami retained tags pull 失敗

症狀：PostgreSQL、Redis、Kafka 與 Kafka topic hook 進入 `ImagePullBackOff`。

原因：dependency charts 使用的舊 retained tags 已不能從 `docker.io/bitnami/*` 拉取。

修正：

- PostgreSQL：`docker.io/bitnamilegacy/postgresql:16.4.0-debian-12-r14`
- Redis：`docker.io/bitnamilegacy/redis:7.2.5-debian-12-r4`
- Kafka：`docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`
- Kafka topic hook：`docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`

### 4. KEDA / hook / worker bootstrap 未經 `tpl`

症狀：

```text
kafka.{{ .Release.Namespace }}.svc.cluster.local:9092
```

曾出現在 Kafka topic hook log 與 worker log 中，造成 DNS 查詢失敗。

修正：

- `deploy/helm/woms/templates/keda-scaledobject.yaml` 使用 `tpl .Values.keda.kafka.bootstrapServers .`。
- `deploy/helm/woms/templates/kafka-topic-job.yaml` 使用 `tpl .Values.keda.kafka.bootstrapServers .`。
- `deploy/helm/woms/templates/worker-deployment.yaml` 使用 `tpl .Values.keda.kafka.bootstrapServers .`。
- `deploy/helm/woms/values.yaml` 預設為 `kafka.{{ .Release.Namespace }}.svc.cluster.local:9092`。

### 5. Single-node Kafka `__consumer_offsets` replication factor

症狀：worker 已能連到 Kafka，但 consumer group 無法建立。

worker log：

```text
scheduler worker starting brokers=kafka.woms.svc.cluster.local:9092 topic=woms.schedule.jobs group=woms-scheduler-workers
scheduler worker read failed: [15] Group Coordinator Not Available
```

Kafka log：

```text
INVALID_REPLICATION_FACTOR
Unable to replicate the partition 3 time(s): The target replication factor of 3 cannot be reached because only 1 broker(s) are registered.
```

根因：single-node Kafka 仍用 Kafka 預設 `offsets.topic.replication.factor=3`，導致 `__consumer_offsets` 建不起來。

修正：在 `deploy/helm/woms/values.yaml` 設定 Bitnami Kafka controller：

```yaml
kafka:
  controller:
    extraConfigYaml:
      default.replication.factor: 1
      min.insync.replicas: 1
      offsets.topic.replication.factor: 1
      transaction.state.log.min.isr: 1
      transaction.state.log.replication.factor: 1
```

修正後確認：

```text
Topic: __consumer_offsets
PartitionCount: 50
ReplicationFactor: 1
Configs: compression.type=producer,min.insync.replicas=1,cleanup.policy=compact,segment.bytes=104857600
```

## 最終驗證證據

### Helm history

```text
REVISION  STATUS     DESCRIPTION
1         superseded Install complete
2         failed     Upgrade "woms" failed: context canceled
3         superseded Upgrade complete
4         superseded Upgrade complete
5         deployed   Upgrade complete
```

### Platform 與 WOMS pods

```text
keda          keda-admission-...             1/1 Running
keda          keda-metrics-apiserver-...     1/1 Running
keda          keda-operator-...              1/1 Running
kube-system   coredns-...                    1/1 Running
kube-system   hostpath-provisioner-...       1/1 Running
kube-system   metrics-server-...             1/1 Running
woms          kafka-controller-0             1/1 Running
woms          postgres-0                     1/1 Running
woms          redis-master-0                 1/1 Running
woms          woms-woms-api-...              1/1 Running
woms          woms-woms-web-...              1/1 Running
woms          woms-woms-worker-...           1/1 Running
```

### Requested resource check

```text
deployment.apps/woms-woms-api      2/2
deployment.apps/woms-woms-web      2/2
deployment.apps/woms-woms-worker   1/1
statefulset.apps/kafka-controller  1/1
statefulset.apps/postgres          1/1
statefulset.apps/redis-master      1/1
persistentvolumeclaim/data-kafka-controller-0     Bound
persistentvolumeclaim/data-postgres-0             Bound
persistentvolumeclaim/redis-data-redis-master-0   Bound
scaledobject.keda.sh/woms-woms-worker READY=True ACTIVE=True
horizontalpodautoscaler.autoscaling/woms-woms-worker-hpa TARGETS 10/10 (avg), 2%/70%
```

### `verify-k8s.sh`

```text
deployment "woms-woms-api" successfully rolled out
deployment "woms-woms-web" successfully rolled out
deployment "woms-woms-worker" successfully rolled out
Health:
  s0-kafka-woms-schedule-jobs:
    Number Of Failures:  0
    Status:              Happy
Kubernetes static and resource verification passed
```

額外直接確認：

```text
microk8s kubectl exec -n woms kafka-controller-0 -- kafka-consumer-groups.sh --bootstrap-server kafka.woms.svc.cluster.local:9092 --list
woms-scheduler-workers
```

```text
microk8s kubectl get scaledobject woms-woms-worker -n woms -o jsonpath=...
Happy failures=0 ready=True active=True
```

## 更新的檔案

- `deploy/helm/woms/values.yaml`：更新到 v0.1.27 image tags；保留 `bitnamilegacy` dependency images；KEDA Kafka bootstrap 改為 namespace FQDN；新增 single-node Kafka internal topic replication settings。
- `deploy/helm/woms/templates/keda-scaledobject.yaml`：Kafka bootstrap 使用 `tpl` render。
- `deploy/helm/woms/templates/kafka-topic-job.yaml`：Kafka topic hook bootstrap 使用 `tpl` render。
- `deploy/helm/woms/templates/worker-deployment.yaml`：worker `KAFKA_BROKERS` 使用 `tpl` render。
- `deploy/helm/woms/chart-static.test.mjs`：新增 retained image、FQDN bootstrap、hook `tpl`、worker `tpl`、single-node Kafka replication settings 的靜態測試。
- `scripts/verify-k8s.sh`：支援 `KUBECTL` / `HELM` override；加強檢查 Helm status、rollouts、PVC、services、worker broker env、Kafka topic、Kafka consumer group、ScaledObject Ready 與 KEDA external metric。
- `README.md`、`README.en.md`、`README.zh-TW.md`：補充 clean VM MicroK8s 平台流程、ClusterDNS 檢查、部署後驗證、`bitnamilegacy/*` retained tags、single-node Kafka internal topic replication 設定，以及 Kafka hook 排查方式。
- `docs/clean-vm-k8s-deploy-audit.zh-TW.md`、`docs/clean-vm-k8s-deploy-audit.en.md`：記錄本次實測流程、失敗原因、修正方式與最終通過證據。

## README 仍建議保留的提醒

- 不要在 platform pods 未健康、或出現 `MissingClusterDNS` 時繼續部署 WOMS。
- MicroK8s 使用者若未安裝 standalone `kubectl` / `helm`，可用 `KUBECTL=microk8s.kubectl HELM=microk8s.helm3` 執行 verifier。
- 乾淨 VM demo 與 production upgrade 要分開：demo 可刪舊 release/PVC 後重裝；正式升級要保留並帶入既有 secrets。
