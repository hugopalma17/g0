// Admin Panel Logic - g0 CMS

// Global editor reference
let editor = null;
let editorReady = false;

// Wait for dependencies
document.addEventListener("DOMContentLoaded", () => {
  const d = new Date();
  const dateInput = document.getElementById("entry-date");
  if (dateInput) dateInput.value = d.toISOString().split("T")[0];

  // Initialize Monaco Editor
  initMonacoEditor();
});

// --- Monaco Editor Setup ---
function initMonacoEditor() {
  const container = document.getElementById("monaco-container");
  if (!container) return;

  // Wait for Monaco to be available
  if (typeof require === "undefined") {
    setTimeout(initMonacoEditor, 100);
    return;
  }

  require(["vs/editor/editor.main"], function () {
    // Define custom dark theme matching admin panel
    monaco.editor.defineTheme("g0-dark", {
      base: "vs-dark",
      inherit: true,
      rules: [
        { token: "comment", foreground: "6A737D" },
        { token: "keyword", foreground: "60a5fa" },
        { token: "string", foreground: "22c55e" },
        { token: "number", foreground: "f59e0b" },
        { token: "tag", foreground: "60a5fa" },
        { token: "attribute.name", foreground: "c084fc" },
        { token: "attribute.value", foreground: "22c55e" },
      ],
      colors: {
        "editor.background": "#0a0a0a",
        "editor.foreground": "#e5e5e5",
        "editor.lineHighlightBackground": "#1a1a1a",
        "editor.selectionBackground": "#3b82f640",
        "editorCursor.foreground": "#3b82f6",
        "editorLineNumber.foreground": "#525252",
        "editorLineNumber.activeForeground": "#a3a3a3",
        "editor.inactiveSelectionBackground": "#3b82f620",
      },
    });

    editor = monaco.editor.create(container, {
      value: "",
      language: "html",
      theme: "g0-dark",
      automaticLayout: true,
      fontSize: 13,
      fontFamily: "'JetBrains Mono', 'Menlo', 'Consolas', monospace",
      lineNumbers: "on",
      minimap: { enabled: false },
      scrollBeyondLastLine: false,
      wordWrap: "on",
      tabSize: 2,
      insertSpaces: true,
      renderWhitespace: "selection",
      bracketPairColorization: { enabled: true },
      padding: { top: 10, bottom: 10 },
      smoothScrolling: true,
      cursorBlinking: "smooth",
      cursorSmoothCaretAnimation: "on",
    });

    // Sync to hidden textarea and update preview on change
    editor.onDidChangeModelContent(() => {
      const content = editor.getValue();
      document.getElementById("raw-injector").value = content;
      updatePreview();
    });

    // Store reference globally
    window.monacoEditor = editor;
    editorReady = true;

    // If there's already content in the textarea, load it
    const existingContent = document.getElementById("raw-injector").value;
    if (existingContent) {
      editor.setValue(existingContent);
    }
  });
}

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

  // Refresh Monaco layout when switching to editor tab
  if (tab === "editor" && editor) {
    setTimeout(() => editor.layout(), 100);
  }
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

  // Set content in Monaco editor
  if (editor && editorReady) {
    editor.setValue(entry.content || "");
  } else {
    document.getElementById("raw-injector").value = entry.content || "";
  }

  updatePreview();
}

function resetForm() {
  document.getElementById("entry-id").value = "";
  document.getElementById("entry-title").value = "";
  document.getElementById("entry-meta").value = "";
  document.getElementById("live-preview").innerHTML = "";

  if (editor && editorReady) {
    editor.setValue("");
  } else {
    document.getElementById("raw-injector").value = "";
  }

  updatePreview();
}

