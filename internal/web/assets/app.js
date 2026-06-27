const state = {
  processes: [],
  events: [],
  selectedId: null,
  selectedProcess: null,
  metrics: [],
  filter: "all",
  query: "",
  envQuery: "",
  readOnly: false,
  logStream: "both",
  logOffsets: { out: 0, err: 0 },
  logLines: { out: [], err: [] },
  logLive: true,
  logQuery: "",
  pendingAction: "",
  toastTimer: null,
};

const processList = document.querySelector("#process-list");
const emptyNotice = document.querySelector("#empty");
const errorBox = document.querySelector("#error");
const updatedTimestamp = document.querySelector("#last-updated");
const searchInput = document.querySelector("#search");
const detailsPanel = document.querySelector("#selected-bar");
const autoRefresh = document.querySelector("#auto-refresh");
const confirmModal = document.querySelector("#confirm-modal");
const logStreamSelect = document.querySelector("#log-stream");
const logLiveCheckbox = document.querySelector("#log-live");
const logAutoscrollCheckbox = document.querySelector("#log-autoscroll");
const logSearchInput = document.querySelector("#log-search");
const logViewerElement = document.querySelector("#log-viewer");
const toastElement = document.querySelector("#toast");
const envSearchInput = document.querySelector("#env-search");

const logsModal = document.querySelector("#logs-modal");
const modalLogViewer = document.querySelector("#modal-log-viewer");
let logsModalOpen = false;

// Event listeners
document.querySelector("#refresh").addEventListener("click", refreshAll);
document.querySelector("#copy-config").addEventListener("click", copySelectedConfig);
document.querySelector("#confirm-cancel").addEventListener("click", closeConfirm);
document.querySelector("#confirm-run").addEventListener("click", () => {
  if (state.pendingAction) performAction(state.pendingAction);
});

document.querySelector("#log-copy").addEventListener("click", () => {
  const content = logViewerElement.textContent;
  if (!content || !navigator.clipboard) return;
  navigator.clipboard.writeText(content).then(
    () => showToast("Logs copied to clipboard."),
    () => showToast("Failed to copy logs.")
  );
});

document.querySelector("#log-maximize").addEventListener("click", () => {
  const process = selectedProcessSummary();
  if (!process) return;
  logsModalOpen = true;
  logsModal.hidden = false;
  document.body.classList.add("modal-open");
  document.querySelector(".modal-title-text").textContent = `Terminal Log Viewer - ${process.name || process.id}`;
  renderLogs();
});

document.querySelector("#close-logs-modal").addEventListener("click", closeLogsModal);

searchInput.addEventListener("input", (event) => {
  state.query = event.target.value.trim().toLowerCase();
  renderProcessList();
});

envSearchInput.addEventListener("input", (event) => {
  state.envQuery = event.target.value.trim().toLowerCase();
  if (state.selectedProcess) {
    renderEnvKeys(state.selectedProcess.env_keys || []);
  }
});

logStreamSelect.addEventListener("change", () => {
  state.logStream = logStreamSelect.value;
  loadLogs(true);
});

logLiveCheckbox.addEventListener("change", () => {
  state.logLive = logLiveCheckbox.checked;
});

logSearchInput.addEventListener("input", (event) => {
  state.logQuery = event.target.value.trim().toLowerCase();
  renderLogs();
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") {
    closeConfirm();
    if (logsModalOpen) closeLogsModal();
  }
});

confirmModal.addEventListener("click", (event) => {
  if (event.target === confirmModal) closeConfirm();
});

document.querySelectorAll("[data-filter-shortcut]").forEach((button) => {
  button.addEventListener("click", () => setFilter(button.dataset.filterShortcut));
});

// Accordion collapse/expand
document.querySelectorAll(".accordion-trigger").forEach((button) => {
  button.addEventListener("click", () => toggleAccordion(button));
});

