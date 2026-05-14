# WOMS Clean VM Kubernetes Deployment Audit

Updated at: 2026-05-14 UTC

## Conclusion

The WOMS clean-VM Kubernetes deployment was verified on the `origin/main` / `v0.1.27` base. No push was performed.

Final status: passed.

- `helm template woms ./deploy/helm/woms --dependency-update --namespace woms`: passed.
- `helm upgrade --install woms ./deploy/helm/woms --dependency-update --namespace woms --create-namespace --timeout 15m`: passed, revision 5 is `deployed`.
- `microk8s kubectl get pod,deploy,statefulset,job,pvc,scaledobject,hpa,pdb -n woms`: all WOMS workloads are Running / ready, PVCs are Bound, and the ScaledObject is Ready/Active.
- `KUBECTL=microk8s.kubectl HELM=microk8s.helm3 NAMESPACE=woms ./scripts/verify-k8s.sh`: passed.

The final verifier checks more than pod readiness:

- worker `KAFKA_BROKERS` renders to `kafka.woms.svc.cluster.local:9092`.
- Kafka topic `woms.schedule.jobs` exists.
- Kafka consumer group `woms-scheduler-workers` exists.
- The KEDA external metric is readable.
- ScaledObject `Health` is `Happy` with `0` failures.
- HPA shows the Kafka lag metric: `10/10 (avg)`.

## Deployment From Zero

A clean VM deployment should have two layers: Kubernetes platform setup first, then the WOMS Helm deployment.

### 1. Install And Enable The MicroK8s Platform

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

Do not deploy WOMS until platform pods are healthy. If namespace events show `MissingClusterDNS`, confirm kubelet args first:

```bash
grep -E 'cluster-dns|cluster-domain' /var/snap/microk8s/current/args/kubelet
```

They must include:

```text
--cluster-dns=10.152.183.10
--cluster-domain=cluster.local
```

### 2. Confirm There Are No Stale App Namespaces

Before the first clean install, there should be no old app namespace beyond `default`, `kube-*`, and platform namespaces. After clearing the stuck namespaces in this run, the cluster briefly had only:

```text
default
kube-node-lease
kube-public
kube-system
```

After deployment, the current namespace state is:

```text
default           Active
keda              Active
kube-node-lease   Active
kube-public       Active
kube-system       Active
woms              Active
```

### 3. Render, Deploy, And Verify WOMS

If using MicroK8s wrappers, either set shell aliases or pass env overrides to the verifier:

```bash
helm template woms ./deploy/helm/woms --dependency-update --namespace woms
helm upgrade --install woms ./deploy/helm/woms --dependency-update \
  --namespace woms --create-namespace --timeout 15m
microk8s kubectl get pod,deploy,statefulset,job,pvc,scaledobject,hpa,pdb -n woms
KUBECTL=microk8s.kubectl HELM=microk8s.helm3 NAMESPACE=woms ./scripts/verify-k8s.sh
```

## Earlier Failures And Fixes

### 1. Namespace Terminating / Finalizer

Initial state: `woms`, `free5gc`, and `keda` were stuck in `Terminating`.

Cause:

```text
NamespaceDeletionDiscoveryFailure=True
external.metrics.k8s.io/v1beta1: stale GroupVersion discovery
Some resources are remaining: scaledobjects.keda.sh has 1 resource instances
Some content in the namespace has finalizers remaining: finalizer.keda.sh in 1 resource instances
```

Safe fix:

```bash
microk8s kubectl patch scaledobject woms-woms-worker -n woms --type=merge -p '{"metadata":{"finalizers":[]}}'
microk8s kubectl delete apiservice v1beta1.external.metrics.k8s.io --ignore-not-found --request-timeout=20s
microk8s kubectl wait --for=delete namespace/woms namespace/free5gc namespace/keda --timeout=120s
```

### 2. Missing ClusterDNS

Symptom:

```text
Warning MissingClusterDNS
kubelet does not have ClusterDNS IP configured and cannot create Pod using "ClusterFirst" policy.
```

Pod DNS fell back to the external resolver:

```text
lookup postgres on 8.8.8.8:53: no such host
```

Fixed kubelet args:

```text
--cluster-dns=10.152.183.10
--cluster-domain=cluster.local
```

Fixed WOMS pod `/etc/resolv.conf`:

```text
search woms.svc.cluster.local svc.cluster.local cluster.local
nameserver 10.152.183.10
options ndots:5
```

### 3. Bitnami Retained Tags Failed To Pull

Symptom: PostgreSQL, Redis, Kafka, and the Kafka topic hook entered `ImagePullBackOff`.

Cause: the old retained tags used by the dependency charts are no longer served from `docker.io/bitnami/*`.

Fix:

