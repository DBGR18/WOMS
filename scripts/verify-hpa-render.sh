#!/usr/bin/env sh
set -eu

RELEASE="${RELEASE:-woms}"
CHART="${CHART:-./deploy/helm/woms}"
RENDERED_MANIFEST="${RENDERED_MANIFEST:-}"
GTHULHU_ENABLED="${GTHULHU_ENABLED:-false}"

if [ -n "$RENDERED_MANIFEST" ]; then
  rendered="$RENDERED_MANIFEST"
else
  rendered="$(mktemp)"
  trap 'rm -f "$rendered"' EXIT

  if [ "$GTHULHU_ENABLED" = "true" ]; then
    helm template "$RELEASE" "$CHART" --dependency-update --set keda.gthulhu.enabled=true >"$rendered"
  else
    helm template "$RELEASE" "$CHART" --dependency-update >"$rendered"
  fi
fi

grep -q "kind: ScaledObject" "$rendered"
grep -q "name: ${RELEASE}-woms-worker-hpa" "$rendered"
grep -q "horizontalPodAutoscalerConfig:" "$rendered"
grep -q "scaleTargetRef:" "$rendered"
grep -q "name: ${RELEASE}-woms-worker" "$rendered"
grep -q "minReplicaCount: 1" "$rendered"
grep -q "maxReplicaCount: 10" "$rendered"
grep -q "type: kafka" "$rendered"
grep -q 'topic: "woms.schedule.jobs"' "$rendered"
grep -q 'consumerGroup: "woms-scheduler-workers"' "$rendered"
grep -q 'lagThreshold: "10"' "$rendered"
grep -q "type: cpu" "$rendered"
grep -q "metricType: Utilization" "$rendered"
grep -q 'value: "70"' "$rendered"
if [ "$GTHULHU_ENABLED" = "true" ]; then
  grep -q "type: prometheus" "$rendered"
  grep -q 'serverAddress: "http://monitoring-kube-prometheus-prometheus.monitoring:9090"' "$rendered"
  grep -q 'metricName: "woms_worker_gthulhu_involuntary_ctx_switches_rate"' "$rendered"
  grep -Fq 'gthulhu_pod_involuntary_ctx_switches_total{exported_namespace=\"woms\",pod_name=~\"woms-woms-worker-.*\"}' "$rendered"
  grep -q 'threshold: "20"' "$rendered"
else
  ! grep -q "woms_worker_gthulhu_involuntary_ctx_switches_rate" "$rendered"
fi
grep -q "scaleUp:" "$rendered"
grep -q "stabilizationWindowSeconds: 0" "$rendered"
grep -q "scaleDown:" "$rendered"
grep -q "stabilizationWindowSeconds: 120" "$rendered"
grep -q "kind: PodDisruptionBudget" "$rendered"
grep -q "name: ${RELEASE}-woms-api" "$rendered"
grep -q "name: ${RELEASE}-woms-web" "$rendered"
grep -q "minAvailable: 1" "$rendered"

echo "HPA/KEDA render verification passed"
