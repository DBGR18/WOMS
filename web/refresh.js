(function () {
    var GRAFANA_BASE = "http://localhost:3000";
    var DASHBOARD_PATH = "/d/woms-monitoring/woms-monitoring?orgId=1&kiosk";
    var REFRESH_INTERVAL_MS = 30000;  // reload iframe content every 30 s
    var HEALTH_CHECK_MS = 15000;      // check Grafana health every 15 s

    var iframe = document.getElementById("grafana-frame");
    var fallback = document.getElementById("grafana-fallback");
    var grafanaReachable = false;
    var refreshTimer = null;

    // --- UI helpers ---------------------------------------------------
    function showIframe() {
        iframe.hidden = false;
        fallback.hidden = true;
        grafanaReachable = true;
        startAutoRefresh();
        updateLiveBadge(true);
    }

    function showFallback() {
        iframe.hidden = true;
        fallback.hidden = false;
        grafanaReachable = false;
        stopAutoRefresh();
        updateLiveBadge(false);
    }

    // --- Live badge ---------------------------------------------------
    function updateLiveBadge(isLive) {
        var badge = document.getElementById("live-badge");
        if (!badge) return;
        if (isLive) {
            badge.textContent = "● LIVE — 每 30 秒自動更新";
            badge.classList.add("is-live");
        } else {
            badge.textContent = "● OFFLINE";
            badge.classList.remove("is-live");
        }
    }

    // --- Auto-refresh: reload iframe src periodically -----------------
    function refreshIframe() {
        // Append timestamp to bust cache and force Grafana to serve fresh data
        iframe.src = GRAFANA_BASE + DASHBOARD_PATH + "&_t=" + Date.now();
    }

    function startAutoRefresh() {
        if (refreshTimer) return;
        refreshTimer = setInterval(refreshIframe, REFRESH_INTERVAL_MS);
    }

    function stopAutoRefresh() {
        if (refreshTimer) {
            clearInterval(refreshTimer);
            refreshTimer = null;
        }
    }

    // --- Health check: detect Grafana going up / down -----------------
    function checkGrafanaHealth() {
        fetch(GRAFANA_BASE + "/api/health", { mode: "no-cors" })
            .then(function () {
                // no-cors always resolves — treat as reachable
                if (!grafanaReachable) {
                    // Grafana came back — load iframe
                    refreshIframe();
                    showIframe();
                }
            })
            .catch(function () {
                if (grafanaReachable) {
                    showFallback();
                }
            });
    }

    // --- Initial load -------------------------------------------------
    fetch(GRAFANA_BASE + "/api/health", { mode: "no-cors" })
        .then(function () {
            showIframe();
            iframe.addEventListener("error", showFallback);
        })
        .catch(function () {
            showFallback();
        });

    // Fallback: if iframe fails to load after 8 s, show fallback.
    setTimeout(function () {
        try {
            if (iframe.hidden) return;
        } catch (_) {
            // expected cross-origin error — Grafana is loaded
        }
    }, 8000);

    // Start periodic health check
    setInterval(checkGrafanaHealth, HEALTH_CHECK_MS);
})();