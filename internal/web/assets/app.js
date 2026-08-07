const state = {
  processes: [],
  events: [],
  session: {},
  selectedId: null,
  selectedProcess: null,
  metrics: [],
  activeView: "overview",
  detailTab: "settings",
  filter: "all",
  query: "",
  envQuery: "",
  readOnly: false,
  logStream: "both",
  logOffsets: { out: 0, err: 0, both: 0 },
  logLines: { out: [], err: [], both: [] },
  logLive: true,
  logQuery: "",
  pendingAction: "",
  pendingProcessId: null,
  toastTimer: null,
  chartTimer: null,
};

const viewMeta = {
  overview: {
    title: "Operations overview",
    copy: "Live process health, resource usage, and recent control activity.",
  },
  processes: {
    title: "Process inventory",
    copy: "Filter, inspect, and control every process managed by the daemon.",
  },
  logs: {
    title: "Log console",
    copy: "Tail stdout and stderr for the selected process with live refresh.",
  },
  events: {
    title: "Event timeline",
    copy: "Recent web dashboard actions and process status changes.",
  },
  settings: {
    title: "Dashboard settings",
    copy: "Runtime mode, bind posture, session settings, and admin guardrails.",
  },
};

function icon(name, className = "") {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.classList.add("icon");
  if (className) {
    svg.classList.add(...className.split(" ").filter(Boolean));
  }
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("focusable", "false");
  const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
  use.setAttribute("href", `#icon-${name}`);
  svg.append(use);
  return svg;
}

function label(text) {
  const span = document.createElement("span");
  span.textContent = text;
  return span;
}

function setIconLabel(element, iconName, text) {
  element.replaceChildren(icon(iconName), label(text));
}

const els = {
  panels: document.querySelectorAll(".view-panel"),
  navButtons: document.querySelectorAll("[data-view]"),
  viewTitle: document.querySelector("#view-title"),
  viewCopy: document.querySelector("#view-copy"),
  error: document.querySelector("#error"),
  refresh: document.querySelector("#refresh"),
  autoRefresh: document.querySelector("#auto-refresh"),
  updated: document.querySelector("#last-updated"),
  themeToggle: document.querySelector("#theme-toggle"),
  readOnlyBadge: document.querySelector("#read-only-badge"),
  railSummary: document.querySelector("#rail-summary"),
  railMode: document.querySelector("#rail-mode"),
  railBind: document.querySelector("#rail-bind"),
  search: document.querySelector("#search"),
  rows: document.querySelector("#process-rows"),
  empty: document.querySelector("#empty"),
  overviewProcesses: document.querySelector("#overview-processes"),
  overviewEvents: document.querySelector("#overview-events"),
  detailEmpty: document.querySelector("#detail-empty"),
  detailContent: document.querySelector("#detail-content"),
  detailName: document.querySelector("#detail-name"),
  detailCommand: document.querySelector("#detail-command"),
  detailStatus: document.querySelector("#detail-status"),
  selectedCPU: document.querySelector("#selected-cpu"),
  selectedMemory: document.querySelector("#selected-memory"),
  selectedUptime: document.querySelector("#selected-uptime"),
  metricCount: document.querySelector("#metric-count"),
  settingsList: document.querySelector("#settings-list"),
  pathsList: document.querySelector("#paths-list"),
  envSearch: document.querySelector("#env-search"),
  envCount: document.querySelector("#env-count"),
  envKeyList: document.querySelector("#env-key-list"),
  copyConfig: document.querySelector("#copy-config"),
  processSelect: document.querySelector("#process-select"),
  logProcessName: document.querySelector("#log-process-name"),
  logStream: document.querySelector("#log-stream"),
  logLive: document.querySelector("#log-live"),
  logAutoscroll: document.querySelector("#log-autoscroll"),
  logSearch: document.querySelector("#log-search"),
  logViewer: document.querySelector("#log-viewer"),
  logCopy: document.querySelector("#log-copy"),
  logMaximize: document.querySelector("#log-maximize"),
  logsModal: document.querySelector("#logs-modal"),
  modalLogViewer: document.querySelector("#modal-log-viewer"),
  closeLogsModal: document.querySelector("#close-logs-modal"),
  modalTitle: document.querySelector(".modal-title-text"),
  eventList: document.querySelector("#event-list"),
  eventsRefresh: document.querySelector("#events-refresh"),
  adminSettings: document.querySelector("#admin-settings-list"),
  settingsModePill: document.querySelector("#settings-mode-pill"),
  securityNotice: document.querySelector("#security-notice"),
  confirmModal: document.querySelector("#confirm-modal"),
  confirmTitle: document.querySelector("#confirm-title"),
  confirmCopy: document.querySelector("#confirm-copy"),
  confirmCancel: document.querySelector("#confirm-cancel"),
  confirmRun: document.querySelector("#confirm-run"),
  toast: document.querySelector("#toast"),
};

