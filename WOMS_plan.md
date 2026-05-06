## Plan: Implement Comprehensive Monitoring (Go Services + Gthulhu for Infrastructure)

Deploy a monitoring stack using Prometheus and Grafana for Go backend services, integrate the `gthulhu` library/agent for Kubernetes and external component metrics, and create a `monitor` web page to display the unified dashboard.

**Steps**
1. **Backend Instrumentation (Go)**
   - Add `prometheus/client_golang` to `go.mod`.
   - Instrument `internal/api/server.go` (and `cmd/scheduler-worker/main.go`) to expose a `/metrics` HTTP endpoint.
   - Add custom Prometheus counters in the Go API for `client_access_count` and `orders_submitted_count`.
2. **Infrastructure Monitoring (Gthulhu Integration)** (*parallel with step 1*)
   - Add the `gthulhu` agent to `docker-compose.yml` to collect metrics for Postgres, Redis, and Kafka in the local environment.
   - Update `deploy/helm/woms/Chart.yaml` (or `values.yaml`) to deploy the `gthulhu` daemonset/deployment in Kubernetes to monitor the K8s cluster state and infrastructure components.
3. **Prometheus & Grafana Setup** (*depends on steps 1 & 2*)
   - Add `prometheus` and `grafana` services to `docker-compose.yml`.
   - Configure Prometheus scrape jobs for the Go `/metrics` endpoints AND the `gthulhu` metrics endpoint.
   - Provision Grafana dashboards covering both the Go application metrics and the infrastructure metrics provided by `gthulhu`.
4. **Monitor Web Page Creation** (*depends on step 3*)
   - Create `web/monitor.html` integrating the combined Grafana dashboards utilizing Grafana's iframe embedding capabilities.
   - Update `web/app.js` and `web/index.html` navigation to link to the new `monitor` dashboard page.
   - Configure Grafana for anonymous read-only access to allow seamless iframe embedding.

**Relevant files**
- go.mod — Add prometheus dependencies
- internal/api/server.go — Mount `/metrics` handler
- docker-compose.yml — Declare monitoring stack (Prometheus, Grafana, Gthulhu agent)
- deploy/helm/woms/values.yaml — Configure metrics collection in K8s (including Gthulhu configuration)
- web/monitor.html — New page containing the dashboard iframe
- web/index.html — Navigation updates

**Verification**
1. Start `docker-compose up` and verify `/metrics` on both the API and the `gthulhu` agent return valid data.
2. Manually test API endpoints and observe the custom counters increment in the Go `/metrics` output.
3. Open Grafana UI and verify data populates for both the services and infrastructure (via Gthulhu).
4. Open the new `monitor.html` page to ensure the Grafana iframe loads without authentication errors.

**Decisions**
- Integrated the external `gthulhu` tool/library specifically as the metrics provider for K8s, Postgres, Redis, and Kafka, keeping Go app logic focused on business metrics.
- Prometheus acts as the central scraper for both Go services and Gthulhu.

## Requirements:

1. You must write `3` tests to test the function of this monitor before you generate the main codes.
2. You must only touch the codes in **Relevant files**.
3. You must make the code as decoupling as possible.
4. You must make the code to add new type of metrics easier.