// Theme switcher initialization
function initTheme() {
  const savedTheme = localStorage.getItem("theme") || "light";
  document.documentElement.setAttribute("data-theme", savedTheme);
  document.querySelector("#theme-toggle").textContent = savedTheme === "dark" ? "☀️ Light" : "🌙 Dark";
}

document.querySelector("#theme-toggle").addEventListener("click", () => {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  const nextTheme = isDark ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", nextTheme);
  localStorage.setItem("theme", nextTheme);
  document.querySelector("#theme-toggle").textContent = nextTheme === "dark" ? "☀️ Light" : "🌙 Dark";
  renderCharts();
});

function closeLogsModal() {
  logsModalOpen = false;
  logsModal.hidden = true;
  document.body.classList.remove("modal-open");
}

async function refreshAll() {
  await loadProcesses();
  if (state.selectedId !== null) {
    await loadSelectedDetails();
  }
}

async function loadSession() {
  const response = await fetchJSON("/api/session");
  state.readOnly = Boolean(response.read_only);
  updateReadOnlyState();
}

async function loadProcesses() {
  setError("");
  try {
    const data = await fetchJSON("/api/processes");
    state.processes = Array.isArray(data.processes) ? data.processes : [];
    state.events = Array.isArray(data.events) ? data.events : state.events;

    // Set first process selected by default if nothing selected
    if (state.selectedId === null && state.processes.length > 0) {
      state.selectedId = state.processes[0].id;
    }
    if (state.selectedId !== null && !state.processes.some((p) => p.id === state.selectedId)) {
      state.selectedId = state.processes[0]?.id ?? null;
      state.selectedProcess = null;
      resetLogs();
    }

    updatedTimestamp.textContent = `Updated ${new Date().toLocaleTimeString()}`;
    renderSummary();
    renderProcessList();
    renderSelectedBar();
    renderEvents();
  } catch (error) {
    setError(error.message);
    state.processes = [];
    renderSummary();
    renderProcessList();
    renderSelectedBar();
  }
}

async function loadSelectedDetails() {
  if (state.selectedId === null) {
    state.selectedProcess = null;
    state.metrics = [];
    renderSelectedBar();
    renderDetails();
    return;
  }
  try {
    const data = await fetchJSON(`/api/processes/${state.selectedId}`);
    state.selectedProcess = data.process;
    state.metrics = Array.isArray(data.metrics) ? data.metrics : [];
    renderSelectedBar();
    renderDetails();
    renderCharts();
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
    state.logOffsets = { out: 0, err: 0 };
    state.logLines = { out: [], err: [] };
    if (state.logLines.both) state.logLines.both = [];
  }

  try {
    if (stream === "both") {
      const [outParams, errParams] = [
        new URLSearchParams({ stream: "out", offset: String(state.logOffsets.out || 0), tail: "150" }),
        new URLSearchParams({ stream: "err", offset: String(state.logOffsets.err || 0), tail: "150" })
      ];
      const [outData, errData] = await Promise.all([
        fetchJSON(`/api/processes/${state.selectedId}/logs?${outParams}`),
        fetchJSON(`/api/processes/${state.selectedId}/logs?${errParams}`)
      ]);

      state.logOffsets.out = outData.offset || 0;
      state.logOffsets.err = errData.offset || 0;

      const incomingOut = (outData.lines || []).map(line => `[stdout] ${line}`);
      const incomingErr = (errData.lines || []).map(line => `[stderr] ${line}`);

      // Merge logs chronologically as far as possible (since we don't have stamps, simple interleave or append is used)
      const combined = incomingOut.concat(incomingErr);

      if (reset) {
        state.logLines.both = combined;
      } else if (combined.length > 0) {
        state.logLines.both = (state.logLines.both || []).concat(combined).slice(-1000);
      }
    } else {
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
    }
    renderLogs();
  } catch (error) {
    const msg = `Failed to read logs: ${error.message}`;
    if (stream === "both") {
      state.logLines.both = [msg];
    } else {
      state.logLines[stream] = [msg];
    }
    renderLogs();
  }
}

