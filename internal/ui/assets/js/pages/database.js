// Full-featured SQLite database editor
(() => {
  const { qs, qsa, on, setHidden, escapeHtml, toast, safeLocalStorageGet, safeLocalStorageSet } = window.Goop.core;
  const api = window.Goop.api.data;
  const mapperApi = window.Goop.api.mapper;
  const schemaApi = window.Goop.api.schema;
  const gsel = window.Goop.select;

  // Only activate on database page
  const dbPage = qs("#db-page");
  if (!dbPage) return;

  // Top-level tab switching (reuses page-tabs styling)
  var viewTables = qs("#db-view-tables");
  var viewSchemas = qs("#db-view-schemas");
  var viewMappers = qs("#db-view-mappers");
  var allViews = [viewTables, viewSchemas, viewMappers];

  qsa("[data-db-tab]").forEach(function(tab) {
    on(tab, "click", function(e) {
      e.preventDefault();
      var target = tab.dataset.dbTab;
      qsa("[data-db-tab]").forEach(function(t) { t.classList.toggle("active", t === tab); });
      allViews.forEach(function(v) { v.classList.remove("active"); });
      if (target === "tables") {
        viewTables.classList.add("active");
      } else if (target === "schemas") {
        viewSchemas.classList.add("active");
        loadSchemas();
      } else {
        viewMappers.classList.add("active");
        loadMappers();
      }
    });
  });

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
      on(li, "click", function() { selectTable(t.name); });
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

  // -------- Data grid --------
  function buildColgroup() {
    var vc = visibleCols();
    var cg = '<colgroup>';
    vc.forEach(function(col) {
      if (col.name === '_id') {
        cg += '<col style="width:50px">';
      } else if (col.name === '_created_at' || col.name === '_updated_at') {
        cg += '<col style="width:170px">';
      } else if (col.name === '_owner' || col.name === '_owner_email') {
        cg += '<col style="width:140px">';
      } else {
        cg += '<col>';
      }
    });
    cg += '<col style="width:40px">';
    cg += '</colgroup>';
    return cg;
  }

  function buildRowHtml(row) {
    var vc = visibleCols();
    var rowId = row._id;
    var html = '<tr data-row-id="' + rowId + '">';
    vc.forEach(function(col) {
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
      gridEl.innerHTML = '<p class="empty-state">No columns found.</p>';
      return;
    }

    // Cache rows for column toggle re-render
    if (!append) {
      lastRows = rows || [];
    } else if (rows && rows.length > 0) {
      lastRows = lastRows.concat(rows);
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
    var vc = visibleCols();
    if (!rows || rows.length === 0) {
      var html = '<table>' + buildColgroup() + '<thead><tr>';
      vc.forEach(function(col) {
        html += '<th>' + escapeHtml(col.name) + '</th>';
      });
      html += '<th></th></tr></thead>';
      html += '<tbody><tr><td colspan="' + (vc.length + 1) + '" class="empty-state">No rows. Click "+ Row" to add data.</td></tr></tbody></table>';
      gridEl.innerHTML = html;
      return;
    }

    var html = '<table>' + buildColgroup() + '<thead><tr>';
    vc.forEach(function(col) {
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
      await api.update({ table: currentTable, id: rowId, data: data });
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
      { value: "DATE", label: "DATE" },
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

    if (!name) { toast("Table name required", true); return; }
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
      toast("Name must be letters, digits, underscore (start with letter or _)", true);
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

    if (cols.length === 0) { toast("Add at least one column", true); return; }

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
      toast("No user columns to fill", true);
      return;
    }

    setHidden(insertFormEl, false);
    var html = '<h4>Insert Row into ' + escapeHtml(currentTable) + '</h4>';
    userCols.forEach(function(col) {
      var colType = (col.type || "text").toLowerCase();
      var inputType = "text";
      if (colType === "integer") inputType = "number";
      else if (colType === "real") inputType = "number";

      var extra = "";
      if (colType === "real") extra = ' step="any"';
      if (colType === "guid") extra = ' readonly placeholder="(auto-generated)"';
      if (colType === "date") extra = ' placeholder="Click to pick a date"';

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

    qsa('.db-insert-field input[data-type="date"]', insertFormEl).forEach(function(el) {
      Goop.datepicker.attach(el);
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
        if (colType === "date") {
          data[input.dataset.col] = val + "T00:00:00Z";
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
    setHidden(createFormEl, true);
    setHidden(insertFormEl, true);

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

    html += '<div class="form-actions">' +
      '<button id="db-alter-close" class="db-action-btn">Close</button>' +
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
        toast("Insert policy set to " + newPolicy);
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
        toast("Invalid table name", true);
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
          showAlterForm(); // Re-render alter form with updated schema
        } catch (err) {
          toast("Drop column failed: " + err.message, true);
        }
      });
    });

    // Bind add column
    on(qs("#db-addcol-btn"), "click", async function() {
      var colName = (qs("#db-addcol-name").value || "").trim();
      var colType = gsel.val(qs("#db-addcol-type"));
      if (!colName) { toast("Column name required", true); return; }
      if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(colName)) {
        toast("Invalid column name", true);
        return;
      }
      try {
        await api.addColumn({
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

  // ======== Tab switching ========
  var tablesContent = qs("#db-content-tables");
  var mappersContent = qs("#db-content-mappers");

  qsa(".db-tab", dbPage).forEach(function(tab) {
    on(tab, "click", function() {
      var target = tab.dataset.tab;
      qsa(".db-tab", dbPage).forEach(function(t) { t.classList.toggle("active", t === tab); });
      qsa(".db-tab-content", dbPage).forEach(function(c) {
        c.classList.toggle("active", c.id === "db-tab-" + target);
      });
      if (target === "tables") {
        setHidden(tablesContent, false);
        setHidden(mappersContent, true);
      } else {
        setHidden(tablesContent, true);
        setHidden(mappersContent, false);
        loadMappers();
      }
    });
  });

  // ======== Schema designer ========
  var schemaListEl    = qs("#db-schema-list");
  var schemaTitleEl   = qs("#db-schema-title");
  var schemaActionsEl = qs("#db-schema-actions");
  var schemaEditorEl  = qs("#db-schema-editor");
  var schemaDdlEl     = qs("#db-schema-ddl");
  var schemaDdlCode   = qs("#db-schema-ddl-code");
  var btnNewSchema    = qs("#db-btn-new-schema");
  var btnSaveSchema   = qs("#db-btn-save-schema");
  var btnApplySchema  = qs("#db-btn-apply-schema");
  var btnDeleteSchema = qs("#db-btn-delete-schema");

  var currentSchema = null;

  function schemaTypeOptions() {
    return [
      { value: "guid", label: "guid" },
      { value: "integer", label: "integer" },
      { value: "text", label: "text" },
      { value: "date", label: "date" },
      { value: "real", label: "real" },
      { value: "blob", label: "blob" },
    ];
  }

  async function loadSchemas(selectName) {
    try {
      var schemas = await schemaApi.list() || [];
      renderSchemaList(schemas);
      if (selectName) selectSchemaItem(selectName);
    } catch (err) {
      schemaListEl.innerHTML = '<li class="db-table-empty">Failed to load schemas</li>';
    }
  }

  function renderSchemaList(schemas) {
    if (!schemas || schemas.length === 0) {
      schemaListEl.innerHTML = '<li class="db-table-empty">No schemas yet</li>';
      return;
    }
    schemaListEl.innerHTML = "";
    schemas.forEach(function(s) {
      var li = document.createElement("li");
      li.className = "sidebar-item";
      li.dataset.schema = s.name;
      var badge = s.has_key ? "owner" : "open";
      li.innerHTML = '<span class="db-table-name">' + escapeHtml(s.name) + '</span>' +
        '<span class="badge badge-' + badge + '">' + s.columns + ' cols</span>';
      on(li, "click", function() { selectSchemaItem(s.name); });
      schemaListEl.appendChild(li);
    });
  }

  function highlightActiveSchema(name) {
    qsa(".sidebar-item", schemaListEl).forEach(function(el) {
      el.classList.toggle("active", el.dataset.schema === name);
    });
  }

  async function selectSchemaItem(name) {
    currentSchema = name;
    highlightActiveSchema(name);
    setHidden(schemaActionsEl, false);

    try {
      var s = await schemaApi.get({ name: name });
      schemaTitleEl.textContent = s.name;
      renderSchemaEditor(s);
      updateDdlPreview();
    } catch (err) {
      schemaEditorEl.innerHTML = '<p class="empty-state">Error: ' + escapeHtml(err.message) + '</p>';
    }
  }

  function renderSchemaEditor(s) {
    var html = '<div class="db-mapper-form">';
    html += '<div class="form-group">' +
      '<label>Table Name</label>' +
      '<input type="text" id="schema-name" class="form-input" value="' + escapeHtml(s.name || '') + '" />' +
    '</div>';

    html += '<div class="form-group">' +
      '<label>Columns</label>' +
      '<div class="schema-col-header">' +
        '<span class="schema-h-name">Name</span>' +
        '<span class="schema-h-type">Type</span>' +
        '<span class="schema-h-key">Key</span>' +
        '<span class="schema-h-req">Required</span>' +
        '<span class="schema-h-def">Default</span>' +
        '<span class="schema-h-rm"></span>' +
      '</div>' +
      '<div id="schema-columns">';
    if (s.columns && s.columns.length > 0) {
      s.columns.forEach(function(c) { html += schemaColRow(c); });
    } else {
      html += schemaColRow({ type: "text" });
    }
    html += '</div>';
    html += '<button id="schema-add-col" class="db-action-btn" style="margin-top:6px">+ Column</button>';
    html += '</div>';

    html += '</div>';
    schemaEditorEl.innerHTML = html;
    initFormSelects(schemaEditorEl);
    bindSchemaColEvents();

    on(qs("#schema-add-col"), "click", function() {
      var container = qs("#schema-columns");
      var div = document.createElement("div");
      div.innerHTML = schemaColRow({ type: "text" });
      var row = div.firstElementChild;
      container.appendChild(row);
      initFormSelects(row);
      bindSchemaColEvents();
      qs(".schema-col-name", row).focus();
    });

    setHidden(schemaDdlEl, false);
  }

  function schemaColRow(c) {
    var def = c.default !== undefined && c.default !== null ? String(c.default) : "";
    return '<div class="schema-col-row">' +
      '<input type="text" class="form-input schema-col-name" placeholder="column_name" value="' + escapeHtml(c.name || '') + '" />' +
      gsel.html({ className: "schema-col-type", value: c.type || "text", options: schemaTypeOptions() }) +
      '<label class="schema-col-check"><input type="checkbox" class="schema-col-key"' + (c.key ? ' checked' : '') + ' /></label>' +
      '<label class="schema-col-check"><input type="checkbox" class="schema-col-req"' + (c.required ? ' checked' : '') + ' /></label>' +
      '<input type="text" class="form-input schema-col-def" placeholder="" value="' + escapeHtml(def) + '" />' +
      '<button class="db-col-remove schema-col-remove">x</button>' +
    '</div>';
  }

  function bindSchemaColEvents() {
    qsa(".schema-col-remove", schemaEditorEl).forEach(function(btn) {
      btn.onclick = function() {
        var container = qs("#schema-columns");
        if (container && container.children.length > 1) {
          btn.closest(".schema-col-row").remove();
          updateDdlPreview();
        }
      };
    });
    qsa("#schema-columns input, #schema-columns .gsel", schemaEditorEl).forEach(function(el) {
      on(el, "change", function() { updateDdlPreview(); });
      if (el.tagName === "INPUT") on(el, "input", function() { updateDdlPreview(); });
    });
  }

  function collectSchemaData() {
    var name = (qs("#schema-name").value || "").trim();
    var cols = [];
    qsa("#schema-columns .schema-col-row").forEach(function(row) {
      var colName = qs(".schema-col-name", row).value.trim();
      if (!colName) return;
      var col = {
        name: colName,
        type: gsel.val(qs(".schema-col-type", row)) || "text",
      };
      if (qs(".schema-col-key", row).checked) col.key = true;
      if (qs(".schema-col-req", row).checked) col.required = true;
      var def = qs(".schema-col-def", row).value.trim();
      if (def) {
        var num = Number(def);
        col.default = isNaN(num) ? def : num;
      }
      cols.push(col);
    });
    return { name: name, columns: cols };
  }

  async function updateDdlPreview() {
    var data = collectSchemaData();
    if (!data.name || data.columns.length === 0) {
      schemaDdlCode.textContent = "-- add a table name and at least one column";
      return;
    }
    try {
      var resp = await schemaApi.ddl(data);
      schemaDdlCode.textContent = resp.ddl || "";
    } catch (err) {
      schemaDdlCode.textContent = "-- " + err.message;
    }
  }

  async function saveSchema() {
    var data = collectSchemaData();
    if (!data.name) { toast("Table name required", true); return; }
    if (data.columns.length === 0) { toast("Add at least one column", true); return; }

    try {
      await schemaApi.save(data);
      toast("Schema " + data.name + " saved");
      currentSchema = data.name;
      await loadSchemas(data.name);
    } catch (err) {
      toast("Save failed: " + err.message, true);
    }
  }

  async function applySchema() {
    if (!currentSchema) return;
    var name = currentSchema;

    var ok = await Goop.dialog.confirm('Create ORM table "' + name + '" from this schema?', "Create Table");
    if (!ok) return;

    try {
      await schemaApi.apply({ name: name });
      toast("Table " + name + " created");
    } catch (err) {
      toast("Create failed: " + err.message, true);
    }
  }

  async function deleteSchema() {
    if (!currentSchema) return;
    var name = currentSchema;
    var answer = await Goop.dialog.confirmDanger({
      title: "Delete Schema",
      message: 'Delete schema "' + name + '"?',
      match: name,
      placeholder: name,
      okText: "Delete",
    });
    if (answer !== name) return;

    try {
      await schemaApi.delete({ name: name });
      toast("Schema " + name + " deleted");
      currentSchema = null;
      schemaTitleEl.textContent = "Select a schema";
      setHidden(schemaActionsEl, true);
      setHidden(schemaDdlEl, true);
      schemaEditorEl.innerHTML = '<p class="empty-state">Select a schema from the sidebar to edit it.</p>';
      await loadSchemas();
    } catch (err) {
      toast("Delete failed: " + err.message, true);
    }
  }

  function showNewSchemaForm() {
    currentSchema = null;
    schemaTitleEl.textContent = "New Schema";
    highlightActiveSchema(null);
    setHidden(schemaActionsEl, false);
    renderSchemaEditor({ name: "", columns: [{ type: "integer", name: "", key: true }] });
    updateDdlPreview();
    qs("#schema-name").focus();
  }

  on(btnNewSchema, "click", showNewSchemaForm);
  on(btnSaveSchema, "click", saveSchema);
  on(btnApplySchema, "click", applySchema);
  on(btnDeleteSchema, "click", deleteSchema);

  // ======== Mapper editor ========
  var mapperListEl   = qs("#db-mapper-list");
  var mapperTitleEl  = qs("#db-mapper-title");
  var mapperActionsEl = qs("#db-mapper-actions");
  var mapperEditorEl = qs("#db-mapper-editor");
  var btnNewMapper   = qs("#db-btn-new-mapper");
  var btnSaveMapper  = qs("#db-btn-save-mapper");
  var btnRunMapper   = qs("#db-btn-run-mapper");
  var btnDeleteMapper = qs("#db-btn-delete-mapper");

  var currentMapper = null;
  var availableTransforms = [];
  var availableTables = [];

  async function loadTransforms() {
    if (availableTransforms.length > 0) return;
    try {
      availableTransforms = await mapperApi.transforms() || [];
    } catch (e) { /* ignore */ }
  }

  async function loadMappers(selectName) {
    await loadTransforms();
    try {
      var results = await Promise.all([
        mapperApi.list(),
        api.tables(),
      ]);
      var mappers = results[0] || [];
      availableTables = results[1] || [];
      renderMapperList(mappers);
      if (selectName) {
        selectMapper(selectName);
      }
    } catch (err) {
      mapperListEl.innerHTML = '<li class="db-table-empty">Failed to load mappings</li>';
    }
  }

  function renderMapperList(mappers) {
    if (!mappers || mappers.length === 0) {
      mapperListEl.innerHTML = '<li class="db-table-empty">No mappings yet</li>';
      return;
    }
    mapperListEl.innerHTML = "";
    mappers.forEach(function(m) {
      var li = document.createElement("li");
      li.className = "sidebar-item";
      li.dataset.mapper = m.name;
      li.innerHTML = '<span class="db-table-name">' + escapeHtml(m.name) + '</span>' +
        '<span class="badge badge-owner">' + m.field_count + ' fields</span>';
      on(li, "click", function() { selectMapper(m.name); });
      mapperListEl.appendChild(li);
    });
  }

  function highlightActiveMapper(name) {
    qsa(".sidebar-item", mapperListEl).forEach(function(el) {
      el.classList.toggle("active", el.dataset.mapper === name);
    });
  }

  async function selectMapper(name) {
    currentMapper = name;
    highlightActiveMapper(name);
    setHidden(mapperActionsEl, false);

    try {
      var m = await mapperApi.get({ name: name });
      mapperTitleEl.textContent = m.name;

      if (m.source_table && m.target_table) {
        try {
          var descs = await Promise.all([
            api.describeTable({ table: m.source_table }),
            api.describeTable({ table: m.target_table }),
          ]);
          var srcCols = (descs[0].schema && descs[0].schema.columns) || descs[0].columns || [];
          var tgtCols = (descs[1].schema && descs[1].schema.columns) || descs[1].columns || [];

          var srcTypes = {};
          srcCols.forEach(function(c) { srcTypes[c.name.toLowerCase()] = (c.type || "").toLowerCase(); });
          var tgtTypes = {};
          tgtCols.forEach(function(c) { tgtTypes[c.name.toLowerCase()] = (c.type || "").toLowerCase(); });

          (m.fields || []).forEach(function(f) {
            if (f.target) f._targetType = tgtTypes[f.target.toLowerCase()] || "";
            if (f.sources && f.sources.length > 0) f._sourceType = srcTypes[f.sources[0].toLowerCase()] || "";
          });
        } catch (e) { /* tables may not exist yet */ }
      }

      renderMapperEditor(m);
    } catch (err) {
      mapperEditorEl.innerHTML = '<p class="empty-state">Error loading mapping: ' + escapeHtml(err.message) + '</p>';
    }
  }

  function transformOptions() {
    var opts = [{ value: "", label: "(none)" }];
    availableTransforms.forEach(function(t) {
      opts.push({ value: t, label: t });
    });
    return opts;
  }

  function tableSelectOptions() {
    var opts = [{ value: "", label: "(select table)" }];
    availableTables.forEach(function(t) {
      opts.push({ value: t.name, label: t.name + (t.mode === "orm" ? " (ORM)" : "") });
    });
    return opts;
  }

  function renderMapperEditor(m) {
    var html = '<div class="db-mapper-form">';
    html += '<div class="form-group">' +
      '<label>Name</label>' +
      '<input type="text" id="mapper-name" class="form-input" value="' + escapeHtml(m.name || '') + '" />' +
    '</div>';
    html += '<div class="form-group">' +
      '<label>Description</label>' +
      '<input type="text" id="mapper-desc" class="form-input" value="' + escapeHtml(m.description || '') + '" />' +
    '</div>';

    html += '<div class="form-group">' +
      '<label>Source / Target</label>' +
      '<div class="mapper-table-selectors">' +
        gsel.html({ id: "mapper-source-table", value: m.source_table || "", options: tableSelectOptions(), placeholder: "Source table" }) +
        '<span class="mapper-arrow">&#8594;</span>' +
        gsel.html({ id: "mapper-target-table", value: m.target_table || "", options: tableSelectOptions(), placeholder: "Target table" }) +
        '<button id="mapper-auto-map" class="db-action-btn">Auto-map</button>' +
      '</div>' +
    '</div>';

    html += '<div class="form-group">' +
      '<label>Field Mappings</label>' +
      '<div id="mapper-fields">';
    if (m.fields && m.fields.length > 0) {
      m.fields.forEach(function(f) { html += mapperFieldRow(f); });
    } else {
      html += mapperFieldRow({});
    }
    html += '</div>';
    html += '<button id="mapper-add-field" class="db-action-btn" style="margin-top:6px">+ Field</button>';
    html += '</div>';

    html += '</div>';
    mapperEditorEl.innerHTML = html;
    initFormSelects(mapperEditorEl);
    bindMapperFieldEvents();

    gsel.init(qs("#mapper-source-table"));
    gsel.init(qs("#mapper-target-table"));

    on(qs("#mapper-auto-map"), "click", autoMapFields);

    on(qs("#mapper-add-field"), "click", function() {
      var container = qs("#mapper-fields");
      var div = document.createElement("div");
      div.innerHTML = mapperFieldRow({});
      var row = div.firstElementChild;
      container.appendChild(row);
      initFormSelects(row);
      bindMapperFieldEvents();
      qs(".mapper-field-target", row).focus();
    });
  }

  async function autoMapFields() {
    var sourceTable = gsel.val(qs("#mapper-source-table"));
    var targetTable = gsel.val(qs("#mapper-target-table"));

    if (!sourceTable || !targetTable) {
      toast("Select both source and target tables", true);
      return;
    }

    try {
      var results = await Promise.all([
        api.describeTable({ table: sourceTable }),
        api.describeTable({ table: targetTable }),
      ]);

      var sourceCols = (results[0].schema && results[0].schema.columns) || results[0].columns || [];
      var targetCols = (results[1].schema && results[1].schema.columns) || results[1].columns || [];

      var sourceInfo = {};
      sourceCols.forEach(function(c) {
        if (systemCols.indexOf(c.name) === -1) {
          sourceInfo[c.name.toLowerCase()] = { name: c.name, type: (c.type || "").toLowerCase() };
        }
      });

      var fields = [];
      targetCols.forEach(function(tc) {
        if (systemCols.indexOf(tc.name) === -1) {
          var colType = (tc.type || "").toLowerCase();
          var match = sourceInfo[tc.name.toLowerCase()];
          var f = { target: tc.name, _targetType: colType };

          if (colType === "guid") {
            f.transform = "guid";
          } else if (match) {
            f.sources = [match.name];
            f._sourceType = match.type;
          } else if (colType === "date") {
            f.transform = "date";
          } else {
            f.sources = [];
          }

          fields.push(f);
        }
      });

      var container = qs("#mapper-fields");
      container.innerHTML = "";
      if (fields.length === 0) {
        container.innerHTML = mapperFieldRow({});
      } else {
        fields.forEach(function(f) {
          container.innerHTML += mapperFieldRow(f);
        });
      }
      initFormSelects(container);
      bindMapperFieldEvents();
      toast("Mapped " + fields.length + " fields");
    } catch (err) {
      toast("Auto-map failed: " + err.message, true);
    }
  }

  function mapperFieldRow(f) {
    var sources = (f.sources || []).join(", ");
    var args = (f.args || []).map(function(a) { return JSON.stringify(a); }).join(", ");
    var constant = f.constant !== undefined && f.constant !== null ? JSON.stringify(f.constant) : "";
    var targetType = f._targetType ? ' <span class="mapper-type-hint">' + escapeHtml(f._targetType) + '</span>' : "";
    var sourceType = f._sourceType ? ' <span class="mapper-type-hint">' + escapeHtml(f._sourceType) + '</span>' : "";
    return '<div class="mapper-field-row">' +
      '<div class="mapper-field-cell mapper-field-target-wrap">' +
        '<input type="text" class="form-input mapper-field-target" placeholder="target" value="' + escapeHtml(f.target || '') + '" title="Target field name" />' +
        targetType +
      '</div>' +
      '<div class="mapper-field-cell mapper-field-sources-wrap">' +
        '<input type="text" class="form-input mapper-field-sources" placeholder="sources (comma sep)" value="' + escapeHtml(sources) + '" title="Source field(s), comma-separated" />' +
        sourceType +
      '</div>' +
      gsel.html({ className: "mapper-field-transform", value: f.transform || "", options: transformOptions() }) +
      '<input type="text" class="form-input mapper-field-args" placeholder="args" value="' + escapeHtml(args) + '" title="Transform arguments (JSON values, comma-separated)" />' +
      '<input type="text" class="form-input mapper-field-constant" placeholder="constant" value="' + escapeHtml(constant) + '" title="Constant value (JSON)" />' +
      '<button class="db-col-remove mapper-field-remove">x</button>' +
    '</div>';
  }

  function bindMapperFieldEvents() {
    qsa(".mapper-field-remove", mapperEditorEl).forEach(function(btn) {
      btn.onclick = function() {
        var container = qs("#mapper-fields");
        if (container && container.children.length > 1) {
          btn.closest(".mapper-field-row").remove();
        }
      };
    });
  }

  function collectMapperData() {
    var name = (qs("#mapper-name").value || "").trim();
    var description = (qs("#mapper-desc").value || "").trim();
    var fields = [];
    qsa("#mapper-fields .mapper-field-row").forEach(function(row) {
      var target = qs(".mapper-field-target", row).value.trim();
      if (!target) return;

      var field = { target: target };
      var sourcesStr = qs(".mapper-field-sources", row).value.trim();
      if (sourcesStr) {
        field.sources = sourcesStr.split(",").map(function(s) { return s.trim(); }).filter(Boolean);
      }

      var transform = gsel.val(qs(".mapper-field-transform", row));
      if (transform) field.transform = transform;

      var argsStr = qs(".mapper-field-args", row).value.trim();
      if (argsStr) {
        try {
          field.args = JSON.parse("[" + argsStr + "]");
        } catch (e) {
          toast("Invalid args JSON in field " + target, true);
        }
      }

      var constStr = qs(".mapper-field-constant", row).value.trim();
      if (constStr) {
        try {
          field.constant = JSON.parse(constStr);
        } catch (e) {
          field.constant = constStr;
        }
      }

      fields.push(field);
    });

    var sourceTable = "";
    var targetTable = "";
    var srcEl = qs("#mapper-source-table");
    var tgtEl = qs("#mapper-target-table");
    if (srcEl) sourceTable = gsel.val(srcEl) || "";
    if (tgtEl) targetTable = gsel.val(tgtEl) || "";

    return { name: name, description: description, source_table: sourceTable, target_table: targetTable, fields: fields };
  }

  async function saveMapper() {
    var data = collectMapperData();
    if (!data.name) { toast("Mapping name required", true); return; }
    if (data.fields.length === 0) { toast("Add at least one field", true); return; }

    try {
      await mapperApi.save(data);
      toast("Mapping " + data.name + " saved");
      currentMapper = data.name;
      await loadMappers(data.name);
    } catch (err) {
      toast("Save failed: " + err.message, true);
    }
  }

  async function executeMapper() {
    if (!currentMapper) { toast("Save the mapping first", true); return; }

    var sourceTable = gsel.val(qs("#mapper-source-table"));
    var targetTable = gsel.val(qs("#mapper-target-table"));

    if (!sourceTable || !targetTable) {
      toast("Select both source and target tables", true);
      return;
    }

    var ok = await Goop.dialog.confirm(
      'Execute mapping "' + currentMapper + '"?\n\nSource: ' + sourceTable + '\nTarget: ' + targetTable,
      "Execute Mapping"
    );
    if (!ok) return;

    try {
      var result = await mapperApi.execute({
        name: currentMapper,
        source_table: sourceTable,
        target_table: targetTable,
      });
      toast("Inserted " + (result.inserted || 0) + " rows into " + targetTable);
    } catch (err) {
      toast("Execute failed: " + err.message, true);
    }
  }

  async function deleteMapper() {
    if (!currentMapper) return;
    var name = currentMapper;
    var answer = await Goop.dialog.confirmDanger({
      title: "Delete Mapping",
      message: 'Delete mapping "' + name + '"?',
      match: name,
      placeholder: name,
      okText: "Delete",
    });
    if (answer !== name) return;

    try {
      await mapperApi.delete({ name: name });
      toast("Mapping " + name + " deleted");
      currentMapper = null;
      mapperTitleEl.textContent = "Select a mapping";
      setHidden(mapperActionsEl, true);
      mapperEditorEl.innerHTML = '<p class="empty-state">Select a mapping from the sidebar to edit it.</p>';
      await loadMappers();
    } catch (err) {
      toast("Delete failed: " + err.message, true);
    }
  }

  function showNewMapperForm() {
    currentMapper = null;
    mapperTitleEl.textContent = "New Mapping";
    setHidden(mapperActionsEl, true);
    highlightActiveMapper(null);

    renderMapperEditor({ name: "", description: "", fields: [{}] });

    setHidden(mapperActionsEl, false);
    qs("#mapper-name").focus();
  }

  on(btnNewMapper, "click", showNewMapperForm);
  on(btnSaveMapper, "click", saveMapper);
  on(btnRunMapper, "click", executeMapper);
  on(btnDeleteMapper, "click", deleteMapper);

  // -------- Init --------
  loadTables();

  window.Goop = window.Goop || {};
  window.Goop.database = { refresh: loadTables };
})();
