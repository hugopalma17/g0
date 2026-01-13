// Admin Panel Logic

// Wait for dependencies (CustomEditor, XTerm, etc.)
document.addEventListener("DOMContentLoaded", () => {
  const d = new Date();
  const dateInput = document.getElementById("entry-date");
  if (dateInput) dateInput.value = d.toISOString().split("T")[0];

  // Initialize Custom Editor
  const textarea = document.getElementById("raw-injector");
  const preview = document.getElementById("live-preview");
  if (textarea && preview && typeof CustomEditor === "function") {
    window.customEditor = new CustomEditor("raw-injector", "live-preview");
  }
});

// --- Tabs ---
function switchTab(tab) {
  document
    .querySelectorAll(".view-section")
    .forEach((el) => el.classList.remove("active"));
  document
    .querySelectorAll(".nav-btn")
    .forEach((el) => el.classList.remove("active"));

  document.getElementById("section-" + tab).classList.add("active");
  document.getElementById("nav-" + tab).classList.add("active");

  if (tab === "terminal" && typeof initTerminal === "function") initTerminal();
  if (tab === "database" && typeof loadTable === "function") loadTable();
}

// --- Entry Management ---
function loadEntry(id) {
  if (!id) {
    resetForm();
    return;
  }

  const entry = window.postsData
    ? window.postsData.find((p) => p.id === id)
    : null;
  if (!entry) return;

  document.getElementById("entry-id").value = entry.id;
  document.getElementById("entry-type").value = entry.type;
  document.getElementById("entry-title").value = entry.title;
  document.getElementById("entry-date").value = entry.date;
  document.getElementById("entry-meta").value = entry.metadata || "";
  document.getElementById("raw-injector").value = entry.content;

  updatePreview();
}

function resetForm() {
  document.getElementById("entry-id").value = "";
  document.getElementById("entry-title").value = "";
  document.getElementById("entry-meta").value = "";
  document.getElementById("raw-injector").value = "";
  document.getElementById("live-preview").innerHTML = "";
  updatePreview();
}

function updatePreview() {
  if (window.customEditor) {
    window.customEditor.updatePreview();
  } else {
    const content = document.getElementById("raw-injector").value;
    const preview = document.getElementById("live-preview");
    if (preview) preview.innerHTML = content;
  }
}

async function saveEntry() {
  const id = document.getElementById("entry-id").value;
  const formData = new FormData();
  formData.append("id", id);
  formData.append("type", document.getElementById("entry-type").value);
  formData.append("title", document.getElementById("entry-title").value);
  formData.append("date", document.getElementById("entry-date").value);
  formData.append("metadata", document.getElementById("entry-meta").value);
  formData.append("content", document.getElementById("raw-injector").value);

  try {
    const res = await fetch("/api/save", {
      method: "POST",
      body: formData,
      headers: { "HX-Request": "true" },
    });
    const text = await res.text();
    if (text.includes("PROTOCOL")) {
      showStatus("SAVED_OK", "var(--accent)");
      setTimeout(() => {
        showStatus("SYSTEM_ACTIVE", "var(--text-mute)");
        if (!id) location.reload();
      }, 1000);
    } else {
      alert("Error: " + text);
    }
  } catch (e) {
    alert("Network Error");
  }
}

async function deleteEntry() {
  const id = document.getElementById("entry-id").value;
  if (!id) return;
  if (confirm("Delete?")) {
    await fetch(`/api/delete?id=${id}`, { method: "POST" });
    location.reload();
  }
}

async function generateMetadata() {
  const content = document.getElementById("raw-injector").value;
  if (!content || content.trim().length < 50) {
    alert("Please add content first (min 50 chars)");
    return;
  }

  showStatus("GENERATING...", "var(--accent)");

  try {
    const formData = new FormData();
    formData.append("content", content);
    const res = await fetch("/api/generate-metadata", {
      method: "POST",
      body: formData,
    });

    if (!res.ok) throw new Error(await res.text());

    const metadata = await res.text();
    try {
      JSON.parse(metadata);
    } catch (e) {
      alert("Invalid JSON from AI");
      return;
    }

    document.getElementById("entry-meta").value = metadata;
    showStatus("METADATA_GENERATED", "var(--accent)");
  } catch (e) {
    showStatus("ERROR", "var(--danger)");
    alert("Error: " + e.message);
  } finally {
    setTimeout(() => showStatus("SYSTEM_ACTIVE", "var(--text-mute)"), 2000);
  }
}

function showStatus(msg, color) {
  const status = document.getElementById("system-status");
  if (status) {
    status.innerText = msg;
    status.style.color = color;
  }
}