function setFilter(filter) {
  state.filter = filter;
  document.querySelectorAll("[data-filter-shortcut]").forEach((button) => {
    button.classList.toggle("active", button.dataset.filterShortcut === filter);
  });
  renderProcessList();
}

function renderSummary() {
  const counts = state.processes.reduce(
    (acc, p) => {
      acc.total += 1;
      const status = normalizeStatus(p.status);
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
  processList.replaceChildren(...filtered.map(processRow));
  emptyNotice.hidden = filtered.length !== 0 || Boolean(errorBox.textContent);
}

function visibleProcesses() {
  return state.processes.filter((p) => {
    const status = normalizeStatus(p.status);
    const matchesFilter = state.filter === "all" || status === state.filter;
    const haystack = [
      p.name,
      p.executable_path,
      p.cwd,
      ...(p.args || []),
    ]
      .join(" ")
      .toLowerCase();
    return matchesFilter && haystack.includes(state.query);
  });
}

function getProcessEmblem(process) {
  const path = (process.executable_path || "").toLowerCase();
  const name = (process.name || "").toLowerCase();
  if (path.includes("python") || name.includes("python") || name.endsWith(".py")) {
    return { text: "Py", className: "proc-emblem-py" };
  }
  if (path.includes("node") || path.includes("npm") || name.includes("node") || name.endsWith(".js")) {
    return { text: "Js", className: "proc-emblem-js" };
  }
  if (path.includes("go") || name.includes("go")) {
    return { text: "Go", className: "proc-emblem-go" };
  }
  return { text: "Sh", className: "proc-emblem-sh" };
}

function processRow(process) {
  const card = document.createElement("div");
  card.tabIndex = 0;
  const isSelected = process.id === state.selectedId;
  card.className = `process-card ${isSelected ? "active" : ""}`;
  card.addEventListener("click", () => selectProcess(process.id));
  card.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      selectProcess(process.id);
    }
  });

  // Left card section: Language emblem, Glowing Status Dot & Info
  const leftBlock = document.createElement("div");
  leftBlock.className = "card-left";

  const emblemData = getProcessEmblem(process);
  const emblem = document.createElement("span");
  emblem.className = `proc-emblem ${emblemData.className}`;
  emblem.textContent = emblemData.text;

  const dot = document.createElement("span");
  const normalized = normalizeStatus(process.status);
  dot.className = `status-dot status-dot-${normalized}`;

  const infoBlock = document.createElement("div");
  infoBlock.className = "card-info";

  const name = document.createElement("strong");
  name.textContent = process.name || `process-${process.id}`;

  const metaLine = document.createElement("span");
  metaLine.className = "card-meta";
  metaLine.textContent = `ID ${process.id} • PID ${process.pid || "-"}`;

  infoBlock.append(name, metaLine);
  leftBlock.append(emblem, dot, infoBlock);

  // Right card section: Live metrics preview values
  const rightBlock = document.createElement("div");
  rightBlock.className = "card-right";

  const cpuVal = document.createElement("span");
  cpuVal.className = "card-metric";
  cpuVal.textContent = process.cpu || "0.0%";

  const memVal = document.createElement("span");
  memVal.className = "card-metric card-mem";
  memVal.textContent = process.memory || "0.0MB";

  rightBlock.append(cpuVal, memVal);
  card.append(leftBlock, rightBlock);

  return card;
}

async function selectProcess(id) {
  if (state.selectedId === id && state.selectedProcess) {
    renderProcessList();
    renderSelectedBar();
    return;
  }
  state.selectedId = id;
  state.selectedProcess = null;
  state.metrics = [];
  state.envQuery = ""; // reset env query on process switch
  envSearchInput.value = "";
  resetLogs();
  renderProcessList();
  renderSelectedBar();
  await loadSelectedDetails();
}

