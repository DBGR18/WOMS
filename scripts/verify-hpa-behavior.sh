#!/usr/bin/env bash
set -euo pipefail

NAMESPACE="${NAMESPACE:-woms}"
RELEASE="${RELEASE:-woms}"
CHART="${CHART:-./deploy/helm/woms}"
VALUES_FILE="${VALUES_FILE:-./deploy/helm/woms/values-gthulhu-monitor.yaml}"
KUBECTL="${KUBECTL:-kubectl}"
HELM="${HELM:-helm}"
HPA_SCENARIO="${HPA_SCENARIO:-cpu}"
TIMEOUT_SECONDS="${TIMEOUT_SECONDS:-360}"
GTHULHU_IMAGE_TAG="${GTHULHU_IMAGE_TAG:-woms-integration-f71f78a}"
WORKER_DEPLOY="${RELEASE}-woms-worker"
LOAD_LABEL="app=woms-hpa-load,scenario=${HPA_SCENARIO}"

cleanup() {
  "$KUBECTL" delete job,pod -n "$NAMESPACE" -l "$LOAD_LABEL" --ignore-not-found=true >/dev/null 2>&1 || true
}
trap cleanup EXIT

wait_replicas() {
  want="$1"
  op="$2"
  deadline=$((SECONDS + TIMEOUT_SECONDS))
  while [ "$SECONDS" -lt "$deadline" ]; do
    replicas="$("$KUBECTL" get deploy "$WORKER_DEPLOY" -n "$NAMESPACE" -o jsonpath='{.status.readyReplicas}' 2>/dev/null || true)"
    replicas="${replicas:-0}"
    if [ "$op" = "ge" ] && [ "$replicas" -ge "$want" ]; then
      return 0
    fi
    if [ "$op" = "le" ] && [ "$replicas" -le "$want" ]; then
      return 0
    fi
    sleep 10
  done
  echo "Timed out waiting for ${WORKER_DEPLOY} replicas ${op} ${want}" >&2
  "$KUBECTL" get deploy,hpa,scaledobject -n "$NAMESPACE"
  return 1
}

helm_upgrade() {
  "$HELM" upgrade --install "$RELEASE" "$CHART" \
    --namespace "$NAMESPACE" --create-namespace \
    -f "$VALUES_FILE" \
    --set "gthulhu.scheduler.image.tag=${GTHULHU_IMAGE_TAG}" \
    --set "gthulhu.scheduler.sidecar.image.tag=${GTHULHU_IMAGE_TAG}" \
    --set "gthulhu.manager.image.tag=${GTHULHU_IMAGE_TAG}" \
    "$@"
}

run_worker_like_load_pod() {
  name="$1"
  "$KUBECTL" apply -f - <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${name}
  namespace: ${NAMESPACE}
  labels:
    app: woms-hpa-load
    scenario: ${HPA_SCENARIO}
    app.kubernetes.io/component: scheduler-worker
    app.kubernetes.io/instance: ${RELEASE}
spec:
  restartPolicy: Never
  containers:
    - name: load
      image: busybox:1.36
      resources:
        requests:
          cpu: 100m
          memory: 32Mi
        limits:
          cpu: "1"
          memory: 128Mi
      command:
        - sh
        - -c
        - 'i=0; while [ \$i -lt 240 ]; do sha256sum /dev/zero >/dev/null 2>&1 & i=\$((i+1)); sleep 1; done'
EOF
}

case "$HPA_SCENARIO" in
  cpu)
    helm_upgrade \
      --set keda.kafka.enabled=false \
      --set keda.cpu.enabled=true \
      --set keda.cpu.targetUtilization=10 \
      --set keda.gthulhu.enabled=false
    run_worker_like_load_pod "${WORKER_DEPLOY}-cpu-load"
    ;;
  kafka)
    helm_upgrade \
      --set keda.kafka.enabled=true \
      --set keda.kafka.lagThreshold=1 \
      --set keda.cpu.enabled=false \
      --set keda.gthulhu.enabled=false \
      --set worker.env.minJobDurationMs=5000
    "$KUBECTL" create job "woms-hpa-kafka-load" -n "$NAMESPACE" \
      --image=docker.io/bitnamilegacy/kafka:3.7.1-debian-12-r4 -- \
      sh -c 'for i in $(seq 1 80); do echo "{\"orderId\":\"hpa-$i\"}"; done | kafka-console-producer.sh --bootstrap-server kafka.woms.svc.cluster.local:9092 --topic woms.schedule.jobs'
    "$KUBECTL" label job "woms-hpa-kafka-load" -n "$NAMESPACE" app=woms-hpa-load "scenario=${HPA_SCENARIO}" --overwrite
    ;;
  gthulhu)
    helm_upgrade \
      --set keda.kafka.enabled=false \
      --set keda.cpu.enabled=false \
      --set keda.gthulhu.enabled=true \
      --set keda.gthulhu.threshold=1
    run_worker_like_load_pod "${WORKER_DEPLOY}-gthulhu-load"
    ;;
  *)
    echo "HPA_SCENARIO must be cpu, kafka, or gthulhu" >&2
    exit 2
    ;;
esac

"$KUBECTL" get scaledobject "$WORKER_DEPLOY" -n "$NAMESPACE" -o yaml
wait_replicas 2 ge
cleanup

helm_upgrade \
  --set keda.kafka.enabled=true \
  --set keda.cpu.enabled=true \
  --set keda.gthulhu.enabled=true
wait_replicas 1 le

echo "HPA ${HPA_SCENARIO} behavior verification passed"