- PostgreSQL: `docker.io/bitnamilegacy/postgresql:16.4.0-debian-12-r14`
- Redis: `docker.io/bitnamilegacy/redis:7.2.5-debian-12-r4`
- Kafka: `docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`
- Kafka topic hook: `docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4`

### 4. KEDA / Hook / Worker Bootstrap Was Not Rendered Through `tpl`

Symptom:

```text
kafka.{{ .Release.Namespace }}.svc.cluster.local:9092
```

This appeared in the Kafka topic hook and worker logs, causing DNS failures.

Fix:

- `deploy/helm/woms/templates/keda-scaledobject.yaml` uses `tpl .Values.keda.kafka.bootstrapServers .`.
- `deploy/helm/woms/templates/kafka-topic-job.yaml` uses `tpl .Values.keda.kafka.bootstrapServers .`.
- `deploy/helm/woms/templates/worker-deployment.yaml` uses `tpl .Values.keda.kafka.bootstrapServers .`.
- `deploy/helm/woms/values.yaml` defaults to `kafka.{{ .Release.Namespace }}.svc.cluster.local:9092`.

### 5. Single-Node Kafka `__consumer_offsets` Replication Factor

Symptom: the worker could reach Kafka, but the consumer group could not be created.

Worker log:

```text
scheduler worker starting brokers=kafka.woms.svc.cluster.local:9092 topic=woms.schedule.jobs group=woms-scheduler-workers
scheduler worker read failed: [15] Group Coordinator Not Available
```

Kafka log:

```text
INVALID_REPLICATION_FACTOR
Unable to replicate the partition 3 time(s): The target replication factor of 3 cannot be reached because only 1 broker(s) are registered.
```

Root cause: single-node Kafka was still using Kafka's default `offsets.topic.replication.factor=3`, so `__consumer_offsets` could not be created.

Fix: configure the Bitnami Kafka controller in `deploy/helm/woms/values.yaml`:

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

Post-fix evidence:

```text
Topic: __consumer_offsets
PartitionCount: 50
ReplicationFactor: 1
Configs: compression.type=producer,min.insync.replicas=1,cleanup.policy=compact,segment.bytes=104857600
```

## Final Verification Evidence

### Helm History

```text
REVISION  STATUS     DESCRIPTION
1         superseded Install complete
2         failed     Upgrade "woms" failed: context canceled
3         superseded Upgrade complete
4         superseded Upgrade complete
5         deployed   Upgrade complete
```

### Platform And WOMS Pods

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

### Requested Resource Check

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

Additional direct checks:

```text
microk8s kubectl exec -n woms kafka-controller-0 -- kafka-consumer-groups.sh --bootstrap-server kafka.woms.svc.cluster.local:9092 --list
woms-scheduler-workers
```

```text
microk8s kubectl get scaledobject woms-woms-worker -n woms -o jsonpath=...
Happy failures=0 ready=True active=True
```

## Updated Files

- `deploy/helm/woms/values.yaml`: updated to v0.1.27 app image tags; keeps `bitnamilegacy` dependency images; changes KEDA Kafka bootstrap to a namespace FQDN; adds single-node Kafka internal topic replication settings.
- `deploy/helm/woms/templates/keda-scaledobject.yaml`: renders Kafka bootstrap with `tpl`.
- `deploy/helm/woms/templates/kafka-topic-job.yaml`: renders Kafka topic hook bootstrap with `tpl`.
- `deploy/helm/woms/templates/worker-deployment.yaml`: renders worker `KAFKA_BROKERS` with `tpl`.
- `deploy/helm/woms/chart-static.test.mjs`: adds static coverage for retained images, FQDN bootstrap, hook `tpl`, worker `tpl`, and single-node Kafka replication settings.
- `scripts/verify-k8s.sh`: supports `KUBECTL` / `HELM` overrides and checks Helm status, rollouts, PVCs, services, worker broker env, Kafka topic, Kafka consumer group, ScaledObject Ready, and KEDA external metric.
- `README.md`, `README.en.md`, `README.zh-TW.md`: document clean-VM MicroK8s setup, ClusterDNS checks, post-deploy verification, `bitnamilegacy/*` retained tags, single-node Kafka internal topic replication, and Kafka hook troubleshooting.
- `docs/clean-vm-k8s-deploy-audit.zh-TW.md`, `docs/clean-vm-k8s-deploy-audit.en.md`: record the run, failures, fixes, and final passing evidence.

## README Notes To Keep

- Do not deploy WOMS while platform pods are unhealthy or `MissingClusterDNS` appears.
- MicroK8s users without standalone `kubectl` / `helm` can run the verifier with `KUBECTL=microk8s.kubectl HELM=microk8s.helm3`.
- Clean VM demos and production upgrades are different: demos may intentionally remove old releases/PVCs before reinstalling, while real upgrades must keep and pass existing secrets.
