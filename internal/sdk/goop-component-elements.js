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
