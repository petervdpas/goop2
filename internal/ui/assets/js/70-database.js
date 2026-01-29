// internal/ui/assets/js/70-database.js
// Full-featured SQLite database editor
(() => {
  const { qs, qsa, on, setHidden } = window.Goop.core;

  // Only activate on database page
  const dbPage = qs("#db-page");
  if (!dbPage) return;

  // DOM refs
  const tableListEl  = qs("#db-table-list");
  const tableTitleEl  = qs("#db-table-title");
  const actionsEl     = qs("#db-content-actions");
  const createFormEl  = qs("#db-create-form");
  const insertFormEl  = qs("#db-insert-form");
  const gridEl        = qs("#db-grid");
  const btnNew        = qs("#db-btn-new-table");
  const btnInsert     = qs("#db-btn-insert");
  const btnRefresh    = qs("#db-btn-refresh");
  const btnDrop       = qs("#db-btn-delete-table");

  // State
  let currentTable = null;
  let columns = [];      // ColumnInfo[] from describe
  let systemCols = ["_id", "_owner", "_created_at"];

  // -------- API helper --------
  async function api(url, body) {
    const resp = await fetch(url, {
      method: body !== undefined ? "POST" : "GET",
      headers: body !== undefined ? { "Content-Type": "application/json" } : {},
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    if (!resp.ok) {
      const text = await resp.text();
      throw new Error(text || resp.statusText);
    }
    const ct = resp.headers.get("Content-Type") || "";
    if (ct.includes("application/json")) {
      return resp.json();
    }
    return null;
  }

  function toast(msg, isError) {
    if (window.Goop && window.Goop.toast) {
      window.Goop.toast({
        icon: isError ? "!" : "ok",
        title: isError ? "Error" : "Success",
        message: msg,
        duration: isError ? 6000 : 3000,
      });
    }
  }

  function escapeHtml(str) {
    const d = document.createElement("div");
    d.textContent = String(str);
    return d.innerHTML;
  }

  // -------- Table list --------
  async function loadTables(selectName) {
    try {
      const tables = await api("/api/data/tables") || [];
      renderTableList(tables);
      // Auto-select
      if (selectName) {
        selectTable(selectName);
      } else if (tables.length > 0 && !currentTable) {
        selectTable(tables[0].name);
      } else if (currentTable) {
        // Re-highlight current
        highlightActive(currentTable);
      }
    } catch (err) {
      tableListEl.innerHTML = '<li class="db-table-empty">Failed to load tables</li>';
    }
  }

  function renderTableList(tables) {
    if (!tables || tables.length === 0) {
      tableListEl.innerHTML = '<li class="db-table-empty">No tables yet</li>';
      return;
    }
    tableListEl.innerHTML = "";
    tables.forEach(function(t) {
      const li = document.createElement("li");
      li.className = "db-table-item";
      li.dataset.table = t.name;
      li.innerHTML =
        '<span class="db-table-name">' + escapeHtml(t.name) + '</span>' +
        '<span class="db-table-vis">' + escapeHtml(t.visibility) + '</span>';
      on(li, "click", function() { selectTable(t.name); });
      tableListEl.appendChild(li);
    });
  }

  function highlightActive(name) {
    qsa(".db-table-item", tableListEl).forEach(function(el) {
      el.classList.toggle("active", el.dataset.table === name);
    });
  }

  // -------- Select table --------
  async function selectTable(name) {
    currentTable = name;
    highlightActive(name);
    tableTitleEl.textContent = name;
    setHidden(actionsEl, false);
    setHidden(createFormEl, true);
    setHidden(insertFormEl, true);

    try {
      // Fetch schema and data in parallel
      const [cols, rows] = await Promise.all([
        api("/api/data/tables/describe", { table: name }),
        api("/api/data/query", { table: name }),
      ]);
      columns = cols || [];
      renderDataGrid(rows || []);
    } catch (err) {
      gridEl.innerHTML = '<p class="db-empty">Error loading table: ' + escapeHtml(err.message) + '</p>';
    }
  }

  // -------- Data grid --------
  function renderDataGrid(rows) {
    if (columns.length === 0) {
      gridEl.innerHTML = '<p class="db-empty">No columns found.</p>';
      return;
    }

    // Build colgroup for smart column widths
    function buildColgroup() {
      var cg = '<colgroup>';
      columns.forEach(function(col) {
        if (col.name === '_id') {
          cg += '<col style="width:50px">';
        } else if (col.name === '_owner') {
          cg += '<col style="width:120px">';
        } else if (col.name === '_created_at') {
          cg += '<col style="width:170px">';
        } else {
          cg += '<col>';
        }
      });
      cg += '<col style="width:40px">';
      cg += '</colgroup>';
      return cg;
    }

    if (!rows || rows.length === 0) {
      // Show header-only table
      let html = '<table>' + buildColgroup() + '<thead><tr>';
      columns.forEach(function(col) {
        html += '<th>' + escapeHtml(col.name) + '</th>';
      });
      html += '<th></th></tr></thead>';
      html += '<tbody><tr><td colspan="' + (columns.length + 1) + '" class="db-empty">No rows. Click "+ Row" to add data.</td></tr></tbody></table>';
      gridEl.innerHTML = html;
      return;
    }

    let html = '<table>' + buildColgroup() + '<thead><tr>';
    columns.forEach(function(col) {
      html += '<th>' + escapeHtml(col.name) + '</th>';
    });
    html += '<th></th></tr></thead><tbody>';

    rows.forEach(function(row) {
      const rowId = row._id;
      html += '<tr data-row-id="' + rowId + '">';
      columns.forEach(function(col) {
        const val = row[col.name];
        const isSystem = systemCols.indexOf(col.name) !== -1;
        const isNull = val === null || val === undefined;

        if (isSystem) {
          html += '<td class="db-cell-system">';
          if (isNull) {
            html += '<span class="db-cell-null">NULL</span>';
          } else {
            html += '<span class="db-cell-truncate" title="' + escapeHtml(val) + '">' + escapeHtml(val) + '</span>';
          }
          html += '</td>';
        } else {
          html += '<td class="db-cell-editable" data-col="' + escapeHtml(col.name) + '" data-row-id="' + rowId + '">';
          html += isNull ? '<span class="db-cell-null">NULL</span>' : escapeHtml(val);
          html += '</td>';
        }
      });
      html += '<td class="db-row-actions"><button class="db-row-delete" data-row-id="' + rowId + '" title="Delete row">x</button></td>';
      html += '</tr>';
    });

    html += '</tbody></table>';
    gridEl.innerHTML = html;

    // Bind inline edit on click
    qsa(".db-cell-editable", gridEl).forEach(function(td) {
      on(td, "click", function() {
        startEdit(td);
      });
    });

    // Bind delete row
    qsa(".db-row-delete", gridEl).forEach(function(btn) {
      on(btn, "click", function(e) {
        e.stopPropagation();
        deleteRow(parseInt(btn.dataset.rowId, 10));
      });
    });
  }

  // -------- Inline editing --------
  function startEdit(td) {
    // Already editing?
    if (td.classList.contains("db-cell-editing")) return;

    const colName = td.dataset.col;
    const rowId = parseInt(td.dataset.rowId, 10);
    const nullSpan = qs(".db-cell-null", td);
    const oldValue = nullSpan ? "" : td.textContent;

    td.classList.add("db-cell-editing");
    const input = document.createElement("input");
    input.type = "text";
    input.value = oldValue;
    td.textContent = "";
    td.appendChild(input);

    input.focus();
    input.select();

    function commit() {
      const newValue = input.value;
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
      const data = {};
      // Send empty string as null
      data[colName] = newValue === "" ? null : newValue;
      await api("/api/data/update", { table: currentTable, id: rowId, data: data });
      // If value is null, render as NULL span
      if (newValue === "") {
        td.innerHTML = '<span class="db-cell-null">NULL</span>';
      }
      toast(colName + " updated");
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
    if (!window.Goop || !window.Goop.dialogs) {
      if (!confirm("Delete row " + rowId + "?")) return;
    } else {
      const answer = await window.Goop.dialogs.dlgAsk({
        title: "Delete Row",
        message: 'Type "DELETE" to confirm deleting row ' + rowId,
        placeholder: "DELETE",
        okText: "Delete",
        dangerOk: true,
      });
      if (answer !== "DELETE") {
        if (answer !== null) toast("Type DELETE to confirm", true);
        return;
      }
    }

    try {
      await api("/api/data/delete", { table: currentTable, id: rowId });
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
      '<div class="db-form-group">' +
        '<label>Table Name</label>' +
        '<input type="text" id="db-new-name" class="db-input" placeholder="my_table" />' +
      '</div>' +
      '<div class="db-form-group">' +
        '<label>Visibility</label>' +
        '<select id="db-new-vis" class="db-input">' +
          '<option value="private">Private</option>' +
          '<option value="public">Public</option>' +
        '</select>' +
      '</div>' +
      '<div class="db-form-group">' +
        '<label>Columns</label>' +
        '<div id="db-col-defs">' +
          colRowHtml() +
        '</div>' +
        '<button id="db-add-col" class="db-action-btn" style="margin-top:6px">+ Column</button>' +
      '</div>' +
      '<div class="db-form-actions">' +
        '<button id="db-create-cancel" class="db-action-btn">Cancel</button>' +
        '<button id="db-create-submit" class="db-action-btn" style="background:color-mix(in srgb,var(--accent) 22%,transparent);border-color:color-mix(in srgb,var(--accent) 40%,transparent)">Create</button>' +
      '</div>';

    on(qs("#db-add-col"), "click", addColRow);
    on(qs("#db-create-cancel"), "click", function() { setHidden(createFormEl, true); });
    on(qs("#db-create-submit"), "click", submitCreateTable);

    // Bind remove on initial row
    bindColRemove();

    qs("#db-new-name").focus();
  }

  function colRowHtml() {
    return '<div class="db-col-row">' +
      '<input type="text" class="db-input db-col-name" placeholder="column name" />' +
      '<select class="db-input db-col-type">' +
        '<option value="TEXT">TEXT</option>' +
        '<option value="INTEGER">INTEGER</option>' +
        '<option value="REAL">REAL</option>' +
        '<option value="BLOB">BLOB</option>' +
      '</select>' +
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
    var vis = qs("#db-new-vis").value;

    if (!name) { toast("Table name required", true); return; }
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
      toast("Name must be letters, digits, underscore (start with letter or _)", true);
      return;
    }

    var cols = [];
    qsa("#db-col-defs .db-col-row").forEach(function(row) {
      var n = qs(".db-col-name", row).value.trim();
      var t = qs(".db-col-type", row).value;
      var nn = qs(".db-col-notnull input", row).checked;
      if (n) {
        cols.push({ name: n, type: t, not_null: nn });
      }
    });

    if (cols.length === 0) { toast("Add at least one column", true); return; }

    try {
      await api("/api/data/tables/create", { name: name, columns: cols, visibility: vis });
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
    if (!window.Goop || !window.Goop.dialogs) {
      if (!confirm("Drop table " + tableName + "?")) return;
    } else {
      var answer = await window.Goop.dialogs.dlgAsk({
        title: "Drop Table",
        message: 'Type "' + tableName + '" to confirm dropping this table and all its data.',
        placeholder: tableName,
        okText: "Drop Table",
        dangerOk: true,
      });
      if (answer !== tableName) {
        if (answer !== null) toast("Type the table name to confirm", true);
        return;
      }
    }

    try {
      await api("/api/data/tables/delete", { table: tableName });
      toast("Table " + tableName + " dropped");
      currentTable = null;
      tableTitleEl.textContent = "Select a table";
      setHidden(actionsEl, true);
      gridEl.innerHTML = '<p class="db-empty">Select a table from the sidebar to view its data.</p>';
      await loadTables();
    } catch (err) {
      toast("Drop failed: " + err.message, true);
    }
  }

  // -------- Insert row --------
  function showInsertForm() {
    if (!currentTable || columns.length === 0) return;

    var userCols = columns.filter(function(c) { return systemCols.indexOf(c.name) === -1; });

    if (userCols.length === 0) {
      toast("No user columns to fill", true);
      return;
    }

    setHidden(insertFormEl, false);
    var html = '<h4>Insert Row into ' + escapeHtml(currentTable) + '</h4>';
    userCols.forEach(function(col) {
      html += '<div class="db-insert-field">' +
        '<label>' + escapeHtml(col.name) + ' <span style="opacity:0.5;font-size:11px">(' + escapeHtml(col.type) + ')</span></label>' +
        '<input type="text" data-col="' + escapeHtml(col.name) + '" class="db-input" />' +
      '</div>';
    });
    html += '<div class="db-form-actions">' +
      '<button id="db-insert-cancel" class="db-action-btn">Cancel</button>' +
      '<button id="db-insert-submit" class="db-action-btn" style="background:color-mix(in srgb,var(--accent) 22%,transparent);border-color:color-mix(in srgb,var(--accent) 40%,transparent)">Insert</button>' +
    '</div>';

    insertFormEl.innerHTML = html;
    on(qs("#db-insert-cancel"), "click", function() { setHidden(insertFormEl, true); });
    on(qs("#db-insert-submit"), "click", submitInsertRow);

    // Focus first input
    var firstInput = qs(".db-insert-field input", insertFormEl);
    if (firstInput) firstInput.focus();
  }

  async function submitInsertRow() {
    var data = {};
    qsa(".db-insert-field input", insertFormEl).forEach(function(input) {
      var val = input.value;
      if (val !== "") {
        data[input.dataset.col] = val;
      }
    });

    try {
      await api("/api/data/insert", { table: currentTable, data: data });
      setHidden(insertFormEl, true);
      toast("Row inserted");
      selectTable(currentTable);
    } catch (err) {
      toast("Insert failed: " + err.message, true);
    }
  }

  // -------- Event bindings --------
  on(btnNew, "click", showCreateForm);
  on(btnInsert, "click", showInsertForm);
  on(btnRefresh, "click", function() { if (currentTable) selectTable(currentTable); });
  on(btnDrop, "click", dropTable);

  // -------- Init --------
  loadTables();

  window.Goop = window.Goop || {};
  window.Goop.database = { refresh: loadTables };
})();