function bindEvents() {
  els.refresh.addEventListener("click", refreshAll);
  els.themeToggle.addEventListener("click", toggleTheme);
  els.search.addEventListener("input", (event) => {
    state.query = event.target.value.trim().toLowerCase();
    renderProcessInventory();
  });
  els.copyConfig.addEventListener("click", copySelectedConfig);
  els.envSearch.addEventListener("input", (event) => {
    state.envQuery = event.target.value.trim().toLowerCase();
    renderEnvKeys(currentProcessDetail()?.env_keys || []);
  });
  els.processSelect.addEventListener("change", () => {
    const id = Number(els.processSelect.value);
    if (isProcessId(id)) selectProcess(id);
  });
  els.logStream.addEventListener("change", () => {
    state.logStream = els.logStream.value;
    loadLogs(true);
  });
  els.logLive.addEventListener("change", () => {
    state.logLive = els.logLive.checked;
  });
  els.logSearch.addEventListener("input", (event) => {
    state.logQuery = event.target.value.trim().toLowerCase();
    renderLogs();
  });
  els.logCopy.addEventListener("click", copyVisibleLogs);
  els.logMaximize.addEventListener("click", openLogsModal);
  els.closeLogsModal.addEventListener("click", closeLogsModal);
  els.eventsRefresh.addEventListener("click", loadEvents);
  els.confirmCancel.addEventListener("click", closeConfirm);
  els.confirmRun.addEventListener("click", () => performAction());
  els.confirmModal.addEventListener("click", (event) => {
    if (event.target === els.confirmModal) closeConfirm();
  });
  els.logsModal.addEventListener("click", (event) => {
    if (event.target === els.logsModal) closeLogsModal();
  });
  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      closeConfirm();
      closeLogsModal();
    }
  });
  document.querySelectorAll("[data-view]").forEach((button) => {
    button.addEventListener("click", () => switchView(button.dataset.view));
  });
  document.querySelectorAll("[data-view-jump]").forEach((button) => {
    button.addEventListener("click", () => switchView(button.dataset.viewJump));
  });
  document.querySelectorAll("[data-filter-shortcut]").forEach((button) => {
    button.addEventListener("click", () => {
      const shouldJump = button.classList.contains("summary-card");
      setFilter(button.dataset.filterShortcut, shouldJump);
    });
  });
  document.querySelectorAll("[data-detail-tab]").forEach((button) => {
    button.addEventListener("click", () => setDetailTab(button.dataset.detailTab));
  });
  document.querySelectorAll(".action-strip [data-action]").forEach((button) => {
    button.addEventListener("click", () => runAction(button.dataset.action));
  });
  window.addEventListener("resize", () => {
    window.clearTimeout(state.chartTimer);
    state.chartTimer = window.setTimeout(renderCharts, 120);
  });
}

function initTheme() {
  const theme = window.localStorage.getItem("pm2-go-theme") || "light";
  document.documentElement.setAttribute("data-theme", theme);
  renderThemeButton(theme);
}

function toggleTheme() {
  const current = document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light";
  const next = current === "dark" ? "light" : "dark";
  document.documentElement.setAttribute("data-theme", next);
  window.localStorage.setItem("pm2-go-theme", next);
  renderThemeButton(next);
  renderCharts();
}

function renderThemeButton(theme) {
  setIconLabel(els.themeToggle, theme === "dark" ? "sun" : "moon", theme === "dark" ? "Light" : "Dark");
}

function switchView(view) {
  if (!viewMeta[view]) return;
  state.activeView = view;
  els.panels.forEach((panel) => {
    panel.hidden = panel.id !== `${view}-view`;
  });
  els.navButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.view === view);
  });
  els.viewTitle.textContent = viewMeta[view].title;
  els.viewCopy.textContent = viewMeta[view].copy;
  if (view === "logs") loadLogs(false);
  if (view === "events") loadEvents();
  if (view === "processes") renderCharts();
}

async function loadSession() {
  const response = await fetchJSON("/api/session");
  state.session = response;
  state.readOnly = Boolean(response.read_only);
  renderSession();
  updateReadOnlyState();
}

async function refreshAll() {
  await loadProcesses();
  if (state.selectedId !== null) {
    await loadSelectedDetails();
  }
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
      state.selectedProcess = null;
      resetLogs();
    }

    els.updated.textContent = `Updated ${new Date().toLocaleTimeString()}`;
    renderAll();
  } catch (error) {
    state.processes = [];
    state.selectedId = null;
    state.selectedProcess = null;
    setError(error.message);
    renderAll();
  }
}

async function loadSelectedDetails() {
  if (state.selectedId === null) {
    state.selectedProcess = null;
    state.metrics = [];
    renderSelectedProcess();
    renderLogs();
    return;
  }
  try {
    const data = await fetchJSON(`/api/processes/${state.selectedId}`);
    state.selectedProcess = data.process;
    state.metrics = Array.isArray(data.metrics) ? data.metrics : [];
    renderSelectedProcess();
    renderCharts();
    await loadLogs(false);
  } catch (error) {
    setError(error.message);
  }
}

async function loadEvents() {
  try {
    const data = await fetchJSON("/api/events");
    state.events = Array.isArray(data.events) ? data.events : [];
    renderEvents();
  } catch (error) {
    setError(error.message);
  }
}

async function loadLogs(reset) {
  if (state.selectedId === null) {
    renderLogs();
    return;
  }
  if (reset) resetLogs(false);

  try {
    if (state.logStream === "both") {
      await loadBothStreams(reset);
    } else {
      await loadOneStream(state.logStream, reset);
    }
  } catch (error) {
    const stream = state.logStream;
    state.logLines[stream] = [`Failed to read logs: ${error.message}`];
  }
  renderLogs();
}

