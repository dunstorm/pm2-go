const state = {
  processes: [],
  filter: "all",
  query: "",
};

const rows = document.querySelector("#process-rows");
const empty = document.querySelector("#empty");
const errorBox = document.querySelector("#error");
const updated = document.querySelector("#last-updated");
const search = document.querySelector("#search");

document.querySelector("#refresh").addEventListener("click", loadProcesses);
search.addEventListener("input", (event) => {
  state.query = event.target.value.trim().toLowerCase();
  render();
});

document.querySelectorAll(".segment").forEach((button) => {
  button.addEventListener("click", () => {
    state.filter = button.dataset.filter;
    document.querySelectorAll(".segment").forEach((item) => item.classList.remove("active"));
    button.classList.add("active");
    render();
  });
});

async function loadProcesses() {
  setError("");
  try {
    const response = await fetch("/api/processes", { credentials: "same-origin" });
    if (response.status === 401) {
      window.location.href = "/login";
      return;
    }
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      throw new Error(data.error || "Failed to load processes");
    }
    const data = await response.json();
    state.processes = Array.isArray(data.processes) ? data.processes : [];
    updated.textContent = `Updated ${new Date().toLocaleTimeString()}`;
    render();
  } catch (error) {
    setError(error.message);
    state.processes = [];
    render();
  }
}

function render() {
  updateSummary();
  const filtered = visibleProcesses();
  rows.replaceChildren(...filtered.map(processRow));
  empty.hidden = state.processes.length !== 0 || Boolean(errorBox.textContent);
}

function updateSummary() {
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
  row.append(
    cell(nameBlock(process)),
    cell(statusPill(process.status)),
    textCell(process.pid || "—"),
    textCell(process.cpu || "0.0%"),
    textCell(process.memory || "0.0MB"),
    textCell(process.uptime || "0s"),
    textCell(process.restarts ?? 0),
    cell(commandBlock(process)),
    cell(policyBlock(process)),
  );
  return row;
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

function commandBlock(process) {
  const wrap = document.createElement("div");
  wrap.className = "command";

  const code = document.createElement("code");
  const args = Array.isArray(process.args) ? process.args.join(" ") : "";
  code.textContent = [process.executable_path, args].filter(Boolean).join(" ");

  const cwd = document.createElement("span");
  cwd.textContent = process.cwd || "No cwd";

  wrap.append(code, cwd);
  return wrap;
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

function normalizeStatus(status) {
  return (status || "unknown").toLowerCase();
}

function setError(message) {
  errorBox.textContent = message;
  errorBox.hidden = !message;
}

loadProcesses();
window.setInterval(loadProcesses, 5000);
