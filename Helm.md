# Helm Plan: Web Proxy Access To Grafana

## English

### Goal

Make the Helm-started WOMS web app access Grafana through the same browser path style as Docker Compose:

```text
http(s)://<woms-ingress-host>/grafana
```

Users should not need a separate `kubectl port-forward` just to open Grafana. The browser should enter through the existing WOMS web URL, and the web NGINX container should proxy `/grafana` to the in-cluster Grafana service.

### Current State

- `Dockerfile.web` bakes `web/nginx.conf.template` into the web image.
- `web/nginx.conf.template` already has the required `/grafana` and `/grafana/` proxy locations, plus Grafana Live WebSocket headers.
- Docker Compose works by setting:
  - `GRAFANA_UPSTREAM=grafana:3000` on the web container
  - `GF_SERVER_ROOT_URL=http://localhost:8081/grafana/`
  - `GF_SERVER_SERVE_FROM_SUB_PATH=true`
- Helm currently sets only `API_UPSTREAM` in `templates/web-deployment.yaml`.
- Helm Grafana service is named `{{ include "woms.fullname" . }}-grafana` and listens on service port `3000`.
- Helm Grafana deployment currently does not set `GF_SERVER_ROOT_URL`, `GF_SERVER_DOMAIN`, or `GF_SERVER_SERVE_FROM_SUB_PATH`.
- Helm ingress already routes `/` to the web service, so `/grafana` can be handled by web NGINX without adding a separate public Grafana ingress.

### Design

Use the existing public WOMS ingress host as the single browser entry point:

```text
http(s)://{{ .Values.ingress.host }}/grafana
```

The request path should be:

```text
Browser -> NGINX Ingress -> WOMS web service -> web NGINX /grafana proxy -> WOMS Grafana service
```

Do not expose Grafana with a separate host, NodePort, LoadBalancer, or required port-forward. Keep Grafana as an internal ClusterIP service; only web NGINX reaches it.

### Implementation Plan

1. Add Helm values for Grafana subpath exposure.
   - Extend `deploy/helm/woms/values.yaml`:

     ```yaml
     monitoring:
       grafana:
         externalPath: /grafana
         env:
           anonymousEnabled: "true"
           anonymousOrgRole: Viewer
           allowEmbedding: "true"
           serveFromSubPath: "true"
           rootUrl: ""
     ```

   - `rootUrl` should be optional. When empty, templates derive it from ingress:

     ```text
     {{ ternary "https" "http" .Values.ingress.tls.enabled }}://{{ .Values.ingress.host }}{{ .Values.monitoring.grafana.externalPath }}/
     ```

   - Keep an override because production deployments may use an external DNS name, TLS terminator, or non-default path.

2. Add `GRAFANA_UPSTREAM` to the Helm web deployment.
   - Update `deploy/helm/woms/templates/web-deployment.yaml`:

     ```yaml
     - name: GRAFANA_UPSTREAM
       value: {{ printf "%s-grafana:3000" (include "woms.fullname" .) | quote }}
     ```

   - This matches Docker Compose’s method but uses the Helm service DNS name.
   - Render it only when monitoring and Grafana are enabled, or render a harmless fallback if the template should stay simple. Preferred:

     ```yaml
     {{- if and .Values.monitoring.enabled .Values.monitoring.grafana.enabled }}
     - name: GRAFANA_UPSTREAM
       value: {{ printf "%s-grafana:3000" (include "woms.fullname" .) | quote }}
     {{- end }}
     ```

3. Configure the Helm Grafana deployment for `/grafana`.
   - Update `deploy/helm/woms/templates/grafana-deployment.yaml` env:

     ```yaml
     - name: GF_SERVER_DOMAIN
       value: {{ .Values.ingress.host | quote }}
     - name: GF_SERVER_ROOT_URL
       value: {{ include "woms.grafanaRootUrl" . | quote }}
     - name: GF_SERVER_SERVE_FROM_SUB_PATH
       value: {{ .Values.monitoring.grafana.env.serveFromSubPath | quote }}
     ```

   - Keep:

     ```yaml
     GF_AUTH_ANONYMOUS_ENABLED
     GF_AUTH_ANONYMOUS_ORG_ROLE
     GF_SECURITY_ALLOW_EMBEDDING
     GF_SECURITY_COOKIE_SAMESITE
     GF_SECURITY_COOKIE_SECURE
     ```

   - Consider making cookie secure depend on ingress TLS:

     ```yaml
     GF_SECURITY_COOKIE_SECURE={{ .Values.ingress.tls.enabled | quote }}
     ```

