//
// HTML partials — load, cache, render.
// Put HTML files in site/partials/, use them from JS.
//
// Syntax:
//   {{name}}                — insert escaped text
//   {{{name}}}             — insert raw HTML
//   {{#if name}}...{{/if}} — conditional block
//   {{#each items}}...{{/each}} — loop (use {{.}} for current item, {{@index}} for index)
//   {{#unless name}}...{{/unless}} — inverse conditional
//
// Usage:
//
//   // render one item
//   var el = await Goop.partial("note-card", { title: "Buy milk", category: "general" });
//   container.appendChild(el);
//
//   // render a list
//   Goop.list(container, notes, "note-card");
//
//   // preload (optional)
//   await Goop.preload("note-card", "post-item");
//
(() => {
  window.Goop = window.Goop || {};

  var cache = {};
  var basePath = "";
  var m = window.location.pathname.match(/^(\/p\/[^/]+)/);
  if (m) basePath = m[1];
  var partialsDir = basePath + "/partials/";

  function fetch_(name) {
    if (cache[name]) return Promise.resolve(cache[name]);
    return fetch(partialsDir + name + ".html")
      .then(function(r) {
        if (!r.ok) throw new Error("partial not found: " + name);
        return r.text();
      })
      .then(function(html) {
        cache[name] = html;
        return html;
      });
  }

  function esc(s) {
    if (s == null) return "";
    var d = document.createElement("div");
    d.textContent = String(s);
    return d.innerHTML;
  }

  function resolve(key, data) {
    if (key === ".") return data;
    if (key === "@index") return data._index;
    var parts = key.split(".");
    var v = data;
    for (var i = 0; i < parts.length; i++) {
      if (v == null) return "";
      v = v[parts[i]];
    }
    return v;
  }

  function truthy(v) {
    if (v == null || v === false || v === 0 || v === "") return false;
    if (Array.isArray(v) && v.length === 0) return false;
    return true;
  }

  function render(template, data) {
    var result = template;

    // {{#each key}}...{{/each}}
    result = result.replace(/\{\{#each\s+(\w+(?:\.\w+)*)\}\}([\s\S]*?)\{\{\/each\}\}/g, function(_, key, body) {
      var arr = resolve(key, data);
      if (!Array.isArray(arr)) return "";
      return arr.map(function(item, idx) {
        var ctx = typeof item === "object" ? Object.assign({ _index: idx }, item) : { ".": item, _index: idx };
        return render(body, ctx);
      }).join("");
    });

    // {{#if key}}...{{/if}}
    result = result.replace(/\{\{#if\s+(\w+(?:\.\w+)*)\}\}([\s\S]*?)\{\{\/if\}\}/g, function(_, key, body) {
      return truthy(resolve(key, data)) ? render(body, data) : "";
    });

    // {{#unless key}}...{{/unless}}
    result = result.replace(/\{\{#unless\s+(\w+(?:\.\w+)*)\}\}([\s\S]*?)\{\{\/unless\}\}/g, function(_, key, body) {
      return truthy(resolve(key, data)) ? "" : render(body, data);
    });

    // {{{raw}}} — unescaped
    result = result.replace(/\{\{\{(\w+(?:\.\w+)*)\}\}\}/g, function(_, key) {
      var v = resolve(key, data);
      return v == null ? "" : String(v);
    });

    // {{key}} — escaped
    result = result.replace(/\{\{(\w+(?:\.\w+|@\w+)*)\}\}/g, function(_, key) {
      return esc(resolve(key, data));
    });

    return result;
  }

  function toElement(html) {
    var tpl = document.createElement("template");
    tpl.innerHTML = html.trim();
    var content = tpl.content;
    if (content.children.length === 1) return content.children[0];
    var wrap = document.createElement("div");
    while (content.firstChild) wrap.appendChild(content.firstChild);
    return wrap;
  }

  Goop.partial = async function(name, data) {
    var tpl = await fetch_(name);
    var html = render(tpl, data || {});
    return toElement(html);
  };

  Goop.preload = function() {
    var promises = [];
    for (var i = 0; i < arguments.length; i++) promises.push(fetch_(arguments[i]));
    return Promise.all(promises);
  };

  // Extend Goop.list to accept partial name as renderFn
  var _origList = Goop.list;
  Goop.list = function(el, rows, renderFn, opts) {
    if (typeof renderFn === "string") {
      var name = renderFn;
      el.innerHTML = "";
      if (!rows || rows.length === 0) {
        opts = opts || {};
        if (opts.empty) {
          if (typeof opts.empty === "string") el.appendChild(toElement('<div class="' + (opts.emptyClass || "") + '">' + esc(opts.empty) + '</div>'));
          else if (opts.empty instanceof Node) el.appendChild(opts.empty);
        }
        return Promise.resolve();
      }
      return Goop.preload(name).then(function() {
        return Promise.all(rows.map(function(row) {
          return Goop.partial(name, row);
        }));
      }).then(function(elements) {
        el.innerHTML = "";
        elements.forEach(function(node) { el.appendChild(node); });
      });
    }
    return _origList ? _origList(el, rows, renderFn, opts) : undefined;
  };
})();
