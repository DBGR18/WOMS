# WOMS Post-Implementation Verification Guide

## 1. Local Static And Unit Tests

```bash
go test ./...
npm run test:web
test -z "$(gofmt -l .)"
```

Expected:

- All Go tests pass.
- Frontend mock tests pass.
- `gofmt` produces no output.

## 2. API/JWT/RBAC Verification

Start the API:

```bash
JWT_SECRET=local-dev-secret go run ./cmd/api
```

Log in as sales:

```bash
curl -s http://localhost:8080/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"sales","password":"demo"}'
```

Check missing token:

```bash
curl -i http://localhost:8080/internal/auth/verify
```

Expected: `401 Unauthorized`.

Check that sales cannot create schedule jobs:

```bash
curl -i http://localhost:8080/api/schedules/jobs \
  -H "Authorization: Bearer <sales-token>" \
  -H 'Content-Type: application/json' \
  -d '{"lineId":"A","startDate":"2026-05-01"}'
```

Expected: `403 Forbidden`.

Check scheduler line isolation:

- Create a line B job as `scheduler-b`.
- Query that job as `scheduler-a`.
- Expected: `403 Forbidden`.

Check calendar behavior:

- Create a schedule job as a scheduler user.
- `GET /api/schedules/calendar?lineId=A&month=2026-05` returns persisted allocations.
- Querying another scheduler's line returns an error.

## 3. Docker Verification

```bash
docker build -f Dockerfile.api -t woms-api:local .
docker build -f Dockerfile.worker -t woms-scheduler-worker:local .
docker build -f Dockerfile.web -t woms-web:local .
docker compose up --build
```

Expected:

- API health: `curl http://localhost:8080/healthz`
- Web: `http://localhost:8081`

## 4. Helm Render Verification

```bash
helm template woms ./deploy/helm/woms
./scripts/verify-hpa-render.sh
```

Expected output includes:

- `Deployment`: api, worker, web.
- `Ingress`: public, api-secure.
- `ScaledObject`: worker Kafka/CPU triggers.
- `ScaledObject.spec.advanced.horizontalPodAutoscalerConfig.name`: `woms-woms-worker-hpa`.
- `PodDisruptionBudget`: api and web with `minAvailable: 1`.

## 5. Ingress / Gateway Verification

After deployment:

```bash
curl -i https://woms.local/api/orders
curl -i https://woms.local/api/orders -H "Authorization: Bearer <valid-token>"
```

Expected:

- Missing token returns `401`.
- Valid token passes Ingress auth.
- API still performs its own JWT/RBAC checks.
- HTTP redirects to HTTPS.

## 6. KEDA / HPA Verification

Check resources:

```bash
kubectl get scaledobject,hpa -n woms
kubectl describe scaledobject -n woms
```

Log in to the web UI as admin and click the multi-line scheduling peak creation button. Confirm that the panel shows 200 lines, 1,000 orders, 200 queued jobs, and the Kafka topic, consumer group, HPA, and deployment names. Then watch:

```bash
kubectl get deploy -n woms -w
kubectl get hpa -n woms -w
NAMESPACE=woms ./scripts/verify-k8s.sh
```

`verify-k8s.sh` verifies the default no-Ingress render. If the deployment enables Ingress, install with `--set ingress.enabled=true` and run `INGRESS_ENABLED=true NAMESPACE=woms ./scripts/verify-k8s.sh`.

Expected:

- Kafka lag increases.
- Worker replicas rise above `minReplicaCount`.
- Replicas scale down after lag drains and cooldown passes.
- If CPU trigger does not work, verify metrics-server and pod resource requests first.
- After the demo, click the clear peak data button and confirm `L001-L200` orders and jobs are removed.

## 7. API/Web High Availability Verification

```bash
kubectl get deploy,pdb -n woms
kubectl describe pdb woms-woms-api -n woms
kubectl describe pdb woms-woms-web -n woms
```

Expected:

- API and web each run two replicas by default.
- API and web PDBs each require `minAvailable: 1`.
- During voluntary disruption on a multi-node cluster, at least one API and one web pod remain available.

## 8. Gthulhu HPA Demo Verification

