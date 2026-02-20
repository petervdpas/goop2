//
// Auto-generated CRUD forms from table schemas.
// Depends on goop-data.js and goop-ui.js.
//
// Usage:
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="/sdk/goop-ui.js"></script>
//   <script src="/sdk/goop-forms.js"></script>
//
//   // render a full CRUD interface for a table into an element
//   Goop.forms.render(document.getElementById("my-div"), "bookmarks");
//
//   // render just an insert form
//   Goop.forms.insertForm(document.getElementById("form-div"), "bookmarks", function() {
//     console.log("row inserted!");
//   });
//
(() => {
  window.Goop = window.Goop || {};

  const STYLE_ID = "goop-forms-style";
  if (!document.getElementById(STYLE_ID)) {
    const s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .gf-form{display:flex;flex-direction:column;gap:.5rem;margin:.75rem 0}
      .gf-field label{display:block;font-size:.8rem;color:#9aa3b2;margin-bottom:.15rem}
      .gf-field input{width:100%;box-sizing:border-box;padding:.4rem .55rem;border:1px solid #2a3142;border-radius:6px;background:#0f1115;color:#e6e9ef;font:inherit}
      .gf-btns{display:flex;gap:.5rem;margin-top:.25rem}
      .gf-btns button{padding:.35rem .75rem;border:1px solid #2a3142;border-radius:6px;cursor:pointer;font:inherit;background:#1e2433;color:#e6e9ef}
      .gf-btns button.primary{background:#7aa2ff;color:#0f1115;border-color:#7aa2ff}
      .gf-btns button.danger{background:#f87171;color:#0f1115;border-color:#f87171}
      .gf-table{width:100%;border-collapse:collapse;font-size:.9rem;margin:.5rem 0}
      .gf-table th{text-align:left;padding:.4rem .6rem;border-bottom:1px solid #2a3142;color:#9aa3b2;font-weight:500;font-size:.8rem;text-transform:uppercase}
      .gf-table td{padding:.4rem .6rem;border-bottom:1px solid #1e2433}
      .gf-table tr:last-child td{border-bottom:none}
      .gf-actions button{padding:.2rem .5rem;border:1px solid #2a3142;border-radius:4px;cursor:pointer;font-size:.8rem;background:#1e2433;color:#e6e9ef;margin-right:.25rem}
      .gf-actions button.danger{color:#f87171;border-color:#f87171}
    `;
    document.head.appendChild(s);
  }

  function esc(s) {
    return String(s).replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;");
  }

  // columns to skip in forms
  const SKIP = new Set(["_id", "_owner", "_owner_email", "_created_at", "_updated_at"]);

  /**
   * Render a full CRUD interface for a table.
   */
  async function render(el, table) {
    const db = window.Goop.data;
    if (!db) { el.innerHTML = '<p style="color:#f87171">goop-data.js required</p>'; return; }

    async function refresh() {
      const [cols, rows] = await Promise.all([
        db.describe(table),
        db.query(table, { limit: 200 }),
      ]);

      const userCols = cols.filter((c) => !SKIP.has(c.name));
      let html = "<h3>" + esc(table) + "</h3>";

      // insert form
      html += '<div class="gf-form" id="gf-insert">';
      userCols.forEach((c) => {
        html += '<div class="gf-field"><label>' + esc(c.name) + "</label>";
        html += '<input data-col="' + esc(c.name) + '" /></div>';
      });
      html += '<div class="gf-btns"><button class="primary" id="gf-btn-insert">Add row</button></div>';
      html += "</div>";

      // data table
      if (rows && rows.length > 0) {
        html += '<table class="gf-table"><thead><tr>';
        userCols.forEach((c) => { html += "<th>" + esc(c.name) + "</th>"; });
        html += "<th></th></tr></thead><tbody>";

        rows.forEach((row) => {
          html += "<tr>";
          userCols.forEach((c) => {
            html += "<td>" + esc(row[c.name]) + "</td>";
          });
          html += '<td class="gf-actions">';
          html += '<button class="danger" data-delete="' + row._id + '">Delete</button>';
          html += "</td></tr>";
        });
        html += "</tbody></table>";
      } else {
        html += '<p style="color:#9aa3b2">No rows yet.</p>';
      }

      el.innerHTML = html;

      // wire insert button
      el.querySelector("#gf-btn-insert").addEventListener("click", async () => {
        const data = {};
        el.querySelectorAll("#gf-insert input[data-col]").forEach((inp) => {
          const v = inp.value.trim();
          if (v !== "") data[inp.getAttribute("data-col")] = v;
        });
        await db.insert(table, data);
        refresh();
      });

      // wire delete buttons
      el.querySelectorAll("[data-delete]").forEach((btn) => {
        btn.addEventListener("click", async () => {
          const id = parseInt(btn.getAttribute("data-delete"), 10);
          if (window.Goop.ui) {
            const ok = await window.Goop.ui.confirm("Delete row #" + id + "?");
            if (!ok) return;
          }
          await db.remove(table, id);
          refresh();
        });
      });
    }

    await refresh();
  }

  /**
   * Render just an insert form for a table.
   */
  async function insertForm(el, table, onInsert) {
    const db = window.Goop.data;
    if (!db) { el.innerHTML = '<p style="color:#f87171">goop-data.js required</p>'; return; }

    const cols = await db.describe(table);
    const userCols = cols.filter((c) => !SKIP.has(c.name));

    let html = '<div class="gf-form">';
    userCols.forEach((c) => {
      html += '<div class="gf-field"><label>' + esc(c.name) + "</label>";
      html += '<input data-col="' + esc(c.name) + '" /></div>';
    });
    html += '<div class="gf-btns"><button class="primary" id="gf-btn-add">Add</button></div>';
    html += "</div>";
    el.innerHTML = html;

    el.querySelector("#gf-btn-add").addEventListener("click", async () => {
      const data = {};
      el.querySelectorAll("input[data-col]").forEach((inp) => {
        const v = inp.value.trim();
        if (v !== "") data[inp.getAttribute("data-col")] = v;
      });
      await db.insert(table, data);
      // clear inputs
      el.querySelectorAll("input[data-col]").forEach((inp) => { inp.value = ""; });
      if (onInsert) onInsert();
    });
  }

  window.Goop.forms = { render, insertForm };
})();
