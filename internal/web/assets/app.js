const state = {
  processes: [],
  events: [],
  selectedId: null,
  selectedProcess: null,
  metrics: [],
  filter: "all",
  query: "",
  readOnly: false,
  logStream: "out",
  logOffsets: { out: 0, err: 0 },
  logLines: { out: [], err: [] },
  logLive: true,
  logQuery: "",
};

const rows = document.querySelector("#process-rows");
const empty = document.querySelector("#empty");
const errorBox = document.querySelector("#error");
const updated = document.querySelector("#last-updated");
const search = document.querySelector("#search");
const inspectorEmpty = document.querySelector("#inspector-empty");
const inspectorContent = document.querySelector("#inspector-content");
const logViewer = document.querySelector("#log-viewer");
const autoRefresh = document.querySelector("#auto-refresh");
const logLive = document.querySelector("#log-live");
const logAutoscroll = document.querySelector("#log-autoscroll");
const logSearch = document.querySelector("#log-search");

document.querySelector("#refresh").addEventListener("click", refreshAll);
document.querySelector("#events-refresh").addEventListener("click", loadEvents);
document.querySelector("#copy-config").addEventListener("click", copySelectedConfig);
search.addEventListener("input", (event) => {
  state.query = event.target.value.trim().toLowerCase();
  renderProcessList();
});
logLive.addEventListener("change", () => {
  state.logLive = logLive.checked;
});
logSearch.addEventListener("input", (event) => {
  state.logQuery = event.target.value.trim().toLowerCase();
  renderLogs();
});

document.querySelectorAll(".segment").forEach((button) => {
  button.addEventListener("click", () => {
    state.filter = button.dataset.filter;
    document.querySelectorAll(".segment").forEach((item) => item.classList.remove("active"));
    button.classList.add("active");
    renderProcessList();
  });
});

document.querySelectorAll("[data-log-stream]").forEach((button) => {
  button.addEventListener("click", () => {
    state.logStream = button.dataset.logStream;
    document.querySelectorAll("[data-log-stream]").forEach((item) => item.classList.remove("active"));
    button.classList.add("active");
    loadLogs(true);
  });
});

document.querySelectorAll("[data-action]").forEach((button) => {
  button.addEventListener("click", () => runAction(button.dataset.action));
});

async function refreshAll() {
  await loadProcesses();
  if (state.selectedId !== null) {
    await loadSelectedDetails();
  }
}

async function loadSession() {
  const response = await fetchJSON("/api/session");
  state.readOnly = Boolean(response.read_only);
  document.querySelectorAll("[data-action]").forEach((button) => {
    button.disabled = state.readOnly;
  });
}

async function loadProcesses() {
  setError("");
  try {
    const data = await fetchJSON("/api/processes");
    state.processes = Array.isArray(data.processes) ? data.processes : [];
    state.events = Array.isArray(data.events) ? data.events : state.events;
    if (state.selectedId === null && state.processes.length > 0) {
      state.selectedId = state.processes[0].id;
    }
    if (state.selectedId !== null && !state.processes.some((process) => process.id === state.selectedId)) {
      state.selectedId = state.processes[0]?.id ?? null;
      resetLogs();
    }
    updated.textContent = `Updated ${new Date().toLocaleTimeString()}`;
    renderSummary();
    renderProcessList();
    renderEvents();
  } catch (error) {
    setError(error.message);
    state.processes = [];
    renderSummary();
    renderProcessList();
  }
}

async function loadSelectedDetails() {
  if (state.selectedId === null) {
    renderInspector();
    return;
  }
  try {
    const data = await fetchJSON(`/api/processes/${state.selectedId}`);
    state.selectedProcess = data.process;
    state.metrics = Array.isArray(data.metrics) ? data.metrics : [];
    renderInspector();
    await loadLogs(false);
  } catch (error) {
    setError(error.message);
  }
}

async function loadEvents() {
  const data = await fetchJSON("/api/events");
  state.events = Array.isArray(data.events) ? data.events : [];
  renderEvents();
}

