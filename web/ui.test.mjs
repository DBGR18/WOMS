import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import {
  defaultLine,
  conflictExplanation,
  customerFilterValues,
  dateKeyInTimeZone,
  escapeHtml,
  exactFilterOrders,
  groupAllocationsByDate,
  isFutureDateKey,
  lineScopedOrders,
  mergePreviewCalendarAllocations,
  matchesOrder,
  monthGrid,
  priorityLabel,
  sortOrdersForWorkstation,
  statusClass,
  statusCounts,
  tomorrowDateKey,
  uniqueValues,
  unacceptableDueDateMessage,
  waterlineMetrics,
} from "./ui.js";

test("preview copy uses state-specific titles instead of mixed conflict/allocation wording", () => {
  const html = readFileSync(new URL("./index.html", import.meta.url), "utf8");
  const app = readFileSync(new URL("./app.js", import.meta.url), "utf8");
  assert.equal(html.includes("衝突與分配"), false);
  assert.equal(app.includes("衝突與分配"), false);
  assert.equal(app.includes("衝突處理"), true);
  assert.equal(app.includes("訂單分配預覽"), true);
});

test("front-end visible HPA status labels are zh-TW", () => {
  const app = readFileSync(new URL("./app.js", import.meta.url), "utf8");
  const html = readFileSync(new URL("./index.html", import.meta.url), "utf8");
  assert.equal(app.includes(">Jobs "), false);
  assert.equal(app.includes(">queued "), false);
  assert.equal(app.includes(">running "), false);
  assert.equal(app.includes(">completed "), false);
  assert.equal(app.includes(">failed "), false);
  assert.equal(app.includes(">cancelled "), false);
  assert.equal(app.includes("Kafka topic"), false);
  assert.equal(app.includes("Consumer group"), false);
  assert.equal(app.includes("Deployment 名稱"), false);
  assert.equal(html.includes(">Orders<"), false);
  assert.equal(html.includes(">Status<"), false);
  assert.equal(html.includes(">Sales Follow-up<"), false);
});