async function loadBothStreams(reset) {
  try {
    const data = await fetchLogStream("both", 360);
    state.logOffsets.both = data.offset || 0;
    const incoming = Array.isArray(data.lines) ? data.lines : [];
    if (reset) {
      state.logLines.both = incoming;
    } else if (incoming.length > 0) {
      state.logLines.both = state.logLines.both.concat(incoming).slice(-1200);
    }
    return;
  } catch (_error) {
  }

  const [outResult, errResult] = await Promise.allSettled([
    fetchLogStream("out", 180),
    fetchLogStream("err", 180),
  ]);
  const incoming = [];

  if (outResult.status === "fulfilled") {
    state.logOffsets.out = outResult.value.offset || 0;
    incoming.push(...(outResult.value.lines || []).map((line) => `[stdout] ${line}`));
  } else if (reset) {
    incoming.push(`[stdout] unavailable: ${outResult.reason.message}`);
  }

  if (errResult.status === "fulfilled") {
    state.logOffsets.err = errResult.value.offset || 0;
    incoming.push(...(errResult.value.lines || []).map((line) => `[stderr] ${line}`));
  } else if (reset) {
    incoming.push(`[stderr] unavailable: ${errResult.reason.message}`);
  }

  if (reset) {
    state.logLines.both = incoming;
  } else if (incoming.length > 0) {
    state.logLines.both = state.logLines.both.concat(incoming).slice(-1200);
  }
}

async function loadOneStream(stream, reset) {
  const data = await fetchLogStream(stream, 360);
  state.logOffsets[stream] = data.offset || 0;
  const incoming = Array.isArray(data.lines) ? data.lines : [];
  if (reset) {
    state.logLines[stream] = incoming;
  } else if (incoming.length > 0) {
    state.logLines[stream] = state.logLines[stream].concat(incoming).slice(-1200);
  }
}

function fetchLogStream(stream, tail) {
  const params = new URLSearchParams({
    stream,
    offset: String(state.logOffsets[stream] || 0),
    tail: String(tail),
  });
  return fetchJSON(`/api/processes/${state.selectedId}/logs?${params}`);
}

function renderAll() {
  renderSummary();
  renderOverview();
  renderProcessInventory();
  renderProcessSelect();
  renderSelectedProcess();
  renderEvents();
  renderAdminSettings();
}

function renderSession() {
  const readOnly = Boolean(state.session.read_only);
  const remote = Boolean(state.session.allow_remote);
  const modeText = readOnly ? "Read-only" : "Read-write";
  els.railMode.textContent = modeText;
  setIconLabel(els.railBind, remote ? "activity" : "shield", remote ? "remote bind" : "loopback");
  setIconLabel(els.readOnlyBadge, readOnly ? "shield" : "key", modeText);
  setIconLabel(els.settingsModePill, readOnly ? "shield" : "key", modeText);
  els.readOnlyBadge.className = `mode-badge ${readOnly ? "mode-read-only" : "mode-read-write"}`;
  els.settingsModePill.className = els.readOnlyBadge.className;
}

function renderSummary() {
  const counts = state.processes.reduce(
    (acc, process) => {
      acc.total += 1;
      const status = normalizeStatus(process.status);
      if (status === "online") acc.online += 1;
      if (status === "unhealthy") acc.unhealthy += 1;
      if (status === "stopped" || status === "errored") acc.stopped += 1;
      acc.cpu += parsePercent(process.cpu);
      acc.memory += parseMemoryMB(process.memory);
      acc.restarts += Number(process.restarts || 0);
      return acc;
    },
    { total: 0, online: 0, unhealthy: 0, stopped: 0, cpu: 0, memory: 0, restarts: 0 },
  );
  document.querySelector("#total-count").textContent = counts.total;
  document.querySelector("#online-count").textContent = counts.online;
  document.querySelector("#unhealthy-count").textContent = counts.unhealthy;
  document.querySelector("#stopped-count").textContent = counts.stopped;
  document.querySelector("#aggregate-cpu").textContent = `${counts.cpu.toFixed(1)}%`;
  document.querySelector("#aggregate-memory").textContent = formatMemory(counts.memory);
  document.querySelector("#aggregate-restarts").textContent = String(counts.restarts);
  els.railSummary.textContent = `${counts.online}/${counts.total} online`;

  const title = document.querySelector("#daemon-health-title");
  const pill = document.querySelector("#daemon-health-pill");
  if (counts.total === 0) {
    title.textContent = "No managed processes";
    replacePill(pill, "unknown");
  } else if (counts.unhealthy > 0 || counts.stopped > 0) {
    title.textContent = `${counts.unhealthy + counts.stopped} process${counts.unhealthy + counts.stopped === 1 ? "" : "es"} need attention`;
    replacePill(pill, counts.unhealthy > 0 ? "unhealthy" : "stopped");
  } else {
    title.textContent = "All tracked processes online";
    replacePill(pill, "online");
  }
}

function renderOverview() {
  const sorted = [...state.processes].sort((a, b) => processPriority(a) - processPriority(b));
  const priority = sorted.slice(0, 8);
  if (priority.length === 0) {
    els.overviewProcesses.replaceChildren(emptyText("No processes registered."));
  } else {
    els.overviewProcesses.replaceChildren(...priority.map(priorityRow));
  }
  renderHealthBreakdown();
}