function renderSelectedBar() {
  const process = selectedProcessSummary();
  if (!process) {
    detailsPanel.hidden = true;
    return;
  }

  detailsPanel.hidden = false;
  replaceStatus("#detail-status", process.status);
  document.querySelector("#detail-name").textContent = process.name || `process-${process.id}`;
  document.querySelector("#detail-command").textContent = commandText(process) || process.cwd || "No command";
  document.querySelector("#selected-cpu").textContent = process.cpu || "0.0%";
  document.querySelector("#selected-memory").textContent = process.memory || "0.0MB";
  document.querySelector("#selected-uptime").textContent = process.uptime || "0s";
}

function renderDetails() {
  const process = state.selectedProcess;
  if (!process) {
    clearCharts();
    return;
  }

  document.querySelector("#detail-name").textContent = process.name || `process-${process.id}`;
  document.querySelector("#detail-command").textContent = commandText(process) || process.cwd || "No command";
  replaceStatus("#detail-status", process.status);

  // Set values on action buttons
  document.querySelectorAll(".action-btn").forEach((button) => {
    button.onclick = () => runAction(button.dataset.action);
  });

  renderSettings(process);
  renderEnvKeys(process.env_keys || []);
  renderEvents();
  renderLogs();
}

function renderSettings(process) {
  const settings = [
    ["ID", process.id],
    ["PID", process.pid || "-"],
    ["Parent PID", process.parent_pid || "-"],
    ["Executable", process.executable_path || "-"],
    ["Arguments", (process.args || []).join(" ") || "-"],
    ["Working directory", process.cwd || "-"],
    ["Auto restart", yesNo(process.auto_restart)],
    ["Cron restart", process.cron_restart || "-"],
    ["Max restarts", process.max_restarts || "-"],
    ["Min uptime", ms(process.min_uptime_ms)],
    ["Restart delay", ms(process.restart_delay_ms)],
    ["Backoff delay", ms(process.exp_backoff_restart_delay_ms)],
    ["Max memory restart", bytes(process.max_memory_restart)],
    ["Health URL", process.health_check_url || "-"],
    ["Health interval", ms(process.health_check_interval_ms)],
    ["Health timeout", ms(process.health_check_timeout_ms)],
    ["Watch", yesNo(process.watch)],
    ["Watch paths", (process.watch_paths || []).join(", ") || "-"],
    ["Watch interval", ms(process.watch_interval_ms)],
    ["Stdout log", process.log_file_path || "-"],
    ["Stderr log", process.err_file_path || "-"],
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

function renderEnvKeys(keys) {
  const filtered = keys.filter((key) => key.toLowerCase().includes(state.envQuery));

  document.querySelector("#env-count").textContent = `${filtered.length} key${filtered.length === 1 ? "" : "s"}`;
  const list = document.querySelector("#env-key-list");
  if (filtered.length === 0) {
    const emptyState = document.createElement("span");
    emptyState.className = "muted-line";
    emptyState.textContent = state.envQuery ? "No matching environment keys." : "No environment keys.";
    list.replaceChildren(emptyState);
    return;
  }
  list.replaceChildren(
    ...filtered.map((key) => {
      const chip = document.createElement("span");
      chip.className = "key-chip";
      chip.textContent = key;
      chip.title = key;
      return chip;
    }),
  );
}

function renderCharts() {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  const cpuColor = isDark ? "#06b6d4" : "#2563eb";
  const memColor = isDark ? "#8b5cf6" : "#7c3aed";
  drawChart(document.querySelector("#cpu-chart"), state.metrics, "cpu", cpuColor, "%");
  drawChart(document.querySelector("#memory-chart"), state.metrics, "memory_mb", memColor, "MB");
}

function clearCharts() {
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  const cpuColor = isDark ? "#06b6d4" : "#2563eb";
  const memColor = isDark ? "#8b5cf6" : "#7c3aed";
  drawChart(document.querySelector("#cpu-chart"), [], "cpu", cpuColor, "%");
  drawChart(document.querySelector("#memory-chart"), [], "memory_mb", memColor, "MB");
}

function hexToRgba(hex, alpha) {
  hex = hex.replace("#", "");
  if (hex.length === 3) {
    hex = hex.split("").map((c) => c + c).join("");
  }
  const r = parseInt(hex.substring(0, 2), 16);
  const g = parseInt(hex.substring(2, 4), 16);
  const b = parseInt(hex.substring(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

function drawChart(canvas, points, key, color, suffix) {
  if (!canvas) return;
  const context = canvas.getContext("2d");
  const ratio = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  const width = Math.max(1, rect.width || 400);
  const height = Math.max(120, rect.height || 150);
  canvas.width = Math.floor(width * ratio);
  canvas.height = Math.floor(height * ratio);
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);

  // Soft grid lines based on theme state
  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  context.strokeStyle = isDark ? "rgba(255, 255, 255, 0.05)" : "rgba(15, 23, 42, 0.04)";
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
  context.fillStyle = isDark ? "rgba(255, 255, 255, 0.4)" : "rgba(15, 23, 42, 0.45)";
  context.font = "10px ui-sans-serif, system-ui, sans-serif";
  context.fillText(`${max.toFixed(key === "cpu" ? 0 : 1)}${suffix}`, 10, 18);

  if (points.length === 0) {
    context.fillText("No samples yet", 10, height - 12);
    return;
  }

  // Draw chart lines
  context.strokeStyle = color;
  context.lineWidth = 2.5;
  context.beginPath();
  const coords = [];
  values.forEach((value, index) => {
    const x = points.length === 1 ? width - 10 : 10 + (index / (points.length - 1)) * (width - 20);
    const y = height - 16 - (value / max) * (height - 34);
    coords.push({ x, y });
    if (index === 0) context.moveTo(x, y);
    else context.lineTo(x, y);
  });
  context.stroke();

  // Draw gradient area under the line
  if (coords.length > 0) {
    context.beginPath();
    context.moveTo(coords[0].x, coords[0].y);
    for (let i = 1; i < coords.length; i += 1) {
      context.lineTo(coords[i].x, coords[i].y);
    }
    context.lineTo(coords[coords.length - 1].x, height - 12);
    context.lineTo(coords[0].x, height - 12);
    context.closePath();
    const grad = context.createLinearGradient(0, 0, 0, height);
    grad.addColorStop(0, hexToRgba(color, 0.12));
    grad.addColorStop(1, hexToRgba(color, 0.0));
    context.fillStyle = grad;
    context.fill();

    // Highlight the latest sample point
    const last = coords[coords.length - 1];
    context.beginPath();
    context.arc(last.x, last.y, 4, 0, 2 * Math.PI);
    context.fillStyle = color;
    context.fill();

    context.beginPath();
    context.arc(last.x, last.y, 8, 0, 2 * Math.PI);
    context.strokeStyle = hexToRgba(color, 0.35);
    context.lineWidth = 1;
    context.stroke();
  }
}

function escapeHTML(str) {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function formatLogLine(line) {
  const escaped = escapeHTML(line);
  if (escaped.startsWith("[stdout] ")) {
    return `<span class="log-prefix log-stdout">[stdout]</span> ${escaped.substring(9)}`;
  }
  if (escaped.startsWith("[stderr] ")) {
    return `<span class="log-prefix log-stderr">[stderr]</span> ${escaped.substring(9)}`;
  }
  return escaped;
}

function renderLogs() {
  const stream = state.logStream;
  const lines = state.logLines[stream] || [];
  const filtered = state.logQuery
    ? lines.filter((line) => line.toLowerCase().includes(state.logQuery))
    : lines;

  const targetViewer = logsModalOpen ? modalLogViewer : logViewerElement;

  // Check if scroll is near bottom BEFORE updating content
  const isAtBottom = targetViewer.scrollHeight - targetViewer.scrollTop <= targetViewer.clientHeight + 40;

  // Format lines with colored spans
  const htmlContent = filtered.length ? filtered.map(formatLogLine).join("\n") : "No log lines.";
  targetViewer.innerHTML = htmlContent;

  if (logAutoscrollCheckbox.checked && isAtBottom) {
    targetViewer.scrollTop = targetViewer.scrollHeight;
  }

  // If maximized modal is active, keep inline logs element in sync in case modal closes
  if (logsModalOpen) {
    logViewerElement.innerHTML = htmlContent;
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
      title.textContent = [event.process_name, event.type].filter(Boolean).join(" / ") || event.type;
      const body = document.createElement("span");
      body.textContent = `${formatTime(event.timestamp)} - ${event.message}`;
      item.append(title, body);
      return item;
    }),
  );
}

function toggleAccordion(trigger) {
  const expanded = trigger.getAttribute("aria-expanded") === "true";
  const panel = document.querySelector(`#${trigger.getAttribute("aria-controls")}`);
  trigger.setAttribute("aria-expanded", String(!expanded));
  if (panel) panel.hidden = expanded;
}

function runAction(action) {
  if (state.selectedId === null) return;
  if (state.readOnly) {
    showToast("Dashboard is read-only.");
    return;
  }
  if (action === "delete") {
    state.pendingAction = action;
    confirmModal.hidden = false;
    document.body.classList.add("modal-open");
    return;
  }
  performAction(action);
}

async function performAction(action) {
  if (state.selectedId === null) return;
  try {
    await fetchJSON(`/api/processes/${state.selectedId}/actions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": csrfToken(),
      },
      body: JSON.stringify({ action }),
    });
    closeConfirm();
    resetLogs();
    showToast(`${actionLabel(action)} requested.`);
    await refreshAll();
    await loadEvents();
  } catch (error) {
    setError(error.message);
    showToast(error.message);
  }
}

function closeConfirm() {
  state.pendingAction = "";
  confirmModal.hidden = true;
  document.body.classList.remove("modal-open");
}

function updateReadOnlyState() {
  document.querySelectorAll("[data-action]").forEach((button) => {
    button.disabled = state.readOnly;
  });
}

function replaceStatus(selector, status) {
  const current = document.querySelector(selector);
  if (!current) return;
  const pill = statusPill(status);
  pill.id = current.id;
  current.replaceWith(pill);
}

function statusPill(status) {
  const pill = document.createElement("span");
  const normalized = normalizeStatus(status);
  pill.className = `status-pill status-${normalized}`;
  pill.textContent = normalized;
  return pill;
}

function selectedProcessSummary() {
  return state.selectedProcess || state.processes.find((p) => p.id === state.selectedId) || null;
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
  if (state.logLines.both) state.logLines.both = [];
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
  navigator.clipboard.writeText(JSON.stringify(state.selectedProcess, null, 2)).then(
    () => showToast("Process JSON copied."),
    () => showToast("Clipboard copy failed."),
  );
}

function showToast(message) {
  window.clearTimeout(state.toastTimer);
  toastElement.textContent = message;
  toastElement.hidden = false;
  state.toastTimer = window.setTimeout(() => {
    toastElement.hidden = true;
  }, 2600);
}

function commandText(process) {
  return [process.executable_path, ...(process.args || [])].filter(Boolean).join(" ");
}

function normalizeStatus(status) {
  return (status || "unknown").toLowerCase();
}

// Check values are yes / no
function yesNo(value) {
  return value ? "yes" : "no";
}

function ms(value) {
  return value ? `${value}ms` : "-";
}

function bytes(value) {
  return value ? `${value} bytes` : "-";
}

function formatTime(value) {
  if (!value) return "now";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleTimeString();
}

function actionLabel(action) {
  return action.charAt(0).toUpperCase() + action.slice(1);
}

async function bootstrap() {
  initTheme();
  await loadSession();
  await refreshAll();
  if (state.selectedId !== null) {
    await loadSelectedDetails();
  }
  window.setInterval(() => {
    if (autoRefresh.checked) refreshAll();
  }, 1000);
  window.setInterval(() => {
    if (state.logLive && state.selectedId !== null) loadLogs(false);
  }, 2000);
}

bootstrap().catch((error) => setError(error.message));
