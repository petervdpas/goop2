// Database — Tables tab
(() => {
  var G = window.Goop;
  if (!G || !G.core) return;
  var qs = G.core.qs, qsa = G.core.qsa, on = G.core.on;
  var setHidden = G.core.setHidden, escapeHtml = G.core.escapeHtml, toast = G.core.toast;
  var safeLocalStorageGet = G.core.safeLocalStorageGet, safeLocalStorageSet = G.core.safeLocalStorageSet;
  var api = G.api.data;
  var gsel = G.select;

  if (!qs("#db-page")) return;
  G.database = G.database || {};

// DOM refs
const tableListEl  = qs("#db-table-list");
const tableTitleEl  = qs("#db-table-title");
const actionsEl     = qs("#db-content-actions");
const createFormEl  = qs("#db-create-form");
const alterFormEl   = qs("#db-alter-form");
const insertFormEl  = qs("#db-insert-form");
const gridEl        = qs("#db-grid");
const btnNew        = qs("#db-btn-new-table");
const btnInsert     = qs("#db-btn-insert");
const btnRefresh    = qs("#db-btn-refresh");
const btnDrop       = qs("#db-btn-delete-table");
const tableSubtabs  = qs("#db-table-subtabs");
const alterPane     = qs("#db-table-alter-pane");
const dataPane      = qs("#db-table-data-pane");
const searchBarEl    = qs("#db-search-bar");
const searchColEl    = qs("#db-search-col");
const searchInputEl  = qs("#db-search-input");
const searchClearEl  = qs("#db-search-clear");
const colPickerBtn   = qs("#db-btn-columns");
const colPickerEl    = qs("#db-col-picker");

// State
let currentTable = null;
let currentPolicy = "owner"; // insert_policy of current table
let tablesMeta = {};   // name -> { insert_policy }
let columns = [];      // ColumnInfo[] from describe
let systemCols = ["_id", "_owner", "_owner_email", "_created_at", "_updated_at"];
let defaultHidden = ["_owner", "_owner_email", "_created_at", "_updated_at"];  // hidden by default in grid
let hiddenCols = new Set();
let searchTimer = null;
let pageSize = 50;
let currentOffset = 0;
let hasMore = true;
let loadingMore = false;
let lastRows = [];      // cached rows for column toggle re-render

// Column visibility persistence
function colVisKey(table) { return "db-hidden-cols:" + table; }

function loadHiddenCols(table) {
  var raw = safeLocalStorageGet(colVisKey(table));
  try {
    if (raw) return new Set(JSON.parse(raw));
  } catch (e) { /* ignore */ }
  return new Set(defaultHidden);
}

function saveHiddenCols(table) {
  safeLocalStorageSet(colVisKey(table), JSON.stringify(Array.from(hiddenCols)));
}

function visibleCols() {
  return columns.filter(function(c) { return !hiddenCols.has(c.name); });
}

// -------- Sub-tab switching --------
function switchTableView(view) {
  qsa(".sub-tab", tableSubtabs).forEach(function(t) {
    t.classList.toggle("active", t.dataset.tableView === view);
  });
  setHidden(alterPane, view !== "alter");
  setHidden(dataPane, view !== "data");
  setHidden(btnInsert, view !== "data");
  setHidden(btnRefresh, view !== "data");
}

qsa(".sub-tab", tableSubtabs).forEach(function(tab) {
  on(tab, "click", function() {
    switchTableView(tab.dataset.tableView);
  });
});

// -------- Table list --------
async function loadTables(selectName) {
  try {
    const tables = await api.tables() || [];
    // Cache table metadata
    tablesMeta = {};
    tables.forEach(function(t) { tablesMeta[t.name] = { insert_policy: t.insert_policy || "owner" }; });
    renderTableList(tables);
    // Auto-select
    if (selectName) {
      selectTable(selectName).then(function() { switchTableView("alter"); });
    } else if (tables.length > 0 && !currentTable) {
      selectTable(tables[0].name).then(function() { switchTableView("alter"); });
    } else if (currentTable) {
      // Re-highlight current
      highlightActive(currentTable);
    }
  } catch (err) {
    tableListEl.innerHTML = '<li class="db-table-empty">Failed to load tables</li>';
  }
}

var policyLabels = { owner: "owner", email: "email", open: "open" };

function renderTableList(tables) {
  if (!tables || tables.length === 0) {
    tableListEl.innerHTML = '<li class="db-table-empty">No tables yet</li>';
    return;
  }
  tableListEl.innerHTML = "";
  tables.forEach(function(t) {
    var policy = t.insert_policy || "owner";
    var mode = t.mode || "classic";
    const li = document.createElement("li");
    li.className = "sidebar-item";
    li.dataset.table = t.name;
    var badgeClass = mode === "orm" ? "badge-email" : "badge-" + policy;
    var badgeText = mode === "orm" ? "ORM" : (policyLabels[policy] || policy);
    var badgeTitle = mode === "orm" ? "ORM table (typed schema)" : "Insert policy: " + policy;
    li.innerHTML = '<span class="db-table-name">' + escapeHtml(t.name) + '</span>' +
      '<span class="badge ' + badgeClass + '" title="' + badgeTitle + '">' + badgeText + '</span>';
    on(li, "click", function() { selectTable(t.name).then(function() { switchTableView("alter"); }); });
    tableListEl.appendChild(li);
  });
}

function highlightActive(name) {
  qsa(".sidebar-item", tableListEl).forEach(function(el) {
    el.classList.toggle("active", el.dataset.table === name);
  });
}

// -------- Select table --------
async function selectTable(name) {
  currentTable = name;
  currentPolicy = (tablesMeta[name] && tablesMeta[name].insert_policy) || "owner";
  hiddenCols = loadHiddenCols(name);
  highlightActive(name);
  tableTitleEl.innerHTML = escapeHtml(name) +
    ' <span class="badge badge-' + currentPolicy + '">' + (policyLabels[currentPolicy] || currentPolicy) + '</span>';
  setHidden(actionsEl, false);
  setHidden(createFormEl, true);
  setHidden(insertFormEl, true);
  setHidden(tableSubtabs, false);

  try {
    // Fetch schema and first page in parallel
    const [desc, rows] = await Promise.all([
      api.describeTable({ table: name }),
      api.query({ table: name, limit: pageSize, offset: 0 }),
    ]);
    columns = (desc && desc.schema && desc.schema.columns) || (desc && desc.columns) || [];
    currentOffset = 0;
    hasMore = (rows || []).length >= pageSize;
    populateSearchBar();
    renderColPicker();
    renderDataGrid(rows || [], false);
    showAlterForm();
  } catch (err) {
    gridEl.innerHTML = '<p class="empty-state">Error loading table: ' + escapeHtml(err.message) + '</p>';
  }
}

// -------- Search / filter --------
gsel.init(searchColEl, function() { executeSearch(); });

function populateSearchBar() {
  if (columns.length === 0) {
    setHidden(searchBarEl, true);
    return;
  }
  setHidden(searchBarEl, false);
  var opts = [{ value: "*", label: "All columns" }];
  columns.forEach(function(col) {
    opts.push({ value: col.name, label: col.name });
  });
  gsel.setOpts(searchColEl, { options: opts }, "*");
  searchInputEl.value = "";
}

// -------- Column picker --------
function renderColPicker() {
  if (columns.length === 0) return;
  var html = "";
  columns.forEach(function(col) {
    var checked = !hiddenCols.has(col.name) ? " checked" : "";
    var isSys = systemCols.indexOf(col.name) !== -1;
    var cls = isSys ? "db-colpick-name db-colpick-sys" : "db-colpick-name";
    html += '<label><input type="checkbox" value="' + escapeHtml(col.name) + '"' + checked + ' />' +
      '<span class="' + cls + '">' + escapeHtml(col.name) + '</span></label>';
  });
  colPickerEl.innerHTML = html;

  // Bind change events
  qsa("input[type=checkbox]", colPickerEl).forEach(function(cb) {
    on(cb, "change", function() {
      if (cb.checked) {
        hiddenCols.delete(cb.value);
      } else {
        hiddenCols.add(cb.value);
      }
      saveHiddenCols(currentTable);
      // Re-render grid from cached rows (no server round-trip)
      renderDataGrid(lastRows, false);
    });
  });
}

function toggleColPicker() {
  colPickerEl.classList.toggle("hidden");
}

// Close picker on outside click
document.addEventListener("click", function(e) {
  if (!colPickerEl.classList.contains("hidden") &&
      !colPickerEl.contains(e.target) &&
      e.target !== colPickerBtn) {
    colPickerEl.classList.add("hidden");
  }
});

function applyFilter() {
  if (searchTimer) clearTimeout(searchTimer);
  searchTimer = setTimeout(executeSearch, 250);
}

function buildSearchBody(offset) {
  var query = (searchInputEl.value || "").trim();
  var col = gsel.val(searchColEl);

  var reqBody = { table: currentTable, limit: pageSize, offset: offset };

  if (query) {
    var pattern = "%" + query + "%";
    if (col === "*") {
      var clauses = [];
      var args = [];
      columns.forEach(function(c) {
        clauses.push("CAST(" + c.name + " AS TEXT) LIKE ?");
        args.push(pattern);
      });
      reqBody.where = clauses.join(" OR ");
      reqBody.args = args;
    } else {
      reqBody.where = "CAST(" + col + " AS TEXT) LIKE ?";
      reqBody.args = [pattern];
    }
  }

  return reqBody;
}

async function executeSearch() {
  if (!currentTable) return;

  currentOffset = 0;
  try {
    var rows = await api.query(buildSearchBody(0));
    hasMore = (rows || []).length >= pageSize;
    renderDataGrid(rows || [], false);
  } catch (err) {
    gridEl.innerHTML = '<p class="empty-state">Search error: ' + escapeHtml(err.message) + '</p>';
  }
}

async function loadMore() {
  if (!currentTable || !hasMore || loadingMore) return;
  loadingMore = true;

  var nextOffset = currentOffset + pageSize;
  try {
    var rows = await api.query(buildSearchBody(nextOffset));
    if (!rows || rows.length === 0) {
      hasMore = false;
    } else {
      currentOffset = nextOffset;
      hasMore = rows.length >= pageSize;
      renderDataGrid(rows, true);
    }
  } catch (err) {
    toast("Load failed: " + err.message, true);
  }
  loadingMore = false;
}

// -------- Data grid (div-based CSS grid) --------
function gridTemplate() {
  var vc = visibleCols();
  var cols = [];
  vc.forEach(function(col) {
    if (col.name === '_id') cols.push('60px');
    else if (col.name === '_created_at' || col.name === '_updated_at') cols.push('170px');
    else if (col.name === '_owner' || col.name === '_owner_email') cols.push('140px');
    else cols.push('minmax(80px, 1fr)');
  });
  cols.push('40px');
  return cols.join(' ');
}

function buildRowHtml(row) {
  var vc = visibleCols();
  var rowId = row._id;
  var html = '<div class="dg-row" data-row-id="' + rowId + '">';
  vc.forEach(function(col) {
    var val = row[col.name];
    var isSystem = systemCols.indexOf(col.name) !== -1;
    var isNull = val === null || val === undefined;

    if (isSystem) {
      html += '<div class="dg-cell db-cell-system">';
      html += isNull ? '<span class="db-cell-null">NULL</span>' :
        '<span class="db-cell-truncate" title="' + escapeHtml(val) + '">' + escapeHtml(val) + '</span>';
      html += '</div>';
    } else {
      html += '<div class="dg-cell db-cell-editable" data-col="' + escapeHtml(col.name) + '" data-row-id="' + rowId + '">';
      html += isNull ? '<span class="db-cell-null">NULL</span>' : escapeHtml(val);
      html += '</div>';
    }
  });
  html += '<div class="dg-cell db-row-actions"><button class="db-row-delete" data-row-id="' + rowId + '" title="Delete row">x</button></div>';
  html += '</div>';
  return html;
}

function bindRowEvents(container) {
  qsa(".db-cell-editable:not([data-bound])", container).forEach(function(cell) {
    cell.dataset.bound = "1";
    on(cell, "click", function() { startEdit(cell); });
  });
  qsa(".db-row-delete:not([data-bound])", container).forEach(function(btn) {
    btn.dataset.bound = "1";
    on(btn, "click", function(e) {
      e.stopPropagation();
      deleteRow(parseInt(btn.dataset.rowId, 10));
    });
  });
}

function renderDataGrid(rows, append) {
  if (columns.length === 0) {
    gridEl.innerHTML = '<p class="empty-state">No columns found.</p>';
    return;
  }

  var gt = gridTemplate();

  if (!append) {
    lastRows = rows || [];
  } else if (rows && rows.length > 0) {
    lastRows = lastRows.concat(rows);
  }

  if (append) {
    var body = qs(".dg-body", gridEl);
    if (body && rows && rows.length > 0) {
      var frag = document.createElement("div");
      var html = "";
      rows.forEach(function(row) { html += buildRowHtml(row); });
      frag.innerHTML = html;
      while (frag.firstChild) {
        body.appendChild(frag.firstChild);
      }
      bindRowEvents(body);
    }
    return;
  }

  var vc = visibleCols();
  var html = '<div class="dg-header" style="grid-template-columns:' + gt + '">';
  vc.forEach(function(col) {
    html += '<div class="dg-th">' + escapeHtml(col.name) + '</div>';
  });
  html += '<div class="dg-th"></div></div>';

  html += '<div class="dg-body">';
  if (!rows || rows.length === 0) {
    html += '<div class="empty-state" style="grid-column:1/-1">No rows. Click "+ Row" to add data.</div>';
  } else {
    rows.forEach(function(row) { html += buildRowHtml(row); });
  }
  html += '</div>';

  gridEl.innerHTML = html;
  gridEl.style.setProperty("--dg-cols", gt);
  bindRowEvents(gridEl);
}

// -------- Inline editing --------
function generateFallbackGUID() {
  var d = Date.now();
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, function(c) {
    var r = (d + Math.random() * 16) % 16 | 0;
    d = Math.floor(d / 16);
    return (c === "x" ? r : (r & 0x3 | 0x8)).toString(16);
  });
}

function colType(colName) {
  for (var i = 0; i < columns.length; i++) {
    if (columns[i].name === colName) return (columns[i].type || "text").toLowerCase();
  }
  return "text";
}

function colAuto(colName) {
  for (var i = 0; i < columns.length; i++) {
    if (columns[i].name === colName) return !!columns[i].auto;
  }
  return false;
}

function startEdit(td) {
  if (td.classList.contains("db-cell-editing")) return;

  var colName = td.dataset.col;
  var rowId = parseInt(td.dataset.rowId, 10);
  var nullSpan = qs(".db-cell-null", td);
  var oldValue = nullSpan ? "" : td.textContent;
  var ct = colType(colName);

  if (ct === "enum") {
    var enumCol = null;
    for (var ei = 0; ei < columns.length; ei++) {
      if (columns[ei].name === colName) { enumCol = columns[ei]; break; }
    }
    var enumVals = (enumCol && enumCol.values) || [];
    if (enumVals.length === 0) return;

    td.classList.add("db-cell-editing");
    var enumOpts = enumVals.map(function(v) { return { value: v.key, label: v.label || v.key }; });
    var selDiv = document.createElement("div");
    selDiv.innerHTML = gsel.html({ className: "db-cell-enum-sel", value: oldValue, options: enumOpts });
    td.textContent = "";
    td.appendChild(selDiv.firstElementChild);
    var selEl = qs(".db-cell-enum-sel", td);
    gsel.init(selEl, function(val) {
      td.classList.remove("db-cell-editing");
      if (val !== oldValue) {
        commitEdit(td, rowId, colName, val, oldValue);
      } else {
        restoreCell(td, oldValue);
      }
    });
    qs(".gsel-trigger", selEl).click();
    return;
  }

  if (ct === "guid") {
    Goop.dialog.confirm("Regenerate GUID for " + colName + "?", "Regenerate").then(function(ok) {
      if (!ok) return;
      var newGuid = crypto.randomUUID ? crypto.randomUUID() : generateFallbackGUID();
      commitEdit(td, rowId, colName, newGuid, oldValue);
    });
    return;
  }

  if (ct === "datetime" || ct === "date") {
    td.classList.add("db-cell-editing");
    var dateInput = document.createElement("input");
    dateInput.type = "text";
    dateInput.value = oldValue ? oldValue.substring(0, 10) : "";
    td.textContent = "";
    td.appendChild(dateInput);
    Goop.datepicker.attach(dateInput);
    dateInput.addEventListener("change", function() {
      var val = dateInput.value;
      if (ct === "datetime" && val) val = val + "T00:00:00Z";
      td.classList.remove("db-cell-editing");
      if (val !== oldValue) {
        commitEdit(td, rowId, colName, val, oldValue);
      } else {
        restoreCell(td, oldValue);
      }
    });
    dateInput.click();
    return;
  }

  if (ct === "time") {
    td.classList.add("db-cell-editing");
    var timeInput = document.createElement("input");
    timeInput.type = "text";
    timeInput.value = oldValue || "";
    td.textContent = "";
    td.appendChild(timeInput);
    Goop.timepicker.attach(timeInput);
    timeInput.addEventListener("change", function() {
      var val = timeInput.value;
      td.classList.remove("db-cell-editing");
      if (val !== oldValue) {
        commitEdit(td, rowId, colName, val, oldValue);
      } else {
        restoreCell(td, oldValue);
      }
    });
    timeInput.click();
    return;
  }

  td.classList.add("db-cell-editing");
  var input = document.createElement("input");
  if (ct === "integer") input.type = "number";
  else if (ct === "real") { input.type = "number"; input.step = "any"; }
  else input.type = "text";
  input.value = oldValue;
  td.textContent = "";
  td.appendChild(input);

  input.focus();
  input.select();

  function commit() {
    var newValue = input.value;
    cleanup();
    if (newValue !== oldValue) {
      commitEdit(td, rowId, colName, newValue, oldValue);
    } else {
      restoreCell(td, oldValue);
    }
  }

  function cancel() {
    cleanup();
    restoreCell(td, oldValue);
  }

  function cleanup() {
    input.removeEventListener("blur", onBlur);
    input.removeEventListener("keydown", onKey);
  }

  function onBlur() {
    commit();
  }

  function onKey(e) {
    if (e.key === "Enter") {
      e.preventDefault();
      commit();
    } else if (e.key === "Escape") {
      e.preventDefault();
      cancel();
    }
  }

  on(input, "blur", onBlur);
  on(input, "keydown", onKey);
}

async function commitEdit(td, rowId, colName, newValue, oldValue) {
  td.classList.remove("db-cell-editing");
  td.textContent = newValue || "";

  try {
    var data = {};
    var ct = colType(colName);
    var isRequired = false;
    for (var ci = 0; ci < columns.length; ci++) {
      if (columns[ci].name === colName) { isRequired = !!columns[ci].required; break; }
    }

    if (newValue === "" || newValue === null) {
      if (isRequired) {
        restoreCell(td, oldValue);
        toast(colName + " is required", "warning");
        return;
      }
      data[colName] = null;
    } else if (ct === "integer") {
      var n = Number(newValue);
      if (isNaN(n) || newValue.trim() === "" || n !== Math.floor(n)) {
        restoreCell(td, oldValue);
        toast(colName + ": expected integer", "warning");
        return;
      }
      data[colName] = parseInt(newValue, 10);
    } else if (ct === "real") {
      var f = Number(newValue);
      if (isNaN(f) || newValue.trim() === "") {
        restoreCell(td, oldValue);
        toast(colName + ": expected number", "warning");
        return;
      }
      data[colName] = parseFloat(newValue);
    } else {
      data[colName] = newValue;
    }
    await api.update({ table: currentTable, id: rowId, data: data });
    // If value is null, render as NULL span
    if (newValue === "") {
      td.innerHTML = '<span class="db-cell-null">NULL</span>';
    }
  } catch (err) {
    restoreCell(td, oldValue);
    toast("Update failed: " + err.message, true);
  }
}

function restoreCell(td, value) {
  td.classList.remove("db-cell-editing");
  if (value === "" || value === null || value === undefined) {
    td.innerHTML = '<span class="db-cell-null">NULL</span>';
  } else {
    td.textContent = value;
  }
}

// -------- Delete row --------
async function deleteRow(rowId) {
  var answer = await Goop.dialog.confirmDanger({
    title: "Delete Row",
    message: "Delete row " + rowId + "?",
    match: "DELETE",
    okText: "Delete",
  });
  if (answer !== "DELETE") return;

  try {
    await api.delete({ table: currentTable, id: rowId });
    toast("Row deleted");
    selectTable(currentTable);
  } catch (err) {
    toast("Delete failed: " + err.message, true);
  }
}

// -------- Create table --------
function showCreateForm() {
  setHidden(createFormEl, false);
  setHidden(insertFormEl, true);

  createFormEl.innerHTML =
    '<h3>Create New Table</h3>' +
    '<div class="form-group">' +
      '<label>Table Name</label>' +
      '<input type="text" id="db-new-name" class="form-input" placeholder="my_table" />' +
    '</div>' +
    '<div class="form-group">' +
      '<label>Insert Policy</label>' +
      gsel.html({ id: "db-new-policy", value: "owner", options: policyOptions() }) +
      '<div class="hint">Controls who can insert rows into this table via P2P.</div>' +
    '</div>' +
    '<div class="form-group">' +
      '<label>Columns</label>' +
      '<div id="db-col-defs">' +
        colRowHtml() +
      '</div>' +
      '<button id="db-add-col" class="db-action-btn" style="margin-top:6px">+ Column</button>' +
    '</div>' +
    '<div class="form-actions">' +
      '<button id="db-create-cancel" class="db-action-btn">Cancel</button>' +
      '<button id="db-create-submit" class="db-action-btn" style="background:color-mix(in srgb,var(--accent) 22%,transparent);border-color:color-mix(in srgb,var(--accent) 40%,transparent)">Create</button>' +
    '</div>';

  initFormSelects(createFormEl);
  on(qs("#db-add-col"), "click", addColRow);
  on(qs("#db-create-cancel"), "click", function() { setHidden(createFormEl, true); });
  on(qs("#db-create-submit"), "click", submitCreateTable);

  // Bind remove on initial row
  bindColRemove();

  qs("#db-new-name").focus();
}

function colTypeOptions() {
  return [
    { value: "GUID", label: "GUID" },
    { value: "INTEGER", label: "INTEGER" },
    { value: "TEXT", label: "TEXT" },
    { value: "DATETIME", label: "DATETIME" },
    { value: "DATE", label: "DATE" },
    { value: "TIME", label: "TIME" },
    { value: "REAL", label: "REAL" },
    { value: "BLOB", label: "BLOB" },
  ];
}

function policyOptions() {
  return [
    { value: "owner", label: "owner \u2014 only site owner" },
    { value: "email", label: "email \u2014 peers with email" },
    { value: "open", label: "open \u2014 anyone" },
  ];
}

// Init all .gsel within a container
function initFormSelects(container) {
  container.querySelectorAll(".gsel").forEach(function(el) { gsel.init(el); });
}

function colRowHtml() {
  return '<div class="db-col-row">' +
    '<input type="text" class="form-input db-col-name" placeholder="column name" />' +
    gsel.html({ className: "db-col-type", value: "TEXT", options: colTypeOptions() }) +
    '<label class="db-col-notnull"><input type="checkbox" /> NOT NULL</label>' +
    '<button class="db-col-remove">x</button>' +
  '</div>';
}

function addColRow() {
  var container = qs("#db-col-defs");
  var div = document.createElement("div");
  div.innerHTML = colRowHtml();
  var row = div.firstElementChild;
  container.appendChild(row);
  initFormSelects(row);
  bindColRemove();
  qs(".db-col-name", row).focus();
}

function bindColRemove() {
  qsa(".db-col-remove", createFormEl).forEach(function(btn) {
    btn.onclick = function() {
      var container = qs("#db-col-defs");
      if (container && container.children.length > 1) {
        btn.closest(".db-col-row").remove();
      }
    };
  });
}

async function submitCreateTable() {
  var name = (qs("#db-new-name").value || "").trim();

  if (!name) { toast("Table name required", "warning"); return; }
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
    toast("Name must be letters, digits, underscore (start with letter or _)", "warning");
    return;
  }

  var cols = [];
  qsa("#db-col-defs .db-col-row").forEach(function(row) {
    var n = qs(".db-col-name", row).value.trim();
    var t = gsel.val(qs(".db-col-type", row));
    var nn = qs(".db-col-notnull input", row).checked;
    if (n) {
      cols.push({ name: n, type: t, not_null: nn });
    }
  });

  if (cols.length === 0) { toast("Add at least one column", "warning"); return; }

  var policy = gsel.val(qs("#db-new-policy")) || "owner";

  try {
    await api.createTable({ name: name, columns: cols });
    // Set the insert policy if not the default
    if (policy !== "owner") {
      await api.setPolicy({ table: name, policy: policy });
    }
    setHidden(createFormEl, true);
    toast("Table " + name + " created");
    await loadTables(name);
  } catch (err) {
    toast("Create failed: " + err.message, true);
  }
}