function updatePreview() {
  const content = editor
    ? editor.getValue()
    : document.getElementById("raw-injector").value;
  const preview = document.getElementById("live-preview");
  if (preview) {
    preview.innerHTML = content;

    // Execute inline scripts for Chart.js etc
    const scripts = preview.querySelectorAll("script");
    scripts.forEach((oldScript) => {
      const newScript = document.createElement("script");
      Array.from(oldScript.attributes).forEach((attr) => {
        newScript.setAttribute(attr.name, attr.value);
      });
      newScript.textContent = oldScript.textContent;
      oldScript.parentNode.replaceChild(newScript, oldScript);
    });

    // Trigger Prism syntax highlighting
    if (typeof Prism !== "undefined") {
      Prism.highlightAllUnder(preview);
    }
  }
}

function getEditorContent() {
  if (editor && editorReady) {
    return editor.getValue();
  }
  return document.getElementById("raw-injector").value;
}

async function saveEntry() {
  const id = document.getElementById("entry-id").value;
  const formData = new FormData();
  formData.append("id", id);
  formData.append("type", document.getElementById("entry-type").value);
  formData.append("title", document.getElementById("entry-title").value);
  formData.append("date", document.getElementById("entry-date").value);
  formData.append("metadata", document.getElementById("entry-meta").value);
  formData.append("content", getEditorContent());

  try {
    const res = await fetch("/api/save", {
      method: "POST",
      body: formData,
      headers: { "HX-Request": "true" },
    });
    const text = await res.text();
    if (text.includes("PROTOCOL") || res.ok) {
      showStatus("Saved", "var(--success, var(--accent))");
      setTimeout(() => {
        showStatus("Ready", "var(--text-mute)");
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
  if (confirm("Delete this entry?")) {
    await fetch(`/api/delete?id=${id}`, { method: "POST" });
    location.reload();
  }
}

async function generateMetadata() {
  const content = getEditorContent();
  if (!content || content.trim().length < 50) {
    alert("Please add content first (min 50 chars)");
    return;
  }

  showStatus("Generating...", "var(--accent)");

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
    showStatus("Metadata generated", "var(--success, var(--accent))");
  } catch (e) {
    showStatus("Error", "var(--danger)");
    alert("Error: " + e.message);
  } finally {
    setTimeout(() => showStatus("Ready", "var(--text-mute)"), 2000);
  }
}

function showStatus(msg, color) {
  const status = document.getElementById("system-status");
  if (status) {
    status.innerText = msg;
    status.style.color = color;
  }
}

// Make functions global for inline onclick handlers
window.switchTab = switchTab;
window.loadEntry = loadEntry;
window.saveEntry = saveEntry;
window.deleteEntry = deleteEntry;
window.generateMetadata = generateMetadata;

// --- DATABASE VIEWER ---
async function loadTable() {
  const table = document.getElementById("db-table-select").value;
  const filter = document.getElementById("db-query-input").value;
  document.getElementById("db-status").innerText = "Loading...";
  let url = `/api/db/table/${table}`;
  if (filter) url += `?filter=${encodeURIComponent(filter)}`;
  try {
    const res = await fetch(url);
    const data = await res.json();
    renderMatrix(data, table);
    document.getElementById("db-status").innerText = `${data.length} rows`;
  } catch (e) {
    document.getElementById("db-status").innerText = "Error";
  }
}

function renderMatrix(data, table) {
  const head = document.getElementById("matrix-head");
  const body = document.getElementById("matrix-body");
  head.innerHTML = "";
  body.innerHTML = "";
  if (!data.length) {
    body.innerHTML =
      '<tr><td colspan="100" style="text-align: center; color: var(--text-mute); padding: 20px;">No data. Click "+ Row" to add an entry.</td></tr>';
    return;
  }

  let keys = Object.keys(data[0]).sort((a, b) => {
    const al = a.toLowerCase(),
      bl = b.toLowerCase();
    const isIdA = al.includes("id");
    const isIdB = bl.includes("id");
    if (isIdA && !isIdB) return -1;
    if (!isIdA && isIdB) return 1;
    return a.localeCompare(b);
  });

  // Add header row with extra column for actions
  const headerRow = document.createElement("tr");
  const actionTh = document.createElement("th");
  actionTh.innerText = "";
  actionTh.style.width = "40px";
  headerRow.appendChild(actionTh);

  keys.forEach((k) => {
    const th = document.createElement("th");
    th.innerText = k;
    headerRow.appendChild(th);
  });
  head.appendChild(headerRow);

  data.forEach((row) => {
    const tr = document.createElement("tr");
    let id = row.id || row.jobId || row.job_id || row.jobid;
    let idCol = row.jobid ? "jobid" : row.jobId ? "jobId" : "id";

    // Add delete button cell
    const actionTd = document.createElement("td");
    actionTd.style.textAlign = "center";
    const deleteBtn = document.createElement("button");
    deleteBtn.className = "row-delete-btn";
    deleteBtn.innerHTML = '<i class="fa-solid fa-trash"></i>';
    deleteBtn.title = "Delete row";
    deleteBtn.onclick = (e) => {
      e.stopPropagation();
      deleteRow(table, id);
    };
    actionTd.appendChild(deleteBtn);
    tr.appendChild(actionTd);

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

      if (k === idCol || k === "id") {
        td.style.color = "var(--accent)";
      } else {
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
    oldVal.length > 50 ? "textarea" : "input",
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
      if (res.ok) {
        td.innerText = newVal;
      } else {
        td.innerText = oldVal;
        alert("Save Failed");
      }
    } catch (e) {
      td.innerText = oldVal;
    }
    td.classList.remove("editing");
  };

  input.onblur = save;
  if (input.tagName === "INPUT") {
    input.onkeydown = (e) => {
      if (e.key === "Enter") input.blur();
    };
  }

  td.appendChild(input);
  input.focus();
}

// --- TERMINAL ---
function initTerminal() {
  if (window.terminal) return;

  window.terminal = new Terminal({
    cursorBlink: true,
    fontFamily: "'JetBrains Mono', Menlo, Consolas, monospace",
    fontSize: 13,
    theme: {
      background: "#0a0a0a",
      foreground: "#e5e5e5",
      cursor: "#3b82f6",
      selectionBackground: "#3b82f640",
    },
  });

  window.fitAddon = new FitAddon.FitAddon();
  window.terminal.loadAddon(window.fitAddon);
  window.terminal.open(document.getElementById("terminal-container"));
  window.fitAddon.fit();

  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  window.socket = new WebSocket(`${proto}//${location.host}/api/terminal`);

  window.socket.onmessage = (e) => window.terminal.write(e.data);
  window.terminal.onData((d) =>
    window.socket.send(JSON.stringify({ type: "input", data: d })),
  );

  window.socket.onopen = () =>
    window.socket.send(
      JSON.stringify({
        type: "resize",
        cols: window.terminal.cols,
        rows: window.terminal.rows,
      }),
    );

  window.addEventListener("resize", () => window.fitAddon.fit());
}

window.loadTable = loadTable;
window.initTerminal = initTerminal;

// --- DATABASE MANAGEMENT ---

// Add new row
async function addRow() {
  const table = document.getElementById("db-table-select").value;
  document.getElementById("db-status").innerText = "Adding row...";

  try {
    const params = new URLSearchParams();
    params.append("table", table);

    const res = await fetch("/api/db/insert", {
      method: "POST",
      body: params,
    });

    if (res.ok) {
      document.getElementById("db-status").innerText = "Row added";
      loadTable();
    } else {
      const err = await res.text();
      alert("Error: " + err);
      document.getElementById("db-status").innerText = "Error";
    }
  } catch (e) {
    alert("Network error");
    document.getElementById("db-status").innerText = "Error";
  }
}

// Delete row
async function deleteRow(table, id) {
  if (!confirm(`Delete row ${id}?`)) return;

  document.getElementById("db-status").innerText = "Deleting...";

  try {
    const params = new URLSearchParams();
    params.append("table", table);
    params.append("id", id);

    const res = await fetch("/api/db/delete-row", {
      method: "POST",
      body: params,
    });

    if (res.ok) {
      document.getElementById("db-status").innerText = "Row deleted";
      loadTable();
    } else {
      const err = await res.text();
      alert("Error: " + err);
      document.getElementById("db-status").innerText = "Error";
    }
  } catch (e) {
    alert("Network error");
    document.getElementById("db-status").innerText = "Error";
  }
}

// Show add column modal
function showAddColumnModal() {
  document.getElementById("modal-add-column").classList.remove("hidden");
  document.getElementById("new-col-name").value = "";
  document.getElementById("new-col-name").focus();
}

// Add column
async function addColumn() {
  const table = document.getElementById("db-table-select").value;
  const colName = document.getElementById("new-col-name").value.trim();
  const colType = document.getElementById("new-col-type").value;

  if (!colName) {
    alert("Please enter a column name");
    return;
  }

  document.getElementById("db-status").innerText = "Adding column...";

  try {
    const params = new URLSearchParams();
    params.append("table", table);
    params.append("column", colName);
    params.append("type", colType);

    const res = await fetch("/api/db/add-column", {
      method: "POST",
      body: params,
    });

    if (res.ok) {
      document.getElementById("db-status").innerText = "Column added";
      hideModals();
      loadTable();
    } else {
      const err = await res.text();
      alert("Error: " + err);
      document.getElementById("db-status").innerText = "Error";
    }
  } catch (e) {
    alert("Network error");
    document.getElementById("db-status").innerText = "Error";
  }
}

// Show SQL modal
function showSqlModal() {
  document.getElementById("modal-sql").classList.remove("hidden");
  document.getElementById("sql-query").value = "";
  document.getElementById("sql-result").classList.add("hidden");
  document.getElementById("sql-query").focus();
}

// Execute SQL
async function executeSql() {
  const query = document.getElementById("sql-query").value.trim();
  const resultDiv = document.getElementById("sql-result");

  if (!query) {
    alert("Please enter a query");
    return;
  }

  resultDiv.classList.remove("hidden", "error", "success");
  resultDiv.innerText = "Executing...";

  try {
    const params = new URLSearchParams();
    params.append("query", query);

    const res = await fetch("/api/db/exec", {
      method: "POST",
      body: params,
    });

    if (res.ok) {
      const data = await res.json();
      resultDiv.classList.add("success");

      if (data.type === "select") {
        if (data.rows && data.rows.length > 0) {
          resultDiv.innerText = JSON.stringify(data.rows, null, 2);
        } else {
          resultDiv.innerText = "No results";
        }
      } else {
        resultDiv.innerText = `Query executed. Rows affected: ${data.rowsAffected}`;
        loadTable(); // Refresh table after modification
      }
    } else {
      const err = await res.text();
      resultDiv.classList.add("error");
      resultDiv.innerText = "Error: " + err;
    }
  } catch (e) {
    resultDiv.classList.add("error");
    resultDiv.innerText = "Network error: " + e.message;
  }
}

// Hide all modals
function hideModals() {
  document.querySelectorAll(".modal").forEach((m) => m.classList.add("hidden"));
}

// Close modals on escape key
document.addEventListener("keydown", (e) => {
  if (e.key === "Escape") hideModals();
});

// Close modals on backdrop click
document.addEventListener("click", (e) => {
  if (e.target.classList.contains("modal")) {
    hideModals();
  }
});

// Make database functions global
window.addRow = addRow;
window.deleteRow = deleteRow;
window.showAddColumnModal = showAddColumnModal;
window.addColumn = addColumn;
window.showSqlModal = showSqlModal;
window.executeSql = executeSql;
window.hideModals = hideModals;
