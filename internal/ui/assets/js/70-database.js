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
  const alterFormEl   = qs("#db-alter-form");
  const insertFormEl  = qs("#db-insert-form");
  const gridEl        = qs("#db-grid");
  const btnNew        = qs("#db-btn-new-table");
  const btnInsert     = qs("#db-btn-insert");
  const btnAlter      = qs("#db-btn-alter");
  const btnRefresh    = qs("#db-btn-refresh");
  const btnDrop       = qs("#db-btn-delete-table");
  const searchBarEl    = qs("#db-search-bar");
  const searchColEl    = qs("#db-search-col");
  const searchInputEl  = qs("#db-search-input");
  const searchClearEl  = qs("#db-search-clear");

  // State
  let currentTable = null;
  let columns = [];      // ColumnInfo[] from describe
  let systemCols = ["_id", "_owner", "_created_at"];
  let searchTimer = null;
  let pageSize = 50;
  let currentOffset = 0;
  let hasMore = true;
  let loadingMore = false;

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
      li.innerHTML = '<span class="db-table-name">' + escapeHtml(t.name) + '</span>';
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
      // Fetch schema and first page in parallel
      const [cols, rows] = await Promise.all([
        api("/api/data/tables/describe", { table: name }),
        api("/api/data/query", { table: name, limit: pageSize, offset: 0 }),
      ]);
      columns = cols || [];
      currentOffset = 0;
      hasMore = (rows || []).length >= pageSize;
      populateSearchBar();
      renderDataGrid(rows || [], false);
    } catch (err) {
      gridEl.innerHTML = '<p class="db-empty">Error loading table: ' + escapeHtml(err.message) + '</p>';
    }
  }

  // -------- Search / filter --------
  function populateSearchBar() {
    if (columns.length === 0) {
      setHidden(searchBarEl, true);
      return;
    }
    setHidden(searchBarEl, false);
    searchColEl.innerHTML = '<option value="*">All columns</option>';
    columns.forEach(function(col) {
      searchColEl.innerHTML += '<option value="' + escapeHtml(col.name) + '">' + escapeHtml(col.name) + '</option>';
    });
    searchInputEl.value = "";
  }

  function applyFilter() {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(executeSearch, 250);
  }

  function buildSearchBody(offset) {
    var query = (searchInputEl.value || "").trim();
    var col = searchColEl.value;

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
      var rows = await api("/api/data/query", buildSearchBody(0));
      hasMore = (rows || []).length >= pageSize;
      renderDataGrid(rows || [], false);
    } catch (err) {
      gridEl.innerHTML = '<p class="db-empty">Search error: ' + escapeHtml(err.message) + '</p>';
    }
  }

  async function loadMore() {
    if (!currentTable || !hasMore || loadingMore) return;
    loadingMore = true;

    var nextOffset = currentOffset + pageSize;
    try {
      var rows = await api("/api/data/query", buildSearchBody(nextOffset));
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

  // -------- Data grid --------
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

  function buildRowHtml(row) {
    var rowId = row._id;
    var html = '<tr data-row-id="' + rowId + '">';
    columns.forEach(function(col) {
      var val = row[col.name];
      var isSystem = systemCols.indexOf(col.name) !== -1;
      var isNull = val === null || val === undefined;

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
    return html;
  }

  function bindRowEvents(container) {
    qsa(".db-cell-editable:not([data-bound])", container).forEach(function(td) {
      td.dataset.bound = "1";
      on(td, "click", function() { startEdit(td); });
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
      gridEl.innerHTML = '<p class="db-empty">No columns found.</p>';
      return;
    }

    // Append rows to existing tbody
    if (append) {
      var tbody = qs("tbody", gridEl);
      if (tbody && rows && rows.length > 0) {
        var fragment = document.createElement("tbody");
        var html = "";
        rows.forEach(function(row) { html += buildRowHtml(row); });
        fragment.innerHTML = html;
        while (fragment.firstChild) {
          tbody.appendChild(fragment.firstChild);
        }
        bindRowEvents(tbody);
      }
      return;
    }

    // Full render
    if (!rows || rows.length === 0) {
      var html = '<table>' + buildColgroup() + '<thead><tr>';
      columns.forEach(function(col) {
        html += '<th>' + escapeHtml(col.name) + '</th>';
      });
      html += '<th></th></tr></thead>';
      html += '<tbody><tr><td colspan="' + (columns.length + 1) + '" class="db-empty">No rows. Click "+ Row" to add data.</td></tr></tbody></table>';
      gridEl.innerHTML = html;
      return;
    }

    var html = '<table>' + buildColgroup() + '<thead><tr>';
    columns.forEach(function(col) {
      html += '<th>' + escapeHtml(col.name) + '</th>';
    });
    html += '<th></th></tr></thead><tbody>';
    rows.forEach(function(row) { html += buildRowHtml(row); });
    html += '</tbody></table>';
    gridEl.innerHTML = html;
    bindRowEvents(gridEl);
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
      await api("/api/data/tables/create", { name: name, columns: cols });
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
      setHidden(searchBarEl, true);
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

  // -------- Alter table --------
  function showAlterForm() {
    if (!currentTable || columns.length === 0) return;

    setHidden(alterFormEl, false);
    setHidden(createFormEl, true);
    setHidden(insertFormEl, true);

    var userCols = columns.filter(function(c) { return systemCols.indexOf(c.name) === -1; });

    var html = '<h3>Alter Table: ' + escapeHtml(currentTable) + '</h3>';

    // Rename section
    html += '<div class="db-form-group">' +
      '<label>Rename Table</label>' +
      '<div style="display:flex;gap:8px">' +
        '<input type="text" id="db-rename-input" class="db-input" value="' + escapeHtml(currentTable) + '" style="flex:1" />' +
        '<button id="db-rename-btn" class="db-action-btn">Rename</button>' +
      '</div>' +
    '</div>';

    // Existing columns
    html += '<div class="db-form-group">' +
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
    html += '<div class="db-form-group">' +
      '<label>Add Column</label>' +
      '<div class="db-col-row">' +
        '<input type="text" class="db-input db-col-name" id="db-addcol-name" placeholder="column name" />' +
        '<select class="db-input db-col-type" id="db-addcol-type">' +
          '<option value="TEXT">TEXT</option>' +
          '<option value="INTEGER">INTEGER</option>' +
          '<option value="REAL">REAL</option>' +
          '<option value="BLOB">BLOB</option>' +
        '</select>' +
        '<button id="db-addcol-btn" class="db-action-btn">Add</button>' +
      '</div>' +
    '</div>';

    html += '<div class="db-form-actions">' +
      '<button id="db-alter-close" class="db-action-btn">Close</button>' +
    '</div>';

    alterFormEl.innerHTML = html;

    // Bind rename
    on(qs("#db-rename-btn"), "click", async function() {
      var newName = (qs("#db-rename-input").value || "").trim();
      if (!newName || newName === currentTable) return;
      if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(newName)) {
        toast("Invalid table name", true);
        return;
      }
      try {
        await api("/api/data/tables/rename", { old_name: currentTable, new_name: newName });
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
        if (!window.Goop || !window.Goop.dialogs) {
          if (!confirm("Drop column " + colName + "?")) return;
        } else {
          var answer = await window.Goop.dialogs.dlgAsk({
            title: "Drop Column",
            message: 'Type "' + colName + '" to confirm dropping this column and its data.',
            placeholder: colName,
            okText: "Drop",
            dangerOk: true,
          });
          if (answer !== colName) {
            if (answer !== null) toast("Type the column name to confirm", true);
            return;
          }
        }
        try {
          await api("/api/data/tables/drop-column", { table: currentTable, column: colName });
          toast("Column " + colName + " dropped");
          await selectTable(currentTable);
          showAlterForm(); // Re-render alter form with updated schema
        } catch (err) {
          toast("Drop column failed: " + err.message, true);
        }
      });
    });

    // Bind add column
    on(qs("#db-addcol-btn"), "click", async function() {
      var colName = (qs("#db-addcol-name").value || "").trim();
      var colType = qs("#db-addcol-type").value;
      if (!colName) { toast("Column name required", true); return; }
      if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(colName)) {
        toast("Invalid column name", true);
        return;
      }
      try {
        await api("/api/data/tables/add-column", {
          table: currentTable,
          column: { name: colName, type: colType }
        });
        toast("Column " + colName + " added");
        await selectTable(currentTable);
        showAlterForm(); // Re-render with updated schema
      } catch (err) {
        toast("Add column failed: " + err.message, true);
      }
    });

    // Bind close
    on(qs("#db-alter-close"), "click", function() { setHidden(alterFormEl, true); });
  }

  // -------- Event bindings --------
  on(btnNew, "click", showCreateForm);
  on(btnInsert, "click", showInsertForm);
  on(btnAlter, "click", showAlterForm);
  on(btnRefresh, "click", function() {
    setHidden(alterFormEl, true);
    if (currentTable) selectTable(currentTable);
  });
  on(btnDrop, "click", dropTable);

  // Search bindings
  on(searchInputEl, "input", applyFilter);
  on(searchColEl, "change", executeSearch);
  on(searchClearEl, "click", function() {
    searchInputEl.value = "";
    executeSearch();
  });

  // Infinite scroll
  on(gridEl, "scroll", function() {
    if (!hasMore || loadingMore) return;
    var threshold = 100;
    if (gridEl.scrollTop + gridEl.clientHeight >= gridEl.scrollHeight - threshold) {
      loadMore();
    }
  });

  // -------- Init --------
  loadTables();

  window.Goop = window.Goop || {};
  window.Goop.database = { refresh: loadTables };
})();