function renderHealthBreakdown() {
  const counts = {
    autorestart: state.processes.filter((process) => process.auto_restart).length,
    health: state.processes.filter((process) => process.health_configured).length,
    watch: state.processes.filter((process) => process.watch).length,
    delayed: state.processes.filter((process) => process.next_start_at).length,
  };
  const rows = [
    ["rotate-cw", "Auto restart policies", counts.autorestart],
    ["activity", "Health checks", counts.health],
    ["refresh-cw", "Watch reloads", counts.watch],
    ["history", "Delayed starts", counts.delayed],
  ].map(([iconName, text, value]) => {
    const row = document.createElement("div");
    row.className = "health-row";
    const name = document.createElement("span");
    name.className = "icon-label";
    name.append(icon(iconName), label(text));
    const count = document.createElement("strong");
    count.textContent = String(value);
    row.append(name, count);
    return row;
  });
  document.querySelector("#health-breakdown").replaceChildren(...rows);
}

function priorityRow(process) {
  const row = document.createElement("button");
  row.className = "priority-row";
  row.type = "button";
  row.addEventListener("click", () => selectProcess(process.id, "processes"));

  const name = document.createElement("span");
  name.className = "priority-name";
  const nameHeader = document.createElement("span");
  nameHeader.className = "row-title";
  const strong = document.createElement("strong");
  strong.textContent = process.name || `process-${process.id}`;
  nameHeader.append(processIcon(process), strong);
  const sub = document.createElement("small");
  sub.className = "muted-line";
  sub.textContent = `${process.cpu || "0.0%"} CPU / ${process.memory || "0.0MB"}`;
  name.append(nameHeader, sub);
  row.append(name, statusPill(process.status));
  return row;
}

function renderProcessInventory() {
  const filtered = visibleProcesses();
  els.rows.replaceChildren(...filtered.map(processTableRow));
  els.empty.hidden = filtered.length !== 0 || Boolean(els.error.textContent);
  updateReadOnlyState();
}

function processTableRow(process) {
  const row = document.createElement("tr");
  row.tabIndex = 0;
  row.className = process.id === state.selectedId ? "selected" : "";
  row.addEventListener("click", () => selectProcess(process.id));
  row.addEventListener("keydown", (event) => {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      selectProcess(process.id);
    }
  });

  const name = document.createElement("td");
  const nameWrap = document.createElement("div");
  nameWrap.className = "process-name";
  const nameHeader = document.createElement("span");
  nameHeader.className = "row-title";
  const title = document.createElement("strong");
  title.textContent = process.name || `process-${process.id}`;
  nameHeader.append(processIcon(process), title);
  const subline = document.createElement("span");
  subline.className = "process-subline";
  subline.textContent = commandText(process) || process.cwd || "No command";
  nameWrap.append(nameHeader, subline);
  name.append(nameWrap);

  const status = document.createElement("td");
  status.append(statusPill(process.status));

  const pid = textCell(process.pid || "-", "num-cell");
  const cpu = textCell(process.cpu || "0.0%", "num-cell");
  const memory = textCell(process.memory || "0.0MB", "num-cell");
  const uptime = textCell(process.uptime || "0s", "num-cell");
  const restarts = textCell(String(process.restarts || 0), "num-cell");

  const policy = document.createElement("td");
  policy.append(policyTags(process));

  row.append(name, status, pid, cpu, memory, uptime, restarts, policy);
  return row;
}

function visibleProcesses() {
  return state.processes.filter((process) => {
    const status = normalizeStatus(process.status);
    const matchesFilter = state.filter === "all" || status === state.filter || (state.filter === "stopped" && status === "errored");
    const haystack = [
      process.name,
      process.executable_path,
      process.cwd,
      ...(process.args || []),
    ].join(" ").toLowerCase();
    return matchesFilter && haystack.includes(state.query);
  });
}

function setFilter(filter, jumpToProcesses) {
  state.filter = filter;
  document.querySelectorAll("[data-filter-shortcut]").forEach((button) => {
    button.classList.toggle("active", button.dataset.filterShortcut === filter);
  });
  renderProcessInventory();
  if (jumpToProcesses) switchView("processes");
}

async function selectProcess(id, view) {
  if (view) switchView(view);
  if (state.selectedId !== id) {
    state.selectedId = id;
    state.selectedProcess = null;
    state.metrics = [];
    state.envQuery = "";
    els.envSearch.value = "";
    resetLogs(false);
  }
  renderProcessInventory();
  renderProcessSelect();
  renderSelectedProcess();
  await loadSelectedDetails();
}

function renderProcessSelect() {
  els.processSelect.replaceChildren();
  if (state.processes.length === 0) {
    const option = document.createElement("option");
    option.value = "";
    option.textContent = "No processes";
    els.processSelect.append(option);
    return;
  }
  state.processes.forEach((process) => {
    const option = document.createElement("option");
    option.value = String(process.id);
    option.textContent = process.name || `process-${process.id}`;
    els.processSelect.append(option);
  });
  if (state.selectedId !== null) {
    els.processSelect.value = String(state.selectedId);
  }
}

