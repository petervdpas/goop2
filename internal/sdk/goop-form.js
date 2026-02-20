//
// JSON-driven form renderer for site pages.
// Depends on goop-data.js and goop-identity.js.
//
// Usage:
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="/sdk/goop-identity.js"></script>
//   <script src="/sdk/goop-ui.js"></script>
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

  const STYLE_ID = "goop-form-style";
  if (!document.getElementById(STYLE_ID)) {
    const s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .gform-wrap{display:flex;flex-direction:column;gap:1.1rem}
      .gform-field{display:flex;flex-direction:column;gap:.3rem}
      .gform-label{font-size:.9rem;font-weight:500}
      .gform-input,.gform-textarea,.gform-select{width:100%;box-sizing:border-box;padding:.5rem .65rem;border:1px solid #2a3142;border-radius:6px;background:#0f1115;color:#e6e9ef;font:inherit;font-size:.95rem}
      .gform-textarea{min-height:5rem;resize:vertical}
      .gform-select{appearance:auto}
      .gform-radio-group,.gform-checkbox-wrap{display:flex;flex-wrap:wrap;gap:.6rem;align-items:center}
      .gform-radio-group label,.gform-checkbox-wrap label{display:flex;align-items:center;gap:.3rem;font-size:.9rem;cursor:pointer}
      .gform-radio-group input[type=radio],.gform-checkbox-wrap input[type=checkbox]{accent-color:#7aa2ff}
      .gform-btns{display:flex;gap:.5rem;margin-top:.4rem}
      .gform-btns button{padding:.5rem 1.2rem;border:1px solid #2a3142;border-radius:6px;cursor:pointer;font:inherit;background:#1e2433;color:#e6e9ef}
      .gform-btns button.primary{background:#7aa2ff;color:#0f1115;border-color:#7aa2ff;font-weight:600}
      .gform-btns button.primary:hover{opacity:.9}
      .gform-status{font-size:.85rem;color:#9aa3b2;margin-top:.25rem}
    `;
    document.head.appendChild(s);
  }

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, "&quot;");
  }

  function renderField(f) {
    var html = '<div class="gform-field">';
    var id = "gform-" + f.name;

    if (f.type !== "checkbox") {
      html += '<label class="gform-label" for="' + escAttr(id) + '">' + esc(f.label) + "</label>";
    }

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

      default: // text
        html += '<input class="gform-input" type="text" id="' + escAttr(id) + '" data-field="' + escAttr(f.name) + '"';
        if (f.placeholder) html += ' placeholder="' + escAttr(f.placeholder) + '"';
        if (f.required) html += " required";
        html += ">";
        break;
    }

    html += "</div>";
    return html;
  }

  function fillField(el, f, value) {
    if (value == null) value = "";
    var id = "gform-" + f.name;

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

  function collectValues(el, fields) {
    var data = {};
    fields.forEach(function (f) {
      var id = "gform-" + f.name;

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
    var schema = await db.describe(table);
    var have = new Set();
    schema.forEach(function (c) { have.add(c.name); });

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