async function loadLogs(reset) {
  if (state.selectedId === null) return;
  const stream = state.logStream;
  if (reset) {
    state.logOffsets[stream] = 0;
    state.logLines[stream] = [];
  }
  try {
    const params = new URLSearchParams({
      stream,
      offset: String(state.logOffsets[stream] || 0),
      tail: "300",
    });
    const data = await fetchJSON(`/api/processes/${state.selectedId}/logs?${params}`);
    state.logOffsets[stream] = data.offset || 0;
    const incoming = Array.isArray(data.lines) ? data.lines : [];
    if (reset) {
      state.logLines[stream] = incoming;
    } else if (incoming.length > 0) {
      state.logLines[stream] = state.logLines[stream].concat(incoming).slice(-1000);
    }
    renderLogs();
  } catch (error) {
    state.logLines[stream] = [`Failed to read ${stream === "out" ? "stdout" : "stderr"} logs: ${error.message}`];
    renderLogs();
  }
}

async function runAction(action) {
  if (state.selectedId === null || state.readOnly) return;
  if (action === "delete" && !window.confirm("Delete this process from pm2-go?")) {
    return;
  }
  try {
    await fetchJSON(`/api/processes/${state.selectedId}/actions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": csrfToken(),
      },
      body: JSON.stringify({ action }),
    });
    resetLogs();
    await refreshAll();
    await loadEvents();
  } catch (error) {
    setError(error.message);
  }
}

function renderSummary() {
  const counts = state.processes.reduce(
    (acc, process) => {
      acc.total += 1;
      const status = normalizeStatus(process.status);
      if (status === "online") acc.online += 1;
      if (status === "unhealthy") acc.unhealthy += 1;
      if (status === "stopped") acc.stopped += 1;
      return acc;
    },
    { total: 0, online: 0, unhealthy: 0, stopped: 0 },
  );
  document.querySelector("#total-count").textContent = counts.total;
  document.querySelector("#online-count").textContent = counts.online;
  document.querySelector("#unhealthy-count").textContent = counts.unhealthy;
  document.querySelector("#stopped-count").textContent = counts.stopped;
}

function renderProcessList() {
  const filtered = visibleProcesses();
  rows.replaceChildren(...filtered.map(processRow));
  empty.hidden = state.processes.length !== 0 || Boolean(errorBox.textContent);
}

function visibleProcesses() {
  return state.processes.filter((process) => {
    const status = normalizeStatus(process.status);
    const matchesFilter = state.filter === "all" || status === state.filter;
    const haystack = [
      process.name,
      process.executable_path,
      process.cwd,
      ...(process.args || []),
    ]
      .join(" ")
      .toLowerCase();
    return matchesFilter && haystack.includes(state.query);
  });
}

function processRow(process) {
  const row = document.createElement("tr");
  row.tabIndex = 0;
  row.className = process.id === state.selectedId ? "selected" : "";
  row.addEventListener("click", () => selectProcess(process.id));
  row.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") selectProcess(process.id);
  });
  row.append(
    cell(nameBlock(process)),
    cell(statusPill(process.status)),
    textCell(process.pid || "—"),
    textCell(process.cpu || "0.0%"),
    textCell(process.memory || "0.0MB"),
    textCell(process.uptime || "0s"),
    textCell(process.restarts ?? 0),
    cell(policyBlock(process)),
  );
  return row;
}

async function selectProcess(id) {
  if (state.selectedId === id) return;
  state.selectedId = id;
  state.selectedProcess = null;
  state.metrics = [];
  resetLogs();
  renderProcessList();
  renderInspector();
  await loadSelectedDetails();
}

function renderInspector() {
  if (state.selectedId === null || !state.selectedProcess) {
    inspectorEmpty.hidden = state.selectedId !== null;
    inspectorContent.hidden = true;
    clearCharts();
    return;
  }
  const process = state.selectedProcess;
  inspectorEmpty.hidden = true;
  inspectorContent.hidden = false;
  document.querySelector("#detail-name").textContent = process.name || `process-${process.id}`;
  document.querySelector("#detail-command").textContent = commandText(process);
  document.querySelector("#detail-status").replaceWith(statusPill(process.status));
  const status = document.querySelector(".inspector-header .status-pill");
  status.id = "detail-status";
  document.querySelector("#metric-count").textContent = `${state.metrics.length} sample${state.metrics.length === 1 ? "" : "s"}`;
  renderSettings(process);
  renderCharts();
  renderLogs();
}

function renderSettings(process) {
  const settings = [
    ["ID", process.id],
    ["PID", process.pid || "—"],
    ["Parent PID", process.parent_pid || "—"],
    ["Executable", process.executable_path || "—"],
    ["Arguments", (process.args || []).join(" ") || "—"],
    ["Working directory", process.cwd || "—"],
    ["Auto restart", yesNo(process.auto_restart)],
    ["Cron restart", process.cron_restart || "—"],
    ["Max restarts", process.max_restarts || "—"],
    ["Min uptime", ms(process.min_uptime_ms)],
    ["Restart delay", ms(process.restart_delay_ms)],
    ["Backoff delay", ms(process.exp_backoff_restart_delay_ms)],
    ["Max memory restart", bytes(process.max_memory_restart)],
    ["Health URL", process.health_check_url || "—"],
    ["Health interval", ms(process.health_check_interval_ms)],
    ["Health timeout", ms(process.health_check_timeout_ms)],
    ["Watch", yesNo(process.watch)],
    ["Watch paths", (process.watch_paths || []).join(", ") || "—"],
    ["Watch interval", ms(process.watch_interval_ms)],
    ["Env keys", formatEnvKeys(process.env_keys || [])],
    ["Stdout log", process.log_file_path || "—"],
    ["Stderr log", process.err_file_path || "—"],
  ];
  const list = document.querySelector("#settings-list");
  list.replaceChildren(
    ...settings.flatMap(([key, value]) => {
      const dt = document.createElement("dt");
      dt.textContent = key;
      const dd = document.createElement("dd");
      dd.textContent = String(value);
      return [dt, dd];
    }),
  );
}

function renderCharts() {
  drawChart(document.querySelector("#cpu-chart"), state.metrics, "cpu", "#0f766e", "%");
  drawChart(document.querySelector("#memory-chart"), state.metrics, "memory_mb", "#5b6f1c", "MB");
}

function clearCharts() {
  drawChart(document.querySelector("#cpu-chart"), [], "cpu", "#0f766e", "%");
  drawChart(document.querySelector("#memory-chart"), [], "memory_mb", "#5b6f1c", "MB");
}

function drawChart(canvas, points, key, color, suffix) {
  if (!canvas) return;
  const context = canvas.getContext("2d");
  const ratio = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  const width = Math.max(1, rect.width);
  const height = 120;
  canvas.width = Math.floor(width * ratio);
  canvas.height = Math.floor(height * ratio);
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);
  context.fillStyle = "#fbfcf8";
  context.fillRect(0, 0, width, height);
  context.strokeStyle = "#d9ddd1";
  context.lineWidth = 1;
  for (let i = 1; i < 4; i += 1) {
    const y = (height / 4) * i;
    context.beginPath();
    context.moveTo(0, y);
    context.lineTo(width, y);
    context.stroke();
  }
  const values = points.map((point) => Number(point[key] || 0));
  const max = Math.max(1, ...values);
  context.fillStyle = "#626b60";
  context.font = "11px ui-sans-serif";
  context.fillText(`${max.toFixed(key === "cpu" ? 0 : 1)}${suffix}`, 8, 16);
  if (points.length === 0) {
    context.fillText("No samples yet", 8, height - 12);
    return;
  }
  context.strokeStyle = color;
  context.lineWidth = 2;
  context.beginPath();
  values.forEach((value, index) => {
    const x = points.length === 1 ? width - 8 : 8 + (index / (points.length - 1)) * (width - 16);
    const y = height - 14 - (value / max) * (height - 28);
    if (index === 0) context.moveTo(x, y);
    else context.lineTo(x, y);
  });
  context.stroke();
}

function renderLogs() {
  const lines = state.logLines[state.logStream] || [];
  const filtered = state.logQuery
    ? lines.filter((line) => line.toLowerCase().includes(state.logQuery))
    : lines;
  logViewer.textContent = filtered.length ? filtered.join("\n") : "No log lines.";
  if (logAutoscroll.checked) {
    logViewer.scrollTop = logViewer.scrollHeight;
  }
}

function renderEvents() {
  const list = document.querySelector("#event-list");
  const events = state.events.slice(0, 25);
  if (events.length === 0) {
    const item = document.createElement("li");
    item.textContent = "No web events yet.";
    list.replaceChildren(item);
    return;
  }
  list.replaceChildren(
    ...events.map((event) => {
      const item = document.createElement("li");
      const title = document.createElement("strong");
      title.textContent = [event.process_name, event.type].filter(Boolean).join(" · ") || event.type;
      const body = document.createElement("span");
      body.textContent = `${formatTime(event.timestamp)} — ${event.message}`;
      item.append(title, body);
      return item;
    }),
  );
}

function nameBlock(process) {
  const wrap = document.createElement("div");
  wrap.className = "process-name";
  const name = document.createElement("strong");
  name.textContent = process.name || `process-${process.id}`;
  const id = document.createElement("span");
  id.textContent = `ID ${process.id}`;
  wrap.append(name, id);
  return wrap;
}

function statusPill(status) {
  const pill = document.createElement("span");
  const normalized = normalizeStatus(status);
  pill.className = `status-pill status-${normalized}`;
  pill.textContent = normalized;
  return pill;
}

function policyBlock(process) {
  const wrap = document.createElement("div");
  wrap.className = "policy";
  const labels = [];
  if (process.auto_restart) labels.push("autorestart");
  if (process.health_configured) labels.push("health");
  if (process.watch) labels.push("watch");
  if (labels.length === 0) labels.push("manual");
  labels.forEach((label) => {
    const chip = document.createElement("span");
    chip.className = "chip";
    chip.textContent = label;
    wrap.append(chip);
  });
  return wrap;
}

function cell(content) {
  const td = document.createElement("td");
  td.append(content);
  return td;
}

function textCell(value) {
  const td = document.createElement("td");
  td.textContent = String(value);
  return td;
}

function setError(message) {
  errorBox.textContent = message;
  errorBox.hidden = !message;
}

async function fetchJSON(url, options = {}) {
  const response = await fetch(url, { credentials: "same-origin", ...options });
  if (response.status === 401) {
    window.location.href = "/login";
    throw new Error("authentication required");
  }
  const data = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(data.error || `request failed with ${response.status}`);
  }
  return data;
}

function resetLogs() {
  state.logOffsets = { out: 0, err: 0 };
  state.logLines = { out: [], err: [] };
  renderLogs();
}

function csrfToken() {
  const match = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith("pm2_go_web_csrf="));
  return match ? decodeURIComponent(match.split("=").slice(1).join("=")) : "";
}

function copySelectedConfig() {
  if (!state.selectedProcess || !navigator.clipboard) return;
  navigator.clipboard.writeText(JSON.stringify(state.selectedProcess, null, 2)).catch(() => {});
}

function commandText(process) {
  return [process.executable_path, ...(process.args || [])].filter(Boolean).join(" ");
}

function normalizeStatus(status) {
  return (status || "unknown").toLowerCase();
}

function yesNo(value) {
  return value ? "yes" : "no";
}

function ms(value) {
  return value ? `${value}ms` : "—";
}

function bytes(value) {
  return value ? `${value} bytes` : "—";
}

function formatEnvKeys(keys) {
  if (!keys.length) return "—";
  const visible = keys.slice(0, 16).join(", ");
  const remaining = keys.length - 16;
  return remaining > 0 ? `${visible}, +${remaining} more` : visible;
}

function formatTime(value) {
  if (!value) return "now";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString();
}

async function bootstrap() {
  await loadSession();
  await refreshAll();
  if (state.selectedId !== null) {
    await loadSelectedDetails();
  }
  window.setInterval(() => {
    if (autoRefresh.checked) refreshAll();
  }, 5000);
  window.setInterval(() => {
    if (state.logLive && state.selectedId !== null) loadLogs(false);
  }, 2000);
}

bootstrap().catch((error) => setError(error.message));