function renderSelectedProcess() {
  const process = currentProcessDetail();
  if (!process) {
    els.detailEmpty.hidden = false;
    els.detailContent.hidden = true;
    els.logProcessName.textContent = "No process selected";
    state.metrics = [];
    clearCharts();
    return;
  }

  els.detailEmpty.hidden = true;
  els.detailContent.hidden = false;
  els.detailName.textContent = process.name || `process-${process.id}`;
  els.detailCommand.textContent = commandText(process) || process.cwd || "No command";
  replacePill(els.detailStatus, process.status);
  els.selectedCPU.textContent = process.cpu || "0.0%";
  els.selectedMemory.textContent = process.memory || "0.0MB";
  els.selectedUptime.textContent = process.uptime || "0s";
  els.metricCount.textContent = `${state.metrics.length} sample${state.metrics.length === 1 ? "" : "s"}`;
  els.logProcessName.textContent = process.name || `process-${process.id}`;

  renderSettings(process);
  renderPathSettings(process);
  renderEnvKeys(process.env_keys || []);
  renderDetailTabs();
  updateReadOnlyState();
}

function currentProcessDetail() {
  return state.selectedProcess || state.processes.find((process) => process.id === state.selectedId) || null;
}

function renderSettings(process) {
  const settings = [
    ["ID", process.id],
    ["PID", process.pid || "-"],
    ["Parent PID", process.parent_pid || "-"],
    ["Status", normalizeStatus(process.status)],
    ["Auto restart", yesNo(process.auto_restart)],
    ["Cron restart", process.cron_restart || "-"],
    ["Max restarts", process.max_restarts || "-"],
    ["Min uptime", formatDurationMs(process.min_uptime_ms)],
    ["Restart delay", formatDurationMs(process.restart_delay_ms)],
    ["Backoff delay", formatDurationMs(process.exp_backoff_restart_delay_ms)],
    ["Max memory restart", formatBytes(process.max_memory_restart)],
    ["Health URL", process.health_check_url || "-"],
    ["Health interval", formatDurationMs(process.health_check_interval_ms)],
    ["Health timeout", formatDurationMs(process.health_check_timeout_ms)],
    ["Watch", yesNo(process.watch)],
    ["Watch interval", formatDurationMs(process.watch_interval_ms)],
    ["Next start", process.next_start_at || "-"],
    ["Last health check", process.last_health_check_at || "-"],
    ["Last watch check", process.last_watch_check_at || "-"],
  ];
  fillDefinitionList(els.settingsList, settings);
}

function renderPathSettings(process) {
  const paths = [
    ["Executable", process.executable_path || "-"],
    ["Arguments", (process.args || []).join(" ") || "-"],
    ["Working directory", process.cwd || "-"],
    ["Watch paths", (process.watch_paths || []).join(", ") || "-"],
    ["Stdout log", process.log_file_path || "-"],
    ["Stderr log", process.err_file_path || "-"],
  ];
  fillDefinitionList(els.pathsList, paths);
}

function renderEnvKeys(keys) {
  const filtered = keys.filter((key) => key.toLowerCase().includes(state.envQuery));
  els.envCount.textContent = `${filtered.length} key${filtered.length === 1 ? "" : "s"}`;
  if (filtered.length === 0) {
    els.envKeyList.replaceChildren(emptyText(state.envQuery ? "No matching keys." : "No environment keys."));
    return;
  }
  els.envKeyList.replaceChildren(
    ...filtered.map((key) => {
      const chip = document.createElement("span");
      chip.className = "key-chip";
      chip.textContent = key;
      chip.title = key;
      return chip;
    }),
  );
}

function setDetailTab(tab) {
  state.detailTab = tab;
  renderDetailTabs();
}

function renderDetailTabs() {
  document.querySelectorAll("[data-detail-tab]").forEach((button) => {
    button.classList.toggle("active", button.dataset.detailTab === state.detailTab);
  });
  ["settings", "env", "paths"].forEach((tab) => {
    document.querySelector(`#${tab}-tab`).hidden = tab !== state.detailTab;
  });
}

function renderEvents() {
  fillEventList(els.eventList, state.events, 80);
  fillEventList(els.overviewEvents, state.events, 6);
}

function fillEventList(list, events, limit) {
  const visible = events.slice(0, limit);
  if (visible.length === 0) {
    list.replaceChildren(eventItem({ type: "event", message: "No events recorded yet.", timestamp: "" }));
    return;
  }
  list.replaceChildren(...visible.map(eventItem));
}

function eventItem(event) {
  const item = document.createElement("li");
  const eventIcon = icon(event.type === "action" ? "activity" : "history", "event-icon");
  const title = document.createElement("strong");
  title.textContent = [event.process_name, event.type].filter(Boolean).join(" / ") || event.type || "event";
  const body = document.createElement("span");
  body.textContent = `${formatTime(event.timestamp)} - ${event.message || ""}`;
  item.append(eventIcon, title, body);
  return item;
}

