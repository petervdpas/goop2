//
// Semantic DOM builders — the easy layer.
// Every function returns a real DOM element. No innerHTML, no string concat.
// Templates provide their own CSS targeting gc-* classes.
//
// Usage:
//
//   var myCard = Goop.card({ title: "Hello", body: "World" });
//   document.getElementById("app").appendChild(myCard);
//
//   Goop.list(el, notes, function(note) {
//     return Goop.card({
//       title: note.title,
//       body: note.description,
//       actions: [
//         Goop.button("Edit", { onclick: function() { edit(note); } }),
//         Goop.button("Delete", { danger: true, onclick: function() { del(note); } })
//       ]
//     });
//   }, { empty: "No notes yet." });
//
(() => {
  window.Goop = window.Goop || {};
  var d = Goop.dom;
  if (!d) {
    console.warn("goop-component-elements: Goop.dom not found, load goop-component-base.js first");
    return;
  }

  var SID = "gc-elements-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-card {
        background: var(--goop-panel, #151924);
        border: 1px solid var(--goop-border, #2a3142);
        border-radius: var(--goop-radius, 6px);
        overflow: hidden;
      }
      .gc-card-image { width: 100%; display: block; }
      .gc-card-body { padding: .75rem .85rem; }
      .gc-card-title { margin: 0 0 .25rem; font-size: 1rem; font-weight: 600; color: var(--goop-text, #e6e9ef); }
      .gc-card-subtitle { font-size: .8rem; color: var(--goop-muted, #9aa3b2); margin-bottom: .35rem; }
      .gc-card-text { font-size: .9rem; color: var(--goop-text, #e6e9ef); }
      .gc-card-footer { padding: .5rem .85rem; border-top: 1px solid var(--goop-border, #2a3142); display: flex; align-items: center; justify-content: space-between; }
      .gc-card-actions { display: flex; gap: .35rem; padding: .5rem .85rem; border-top: 1px solid var(--goop-border, #2a3142); }
      .gc-card[data-goop-clickable] { cursor: pointer; }
      .gc-card[data-goop-clickable]:hover { border-color: var(--goop-accent, #7aa2ff); }

      .gc-btn {
        display: inline-flex; align-items: center; justify-content: center; gap: .3rem;
        padding: .45rem .9rem; border: 1px solid var(--goop-border, #2a3142);
        border-radius: var(--goop-radius, 6px); cursor: pointer;
        font: inherit; font-size: .9rem; color: var(--goop-text, #e6e9ef);
        background: var(--goop-field, rgba(0,0,0,.25));
        transition: border-color .15s, opacity .15s;
      }
      .gc-btn:hover { border-color: var(--goop-accent, #7aa2ff); }
      .gc-btn:disabled { opacity: .4; cursor: not-allowed; }
      .gc-btn-primary {
        background: var(--goop-accent, #7aa2ff);
        color: var(--goop-bg, #0f1115);
        border-color: var(--goop-accent, #7aa2ff);
        font-weight: 600;
      }
      .gc-btn-primary:hover { opacity: .9; }
      .gc-btn-danger {
        background: color-mix(in srgb, var(--goop-danger, #f87171) 15%, transparent);
        color: var(--goop-danger, #f87171);
        border-color: var(--goop-danger, #f87171);
      }
      .gc-btn-sm { padding: .25rem .55rem; font-size: .8rem; }
      .gc-btn-icon { padding: .3rem; min-width: 0; }

      .gc-table { width: 100%; border-collapse: collapse; font-size: .9rem; }
      .gc-table th {
        text-align: left; padding: .45rem .6rem;
        border-bottom: 1px solid var(--goop-border, #2a3142);
        color: var(--goop-muted, #9aa3b2); font-weight: 500; font-size: .8rem;
      }
      .gc-table td { padding: .45rem .6rem; border-bottom: 1px solid color-mix(in srgb, var(--goop-border, #2a3142) 50%, transparent); color: var(--goop-text, #e6e9ef); }
      .gc-table tbody tr:last-child td { border-bottom: none; }
      .gc-table tbody tr[data-goop-clickable] { cursor: pointer; }
      .gc-table tbody tr[data-goop-clickable]:hover { background: color-mix(in srgb, var(--goop-accent, #7aa2ff) 6%, transparent); }

      .gc-field { display: flex; flex-direction: column; gap: .25rem; }
      .gc-field-label { font-size: .85rem; font-weight: 500; color: var(--goop-text, #e6e9ef); }
      .gc-field-input {
        width: 100%; box-sizing: border-box; padding: .45rem .65rem;
        border: 1px solid var(--goop-border, #2a3142); border-radius: var(--goop-radius, 6px);
        background: var(--goop-field, rgba(0,0,0,.25)); color: var(--goop-text, #e6e9ef);
        font: inherit; outline: none;
      }
      .gc-field-input:focus { border-color: var(--goop-accent, #7aa2ff); }
      textarea.gc-field-input { min-height: 4rem; resize: vertical; }

      .gc-message { max-width: 75%; padding: .5rem .75rem; border-radius: var(--goop-radius, 6px); margin-bottom: .35rem; }
      .gc-message-self { margin-left: auto; background: color-mix(in srgb, var(--goop-accent, #7aa2ff) 15%, transparent); }
      .gc-message-other { margin-right: auto; background: var(--goop-field, rgba(0,0,0,.25)); }
      .gc-message-from { font-size: .75rem; font-weight: 600; color: var(--goop-accent, #7aa2ff); margin-bottom: .15rem; }
      .gc-message-text { font-size: .9rem; color: var(--goop-text, #e6e9ef); white-space: pre-wrap; word-break: break-word; }
      .gc-message-time { font-size: .7rem; color: var(--goop-muted, #9aa3b2); margin-top: .15rem; text-align: right; }
      .gc-message-system { text-align: center; font-size: .8rem; color: var(--goop-muted, #9aa3b2); padding: .3rem 0; }

      .gc-stats { display: flex; gap: 1rem; flex-wrap: wrap; }
      .gc-stat { display: flex; flex-direction: column; align-items: center; }
      .gc-stat-value { font-size: 1.4rem; font-weight: 700; color: var(--goop-text, #e6e9ef); }
      .gc-stat-label { font-size: .75rem; color: var(--goop-muted, #9aa3b2); }

      .gc-section { margin-bottom: 1rem; }
      .gc-section-title { font-size: .95rem; font-weight: 600; color: var(--goop-text, #e6e9ef); margin-bottom: .5rem; }

      .gc-actions { display: flex; gap: .4rem; flex-wrap: wrap; }

      .gc-item { display: flex; align-items: center; gap: .5rem; padding: .4rem 0; }
      .gc-item-body { flex: 1; min-width: 0; }
      .gc-item-label { font-size: .9rem; color: var(--goop-text, #e6e9ef); }
      .gc-item-sub { font-size: .75rem; color: var(--goop-muted, #9aa3b2); }
      .gc-item[data-goop-clickable] { cursor: pointer; }
    `;
    document.head.appendChild(s);
  }

  // ── Goop.button(label, opts) ──
  //
  //   Goop.button("Save")
  //   Goop.button("Save", { primary: true, onclick: fn })
  //   Goop.button("Delete", { danger: true, onclick: fn })
  //   Goop.button("x", { small: true, icon: true })

  Goop.button = function(label, opts) {
    opts = opts || {};
    var cls = "gc-btn";
    if (opts.primary) cls += " gc-btn-primary";
    if (opts.danger) cls += " gc-btn-danger";
    if (opts.small) cls += " gc-btn-sm";
    if (opts.icon) cls += " gc-btn-icon";
    if (opts.class) cls += " " + opts.class;
    var btn = d("button", { type: "button", class: cls }, label);
    if (opts.disabled) btn.disabled = true;
    if (opts.title) btn.title = opts.title;
    if (opts.onclick) btn.addEventListener("click", opts.onclick);
    return btn;
  };

  // ── Goop.card(opts) ──
  //
  //   Goop.card({ title: "Room", body: "Come chat!", onclick: fn })
  //   Goop.card({ image: "photo.jpg", title: "Sunset", actions: [...] })
  //   Goop.card({ title: "Note", subtitle: "General", body: "Buy milk", footer: "Jan 1" })

  Goop.card = function(opts) {
    opts = opts || {};
    var cls = "gc-card";
    if (opts.class) cls += " " + opts.class;

    var children = [];

    if (opts.image) {
      var img = typeof opts.image === "string"
        ? d("img", { class: "gc-card-image", src: opts.image, alt: "" })
        : opts.image;
      children.push(img);
    }

    var bodyParts = [];
    if (opts.title) bodyParts.push(typeof opts.title === "string" ? d("div", { class: "gc-card-title" }, opts.title) : opts.title);
    if (opts.subtitle) bodyParts.push(typeof opts.subtitle === "string" ? d("div", { class: "gc-card-subtitle" }, opts.subtitle) : opts.subtitle);
    if (opts.body) {
      if (typeof opts.body === "string") bodyParts.push(d("div", { class: "gc-card-text" }, opts.body));
      else if (Array.isArray(opts.body)) bodyParts.push.apply(bodyParts, opts.body);
      else bodyParts.push(opts.body);
    }
    if (bodyParts.length) children.push(d("div", { class: "gc-card-body" }, bodyParts));

    if (opts.footer) {
      var foot = typeof opts.footer === "string" ? d("div", { class: "gc-card-footer" }, opts.footer) : d("div", { class: "gc-card-footer" }, opts.footer);
      children.push(foot);
    }

    if (opts.actions) {
      children.push(d("div", { class: "gc-card-actions" }, opts.actions));
    }

    var el = d("div", { class: cls }, children);
    if (opts.onclick) {
      el.setAttribute("data-goop-clickable", "");
      el.addEventListener("click", function(e) { if (e.target.tagName !== "BUTTON") opts.onclick(e); });
    }
    return el;
  };

  // ── Goop.table(columns, rows, opts) ──
  //
  //   Goop.table(["Name", "Score"], [
  //     ["Alice", 10],
  //     ["Bob", 8]
  //   ])
  //
  //   Goop.table(
  //     [{ label: "Name", key: "name" }, { label: "Score", key: "score" }],
  //     scores,
  //     { onRow: fn, empty: "No scores yet." }
  //   )

  Goop.table = function(columns, rows, opts) {
    opts = opts || {};
    var cols = columns.map(function(c) { return typeof c === "string" ? { label: c } : c; });

    if (!rows || rows.length === 0) {
      return opts.empty
        ? (typeof opts.empty === "string" ? d("div", { class: "gc-empty" }, opts.empty) : opts.empty)
        : d("div", { class: "gc-empty" }, "No data.");
    }

    return d("table", { class: "gc-table" + (opts.class ? " " + opts.class : "") },
      d("thead", {},
        d("tr", {}, cols.map(function(c) { return d("th", {}, c.label || ""); }))
      ),
      d("tbody", {},
        rows.map(function(row, ri) {
          var cells;
          if (Array.isArray(row)) {
            cells = row.map(function(val) { return d("td", {}, val == null ? "" : val); });
          } else {
            cells = cols.map(function(c) {
              var val = c.key ? row[c.key] : "";
              if (c.render) val = c.render(row, ri);
              return d("td", {}, val == null ? "" : val);
            });
          }

          var tr = d("tr", {}, cells);
          if (opts.onRow) {
            tr.setAttribute("data-goop-clickable", "");
            tr.addEventListener("click", function(e) {
              if (e.target.tagName === "BUTTON") return;
              opts.onRow(row, ri);
            });
          }
          return tr;
        })
      )
    );
  };

  // ── Goop.field(label, opts) ──
  //
  //   Goop.field("Your name", { type: "text", id: "name", placeholder: "..." })
  //   Goop.field("Message", { type: "textarea", id: "msg", rows: 3 })
  //   Goop.field("Color", { type: "select", id: "color", options: ["Red", "Blue"] })
  //   Goop.field("Title", myExistingInputElement)

  Goop.field = function(label, opts) {
    var input;

    if (opts instanceof Node) {
      input = opts;
    } else {
      opts = opts || {};
      var tag = opts.type === "textarea" ? "textarea" : (opts.type === "select" ? "select" : "input");
      var attrs = { class: "gc-field-input" };
      if (opts.id) attrs.id = opts.id;
      if (opts.name) attrs.name = opts.name;
      if (opts.placeholder) attrs.placeholder = opts.placeholder;
      if (opts.required) attrs.required = "required";
      if (opts.value != null) attrs.value = String(opts.value);
      if (tag === "input") attrs.type = opts.type || "text";
      if (tag === "textarea" && opts.rows) attrs.rows = opts.rows;

      if (tag === "select") {
        input = d("select", attrs,
          (opts.options || []).map(function(o) {
            var val = typeof o === "string" ? o : o.value;
            var lbl = typeof o === "string" ? o : o.label;
            var opt = d("option", { value: val }, lbl);
            if (opts.value != null && String(opts.value) === String(val)) opt.selected = true;
            return opt;
          })
        );
      } else {
        input = d(tag, attrs);
        if (tag === "textarea" && opts.value) input.textContent = opts.value;
      }
    }

    return d("div", { class: "gc-field" },
      d("label", { class: "gc-field-label", for: input.id || "" }, label),
      input
    );
  };

  // ── Goop.message(opts) ──
  //
  //   Goop.message({ from: "Peter", text: "Hello!", time: "14:30", self: true })
  //   Goop.message({ text: "User joined", system: true })

  Goop.message = function(opts) {
    opts = opts || {};
    if (opts.system) {
      return d("div", { class: "gc-message-system" }, opts.text);
    }
    var cls = "gc-message " + (opts.self ? "gc-message-self" : "gc-message-other");
    return d("div", { class: cls },
      opts.from ? d("div", { class: "gc-message-from" }, opts.self ? "You" : opts.from) : null,
      d("div", { class: "gc-message-text" }, opts.text),
      opts.time ? d("div", { class: "gc-message-time" }, opts.time) : null
    );
  };

  // ── Goop.stats(items) ──
  //
  //   Goop.stats([{ label: "Wins", value: 5 }, { label: "Losses", value: 2 }])

  Goop.stats = function(items) {
    return d("div", { class: "gc-stats" },
      items.map(function(s) {
        return d("div", { class: "gc-stat" },
          d("span", { class: "gc-stat-value" }, String(s.value)),
          d("span", { class: "gc-stat-label" }, s.label)
        );
      })
    );
  };

  // ── Goop.section(title, ...content) ──
  //
  //   Goop.section("Recent Games", myTable)

  Goop.section = function(title) {
    var children = [d("div", { class: "gc-section-title" }, title)];
    for (var i = 1; i < arguments.length; i++) children.push(arguments[i]);
    return d("div", { class: "gc-section" }, children);
  };

  // ── Goop.actions(...buttons) ──
  //
  //   Goop.actions(
  //     Goop.button("Cancel", { onclick: fn }),
  //     Goop.button("Save", { primary: true, onclick: fn })
  //   )

  Goop.actions = function() {
    var children = [];
    for (var i = 0; i < arguments.length; i++) children.push(arguments[i]);
    return d("div", { class: "gc-actions" }, children);
  };

  // ── Goop.item(opts) ──
  //
  //   Goop.item({ avatar: peerId, label: "Peter", subtitle: "Online" })
  //   Goop.item({ label: "Gold", value: "$2,050", onclick: fn })

  Goop.item = function(opts) {
    opts = opts || {};
    var el = d("div", { class: "gc-item" + (opts.class ? " " + opts.class : "") },
      opts.avatar ? Goop.ui.avatar(opts.avatar, { size: opts.avatarSize || 28 }) : null,
      opts.icon ? d("span", { class: "gc-item-icon" }, opts.icon) : null,
      d("div", { class: "gc-item-body" },
        d("div", { class: "gc-item-label" }, opts.label || ""),
        opts.subtitle ? d("div", { class: "gc-item-sub" }, opts.subtitle) : null
      ),
      opts.value ? d("div", { class: "gc-item-value" }, String(opts.value)) : null,
      opts.actions ? d("div", { class: "gc-actions" }, opts.actions) : null,
      opts.badge ? (typeof opts.badge === "string" ? Goop.ui.badge(opts.badge) : opts.badge) : null
    );
    if (opts.onclick) {
      el.setAttribute("data-goop-clickable", "");
      el.addEventListener("click", function(e) { if (e.target.tagName !== "BUTTON") opts.onclick(e); });
    }
    return el;
  };
})();