test("web nginx proxy preserves API request paths", () => {
  const nginx = readFileSync(new URL("./nginx.conf.template", import.meta.url), "utf8");
  const composeNginx = readFileSync(new URL("./nginx.compose.conf.template", import.meta.url), "utf8");
  const compose = readFileSync(new URL("../docker-compose.yml", import.meta.url), "utf8");
  assert.match(nginx, /location \/api\/ \{/);
  assert.match(nginx, /proxy_pass http:\/\/\$\{API_UPSTREAM\};/);
  assert.doesNotMatch(nginx, /resolver 127\.0\.0\.11/);
  assert.doesNotMatch(nginx, /proxy_pass http:\/\/\$\{API_UPSTREAM\}\/api\//);
  assert.doesNotMatch(nginx, /proxy_pass \$api_upstream;/);
  assert.match(composeNginx, /resolver 127\.0\.0\.11 valid=10s ipv6=off;/);
  assert.match(composeNginx, /set \$api_upstream http:\/\/\$\{API_UPSTREAM\};/);
  assert.match(composeNginx, /proxy_pass \$api_upstream;/);
  assert.doesNotMatch(composeNginx, /proxy_pass \$api_upstream\/api\//);
  assert.match(compose, /nginx\.compose\.conf\.template:\/etc\/nginx\/templates\/default\.conf\.template:ro/);
});

test("order cards support pointer fallback drag scheduling", () => {
  const app = readFileSync(new URL("./app.js", import.meta.url), "utf8");
  const styles = readFileSync(new URL("./styles.css", import.meta.url), "utf8");
  assert.match(app, /attachPointerScheduleDrag\(card, order\.id\)/);
  assert.match(app, /attachMouseScheduleDrag\(card, order\.id\)/);
  assert.equal(app.includes('document.elementFromPoint(clientX, clientY)?.closest?.(".calendar-day")'), true);
  assert.match(app, /await scheduleDroppedOrders\(drag\.orderIds, targetDate\)/);
  assert.match(app, /const orderIds = selectedPendingOrderIds\(\)/);
  assert.match(app, /document\.addEventListener\("mousemove", onMouseScheduleDragMove\)/);
  assert.match(styles, /\.order-card\.selectable\s*\{[\s\S]*touch-action:\s+none;/);
});

test("schedule preview uses API currentDate as the confirmation payload source", () => {
  const app = readFileSync(new URL("./app.js", import.meta.url), "utf8");
  assert.doesNotMatch(app, /currentDate:\s*requestData\.currentDate\s*\?\?\s*todayDateInputValue\(\)/);
  assert.match(app, /currentDate:\s*result\.currentDate\s*\?\?\s*payloadData\.currentDate\s*\?\?\s*todayDateInputValue\(\)/);
});

const order = {
  id: "ORD-1",
  customer: "ACME Silicon",
  lineId: "A",
  status: "待排程",
  priority: "high",
};

test("matchesOrder filters by id, customer, line, status, and priority", () => {
  assert.equal(matchesOrder(order, "acme"), true);
  assert.equal(matchesOrder(order, "ORD-1".toLowerCase()), true);
  assert.equal(matchesOrder(order, "待排程"), true);
  assert.equal(matchesOrder(order, "missing"), false);
});

test("statusClass maps WOMS statuses to stable CSS classes", () => {
  assert.equal(statusClass("待排程"), "status-pending");
  assert.equal(statusClass("已排程"), "status-scheduled");
  assert.equal(statusClass("生產中"), "status-running");
  assert.equal(statusClass("已完成"), "status-completed");
  assert.equal(statusClass("需業務處理"), "status-rejected");
  assert.equal(statusClass("unknown"), "");
});

test("exactFilterOrders applies OR within fields and AND across fields", () => {
  const orders = [
    { id: "ORD-1", customer: "ACME", lineId: "A", status: "待排程", priority: "high" },
    { id: "ORD-2", customer: "ACME", lineId: "B", status: "已排程", priority: "low" },
    { id: "ORD-3", customer: "Orion", lineId: "A", status: "待排程", priority: "low" },
  ];
  const result = exactFilterOrders(orders, {
    customers: new Set(["ACME"]),
    lines: new Set(["A", "B"]),
    status: "待排程",
    priorities: new Set(),
  });
  assert.deepEqual(result.map((item) => item.id), ["ORD-1"]);
});

test("exactFilterOrders treats status as single-select", () => {
  const orders = [
    { id: "ORD-1", customer: "ACME", lineId: "A", status: "待排程", priority: "high" },
    { id: "ORD-2", customer: "ACME", lineId: "A", status: "已排程", priority: "low" },
  ];
  const result = exactFilterOrders(orders, {
    customers: new Set(),
    lines: new Set(),
    status: "已排程",
    priorities: new Set(),
  });
  assert.deepEqual(result.map((item) => item.id), ["ORD-2"]);
});

test("customerFilterValues follows the active exact filters except customer", () => {
  const orders = [
    { id: "ORD-1", customer: "TSMC Demo", status: "pending", priority: "high" },
    { id: "ORD-2", customer: "ACME", status: "scheduled", priority: "low" },
    { id: "ORD-3", customer: "ACME Silicon", status: "pending", priority: "low" },
  ];
  assert.deepEqual(customerFilterValues(orders, {
    customers: new Set(),
    status: "pending",
    priorities: new Set(),
  }), ["ACME Silicon", "TSMC Demo"]);
  assert.deepEqual(customerFilterValues(orders, {
    customers: new Set(["ACME"]),
    status: "scheduled",
    priorities: new Set(["low"]),
  }), ["ACME"]);
});

test("sortOrdersForWorkstation sorts by workflow, due date, and natural order number", () => {
  const orders = [
    { id: "ORD-10", status: "待排程", dueDate: "2026-04-30", priority: "low" },
    { id: "ORD-2", status: "已排程", dueDate: "2026-05-04", priority: "low" },
    { id: "ORD-7", status: "待排程", dueDate: "2026-04-30", priority: "low" },
    { id: "ORD-1", status: "已完成", dueDate: "2026-04-29", priority: "high" },
    { id: "ORD-6", status: "待排程", dueDate: "2026-04-30", priority: "low" },
  ];
  assert.deepEqual(sortOrdersForWorkstation(orders).map((item) => item.id), ["ORD-6", "ORD-7", "ORD-10", "ORD-2", "ORD-1"]);
});

test("uniqueValues and statusCounts provide sidebar/filter data", () => {
  const orders = [
    { customer: "ACME", status: "待排程" },
    { customer: "ACME", status: "已完成" },
    { customer: "Orion", status: "待排程" },
  ];
  assert.deepEqual(uniqueValues(orders, "customer"), ["ACME", "Orion"]);
  assert.deepEqual(statusCounts(orders), {
    "待排程": 2,
    "已排程": 0,
    "生產中": 0,
    "已完成": 1,
    "需業務處理": 0,
  });
});

test("defaultLine chooses the lexicographically lowest production line", () => {
  assert.equal(defaultLine(["C", "A", "B"]), "A");
});

test("lineScopedOrders limits status counts and tables to the selected line", () => {
  const orders = [
    { id: "ORD-1", lineId: "A", status: "待排程" },
    { id: "ORD-2", lineId: "B", status: "已排程" },
    { id: "ORD-3", lineId: "A", status: "已完成" },
  ];
  const scoped = lineScopedOrders(orders, "A");
  assert.deepEqual(scoped.map((item) => item.id), ["ORD-1", "ORD-3"]);
  assert.deepEqual(statusCounts(scoped), {
    "待排程": 1,
    "已排程": 0,
    "生產中": 0,
    "已完成": 1,
    "需業務處理": 0,
  });
});

test("monthGrid builds a stable six-week calendar grid", () => {
  const days = monthGrid(2026, 4);
  assert.equal(days.length, 42);
  assert.equal(days[0].key, "2026-04-26");
  assert.equal(days.some((day) => day.key === "2026-05-01" && day.inMonth), true);
});

test("groupAllocationsByDate groups calendar allocations by ISO date", () => {
  const groups = groupAllocationsByDate([
    { orderId: "ORD-1", date: "2026-05-02T00:00:00Z" },
    { orderId: "ORD-2", date: "2026-05-02T00:00:00Z" },
    { orderId: "ORD-3", date: "2026-05-03T00:00:00Z" },
  ]);
  assert.deepEqual(groups["2026-05-02"].map((item) => item.orderId), ["ORD-1", "ORD-2"]);
  assert.deepEqual(groups["2026-05-03"].map((item) => item.orderId), ["ORD-3"]);
});

test("mergePreviewCalendarAllocations replaces touched orders with preview entries", () => {
  const calendar = [
    { orderId: "ORD-1", date: "2026-05-15", quantity: 2500, status: "已排程" },
    { orderId: "ORD-2", date: "2026-05-15", quantity: 2500, status: "已排程" },
  ];
  const preview = [
    { orderId: "ORD-1", date: "2026-05-16", quantity: 2500 },
    { orderId: "ORD-3", date: "2026-05-15", quantity: 2500 },
  ];
  const merged = mergePreviewCalendarAllocations(preview, calendar, ["ORD-2"]);

  assert.equal(merged.some((item) => item.orderId === "ORD-1" && item.date === "2026-05-15"), false);
  assert.equal(merged.some((item) => item.orderId === "ORD-2"), false);
  assert.equal(merged.filter((item) => item.preview).length, 2);
});

test("sales due date helpers allow only tomorrow or later", () => {
  assert.equal(isFutureDateKey("2026-04-29", "2026-04-30"), false);
  assert.equal(isFutureDateKey("2026-04-30", "2026-04-30"), false);
  assert.equal(isFutureDateKey("2026-05-01", "2026-04-30"), true);
  assert.equal(tomorrowDateKey("2026-04-30"), "2026-05-01");
  assert.equal(unacceptableDueDateMessage, "無法被接受的交期");
});

test("dateKeyInTimeZone returns the plant-local calendar date", () => {
  const now = new Date("2026-05-04T16:30:00Z");
  assert.equal(dateKeyInTimeZone(now, "Asia/Taipei"), "2026-05-05");
  assert.equal(dateKeyInTimeZone(now, "America/New_York"), "2026-05-04");
});

test("waterlineMetrics summarizes daily capacity usage", () => {
  const metrics = waterlineMetrics([
    { quantity: 1800 },
    { quantity: 700 },
  ]);
  assert.equal(metrics.total, 2500);
  assert.equal(metrics.capacity, 10000);
  assert.equal(metrics.remaining, 7500);
  assert.equal(metrics.overloaded, false);
  assert.equal(metrics.remainingPercent, 75);
  assert.equal(metrics.percent, 25);
  assert.equal(metrics.tone, "safe");
  assert.match(metrics.color, /^hsl\(\d+ 88% 48%\)$/);

  const full = waterlineMetrics([{ quantity: 12000 }]);
  assert.equal(full.total, 12000);
  assert.equal(full.remaining, 0);
  assert.equal(full.overloaded, true);
  assert.equal(full.remainingPercent, 0);
  assert.equal(full.percent, 100);
  assert.equal(full.tone, "danger");
  assert.equal(full.color, "hsl(0 88% 48%)");

  const warning = waterlineMetrics([{ quantity: 8000 }]);
  assert.equal(warning.tone, "warning");
});

test("conflictExplanation gives actionable guidance", () => {
  assert.match(conflictExplanation({ reason: "capacity cannot satisfy order before due date" }), /提前開始/);
  assert.match(conflictExplanation({ reason: "existing allocations require manual review or reschedule" }), /人工強制介入/);
  assert.match(conflictExplanation({ reason: "unknown" }), /檢查產能/);
});

test("priorityLabel returns zh-TW display labels", () => {
  assert.equal(priorityLabel("high"), "高");
  assert.equal(priorityLabel("low"), "低");
});

test("escapeHtml prevents HTML injection in table rendering", () => {
  assert.equal(escapeHtml(`<script>"x"&'</script>`), "&lt;script&gt;&quot;x&quot;&amp;&#039;&lt;/script&gt;");
});