function renderAdminSettings() {
  const session = state.session || {};
  const values = [
    ["Access mode", session.read_only ? "read-only" : "read-write"],
    ["Bind host", session.host || "-"],
    ["Bind port", session.port || "-"],
    ["Remote binding allowed", yesNo(session.allow_remote)],
    ["Secure cookies forced", yesNo(session.secure_cookies)],
    ["Session TTL", formatDurationSeconds(session.session_ttl_seconds)],
    ["Generated token", yesNo(session.token_generated)],
    ["Dev assets", yesNo(session.dev_assets)],
    ["Dev reload", yesNo(session.dev_reload)],
  ];
  fillDefinitionList(els.adminSettings, values);

  const items = [
    {
      title: session.read_only ? "Lifecycle controls are locked" : "Lifecycle controls are enabled",
      body: session.read_only
        ? "Start, stop, restart, reload, and delete requests are rejected by the server."
        : "Authenticated users can run process lifecycle actions from the dashboard.",
    },
    {
      title: session.allow_remote ? "Remote binding is allowed" : "Loopback binding is enforced",
      body: session.allow_remote
        ? "Only expose this behind trusted network controls or a reverse proxy with TLS."
        : "The dashboard accepts local access unless the host is changed explicitly.",
    },
    {
      title: session.secure_cookies ? "Secure cookies are forced" : "Secure cookies follow the request",
      body: session.secure_cookies
        ? "Session cookies are always marked Secure for TLS-terminated proxy deployments."
        : "Session cookies are marked Secure automatically when the backend request uses TLS.",
    },
    {
      title: session.token_generated ? "Ephemeral token is active" : "Configured token is active",
      body: session.token_generated
        ? "The server generated a token for this run; copy it from startup output if needed."
        : "Authentication is backed by the configured PM2_GO_WEB_TOKEN or flag value.",
    },
    {
      title: session.dev_reload ? "Development reload is active" : "Embedded assets are active",
      body: session.dev_reload
        ? "Asset fingerprint polling is enabled for local dashboard development."
        : "The dashboard is serving the packaged static interface.",
    },
  ];
  els.securityNotice.replaceChildren(
    ...items.map((entry) => {
      const item = document.createElement("div");
      item.className = "posture-item";
      const itemIcon = icon(postureIcon(entry.title), "posture-icon");
      const title = document.createElement("strong");
      title.textContent = entry.title;
      const body = document.createElement("p");
      body.className = "muted-line";
      body.textContent = entry.body;
      item.append(itemIcon, title, body);
      return item;
    }),
  );
}

function renderCharts() {
  const cpuColor = document.documentElement.getAttribute("data-theme") === "dark" ? "#60a5fa" : "#2563eb";
  const memoryColor = document.documentElement.getAttribute("data-theme") === "dark" ? "#2dd4bf" : "#0f766e";
  drawChart(document.querySelector("#cpu-chart"), state.metrics, "cpu", cpuColor, "%");
  drawChart(document.querySelector("#memory-chart"), state.metrics, "memory_mb", memoryColor, "MB");
}

function clearCharts() {
  drawChart(document.querySelector("#cpu-chart"), [], "cpu", "#2563eb", "%");
  drawChart(document.querySelector("#memory-chart"), [], "memory_mb", "#0f766e", "MB");
}

function drawChart(canvas, points, key, color, suffix) {
  if (!canvas) return;
  const context = canvas.getContext("2d");
  const ratio = window.devicePixelRatio || 1;
  const rect = canvas.getBoundingClientRect();
  const width = Math.max(1, rect.width || 360);
  const height = Math.max(120, rect.height || 140);
  canvas.width = Math.floor(width * ratio);
  canvas.height = Math.floor(height * ratio);
  context.setTransform(ratio, 0, 0, ratio, 0, 0);
  context.clearRect(0, 0, width, height);

  const isDark = document.documentElement.getAttribute("data-theme") === "dark";
  context.strokeStyle = isDark ? "rgba(255,255,255,0.07)" : "rgba(18,22,28,0.08)";
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
  context.fillStyle = isDark ? "rgba(237,241,247,0.54)" : "rgba(18,22,28,0.52)";
  context.font = "11px ui-sans-serif, system-ui, sans-serif";
  context.fillText(`${max.toFixed(key === "cpu" ? 0 : 1)}${suffix}`, 10, 18);

  if (values.length === 0) {
    context.fillText("No samples yet", 10, height - 14);
    return;
  }

  const coords = values.map((value, index) => {
    const x = values.length === 1 ? width - 12 : 10 + (index / (values.length - 1)) * (width - 20);
    const y = height - 16 - (value / max) * (height - 38);
    return { x, y };
  });

  context.beginPath();
  coords.forEach((point, index) => {
    if (index === 0) context.moveTo(point.x, point.y);
    else context.lineTo(point.x, point.y);
  });
  context.strokeStyle = color;
  context.lineWidth = 2.4;
  context.stroke();

  context.lineTo(coords[coords.length - 1].x, height - 12);
  context.lineTo(coords[0].x, height - 12);
  context.closePath();
  const gradient = context.createLinearGradient(0, 0, 0, height);
  gradient.addColorStop(0, hexToRgba(color, 0.16));
  gradient.addColorStop(1, hexToRgba(color, 0));
  context.fillStyle = gradient;
  context.fill();

  const last = coords[coords.length - 1];
  context.beginPath();
  context.arc(last.x, last.y, 4, 0, Math.PI * 2);
  context.fillStyle = color;
  context.fill();
}

function renderLogs() {
  const process = currentProcessDetail();
  if (!process) {
    els.logViewer.textContent = "Select a process to read logs.";
    if (!els.logsModal.hidden) els.modalLogViewer.textContent = els.logViewer.textContent;
    return;
  }
  const stream = state.logStream;
  const lines = state.logLines[stream] || [];
  const filtered = state.logQuery ? lines.filter((line) => line.toLowerCase().includes(state.logQuery)) : lines;
  const content = filtered.length ? filtered.map(formatLogLine).join("\n") : "No log lines.";

  const inlineWasBottom = isNearBottom(els.logViewer);
  els.logViewer.innerHTML = content;
  if (els.logAutoscroll.checked && inlineWasBottom) {
    els.logViewer.scrollTop = els.logViewer.scrollHeight;
  }

  if (!els.logsModal.hidden) {
    const modalWasBottom = isNearBottom(els.modalLogViewer);
    els.modalLogViewer.innerHTML = content;
    if (els.logAutoscroll.checked && modalWasBottom) {
      els.modalLogViewer.scrollTop = els.modalLogViewer.scrollHeight;
    }
  }
}