// -------- Delete table --------
async function dropTable() {
  if (!currentTable) return;

  var tableName = currentTable;
  var answer = await Goop.dialog.confirmDanger({
    title: "Drop Table",
    message: 'This will drop "' + tableName + '" and all its data.',
    match: tableName,
    placeholder: tableName,
    okText: "Drop Table",
  });
  if (answer !== tableName) return;

  try {
    await api.dropTable({ table: tableName });
    toast("Table " + tableName + " dropped");
    currentTable = null;
    tableTitleEl.textContent = "Select a table";
    setHidden(actionsEl, true);
    setHidden(searchBarEl, true);
    gridEl.innerHTML = '<p class="empty-state">Select a table from the sidebar to view its data.</p>';
    await loadTables();
  } catch (err) {
    toast("Drop failed: " + err.message, true);
  }
}

// -------- Insert row --------
function showInsertForm() {
  if (!currentTable || columns.length === 0) return;

  var userCols = columns.filter(function(c) {
    return systemCols.indexOf(c.name) === -1;
  });

  if (userCols.length === 0) {
    toast("No user columns to fill", "warning");
    return;
  }

  setHidden(insertFormEl, false);
  var html = '<h4>Insert Row into ' + escapeHtml(currentTable) + '</h4>';
  userCols.forEach(function(col) {
    var colType = (col.type || "text").toLowerCase();

    if (colType === "enum" && col.values && col.values.length > 0) {
      var enumOpts = [{ value: "", label: "(select)" }];
      col.values.forEach(function(v) { enumOpts.push({ value: v.key, label: v.label || v.key }); });
      html += '<div class="db-insert-field">' +
        '<label>' + escapeHtml(col.name) + ' <span style="opacity:0.5;font-size:11px">(enum)</span></label>' +
        gsel.html({ className: "db-insert-enum", value: col.default || "", options: enumOpts }) +
        '<input type="hidden" data-col="' + escapeHtml(col.name) + '" data-type="enum" class="db-insert-enum-val" />' +
      '</div>';
      return;
    }

    var inputType = "text";
    if (colType === "integer") inputType = "number";
    else if (colType === "real") inputType = "number";

    var extra = "";
    if (colType === "real") extra = ' step="any"';
    if (colType === "time") inputType = "time";
    if (col.auto) extra = ' readonly placeholder="(auto-generated)"';
    else if (colType === "datetime" || colType === "date") extra = ' placeholder="Click to pick a date"';

    html += '<div class="db-insert-field">' +
      '<label>' + escapeHtml(col.name) + ' <span style="opacity:0.5;font-size:11px">(' + escapeHtml(col.type) + ')</span></label>' +
      '<input type="' + inputType + '"' + extra + ' data-col="' + escapeHtml(col.name) + '" data-type="' + escapeHtml(colType) + '" class="form-input" />' +
    '</div>';
  });
  html += '<div class="form-actions">' +
    '<button id="db-insert-cancel" class="db-action-btn">Cancel</button>' +
    '<button id="db-insert-submit" class="db-action-btn" style="background:color-mix(in srgb,var(--accent) 22%,transparent);border-color:color-mix(in srgb,var(--accent) 40%,transparent)">Insert</button>' +
  '</div>';

  insertFormEl.innerHTML = html;
  on(qs("#db-insert-cancel"), "click", function() { setHidden(insertFormEl, true); });
  on(qs("#db-insert-submit"), "click", submitInsertRow);

  qsa('.db-insert-enum', insertFormEl).forEach(function(sel) {
    var hiddenInput = sel.parentNode.querySelector('.db-insert-enum-val');
    gsel.init(sel, function(val) { if (hiddenInput) hiddenInput.value = val; });
  });

  qsa('.db-insert-field input[data-type="datetime"], .db-insert-field input[data-type="date"]', insertFormEl).forEach(function(el) {
    if (!el.hasAttribute("readonly")) Goop.datepicker.attach(el);
  });
  qsa('.db-insert-field input[data-type="time"]', insertFormEl).forEach(function(el) {
    if (!el.hasAttribute("readonly")) Goop.timepicker.attach(el);
  });

  var firstInput = qs(".db-insert-field input:not([readonly])", insertFormEl);
  if (firstInput) firstInput.focus();
}