Prerequisites:

- MicroK8s has `registry`, `rbac`, KEDA, and metrics-server enabled.
- Gthulhu monitor-only is deployed and scraped by Prometheus.
- WOMS `ScaledObject/woms-woms-worker` has the third Gthulhu Prometheus trigger enabled.
- The Prometheus query uses `exported_namespace="woms"`, not `namespace="woms"`.

Confirm trigger wiring:

```bash
kubectl get scaledobject woms-woms-worker -n woms -o yaml
kubectl describe hpa woms-woms-worker-hpa -n woms
```

Expected:

- `ScaledObject` has three triggers: Kafka, CPU, and Prometheus.
- Kafka is still `lagThreshold: "10"`.
- CPU is still `value: "70"`.
- Gthulhu scaler health is `Happy`.

Proof demo flow:

1. Temporarily set `keda.gthulhu.threshold` to `1` without changing Kafka or CPU triggers.
2. Run the multi-line scheduling peak demo.
3. Watch HPA events:

```bash
kubectl describe hpa woms-woms-worker-hpa -n woms
```

A successful run shows events like:

```text
New size: 4; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target
New size: 8; reason: external metric s2-prometheus-woms_worker_gthulhu_involuntary_ctx_switches_rate above target
```

Prometheus check:

```promql
avg(rate(gthulhu_pod_involuntary_ctx_switches_total{exported_namespace="woms",pod_name=~"woms-woms-worker-.*"}[2m]))
```

Restore the threshold to a conservative value such as `20` after the demo. On the current single-node MicroK8s VM, threshold `5` plus node-level CPU pressure did not reliably prove realistic Gthulhu-only scale-out. After restarting the Gthulhu monitor, worker-pod ephemeral pressure can push the Gthulhu metric above `5`, but it also triggers the CPU scaler, so the HPA reason can still be CPU. For product demos, describe `threshold=1` as proof/demo calibration, not a production recommendation.

## 9. Redis Lock Verification

Submit two concurrent schedule jobs for the same line:

- Expected: no overlapping schedule version is created.
- One job should wait, retry, or fail cleanly.

Submit jobs for different lines:

- Expected: processing can run in parallel.

## 10. Feature Completion Standard

- Tests pass.
- README zh-TW/en is updated.
- `.gitignore` covers generated/local files.
- Docker/Helm/CI settings are synced.
- `git add`, commit, and push are completed.

## 11. Frontend Smoke Verification

- Login at `http://127.0.0.1:8081`.
- Refresh the browser and confirm the session is restored.
- Confirm the login form is hidden after login and the header shows the current username plus logout.
- Login as `admin` / `demo`, confirm the Admin panel is visible, and confirm non-admin users do not see it.
- Toggle exact filters for customer, line, and priority; confirm status behaves as a single-select filter and the customer menu only lists customers matching the active status/priority scope.
- As a scheduler, confirm a pending order cannot be dropped on today or past calendar dates, then drag it onto a chosen future calendar date, accept the preview, and confirm the persisted calendar allocation stays on the dropped date.
- As a scheduler, create a conflict, select conflicted orders plus movable low-priority scheduled orders in the conflict panel, preview the earliest-completion solution, accept it, and confirm the moved orders' old open allocations are replaced.
- As a scheduler, click a scheduled calendar order to start production, then click the in-progress calendar allocation for the intended production date to open production reporting.
- Submit a partial production quantity and confirm the calendar keeps that date's completed quantity while the same order ID returns to pending scheduling with the remaining quantity.
- Create a sales draft order preview with a future due date, confirm the preview page highlights calendar results, then confirm it into pending orders. Also confirm today and past due dates are blocked with `無法被接受的交期`.
- As a scheduler, select pending orders, preview first, then confirm execution from the preview page. A direct schedule job API call without `previewId` must fail.
- Delete selected pending/scheduled orders and confirm removed scheduled allocations disappear from the calendar.
- Use the conflict demo button, preview the generated same-day orders, and confirm the conflict panel fills the right side of the preview dialog without clipping the solution controls.
- Confirm permission failures and operation mistakes appear in popup dialogs.