function resetLogs(render = true) {
  state.logOffsets = { out: 0, err: 0, both: 0 };
  state.logLines = { out: [], err: [], both: [] };
  if (render) renderLogs();
}

function isProcessId(id) {
  return Number.isInteger(id) && id >= 0;
}

function runAction(action, id = state.selectedId) {
  if (!isProcessId(id)) return;
  if (state.readOnly) {
    showToast("Dashboard is read-only.");
    return;
  }
  state.selectedId = id;
  const process = state.processes.find((item) => item.id === id) || currentProcessDetail();
  if (action === "delete" || action === "stop") {
    openConfirm(action, id, process);
    return;
  }
  performAction(action, id);
}

function openConfirm(action, id, process) {
  state.pendingAction = action;
  state.pendingProcessId = id;
  const name = process?.name || `process-${id}`;
  els.confirmTitle.textContent = `${actionLabel(action)} ${name}?`;
  els.confirmCopy.textContent = action === "delete"
    ? "This removes the process from pm2-go daemon management."
    : "This stops the live process until it is started again.";
  setIconLabel(els.confirmRun, action === "delete" ? "trash" : "square", actionLabel(action));
  els.confirmRun.classList.toggle("button-danger", action === "delete");
  els.confirmModal.hidden = false;
  document.body.classList.add("modal-open");
}

async function performAction(action = state.pendingAction, id = state.pendingProcessId ?? state.selectedId) {
  if (!action || !isProcessId(id)) return;
  try {
    await fetchJSON(`/api/processes/${id}/actions`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-CSRF-Token": csrfToken(),
      },
      body: JSON.stringify({ action }),
    });
    closeConfirm();
    resetLogs(false);
    showToast(`${actionLabel(action)} requested.`);
    await refreshAll();
    await loadEvents();
  } catch (error) {
    closeConfirm();
    setError(error.message);
    showToast(error.message);
  }
}

function closeConfirm() {
  state.pendingAction = "";
  state.pendingProcessId = null;
  els.confirmModal.hidden = true;
  document.body.classList.remove("modal-open");
}

function openLogsModal() {
  const process = currentProcessDetail();
  if (!process) return;
  els.modalTitle.textContent = `${process.name || process.id} logs`;
  els.logsModal.hidden = false;
  document.body.classList.add("modal-open");
  renderLogs();
}

function closeLogsModal() {
  els.logsModal.hidden = true;
  document.body.classList.remove("modal-open");
}

function updateReadOnlyState() {
  document.querySelectorAll("[data-action]").forEach((control) => {
    control.disabled = state.readOnly;
  });
}

function setError(message) {
  els.error.textContent = message;
  els.error.hidden = !message;
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

function fillDefinitionList(list, entries) {
  list.replaceChildren(
    ...entries.flatMap(([key, value]) => {
      const dt = document.createElement("dt");
      dt.textContent = key;
      const dd = document.createElement("dd");
      dd.textContent = String(value);
      return [dt, dd];
    }),
  );
}

function policyTags(process) {
  const wrap = document.createElement("div");
  wrap.className = "policy-tags";
  const tags = [];
  if (process.auto_restart) tags.push(["rotate-cw", "auto", "good"]);
  if (process.health_configured) tags.push(["activity", "health", "good"]);
  if (process.watch) tags.push(["refresh-cw", "watch", "warn"]);
  if (process.cron_restart) tags.push(["history", "cron", "warn"]);
  if (process.max_memory_restart) tags.push(["database", "memory cap", "warn"]);
  if (tags.length === 0) tags.push(["square", "manual", ""]);

  tags.forEach(([iconName, text, tone]) => {
    const tag = document.createElement("span");
    tag.className = `policy-tag ${tone}`;
    tag.append(icon(iconName), label(text));
    wrap.append(tag);
  });
  return wrap;
}

function statusPill(status) {
  const pill = document.createElement("span");
  const normalized = normalizeStatus(status);
  pill.className = `status-pill status-${normalized}`;
  pill.append(icon(statusIcon(normalized)), label(normalized));
  return pill;
}

function replacePill(current, status) {
  const pill = statusPill(status);
  pill.id = current.id;
  current.replaceWith(pill);
  if (pill.id === "detail-status") els.detailStatus = pill;
}

function textCell(text, className = "") {
  const cell = document.createElement("td");
  if (className) cell.className = className;
  cell.textContent = text;
  return cell;
}

function emptyText(text) {
  const item = document.createElement("span");
  item.className = "muted-line icon-label";
  item.append(icon("server"), label(text));
  return item;
}

function copySelectedConfig() {
  const process = currentProcessDetail();
  if (!process || !navigator.clipboard) return;
  navigator.clipboard.writeText(JSON.stringify(process, null, 2)).then(
    () => showToast("Process JSON copied."),
    () => showToast("Clipboard copy failed."),
  );
}

function copyVisibleLogs() {
  if (!navigator.clipboard) return;
  const content = els.logViewer.textContent || "";
  navigator.clipboard.writeText(content).then(
    () => showToast("Logs copied."),
    () => showToast("Clipboard copy failed."),
  );
}

function showToast(message) {
  window.clearTimeout(state.toastTimer);
  els.toast.textContent = message;
  els.toast.hidden = false;
  state.toastTimer = window.setTimeout(() => {
    els.toast.hidden = true;
  }, 2600);
}

function csrfToken() {
  const match = document.cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith("pm2_go_web_csrf="));
  return match ? decodeURIComponent(match.split("=").slice(1).join("=")) : "";
}

