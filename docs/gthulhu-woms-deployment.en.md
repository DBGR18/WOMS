# Gthulhu And WOMS Deployment Alignment Guide

## Current Baseline

As of 2026-05-15, the local Gthulhu refs are:

```text
upstream main:    Gthulhu/Gthulhu origin/main 5e4a72f
upstream develop: Gthulhu/Gthulhu origin/develop 6984d82
WOMS PoC branch:  d11nn/Gthulhu feat/woms-poc f71f78a
```

The WOMS Phase 0-3 validation used `d11nn/feat/woms-poc`.

`feat/woms-poc` is `0 behind / 2 ahead` of upstream `main`. Those two ahead commits are:

```text
00fc41f feat: support per-node runtime scheduler selection (#110)
f71f78a feat: add monitor pod index refresh
```

Compared with latest upstream `develop`, `feat/woms-poc` is `3 behind / 1 ahead`. The only WOMS-required PoC commit that is still on the fork branch and not in upstream `develop` is:

```text
f71f78a feat: add monitor pod index refresh
```

That commit makes the Gthulhu monitor periodically refresh the pod UID index in Kubernetes with `spec.nodeName=<NODE_NAME>`, and it copies `sched_monitor.bpf.o` into the runtime image. Without that fix, the Gthulhu monitor may not reliably map eBPF per-PID metrics back to `woms-woms-worker-*` pods, so the WOMS KEDA Prometheus trigger cannot be assumed to behave like the previously validated environment.

## Conclusion

At this point, `d11nn/feat/woms-poc` is the validated WOMS/Gthulhu PoC baseline. We cannot claim that switching to another Gthulhu branch or a floating image tag is equivalent to the Gthulhu environment that was validated with the WOMS Helm chart.

There are only two acceptable strategies:

1. **Reproduce the validated environment**: use `d11nn/feat/woms-poc` at `f71f78a`, or an equivalent image, and run the Gthulhu metrics, Prometheus scrape, KEDA scaler health, and WOMS HPA event checks in this guide.
2. **Move to latest upstream develop**: reapply the `f71f78a` monitor pod index refresh onto the latest `origin/develop`, rebuild and redeploy the Gthulhu image/chart, then rerun this full validation. Until that passes, do not describe latest upstream `develop` as equivalent to the WOMS PoC environment.

## Version Alignment Check

In the Gthulhu repository, check upstream and fork state first:

```bash
cd /home/ubuntu/Gthulhu
git fetch origin develop
git fetch d11nn develop feat/woms-poc
git rev-list --left-right --count origin/main...d11nn/feat/woms-poc
git rev-list --left-right --count origin/develop...d11nn/feat/woms-poc
git log --oneline --left-right --cherry-pick origin/main...d11nn/feat/woms-poc
git log --oneline --left-right --cherry-pick origin/develop...d11nn/feat/woms-poc
```

You should be able to clearly see that `feat/woms-poc` is the validated PoC branch relative to upstream `main`, and whether it still has a WOMS-required commit that has not landed in upstream `develop`. Only treat upstream `develop` as the new candidate baseline after it contains an equivalent fix.

## Recommended Upgrade Flow

To use latest upstream `develop`, create a refreshed integration branch:

```bash
cd /home/ubuntu/Gthulhu
git fetch origin develop
git switch -c feat/woms-poc-refresh origin/develop
git cherry-pick f71f78a
```

If the cherry-pick conflicts, preserve both requirements:

- the monitor startup path must periodically refresh the Kubernetes pod index so `PodMapper.SetPodIndex()` has production data;
- the runtime image must contain the monitor eBPF object, for example `/gthulhu/sched_monitor.bpf.o`.

Then rebuild the Gthulhu images and point the Helm values at the image tags you just built and pushed. Do not rely on a floating `develop` tag to prove equivalence with this PoC, because that tag changes as upstream moves.

## Gthulhu Deployment Prerequisites

The WOMS PoC keeps this ownership boundary:

- WOMS Helm owns WOMS API, web, worker, PostgreSQL, Redis, Kafka, and the worker `ScaledObject`.
- Gthulhu Helm owns Gthulhu CRDs, the scheduler/monitor DaemonSet, manager, ServiceMonitor, and Gthulhu runtime components.
- Prometheus/Grafana are platform monitoring components. The validated Prometheus service address was:

```text
http://monitoring-kube-prometheus-prometheus.monitoring:9090
```

If your kube-prometheus-stack release name differs, set WOMS `keda.gthulhu.prometheusServerAddress` to the actual service DNS.

Confirm the Prometheus service:

```bash
kubectl get svc -n monitoring
kubectl get servicemonitor -A | grep -i gthulhu
```

## WOMS Observe-only PSM

Create an observe-only `PodSchedulingMetrics` first. Do not enable `spec.scaling`, because that would risk creating a second scaler for `woms-woms-worker`:

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

After applying it, check:

```bash
kubectl get podschedulingmetrics -n woms
kubectl describe podschedulingmetrics woms-scheduler-worker -n woms
```

For now, treat the Gthulhu monitor path as namespace-scoped. Worker filtering must still happen in Prometheus with `pod_name=~"woms-woms-worker-.*"`.

## Prometheus Verification

First confirm the raw Gthulhu metrics include WOMS worker pods:

```bash
kubectl get pod -n gthulhu-system
kubectl port-forward -n gthulhu-system svc/gthulhu-scheduler-sidecar 9091:9091
curl -s http://127.0.0.1:9091/metrics | grep 'woms-woms-worker'
```

Then query Prometheus:

```promql
count(gthulhu_pod_process_count{pod_name!="",namespace!=""})
avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
avg(rate(gthulhu_pod_wait_time_nanoseconds_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m])) / 1000000000
```

With kube-prometheus-stack, the raw Gthulhu endpoint label `namespace="woms"` may be preserved as `exported_namespace="woms"` because the scrape target already has `namespace="gthulhu-system"`. WOMS defaults to `exported_namespace` for that reason.

## Enable The WOMS Gthulhu Trigger

Only enable the WOMS trigger after the Prometheus query returns a worker scalar:

```bash
helm upgrade --install woms ./deploy/helm/woms --dependency-update \
  --namespace woms \
  --set keda.gthulhu.enabled=true \
  --set keda.gthulhu.prometheusServerAddress=http://monitoring-kube-prometheus-prometheus.monitoring:9090 \
  --set keda.gthulhu.threshold=20
```

Confirm the same `ScaledObject` has Kafka, CPU, and Prometheus triggers:

```bash
kubectl get scaledobject woms-woms-worker -n woms -o yaml
kubectl describe hpa woms-woms-worker-hpa -n woms
```

Do not enable the Gthulhu chart example `ScaledObject` for `woms-woms-worker`, and do not create another scaler through `PodSchedulingMetrics.spec.scaling`. The WOMS worker must be owned only by WOMS chart `ScaledObject/woms-woms-worker`.

## WOMS Verification

Run render and Kubernetes checks:

```bash
./scripts/verify-hpa-render.sh
GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh
NAMESPACE=woms ./scripts/verify-k8s.sh
```

After running the web "multi-line scheduling peak" demo, check:

```bash
kubectl get hpa,deploy,pod -n woms -w
kubectl describe hpa woms-woms-worker-hpa -n woms
kubectl logs deploy/woms-woms-worker -n woms -f
```

To prove that the Gthulhu trigger can actually drive HPA, temporarily lower the threshold to `1` for a proof demo:

```bash
helm upgrade --install woms ./deploy/helm/woms --dependency-update \
  --namespace woms \
  --set keda.gthulhu.enabled=true \
  --set keda.gthulhu.threshold=1
```

A successful run should include HPA events like:

```text
New size: 4; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target
New size: 8; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target
```

Restore the threshold to a conservative value such as `20` after the demo. `threshold=1` is proof/demo calibration, not a production recommendation.

## Required Before Merge

- The Gthulhu branch records the exact baseline commit; do not use a vague floating `develop` tag as the only reference.
- The candidate Gthulhu image exposes `gthulhu_pod_*` metrics for `woms-woms-worker-*`.
- The Prometheus query uses the actual label names; if the stack does not use kube-prometheus-style `exported_namespace`, update WOMS values accordingly.
- WOMS has exactly one `ScaledObject/woms-woms-worker`, and it contains Kafka, CPU, and Gthulhu Prometheus triggers together.
- `./scripts/verify-hpa-render.sh`, `GTHULHU_ENABLED=true ./scripts/verify-hpa-render.sh`, and `NAMESPACE=woms ./scripts/verify-k8s.sh` pass.