async function submitInsertRow() {
  var data = {};
  qsa(".db-insert-field input", insertFormEl).forEach(function(input) {
    var val = input.value;
    if (val !== "") {
      var colType = input.dataset.type || "text";
      if (colType === "datetime") {
        data[input.dataset.col] = val + "T00:00:00Z";
      } else if (colType === "date" || colType === "time") {
        data[input.dataset.col] = val;
      } else if (colType === "integer") {
        data[input.dataset.col] = parseInt(val, 10);
      } else if (colType === "real") {
        data[input.dataset.col] = parseFloat(val);
      } else {
        data[input.dataset.col] = val;
      }
    }
  });

  try {
    await api.insert({ table: currentTable, data: data });
    setHidden(insertFormEl, true);
    toast("Row inserted");
    selectTable(currentTable);
  } catch (err) {
    toast("Insert failed: " + err.message, true);
  }
}

// -------- Alter table --------
function showAlterForm() {
  if (!currentTable || columns.length === 0) return;

  setHidden(alterFormEl, false);

  var userCols = columns.filter(function(c) { return systemCols.indexOf(c.name) === -1; });

  var html = '<h3>Alter Table: ' + escapeHtml(currentTable) + '</h3>';

  // Insert policy section
  html += '<div class="form-group">' +
    '<label>Insert Policy</label>' +
    '<div style="display:flex;gap:8px;align-items:center">' +
      gsel.html({ id: "db-policy-select", value: currentPolicy || "owner", options: policyOptions(), style: "flex:1" }) +
      '<button id="db-policy-btn" class="db-action-btn">Save</button>' +
    '</div>' +
    '<div class="hint">Controls who can insert rows into this table via P2P.</div>' +
  '</div>';

  // Rename section
  html += '<div class="form-group">' +
    '<label>Rename Table</label>' +
    '<div style="display:flex;gap:8px">' +
      '<input type="text" id="db-rename-input" class="form-input" value="' + escapeHtml(currentTable) + '" style="flex:1" />' +
      '<button id="db-rename-btn" class="db-action-btn">Rename</button>' +
    '</div>' +
  '</div>';

  // Existing columns
  html += '<div class="form-group">' +
    '<label>Columns</label>';
  userCols.forEach(function(col) {
    html += '<div class="db-alter-col">' +
      '<span class="db-alter-col-name">' + escapeHtml(col.name) + '</span>' +
      '<span class="db-alter-col-type muted">' + escapeHtml(col.type) + '</span>' +
      '<button class="db-col-remove db-drop-col-btn" data-col="' + escapeHtml(col.name) + '">Drop</button>' +
    '</div>';
  });
  if (userCols.length === 0) {
    html += '<div class="db-alter-col muted" style="font-style:italic">No user columns</div>';
  }
  html += '</div>';

  // Add column section
  html += '<div class="form-group">' +
    '<label>Add Column</label>' +
    '<div class="db-col-row">' +
      '<input type="text" class="form-input db-col-name" id="db-addcol-name" placeholder="column name" />' +
      gsel.html({ id: "db-addcol-type", className: "db-col-type", value: "TEXT", options: colTypeOptions() }) +
      '<button id="db-addcol-btn" class="db-action-btn">Add</button>' +
    '</div>' +
  '</div>';

  alterFormEl.innerHTML = html;
  initFormSelects(alterFormEl);

  // Bind policy save
  on(qs("#db-policy-btn"), "click", async function() {
    var newPolicy = gsel.val(qs("#db-policy-select"));
    try {
      await api.setPolicy({ table: currentTable, policy: newPolicy });
      currentPolicy = newPolicy;
      tablesMeta[currentTable] = { insert_policy: newPolicy };
      toast("Insert policy set to " + newPolicy, "info");
      // Update header badge
      tableTitleEl.innerHTML = escapeHtml(currentTable) +
        ' <span class="badge badge-' + newPolicy + '">' + (policyLabels[newPolicy] || newPolicy) + '</span>';
      // Update sidebar badge
      await loadTables();
      highlightActive(currentTable);
    } catch (err) {
      toast("Failed to set policy: " + err.message, true);
    }
  });

  // Bind rename
  on(qs("#db-rename-btn"), "click", async function() {
    var newName = (qs("#db-rename-input").value || "").trim();
    if (!newName || newName === currentTable) return;
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(newName)) {
      toast("Invalid table name", "warning");
      return;
    }
    try {
      await api.renameTable({ old_name: currentTable, new_name: newName });
      toast("Table renamed to " + newName);
      currentTable = newName;
      await loadTables(newName);
    } catch (err) {
      toast("Rename failed: " + err.message, true);
    }
  });

  // Bind drop column buttons
  qsa(".db-drop-col-btn", alterFormEl).forEach(function(btn) {
    on(btn, "click", async function() {
      var colName = btn.dataset.col;
      var answer = await Goop.dialog.confirmDanger({
        title: "Drop Column",
        message: 'This will drop "' + colName + '" and its data.',
        match: colName,
        placeholder: colName,
        okText: "Drop",
      });
      if (answer !== colName) return;
      try {
        await api.dropColumn({ table: currentTable, column: colName });
        toast("Column " + colName + " dropped");
        await selectTable(currentTable);
      } catch (err) {
        toast("Drop column failed: " + err.message, true);
      }
    });
  });

  // Bind add column
  on(qs("#db-addcol-btn"), "click", async function() {
    var colName = (qs("#db-addcol-name").value || "").trim();
    var colType = gsel.val(qs("#db-addcol-type"));
    if (!colName) { toast("Column name required", "warning"); return; }
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(colName)) {
      toast("Invalid column name", "warning");
      return;
    }
    try {
      await api.addColumn({
        table: currentTable,
        column: { name: colName, type: colType }
      });
      toast("Column " + colName + " added");
      await selectTable(currentTable);
    } catch (err) {
      toast("Add column failed: " + err.message, true);
    }
  });

}

// -------- Event bindings --------
on(btnNew, "click", showCreateForm);
on(btnInsert, "click", function() {
  switchTableView("data");
  showInsertForm();
});
on(btnRefresh, "click", function() {
  if (currentTable) {
    switchTableView("data");
    selectTable(currentTable);
  }
});
on(btnDrop, "click", dropTable);

// Search bindings
on(searchInputEl, "input", applyFilter);
on(searchClearEl, "click", function() {
  searchInputEl.value = "";
  executeSearch();
});

// Column picker
on(colPickerBtn, "click", function(e) {
  e.stopPropagation();
  toggleColPicker();
});

// Infinite scroll
on(gridEl, "scroll", function() {
  if (!hasMore || loadingMore) return;
  var threshold = 100;
  if (gridEl.scrollTop + gridEl.clientHeight >= gridEl.scrollHeight - threshold) {
    loadMore();
  }
});


  G.database.initTables = function() { loadTables(); };
  G.database.refresh = loadTables;
})();