// Make functions global for inline onclick handlers (legacy support)
window.switchTab = switchTab;
window.loadEntry = loadEntry;
window.saveEntry = saveEntry;
window.deleteEntry = deleteEntry;
window.generateMetadata = generateMetadata;

// --- DATABASE MATRIX ---
async function loadTable() {
  const table = document.getElementById("db-table-select").value;
  const filter = document.getElementById("db-query-input").value;
  document.getElementById("db-status").innerText = "LOADING...";
  let url = `/api/db/table/${table}`;
  if (filter) url += `?filter=${encodeURIComponent(filter)}`;
  try {
    const res = await fetch(url);
    const data = await res.json();
    renderMatrix(data, table);
    document.getElementById("db-status").innerText = `ROWS: ${data.length}`;
  } catch (e) {
    document.getElementById("db-status").innerText = "ERROR";
  }
}

function renderMatrix(data, table) {
  const head = document.getElementById("matrix-head");
  const body = document.getElementById("matrix-body");
  head.innerHTML = "";
  body.innerHTML = "";
  if (!data.length) return;
  let keys = Object.keys(data[0]).sort((a, b) => {
    const al = a.toLowerCase(),
      bl = b.toLowerCase();
    const isIdA = al.includes("id");
    const isIdB = bl.includes("id");
    if (isIdA && !isIdB) return -1;
    if (!isIdA && isIdB) return 1;
    return a.localeCompare(b);
  });
  const tr = document.createElement("tr");
  keys.forEach((k) => {
    const th = document.createElement("th");
    th.innerText = k;
    tr.appendChild(th);
  });
  head.appendChild(tr);
  data.forEach((row) => {
    const tr = document.createElement("tr");
    let id = row.id || row.jobId || row.job_id || row.jobid;
    let idCol = row.jobid ? "jobid" : row.jobId ? "jobId" : "id";
    keys.forEach((k) => {
      const td = document.createElement("td");
      let cellValue = row[k] !== null ? String(row[k]) : "";
      if (cellValue.length > 50) {
        td.innerText = cellValue.substring(0, 50) + "...";
        td.title = cellValue;
      } else {
        td.innerText = cellValue;
        if (cellValue.length > 0) td.title = cellValue;
      }
      if (k === idCol || k === "id") td.style.color = "var(--accent)";
      else {
        td.onclick = () => {
          if (td.classList.contains("editing")) return;
          makeEditable(td, row, k, table, id, idCol);
        };
      }
      tr.appendChild(td);
    });
    body.appendChild(tr);
  });
}

function makeEditable(td, row, col, table, id, idCol) {
  const oldVal = td.innerText;
  td.innerHTML = "";
  td.classList.add("editing");
  const input = document.createElement(
    oldVal.length > 50 ? "textarea" : "input"
  );
  input.value = oldVal;
  const save = async () => {
    const newVal = input.value;
    if (newVal === oldVal) {
      td.innerHTML = oldVal;
      td.classList.remove("editing");
      return;
    }
    const params = new URLSearchParams();
    params.append("table", table);
    params.append("id", id);
    params.append("id_col", idCol);
    params.append("col", col);
    params.append("val", newVal);
    try {
      const res = await fetch("/api/db/update", {
        method: "POST",
        body: params,
      });
      if (res.ok) td.innerText = newVal;
      else {
        td.innerText = oldVal;
        alert("Save Failed");
      }
    } catch (e) {
      td.innerText = oldVal;
    }
    td.classList.remove("editing");
  };
  input.onblur = save;
  if (input.tagName === "INPUT")
    input.onkeydown = (e) => {
      if (e.key === "Enter") input.blur();
    };
  td.appendChild(input);
  input.focus();
}

// --- TERMINAL ---
function initTerminal() {
  if (window.terminal) return;
  window.terminal = new Terminal({
    cursorBlink: true,
    fontFamily: "Menlo, Consolas, monospace",
    fontSize: 13,
    theme: { background: "#000", foreground: "#0f0" },
  });
  window.fitAddon = new FitAddon.FitAddon();
  window.terminal.loadAddon(window.fitAddon);
  window.terminal.open(document.getElementById("terminal-container"));
  window.fitAddon.fit();
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  window.socket = new WebSocket(`${proto}//${location.host}/api/terminal`);
  window.socket.onmessage = (e) => window.terminal.write(e.data);
  window.terminal.onData((d) =>
    window.socket.send(JSON.stringify({ type: "input", data: d }))
  );
  window.socket.onopen = () =>
    window.socket.send(
      JSON.stringify({
        type: "resize",
        cols: window.terminal.cols,
        rows: window.terminal.rows,
      })
    );
  window.addEventListener("resize", () => window.fitAddon.fit());
}

window.loadTable = loadTable;
window.initTerminal = initTerminal;
