const state = {
    token: localStorage.getItem("woms.token") ?? "",
    user: JSON.parse(localStorage.getItem("woms.user") ?? "null"),
};

document.addEventListener("DOMContentLoaded", () => {
    if (!window.location.pathname.endsWith("/monitor.html")) {
        return;
    }
    if (!state.user || state.user.role !== "admin") {
        window.location.href = "/";
        return;
    }
});