4. Add helper templates for URL construction.
   - Update `deploy/helm/woms/templates/_helpers.tpl` with helpers like:

     ```gotemplate
     {{- define "woms.externalScheme" -}}
     {{- ternary "https" "http" .Values.ingress.tls.enabled -}}
     {{- end -}}

     {{- define "woms.grafanaExternalPath" -}}
     {{- default "/grafana" .Values.monitoring.grafana.externalPath | trimSuffix "/" -}}
     {{- end -}}

     {{- define "woms.grafanaRootUrl" -}}
     {{- if .Values.monitoring.grafana.env.rootUrl -}}
     {{- .Values.monitoring.grafana.env.rootUrl -}}
     {{- else -}}
     {{- printf "%s://%s%s/" (include "woms.externalScheme" .) .Values.ingress.host (include "woms.grafanaExternalPath" .) -}}
     {{- end -}}
     {{- end -}}
     ```

5. Keep Ingress routing through web.
   - No separate Grafana ingress is required.
   - Existing `templates/ingress.yaml` public ingress already has:

     ```yaml
     - path: /
       pathType: Prefix
       backend:
         service:
           name: {{ include "woms.fullname" . }}-web
     ```

   - Because `/grafana` starts with `/`, it will reach the web service and then web NGINX will proxy to Grafana.
   - Confirm the secure API ingress for `/api` still has priority over the public `/` ingress. Grafana API calls must remain `/grafana/api/...`, not `/api/...`; that is why `GF_SERVER_SERVE_FROM_SUB_PATH=true` is required.

6. Update tests first.
   - Extend `deploy/helm/woms/chart-static.test.mjs`:
     - Assert `web-deployment.yaml` includes `GRAFANA_UPSTREAM`.
     - Assert `GRAFANA_UPSTREAM` points to `{{ include "woms.fullname" . }}-grafana:3000`.
     - Assert `grafana-deployment.yaml` includes `GF_SERVER_ROOT_URL`.
     - Assert `grafana-deployment.yaml` includes `GF_SERVER_SERVE_FROM_SUB_PATH`.
     - Assert `_helpers.tpl` contains `woms.grafanaRootUrl`.
     - Assert `ingress.yaml` does not add a separate Grafana ingress path or service backend.
   - Keep the existing web NGINX tests that prevent stripping `/grafana` before proxying.

7. Update documentation.
   - Update `README.md`, `README.en.md`, and `README.zh-TW.md`.
   - Update `docs/verification.en.md` and `docs/verification.zh-TW.md`.
   - Document Helm access as:

     ```text
     http(s)://<ingress.host>/grafana
     ```

   - Explicitly state that no Grafana `port-forward` is needed when ingress is enabled and DNS/hosts resolution points to the NGINX Ingress controller.

8. Verify render output.
   - Run:

     ```bash
     node --test deploy/helm/woms/chart-static.test.mjs
     helm template woms ./deploy/helm/woms --set ingress.enabled=true --set ingress.host=woms.local
     ```

   - Confirm rendered web deployment contains:

     ```yaml
     - name: GRAFANA_UPSTREAM
       value: "woms-woms-grafana:3000"
     ```

   - Confirm rendered Grafana deployment contains:

     ```yaml
     - name: GF_SERVER_ROOT_URL
       value: "http://woms.local/grafana/"
     - name: GF_SERVER_SERVE_FROM_SUB_PATH
       value: "true"
     ```

9. Verify in a cluster without Grafana port-forward.
   - Install or upgrade:

     ```bash
     helm upgrade --install woms ./deploy/helm/woms \
       --namespace woms \
       --create-namespace \
       --set ingress.enabled=true \
       --set ingress.host=woms.local
     ```

   - Point `woms.local` to the NGINX Ingress controller address.
   - Open:

     ```text
     http://woms.local/grafana
     ```

   - Confirm:
     - `/grafana` redirects or normalizes to `/grafana/`.
     - Grafana static assets load successfully.
     - Grafana browser calls use `/grafana/api/...`.
     - The WOMS Go API does not return `找不到 API 路由。` for Grafana requests.
     - Dashboards stay under `/grafana/...`.
     - Grafana Live WebSocket requests are proxied through `/grafana/api/live/`.

### Risk Notes

- If Grafana emits plain `/api/...` requests, those requests will hit WOMS API ingress and fail. The fix is `GF_SERVER_SERVE_FROM_SUB_PATH=true` plus preserving the `/grafana` path in web NGINX.
- If `ingress.tls.enabled=true`, the derived root URL must be `https://.../grafana/`; otherwise browser redirects and secure cookies can behave incorrectly.
- If `monitoring.grafana.enabled=false`, the admin Grafana link should either stay hidden by deployment policy or lead to a documented unavailable route.
- Because the web container has a read-only root filesystem in Helm, keep the existing writable `emptyDir` mounts for `/etc/nginx/conf.d`, `/var/cache/nginx`, `/var/run`, and `/tmp`.

