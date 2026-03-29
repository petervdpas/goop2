// Database — Transformations tab
(() => {
  var G = window.Goop;
  if (!G || !G.core) return;
  var qs = G.core.qs, qsa = G.core.qsa, on = G.core.on;
  var setHidden = G.core.setHidden, escapeHtml = G.core.escapeHtml, toast = G.core.toast;
  var api = G.api.data;
  var txApi = G.api.transform;
  var gsel = G.select;

  if (!qs("#db-page")) return;
  G.database = G.database || {};

  var systemCols = ["_id", "_owner", "_owner_email", "_created_at", "_updated_at"];

  function initFormSelects(container) {
    container.querySelectorAll(".gsel").forEach(function(el) { gsel.init(el); });
  }

  var txListEl      = qs("#db-tx-list");
  var txTitleEl     = qs("#db-tx-title");
  var txActionsEl   = qs("#db-tx-actions");
  var txEditorEl    = qs("#db-tx-editor");
  var txMappingsEl  = qs("#db-tx-mappings");
  var txSubtabs     = qs("#db-tx-subtabs");
  var txConfigPane  = qs("#db-tx-config-pane");
  var txMappingsPane = qs("#db-tx-mappings-pane");
  var btnNewTx    = qs("#db-btn-new-tx");
  var btnSaveTx   = qs("#db-btn-save-tx");
  var btnRunTx    = qs("#db-btn-run-tx");
  var btnDeleteTx = qs("#db-btn-delete-tx");

  function switchTxView(view) {
    qsa(".sub-tab", txSubtabs).forEach(function(t) {
      t.classList.toggle("active", t.dataset.txView === view);
    });
    setHidden(txConfigPane, view !== "config");
    setHidden(txMappingsPane, view !== "mappings");
  }

  qsa(".sub-tab", txSubtabs).forEach(function(tab) {
    on(tab, "click", function() {
      switchTxView(tab.dataset.txView);
    });
  });

  var currentTx = null;
  var availableTransforms = [];
  var availableTables = [];

  async function loadAvailableTransforms() {
    if (availableTransforms.length > 0) return;
    try { availableTransforms = await txApi.transforms() || []; } catch (e) {}
  }

  async function loadTxList(selectName) {
    await loadAvailableTransforms();
    try {
      var results = await Promise.all([txApi.list(), api.tables()]);
      var items = results[0] || [];
      availableTables = results[1] || [];
      renderTxList(items);
      if (selectName) selectTx(selectName);
    } catch (err) {
      txListEl.innerHTML = '<li class="db-table-empty">No transformations yet</li>';
    }
  }

  function renderTxList(items) {
    if (!items || items.length === 0) {
      txListEl.innerHTML = '<li class="db-table-empty">No transformations yet</li>';
      return;
    }
    txListEl.innerHTML = "";
    items.forEach(function(m) {
      var li = document.createElement("li");
      li.className = "sidebar-item";
      li.dataset.tx = m.name;
      li.innerHTML = '<span class="db-table-name">' + escapeHtml(m.name) + '</span>' +
        '<span class="badge badge-owner">' + m.field_count + ' fields</span>';
      on(li, "click", function() { selectTx(m.name); });
      txListEl.appendChild(li);
    });
  }

  function highlightActiveTx(name) {
    qsa(".sidebar-item", txListEl).forEach(function(el) {
      el.classList.toggle("active", el.dataset.tx === name);
    });
  }

  async function selectTx(name) {
    currentTx = name;
    highlightActiveTx(name);
    setHidden(txActionsEl, false);
    setHidden(txSubtabs, false);

    try {
      var m = await txApi.get({ name: name });
      txTitleEl.textContent = m.name;

      var srcName = m.source && m.source.type === "table" && m.source.name || "";
      var tgtName = m.target && m.target.type === "table" && m.target.name || "";
      if (srcName && tgtName) {
        try {
          var descs = await Promise.all([
            api.describeTable({ table: srcName }),
            api.describeTable({ table: tgtName }),
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
        } catch (e) {}
      }

      renderTxEditor(m);
    } catch (err) {
      txEditorEl.innerHTML = '<p class="empty-state">Error: ' + escapeHtml(err.message) + '</p>';
    }
  }

  function transformOptions() {
    var opts = [{ value: "", label: "(none)" }];
    availableTransforms.forEach(function(t) { opts.push({ value: t, label: t }); });
    return opts;
  }

  function endpointTypeOptions() {
    return [
      { value: "table", label: "Table" },
      { value: "csv", label: "CSV" },
      { value: "json", label: "JSON" },
      { value: "api", label: "API" },
    ];
  }

  function tableSelectOptions() {
    var opts = [{ value: "", label: "(select)" }];
    availableTables.forEach(function(t) {
      opts.push({ value: t.name, label: t.name + (t.mode === "orm" ? " (ORM)" : "") });
    });
    return opts;
  }

  function endpointInput(prefix, ep) {
    ep = ep || {};
    var type = ep.type || "table";
    return '<div class="tx-endpoint" id="' + prefix + '-endpoint">' +
      '<div class="tx-ep-row">' +
        gsel.html({ id: prefix + "-type", className: "tx-ep-type", value: type, options: endpointTypeOptions() }) +
        '<div class="tx-ep-value" id="' + prefix + '-value">' +
          endpointValueInput(prefix, type, ep) +
        '</div>' +
      '</div>' +
    '</div>';
  }

  function endpointValueInput(prefix, type, ep) {
    ep = ep || {};
    if (type === "table") {
      return gsel.html({ id: prefix + "-table", className: "tx-ep-table-sel", value: ep.name || "", options: tableSelectOptions() });
    } else if (type === "csv" || type === "json") {
      return '<div class="tx-ep-path-wrap">' +
        '<input type="text" id="' + prefix + '-path" class="form-input" placeholder="file path" value="' + escapeHtml(ep.path || '') + '" />' +
        '<button class="db-action-btn tx-ep-browse" data-target="' + prefix + '-path">Browse</button>' +
      '</div>';
    } else if (type === "api") {
      return '<input type="text" id="' + prefix + '-url" class="form-input" placeholder="https://..." value="' + escapeHtml(ep.url || '') + '" />';
    }
    return '';
  }

  function bindEndpointTypeChange(prefix) {
    var selEl = qs("#" + prefix + "-type");
    if (!selEl) return;
    gsel.init(selEl, function(val) {
      var valueEl = qs("#" + prefix + "-value");
      if (valueEl) {
        valueEl.innerHTML = endpointValueInput(prefix, val, {});
        initFormSelects(valueEl);
        bindBrowseButtons(valueEl);
      }
    });
    var epEl = qs("#" + prefix + "-endpoint");
    if (epEl) bindBrowseButtons(epEl);
  }

  function bindBrowseButtons(container) {
    qsa(".tx-ep-browse", container).forEach(function(btn) {
      btn.onclick = function(e) {
        e.preventDefault();
        var targetId = btn.dataset.target;
        var input = qs("#" + targetId);
        if (!input) return;
        var ext = [];
        var typeEl = btn.closest(".tx-endpoint");
        if (typeEl) {
          var tSel = qs(".tx-ep-type", typeEl);
          var tVal = tSel ? gsel.val(tSel) : "";
          if (tVal === "csv") ext = [".csv"];
          else if (tVal === "json") ext = [".json"];
        }
        var opts = { title: "Select File" };
        if (ext.length > 0) opts.extensions = ext;
        G.dialog.filePicker(opts).then(function(path) {
          if (path) input.value = path;
        });
      };
    });
  }

  function collectEndpoint(prefix) {
    var typeEl = qs("#" + prefix + "-type");
    var type = typeEl ? gsel.val(typeEl) : "table";
    var ep = { type: type };
    if (type === "table") {
      var tblEl = qs("#" + prefix + "-table");
      ep.name = tblEl ? gsel.val(tblEl) : "";
    } else if (type === "csv" || type === "json") {
      var pathEl = qs("#" + prefix + "-path");
      ep.path = pathEl ? pathEl.value.trim() : "";
    } else if (type === "api") {
      var urlEl = qs("#" + prefix + "-url");
      ep.url = urlEl ? urlEl.value.trim() : "";
    }
    return ep;
  }

  function renderTxEditor(m) {
    var html = '<div class="db-tx-form">';
    html += '<div class="form-group">' +
      '<label>Name</label>' +
      '<input type="text" id="tx-name" class="form-input" value="' + escapeHtml(m.name || '') + '" />' +
    '</div>';
    html += '<div class="form-group">' +
      '<label>Description</label>' +
      '<input type="text" id="tx-desc" class="form-input" value="' + escapeHtml(m.description || '') + '" />' +
    '</div>';

    html += '<div class="form-group">' +
      '<label>Source</label>' +
      endpointInput("tx-src", m.source) +
    '</div>';

    html += '<div class="form-group">' +
      '<label>Target</label>' +
      endpointInput("tx-tgt", m.target) +
    '</div>';

    html += '</div>';
    txEditorEl.innerHTML = html;
    initFormSelects(txEditorEl);

    bindEndpointTypeChange("tx-src");
    bindEndpointTypeChange("tx-tgt");

    var mhtml = '<div class="db-tx-form">';
    mhtml += '<div style="margin-bottom:12px"><button id="tx-auto-map" class="db-action-btn">Auto-map fields</button></div>';

    mhtml += '<div class="form-group">' +
      '<label>Field Mappings</label>' +
      '<div class="tx-field-header">' +
        '<span class="tx-h-src">SOURCE</span>' +
        '<span class="tx-h-tx">TRANSFORM</span>' +
        '<span class="tx-h-tgt">TARGET</span>' +
        '<span class="tx-h-args">ARGS</span>' +
        '<span class="tx-h-const">CONSTANT</span>' +
        '<span class="tx-h-rm"></span>' +
      '</div>' +
      '<div id="tx-fields">';
    if (m.fields && m.fields.length > 0) {
      m.fields.forEach(function(f) { mhtml += txFieldRow(f); });
    } else {
      mhtml += txFieldRow({});
    }
    mhtml += '</div>';
    mhtml += '<button id="tx-add-field" class="db-action-btn" style="margin-top:6px">+ Field</button>';
    mhtml += '</div>';

    mhtml += '</div>';
    txMappingsEl.innerHTML = mhtml;
    initFormSelects(txMappingsEl);
    bindTxFieldEvents();

    on(qs("#tx-auto-map"), "click", autoMapFields);

    on(qs("#tx-add-field"), "click", function() {
      var container = qs("#tx-fields");
      var div = document.createElement("div");
      div.innerHTML = txFieldRow({});
      var row = div.firstElementChild;
      container.appendChild(row);
      initFormSelects(row);
      bindTxFieldEvents();
      qs(".tx-field-target", row).focus();
    });

    switchTxView("config");
  }

  async function autoMapFields() {
    var src = collectEndpoint("tx-src");
    var tgt = collectEndpoint("tx-tgt");

    try {
      var srcFieldsRaw = await txApi.sourceFields(src);

      var targetCols = null;
      if (tgt.type === "table" && tgt.name) {
        var tgtDesc = await api.describeTable({ table: tgt.name });
        targetCols = (tgtDesc.schema && tgtDesc.schema.columns) || tgtDesc.columns || [];
      }

      var srcList = [];
      if (Array.isArray(srcFieldsRaw)) {
        srcFieldsRaw.forEach(function(c) {
          if (typeof c === "string") {
            srcList.push({ name: c, type: "" });
          } else if (c.name && systemCols.indexOf(c.name) === -1) {
            srcList.push({ name: c.name, type: (c.type || "").toLowerCase() });
          }
        });
      }

      var sourceInfo = {};
      srcList.forEach(function(c) { sourceInfo[c.name.toLowerCase()] = c; });

      var fields = [];

      if (targetCols) {
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
            } else if (colType === "datetime") {
              f.transform = "datetime";
            } else if (colType === "date") {
              f.transform = "date";
            } else if (colType === "time") {
              f.transform = "time";
            } else {
              f.sources = [];
            }
            fields.push(f);
          }
        });
      } else {
        srcList.forEach(function(c) {
          fields.push({ sources: [c.name], target: c.name, _sourceType: c.type });
        });
      }

      var container = qs("#tx-fields");
      container.innerHTML = "";
      if (fields.length === 0) {
        container.innerHTML = txFieldRow({});
      } else {
        fields.forEach(function(f) { container.innerHTML += txFieldRow(f); });
      }
      initFormSelects(container);
      bindTxFieldEvents();
      toast("Mapped " + fields.length + " fields", "info");
    } catch (err) {
      toast("Auto-map failed: " + err.message, true);
    }
  }

  function txFieldRow(f) {
    var sources = (f.sources || []).join(", ");
    var args = (f.args || []).map(function(a) { return JSON.stringify(a); }).join(", ");
    var constant = f.constant !== undefined && f.constant !== null ? JSON.stringify(f.constant) : "";
    var targetType = f._targetType ? ' <span class="tx-type-hint">' + escapeHtml(f._targetType) + '</span>' : "";
    var sourceType = f._sourceType ? ' <span class="tx-type-hint">' + escapeHtml(f._sourceType) + '</span>' : "";
    return '<div class="tx-field-row">' +
      '<div class="tx-field-cell tx-field-sources-wrap">' +
        '<input type="text" class="form-input tx-field-sources" placeholder="source" value="' + escapeHtml(sources) + '" />' +
        sourceType +
      '</div>' +
      gsel.html({ className: "tx-field-transform", value: f.transform || "", options: transformOptions() }) +
      '<div class="tx-field-cell tx-field-target-wrap">' +
        '<input type="text" class="form-input tx-field-target" placeholder="target" value="' + escapeHtml(f.target || '') + '" />' +
        targetType +
      '</div>' +
      '<input type="text" class="form-input tx-field-args" placeholder="args" value="' + escapeHtml(args) + '" />' +
      '<input type="text" class="form-input tx-field-constant" placeholder="constant" value="' + escapeHtml(constant) + '" />' +
      '<button class="db-col-remove tx-field-remove">x</button>' +
    '</div>';
  }

  function bindTxFieldEvents() {
    qsa(".tx-field-remove", txMappingsEl).forEach(function(btn) {
      btn.onclick = function() {
        var container = qs("#tx-fields");
        if (container && container.children.length > 1) {
          btn.closest(".tx-field-row").remove();
        }
      };
    });
  }

  function collectTxData() {
    var name = (qs("#tx-name").value || "").trim();
    var description = (qs("#tx-desc").value || "").trim();
    var fields = [];
    qsa("#tx-fields .tx-field-row").forEach(function(row) {
      var target = qs(".tx-field-target", row).value.trim();
      if (!target) return;

      var field = { target: target };
      var sourcesStr = qs(".tx-field-sources", row).value.trim();
      if (sourcesStr) {
        field.sources = sourcesStr.split(",").map(function(s) { return s.trim(); }).filter(Boolean);
      }

      var transform = gsel.val(qs(".tx-field-transform", row));
      if (transform) field.transform = transform;

      var argsStr = qs(".tx-field-args", row).value.trim();
      if (argsStr) {
        try { field.args = JSON.parse("[" + argsStr + "]"); }
        catch (e) { toast("Invalid args JSON in field " + target, "warning"); }
      }

      var constStr = qs(".tx-field-constant", row).value.trim();
      if (constStr) {
        try { field.constant = JSON.parse(constStr); }
        catch (e) { field.constant = constStr; }
      }

      fields.push(field);
    });

    return {
      name: name,
      description: description,
      source: collectEndpoint("tx-src"),
      target: collectEndpoint("tx-tgt"),
      fields: fields,
    };
  }

  async function saveTx() {
    var data = collectTxData();
    if (!data.name) { toast("Transformation name required", "warning"); return; }
    if (data.fields.length === 0) { toast("Add at least one field", "warning"); return; }

    try {
      await txApi.save(data);
      toast("Transformation " + data.name + " saved");
      currentTx = data.name;
      await loadTxList(data.name);
    } catch (err) {
      toast("Save failed: " + err.message, true);
    }
  }

  async function executeTx() {
    if (!currentTx) { toast("Save the transformation first", "warning"); return; }

    var data = collectTxData();
    var tgt = data.target || {};
    var msg = 'Execute transformation "' + currentTx + '"?';

    if ((tgt.type === "csv" || tgt.type === "json") && tgt.path) {
      try {
        var check = await txApi.fileExists({ path: tgt.path });
        if (check && check.exists) {
          var overwrite = await G.dialog.confirm(
            'Target file already exists:\n' + tgt.path + '\n\nOverwrite?',
            "File Exists"
          );
          if (!overwrite) return;
          msg = 'Execute transformation "' + currentTx + '"?\n\nTarget file will be overwritten.';
        }
      } catch (e) {}
    }

    var ok = await G.dialog.confirm(msg, "Execute");
    if (!ok) return;

    try {
      var result = await txApi.execute({ name: currentTx });
      var count = result.inserted || result.written || 0;
      toast("Executed: " + count + " rows" + (result.target ? " → " + result.target : ""));
    } catch (err) {
      toast("Execute failed: " + err.message, true);
    }
  }

  async function deleteTx() {
    if (!currentTx) return;
    var name = currentTx;
    var answer = await G.dialog.confirmDanger({
      title: "Delete Transformation",
      message: 'Delete transformation "' + name + '"?',
      match: name,
      placeholder: name,
      okText: "Delete",
    });
    if (answer !== name) return;

    try {
      await txApi.delete({ name: name });
      toast("Transformation " + name + " deleted");
      currentTx = null;
      txTitleEl.textContent = "Select a transformation";
      setHidden(txActionsEl, true);
      setHidden(txSubtabs, true);
      txEditorEl.innerHTML = '<p class="empty-state">Select a transformation from the sidebar to edit it.</p>';
      txMappingsEl.innerHTML = '';
      await loadTxList();
    } catch (err) {
      toast("Delete failed: " + err.message, true);
    }
  }

  function showNewTxForm() {
    currentTx = null;
    txTitleEl.textContent = "New Transformation";
    highlightActiveTx(null);
    setHidden(txActionsEl, false);
    setHidden(txSubtabs, false);
    renderTxEditor({ name: "", description: "", source: { type: "table" }, target: { type: "table" }, fields: [{}] });
    qs("#tx-name").focus();
  }

  on(btnNewTx, "click", showNewTxForm);
  on(btnSaveTx, "click", saveTx);
  on(btnRunTx, "click", executeTx);
  on(btnDeleteTx, "click", deleteTx);

  G.database.loadTransforms = function(name) { loadTxList(name); };
})();