function commandText(process) {
  return [process.executable_path, ...(process.args || [])].filter(Boolean).join(" ");
}

function normalizeStatus(status) {
  return (status || "unknown").toLowerCase();
}

function processPriority(process) {
  const status = normalizeStatus(process.status);
  if (status === "unhealthy") return 0;
  if (status === "errored") return 1;
  if (status === "stopped") return 2;
  if (status === "unknown") return 3;
  return 4;
}

function processIcon(process) {
  const haystack = `${process.name || ""} ${process.executable_path || ""} ${(process.args || []).join(" ")}`.toLowerCase();
  if (haystack.includes("python") || haystack.includes(".py")) return icon("file-code", "process-glyph");
  if (haystack.includes("node") || haystack.includes("npm") || haystack.includes(".js")) return icon("terminal", "process-glyph");
  if (haystack.includes("go")) return icon("server", "process-glyph");
  if (haystack.includes("sh") || haystack.includes("bash") || haystack.includes("zsh")) return icon("terminal", "process-glyph");
  return icon("server", "process-glyph");
}

function statusIcon(status) {
  if (status === "online") return "check-circle";
  if (status === "unhealthy") return "alert-triangle";
  if (status === "stopped" || status === "errored") return "square";
  return "activity";
}

function postureIcon(title) {
  const value = title.toLowerCase();
  if (value.includes("locked") || value.includes("enabled")) return "shield";
  if (value.includes("binding")) return "server";
  if (value.includes("token")) return "key";
  if (value.includes("reload") || value.includes("embedded")) return "refresh-cw";
  return "settings";
}

function parsePercent(value) {
  const parsed = Number(String(value || "0").replace("%", "").trim());
  return Number.isFinite(parsed) ? parsed : 0;
}

function parseMemoryMB(value) {
  const raw = String(value || "0").trim().toUpperCase();
  const parsed = Number.parseFloat(raw);
  if (!Number.isFinite(parsed)) return 0;
  if (raw.endsWith("GB")) return parsed * 1024;
  if (raw.endsWith("KB")) return parsed / 1024;
  if (raw.endsWith("B") && !raw.endsWith("MB")) return parsed / (1024 * 1024);
  return parsed;
}

function formatMemory(value) {
  if (value >= 1024) return `${(value / 1024).toFixed(1)}GB`;
  return `${value.toFixed(1)}MB`;
}

function formatBytes(value) {
  const bytes = Number(value || 0);
  if (!bytes) return "-";
  const mb = bytes / (1024 * 1024);
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)}GB`;
  if (mb >= 1) return `${mb.toFixed(1)}MB`;
  return `${bytes} bytes`;
}

function formatDurationMs(value) {
  const ms = Number(value || 0);
  if (!ms) return "-";
  if (ms >= 60000) return `${Math.round(ms / 60000)}m`;
  if (ms >= 1000) return `${(ms / 1000).toFixed(ms % 1000 === 0 ? 0 : 1)}s`;
  return `${ms}ms`;
}

function formatDurationSeconds(value) {
  const seconds = Number(value || 0);
  if (!seconds) return "-";
  if (seconds >= 86400) return `${Math.round(seconds / 86400)}d`;
  if (seconds >= 3600) return `${Math.round(seconds / 3600)}h`;
  if (seconds >= 60) return `${Math.round(seconds / 60)}m`;
  return `${seconds}s`;
}

function formatTime(value) {
  if (!value) return "now";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function yesNo(value) {
  return value ? "yes" : "no";
}

function actionLabel(action) {
  return action.charAt(0).toUpperCase() + action.slice(1);
}

function escapeHTML(value) {
  return String(value)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function formatLogLine(line) {
  const escaped = escapeHTML(line);
  if (escaped.startsWith("[stdout] ")) {
    return `<span class="log-prefix log-stdout">stdout</span>${escaped.slice(9)}`;
  }
  if (escaped.startsWith("[stderr] ")) {
    return `<span class="log-prefix log-stderr">stderr</span>${escaped.slice(9)}`;
  }
  return escaped;
}

function isNearBottom(element) {
  return element.scrollHeight - element.scrollTop <= element.clientHeight + 40;
}

function hexToRgba(hex, alpha) {
  const normalized = hex.replace("#", "");
  const value = normalized.length === 3
    ? normalized.split("").map((char) => char + char).join("")
    : normalized;
  const r = Number.parseInt(value.slice(0, 2), 16);
  const g = Number.parseInt(value.slice(2, 4), 16);
  const b = Number.parseInt(value.slice(4, 6), 16);
  return `rgba(${r}, ${g}, ${b}, ${alpha})`;
}

async function bootstrap() {
  bindEvents();
  initTheme();
  switchView("overview");
  await loadSession();
  await refreshAll();
  window.setInterval(() => {
    if (els.autoRefresh.checked) refreshAll();
  }, 1000);
  window.setInterval(() => {
    if (state.logLive && state.selectedId !== null) loadLogs(false);
  }, 2000);
}

bootstrap().catch((error) => setError(error.message));
