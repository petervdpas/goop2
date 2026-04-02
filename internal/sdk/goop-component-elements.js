(() => {
  window.Goop = window.Goop || {};
  var d = Goop.dom;
  if (!d) {
    console.warn("goop-component-elements: Goop.dom not found, load goop-component-base.js first");
    return;
  }

  Goop.button = function(label, opts) {
    opts = opts || {};
    var btn = d("button", { type: "button", class: opts.class || "" }, label);
    if (opts.disabled) btn.disabled = true;
    if (opts.title) btn.title = opts.title;
    if (opts.onclick) btn.addEventListener("click", opts.onclick);
    return btn;
  };

  Goop.card = function(opts) {
    opts = opts || {};
    var children = [];

    if (opts.image) {
      var img = typeof opts.image === "string"
        ? d("img", { class: opts.imageClass || "", src: opts.image, alt: "" })
        : opts.image;
      children.push(img);
    }

    var bodyParts = [];
    if (opts.title) bodyParts.push(typeof opts.title === "string" ? d("div", { class: opts.titleClass || "" }, opts.title) : opts.title);
    if (opts.subtitle) bodyParts.push(typeof opts.subtitle === "string" ? d("div", { class: opts.subtitleClass || "" }, opts.subtitle) : opts.subtitle);
    if (opts.body) {
      if (typeof opts.body === "string") bodyParts.push(d("div", { class: opts.textClass || "" }, opts.body));
      else if (Array.isArray(opts.body)) bodyParts.push.apply(bodyParts, opts.body);
      else bodyParts.push(opts.body);
    }
    if (bodyParts.length) children.push(d("div", { class: opts.bodyClass || "" }, bodyParts));

    if (opts.footer) {
      children.push(d("div", { class: opts.footerClass || "" }, typeof opts.footer === "string" ? opts.footer : opts.footer));
    }

    if (opts.actions) {
      children.push(d("div", { class: opts.actionsClass || "" }, opts.actions));
    }

    var el = d("div", { class: opts.class || "" }, children);
    if (opts.onclick) {
      el.addEventListener("click", function(e) { if (e.target.tagName !== "BUTTON") opts.onclick(e); });
    }
    return el;
  };

  Goop.table = function(columns, rows, opts) {
    opts = opts || {};
    var cols = columns.map(function(c) { return typeof c === "string" ? { label: c } : c; });

    if (!rows || rows.length === 0) {
      return opts.empty
        ? (typeof opts.empty === "string" ? d("div", { class: opts.emptyClass || "" }, opts.empty) : opts.empty)
        : d("div", { class: opts.emptyClass || "" }, "No data.");
    }

    return d("table", { class: opts.class || "" },
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

  Goop.field = function(label, opts) {
    var input;

    if (opts instanceof Node) {
      input = opts;
    } else {
      opts = opts || {};
      var tag = opts.type === "textarea" ? "textarea" : (opts.type === "select" ? "select" : "input");
      var attrs = { class: opts.inputClass || "" };
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

    return d("div", { class: opts && opts.class ? opts.class : "" },
      d("label", { class: opts && opts.labelClass ? opts.labelClass : "", for: input.id || "" }, label),
      input
    );
  };

  Goop.message = function(opts) {
    opts = opts || {};
    if (opts.system) {
      return d("div", { class: opts.systemClass || "" }, opts.text);
    }
    return d("div", { class: opts.class || "" },
      opts.from ? d("div", { class: opts.fromClass || "" }, opts.self ? "You" : opts.from) : null,
      d("div", { class: opts.textClass || "" }, opts.text),
      opts.time ? d("div", { class: opts.timeClass || "" }, opts.time) : null
    );
  };

  Goop.stats = function(items, opts) {
    opts = opts || {};
    return d("div", { class: opts.class || "" },
      items.map(function(s) {
        return d("div", { class: opts.statClass || "" },
          d("span", { class: opts.valueClass || "" }, String(s.value)),
          d("span", { class: opts.labelClass || "" }, s.label)
        );
      })
    );
  };

  Goop.section = function(title) {
    var opts = {};
    var startIdx = 1;
    if (arguments.length > 1 && arguments[1] && typeof arguments[1] === "object" && !(arguments[1] instanceof Node) && !Array.isArray(arguments[1])) {
      opts = arguments[1];
      startIdx = 2;
    }
    var children = [d("div", { class: opts.titleClass || "" }, title)];
    for (var i = startIdx; i < arguments.length; i++) children.push(arguments[i]);
    return d("div", { class: opts.class || "" }, children);
  };

  Goop.actions = function() {
    var opts = {};
    var children = [];
    for (var i = 0; i < arguments.length; i++) {
      if (arguments[i] instanceof Node) children.push(arguments[i]);
      else if (typeof arguments[i] === "object" && arguments[i]) opts = arguments[i];
    }
    return d("div", { class: opts.class || "" }, children);
  };

  Goop.item = function(opts) {
    opts = opts || {};
    var el = d("div", { class: opts.class || "" },
      opts.avatar ? Goop.ui.avatar(opts.avatar, { size: opts.avatarSize || 28, class: opts.avatarClass || "" }) : null,
      opts.icon ? d("span", { class: opts.iconClass || "" }, opts.icon) : null,
      d("div", { class: opts.bodyClass || "" },
        d("div", { class: opts.labelClass || "" }, opts.label || ""),
        opts.subtitle ? d("div", { class: opts.subtitleClass || "" }, opts.subtitle) : null
      ),
      opts.value ? d("div", { class: opts.valueClass || "" }, String(opts.value)) : null,
      opts.actions ? d("div", { class: opts.actionsClass || "" }, opts.actions) : null
    );
    if (opts.onclick) {
      el.addEventListener("click", function(e) { if (e.target.tagName !== "BUTTON") opts.onclick(e); });
    }
    return el;
  };
})();
