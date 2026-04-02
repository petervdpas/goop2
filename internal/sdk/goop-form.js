//
// JSON-driven form renderer for site pages.
// Depends on goop-data.js and goop-identity.js.
//
// Usage:
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="/sdk/goop-identity.js"></script>
//   <script src="/sdk/goop-component-base.js"></script>
//   <script src="/sdk/goop-form.js"></script>
//
//   await Goop.form.render(document.getElementById("my-form"), {
//     table: "responses",
//     fields: [
//       { name: "q1", label: "Your name?",      type: "text" },
//       { name: "q2", label: "Thoughts?",        type: "textarea", placeholder: "..." },
//       { name: "q3", label: "Pick one",          type: "select", options: ["A","B","C"] },
//       { name: "q4", label: "Agree?",            type: "radio",  options: ["Yes","No"] },
//       { name: "q5", label: "Subscribe?",        type: "checkbox" },
//       { name: "q6", label: "How many?",         type: "number" },
//     ],
//     submitLabel: "Submit",   // default "Submit"
//     singleResponse: true,    // one response per peer (lookup by _owner)
//     onDone: function() {},   // optional callback after save
//   });
//
(() => {
  window.Goop = window.Goop || {};

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, "&quot;");
  }

  var _formComponents = [];

  function isComponentType(type) {
    return type === "date" || type === "datetime" || type === "color" ||
           type === "toggle" || type === "tags" || type === "stepper";
  }

  function hasComponentLib() {
    return window.Goop && window.Goop.ui && window.Goop.ui.datepicker;
  }

  function renderField(f) {
    var html = '<div class="gform-field">';
    var id = "gform-" + f.name;
    var useComponent = hasComponentLib();

    if (f.type !== "checkbox" && f.type !== "toggle") {
      html += '<label class="gform-label" for="' + escAttr(id) + '">' + esc(f.label) + "</label>";
    }

    if (useComponent && (f.type === "date" || f.type === "datetime")) {
      html += '<div data-goop-form-upgrade="datepicker" data-field="' + escAttr(f.name) + '" data-goop-time="' + (f.type === "datetime" ? "1" : "") + '"></div>';
    } else if (useComponent && f.type === "select" && f.options) {
      html += '<div data-goop-form-upgrade="select" data-field="' + escAttr(f.name) + '" data-goop-options="' + escAttr(JSON.stringify(f.options)) + '"' + (f.multi ? ' data-goop-multi="1"' : '') + (f.searchable === false ? '' : ' data-goop-searchable="1"') + '></div>';
    } else if (useComponent && f.type === "color") {
      html += '<div data-goop-form-upgrade="colorpicker" data-field="' + escAttr(f.name) + '"></div>';
    } else if (useComponent && f.type === "toggle") {
      html += '<div data-goop-form-upgrade="toggle" data-field="' + escAttr(f.name) + '" data-goop-label="' + escAttr(f.label) + '"></div>';
    } else if (useComponent && f.type === "tags") {
      html += '<div data-goop-form-upgrade="taginput" data-field="' + escAttr(f.name) + '"' + (f.placeholder ? ' data-goop-placeholder="' + escAttr(f.placeholder) + '"' : '') + '></div>';
    } else if (useComponent && f.type === "stepper") {
      html += '<div data-goop-form-upgrade="stepper" data-field="' + escAttr(f.name) + '"' + (f.min != null ? ' data-goop-min="' + f.min + '"' : '') + (f.max != null ? ' data-goop-max="' + f.max + '"' : '') + (f.step ? ' data-goop-step="' + f.step + '"' : '') + '></div>';
    } else {
      switch (f.type) {
        case "textarea":
          html += '<textarea class="gform-textarea" id="' + escAttr(id) + '" data-field="' + escAttr(f.name) + '"';
          if (f.placeholder) html += ' placeholder="' + escAttr(f.placeholder) + '"';
          if (f.required) html += " required";
          html += "></textarea>";
          break;

        case "select":
          html += '<select class="gform-select" id="' + escAttr(id) + '" data-field="' + escAttr(f.name) + '">';
          html += '<option value="">— select —</option>';
          (f.options || []).forEach(function (opt) {
            html += '<option value="' + escAttr(opt) + '">' + esc(opt) + "</option>";
          });
          html += "</select>";
          break;

        case "radio":
          html += '<div class="gform-radio-group" data-field="' + escAttr(f.name) + '">';
          (f.options || []).forEach(function (opt, i) {
            var rid = id + "-" + i;
            html += '<label><input type="radio" name="' + escAttr(f.name) + '" id="' + escAttr(rid) + '" value="' + escAttr(opt) + '">';
            html += " " + esc(opt) + "</label>";
          });
          html += "</div>";
          break;

        case "checkbox":
          html += '<div class="gform-checkbox-wrap">';
          html += '<label><input type="checkbox" id="' + escAttr(id) + '" data-field="' + escAttr(f.name) + '" value="1">';
          html += " " + esc(f.label) + "</label>";
          html += "</div>";
          break;

        case "number":
          html += '<input class="gform-input" type="number" id="' + escAttr(id) + '" data-field="' + escAttr(f.name) + '"';
          if (f.placeholder) html += ' placeholder="' + escAttr(f.placeholder) + '"';
          if (f.required) html += " required";
          html += ">";
          break;

        default:
          html += '<input class="gform-input" type="text" id="' + escAttr(id) + '" data-field="' + escAttr(f.name) + '"';
          if (f.placeholder) html += ' placeholder="' + escAttr(f.placeholder) + '"';
          if (f.required) html += " required";
          html += ">";
          break;
      }
    }

    html += "</div>";
    return html;
  }

  function fillField(el, f, value) {
    if (value == null) value = "";
    var id = "gform-" + f.name;

    var comp = findComponent(f.name);
    if (comp) {
      if (comp.type === "taginput" && typeof value === "string") {
        comp.instance.setValue(value ? value.split(",") : []);
      } else if (comp.type === "toggle") {
        comp.instance.setValue(value === "1" || value === 1 || value === true);
      } else {
        comp.instance.setValue(value);
      }
      return;
    }

    switch (f.type) {
      case "radio": {
        var group = el.querySelector('[data-field="' + f.name + '"]');
        if (!group) break;
        var radio = group.querySelector('input[value="' + CSS.escape(String(value)) + '"]');
        if (radio) radio.checked = true;
        break;
      }
      case "checkbox": {
        var cb = el.querySelector("#" + CSS.escape(id));
        if (cb) cb.checked = value === "1" || value === 1 || value === true;
        break;
      }
      default: {
        var inp = el.querySelector("#" + CSS.escape(id));
        if (inp) inp.value = String(value);
        break;
      }
    }
  }

  function upgradeFormComponents(el) {
    var ui = window.Goop && window.Goop.ui;
    if (!ui) return;
    _formComponents = _formComponents.filter(function(c) { return document.body.contains(c.el); });
    el.querySelectorAll("[data-goop-form-upgrade]").forEach(function(ph) {
      var type = ph.getAttribute("data-goop-form-upgrade");
      var name = ph.getAttribute("data-field");
      var inst = null;
      if (type === "datepicker" && ui.datepicker) {
        inst = ui.datepicker(ph, { name: name, time: ph.getAttribute("data-goop-time") === "1" });
      } else if (type === "select" && ui.select) {
        var rawOpts = [];
        try { rawOpts = JSON.parse(ph.getAttribute("data-goop-options") || "[]"); } catch(_) {}
        inst = ui.select(ph, {
          name: name,
          options: rawOpts,
          multi: ph.hasAttribute("data-goop-multi"),
          searchable: ph.hasAttribute("data-goop-searchable"),
        });
      } else if (type === "colorpicker" && ui.colorpicker) {
        inst = ui.colorpicker(ph, { name: name });
      } else if (type === "toggle" && ui.toggle) {
        inst = ui.toggle(ph, { name: name, label: ph.getAttribute("data-goop-label") || "" });
      } else if (type === "taginput" && ui.taginput) {
        inst = ui.taginput(ph, { name: name, placeholder: ph.getAttribute("data-goop-placeholder") || "" });
      } else if (type === "stepper" && ui.stepper) {
        var sOpts = { name: name };
        if (ph.hasAttribute("data-goop-min")) sOpts.min = parseFloat(ph.getAttribute("data-goop-min"));
        if (ph.hasAttribute("data-goop-max")) sOpts.max = parseFloat(ph.getAttribute("data-goop-max"));
        if (ph.hasAttribute("data-goop-step")) sOpts.step = parseFloat(ph.getAttribute("data-goop-step"));
        inst = ui.stepper(ph, sOpts);
      }
      if (inst) _formComponents.push({ name: name, type: type, instance: inst, el: inst.el });
    });
  }

  function findComponent(name) {
    for (var i = 0; i < _formComponents.length; i++) {
      if (_formComponents[i].name === name) return _formComponents[i];
    }
    return null;
  }

  function collectValues(el, fields) {
    var data = {};
    fields.forEach(function (f) {
      var id = "gform-" + f.name;
      var comp = findComponent(f.name);
      if (comp) {
        var v = comp.instance.getValue();
        data[f.name] = Array.isArray(v) ? v.join(",") : String(v);
        return;
      }

      switch (f.type) {
        case "radio": {
          var group = el.querySelector('[data-field="' + f.name + '"]');
          if (!group) break;
          var checked = group.querySelector("input:checked");
          data[f.name] = checked ? checked.value : "";
          break;
        }
        case "checkbox": {
          var cb = el.querySelector("#" + CSS.escape(id));
          data[f.name] = cb && cb.checked ? "1" : "0";
          break;
        }
        default: {
          var inp = el.querySelector("#" + CSS.escape(id));
          data[f.name] = inp ? inp.value : "";
          break;
        }
      }
    });
    return data;
  }

  // System columns managed by the storage layer — skip when comparing.
  var SYS_COLS = new Set(["_id", "_owner", "_owner_email", "_created_at", "_updated_at"]);

  /**
   * Ensure the database table exists and has columns for every field.
   * The JSON fields array is the source of truth:
   *  - table missing  → create it with columns derived from fields
   *  - table exists   → add any columns that are in fields but missing from the table
   */
  async function ensureTable(db, table, fields) {
    var tables = await db.tables();
    var exists = tables && tables.some(function (t) { return t.name === table; });

    if (!exists) {
      // Create table with all field columns (all TEXT — display types, not storage types)
      var cols = fields.map(function (f) {
        return { name: f.name, type: "TEXT", default: "''" };
      });
      await db.createTable(table, cols);
      return;
    }

    // Table exists — check for missing columns
    var info = await db.describe(table);
    var cols = (info && info.schema && info.schema.columns) || (info && info.columns) || [];
    var have = new Set();
    cols.forEach(function (c) { have.add(c.name); });

    for (var i = 0; i < fields.length; i++) {
      if (!have.has(fields[i].name)) {
        await db.addColumn(table, { name: fields[i].name, type: "TEXT", default: "''" });
      }
    }
  }

  /**
   * Render a JSON-driven form into an element.
   *
   * The fields array is the source of truth — the table and its columns
   * are created/migrated automatically before the form is rendered.
   *
   * @param {HTMLElement} el - container element
   * @param {object} opts - { table, fields, submitLabel, singleResponse, onDone }
   */
  async function render(el, opts) {
    var db = window.Goop.data;
    if (!db) {
      el.innerHTML = '<p style="color:#f87171">goop-data.js is required</p>';
      return;
    }

    var fields = opts.fields || [];
    var table = opts.table;
    var submitLabel = opts.submitLabel || "Submit";
    var singleResponse = !!opts.singleResponse;

    // Ensure table + columns match the fields JSON (fields are leading)
    await ensureTable(db, table, fields);

    // Check for existing response if singleResponse
    var existingRow = null;
    if (singleResponse && window.Goop.identity) {
      try {
        var myId = await window.Goop.identity.id();
        var rows = await db.query(table, {
          where: "_owner = ?",
          args: [myId],
          limit: 1,
        });
        if (rows && rows.length > 0) {
          existingRow = rows[0];
        }
      } catch (_) {}
    }

    // Build form HTML
    var html = '<div class="gform-wrap">';
    fields.forEach(function (f) {
      html += renderField(f);
    });

    var btnLabel = existingRow ? "Update" : submitLabel;
    html += '<div class="gform-btns">';
    html += '<button class="primary" id="gform-submit">' + esc(btnLabel) + "</button>";
    html += "</div>";
    html += '<div class="gform-status" id="gform-status"></div>';
    html += "</div>";

    el.innerHTML = html;
    upgradeFormComponents(el);

    // Pre-fill if existing response
    if (existingRow) {
      fields.forEach(function (f) {
        fillField(el, f, existingRow[f.name]);
      });
    }

    // Wire submit
    el.querySelector("#gform-submit").addEventListener("click", async function () {
      var btn = el.querySelector("#gform-submit");
      var statusEl = el.querySelector("#gform-status");
      var data = collectValues(el, fields);

      btn.disabled = true;
      statusEl.textContent = "Saving...";

      try {
        if (existingRow) {
          await db.update(table, existingRow._id, data);
          statusEl.textContent = "Updated!";
        } else {
          var result = await db.insert(table, data);
          existingRow = { _id: result.id };
          btn.textContent = "Update";
          statusEl.textContent = "Submitted!";
        }
        if (opts.onDone) opts.onDone();
      } catch (err) {
        statusEl.textContent = "Error: " + err.message;
      }

      btn.disabled = false;
    });
  }

  window.Goop.form = { render: render };
})();
