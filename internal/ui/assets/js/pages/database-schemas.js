// Database — Schemas tab
(() => {
  var G = window.Goop;
  if (!G || !G.core) return;
  var qs = G.core.qs, qsa = G.core.qsa, on = G.core.on;
  var setHidden = G.core.setHidden, escapeHtml = G.core.escapeHtml, toast = G.core.toast;
  var schemaApi = G.api.schema;
  var gsel = G.select;

  if (!qs("#db-page")) return;
  G.database = G.database || {};

  function initFormSelects(container) {
    container.querySelectorAll(".gsel").forEach(function(el) { gsel.init(el); });
  }

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
    { value: "enum", label: "enum" },
    { value: "datetime", label: "datetime" },
    { value: "date", label: "date" },
    { value: "time", label: "time" },
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
  var html = '<div class="db-tx-form">';
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
      '<span class="schema-h-auto">Auto</span>' +
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
  var isEnum = (c.type || "text") === "enum";
  var enumJSON = isEnum ? escapeHtml(JSON.stringify(c.values || [])) : "[]";
  var enumCount = (c.values || []).length;
  var enumBtnLabel = isEnum ? (enumCount > 0 ? enumCount + " vals" : "edit") : "";

  return '<div class="schema-col-row-wrap" data-enum-values="' + enumJSON + '">' +
    '<div class="schema-col-row">' +
      '<input type="text" class="form-input schema-col-name" placeholder="column_name" value="' + escapeHtml(c.name || '') + '" />' +
      '<div class="schema-type-wrap">' +
        gsel.html({ className: "schema-col-type", value: c.type || "text", options: schemaTypeOptions() }) +
        '<button class="schema-enum-btn' + (isEnum ? '' : ' hidden') + '" title="Edit enum values">' + enumBtnLabel + '</button>' +
      '</div>' +
      '<label class="schema-col-check"><input type="checkbox" class="schema-col-key"' + (c.key ? ' checked' : '') + ' /></label>' +
      '<label class="schema-col-check"><input type="checkbox" class="schema-col-req"' + (c.required ? ' checked' : '') + ' /></label>' +
      '<label class="schema-col-check"><input type="checkbox" class="schema-col-auto"' + (c.auto ? ' checked' : '') + ' /></label>' +
      '<input type="text" class="form-input schema-col-def" placeholder="" value="' + escapeHtml(def) + '" />' +
      '<button class="db-col-remove schema-col-remove">x</button>' +
    '</div>' +
  '</div>';
}

function openEnumEditor(wrap) {
  var existing = [];
  try { existing = JSON.parse(wrap.dataset.enumValues || "[]"); } catch(e) {}

  Goop.dialog.custom({
    title: "Enum Values",
    okText: "Save",
    build: function(body) {
      var html = '<div class="schema-enum-popup-hint">Key is stored in the database. Label is shown to the user.</div>';
      html += '<div class="schema-enum-pairs">';
      if (existing.length > 0) {
        existing.forEach(function(v) { html += enumPairHtml(v.key, v.label); });
      } else {
        html += enumPairHtml("", "");
      }
      html += '</div>';
      html += '<button class="db-action-btn schema-enum-popup-add" style="margin-top:6px">+ Value</button>';
      body.innerHTML = html;

      qs(".schema-enum-popup-add", body).onclick = function() {
        var pairs = qs(".schema-enum-pairs", body);
        var div = document.createElement("div");
        div.innerHTML = enumPairHtml("", "");
        pairs.appendChild(div.firstElementChild);
        bindEnumPopupRemove(body);
        var keys = qsa(".schema-enum-key", pairs);
        if (keys.length > 0) keys[keys.length - 1].focus();
      };
      bindEnumPopupRemove(body);
    },
    collect: function(body) {
      var vals = [];
      qsa(".schema-enum-pair", body).forEach(function(pair) {
        var key = qs(".schema-enum-key", pair).value.trim();
        var label = qs(".schema-enum-label", pair).value.trim();
        if (key) vals.push({ key: key, label: label || key });
      });
      return vals;
    },
  }).then(function(vals) {
    if (vals === null) return;
    wrap.dataset.enumValues = JSON.stringify(vals);
    var btn = qs(".schema-enum-btn", wrap);
    if (btn) btn.textContent = vals.length > 0 ? vals.length + " vals" : "edit";
    updateDdlPreview();
  });
}

function enumPairHtml(key, label) {
  return '<div class="schema-enum-pair">' +
    '<input type="text" class="form-input schema-enum-key" placeholder="key" value="' + escapeHtml(key || '') + '" />' +
    '<input type="text" class="form-input schema-enum-label" placeholder="label" value="' + escapeHtml(label || '') + '" />' +
    '<button class="db-col-remove schema-enum-remove">x</button>' +
  '</div>';
}

function bindEnumPopupRemove(container) {
  qsa(".schema-enum-remove", container).forEach(function(btn) {
    btn.onclick = function() {
      var pairs = qsa(".schema-enum-pair", container);
      if (pairs.length > 1) btn.closest(".schema-enum-pair").remove();
    };
  });
}

function bindSchemaColEvents() {
  qsa(".schema-col-remove", schemaEditorEl).forEach(function(btn) {
    btn.onclick = function() {
      var container = qs("#schema-columns");
      if (container && container.children.length > 1) {
        btn.closest(".schema-col-row-wrap").remove();
        updateDdlPreview();
      }
    };
  });
  qsa("#schema-columns .gsel.schema-col-type", schemaEditorEl).forEach(function(sel) {
    gsel.init(sel, function(val) {
      var wrap = sel.closest(".schema-col-row-wrap");
      if (!wrap) return;
      var enumBtn = qs(".schema-enum-btn", wrap);
      if (enumBtn) {
        if (val === "enum") {
          enumBtn.classList.remove("hidden");
          if (!wrap.dataset.enumValues || wrap.dataset.enumValues === "[]") {
            enumBtn.textContent = "edit";
          }
        } else {
          enumBtn.classList.add("hidden");
        }
      }
      updateDdlPreview();
    });
  });
  qsa(".schema-enum-btn", schemaEditorEl).forEach(function(btn) {
    btn.onclick = function(e) {
      e.preventDefault();
      var wrap = btn.closest(".schema-col-row-wrap");
      if (wrap) openEnumEditor(wrap);
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
  qsa("#schema-columns .schema-col-row-wrap").forEach(function(wrap) {
    var row = qs(".schema-col-row", wrap);
    var colName = qs(".schema-col-name", row).value.trim();
    if (!colName) return;
    var col = {
      name: colName,
      type: gsel.val(qs(".schema-col-type", row)) || "text",
    };
    if (qs(".schema-col-key", row).checked) col.key = true;
    if (qs(".schema-col-req", row).checked) col.required = true;
    if (qs(".schema-col-auto", row).checked) col.auto = true;
    var def = qs(".schema-col-def", row).value.trim();
    if (def) {
      var num = Number(def);
      col.default = isNaN(num) ? def : num;
    }
    if (col.type === "enum") {
      try { col.values = JSON.parse(wrap.dataset.enumValues || "[]"); } catch(e) { col.values = []; }
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
  if (!data.name) { toast("Table name required", "warning"); return; }
  if (data.columns.length === 0) { toast("Add at least one column", "warning"); return; }

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
  renderSchemaEditor({ name: "", columns: [{ type: "guid", name: "Id", key: true, auto: true }] });
  updateDdlPreview();
  qs("#schema-name").focus();
}

on(btnNewSchema, "click", showNewSchemaForm);
on(btnSaveSchema, "click", saveSchema);
on(btnApplySchema, "click", applySchema);
on(btnDeleteSchema, "click", deleteSchema);


  G.database.loadSchemas = function(name) { loadSchemas(name); };
})();
