//
// Data SDK for peer site pages — ORM-first API with DSL handles.
// Usage (in any site page):
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="app.js"></script>
//
// Then in app.js:
//
//   const db = Goop.data;
//
//   // ── ORM DSL (recommended) ──
//   const posts = await db.orm("posts");
//   posts.columns              // [{name, type, required, ...}]
//   posts.access               // {read, insert, update, delete}
//   posts.validate({title: ""}) // {valid: false, errors: ["title is required"]}
//
//   const rows   = await posts.find({ where: "published = ?", args: [1], order: "_id DESC" });
//   const post   = await posts.findOne({ where: "slug = ?", args: ["hello"] });
//   const row    = await posts.get(42);
//   const row    = await posts.getBy("slug", "hello");
//   const all    = await posts.list(50);
//   const titles = await posts.pluck("title", { where: "published = 1" });
//   const yes    = await posts.exists({ where: "slug = ?", args: ["hello"] });
//   const n      = await posts.count({ where: "published = 1" });
//
//   const {id}   = await posts.insert({ title: "Hello", body: "World" });
//   await posts.update(1, { title: "Updated" });
//   await posts.remove(1);
//   await posts.upsert("slug", { slug: "hello", title: "Hello" });
//
//   // ── Low-level (pass table name each time) ──
//   const rows = await db.find("posts", { where: "published = ?", args: [1] });
//   const {id} = await db.insert("posts", { title: "Hello" });
//
//   // ── Schema ──
//   const tables  = await db.tables();
//   const schema  = await db.describe("posts");
//   const schemas = await db.schemas();
//
//   // ── Lua functions ──
//   const result = await db.call("api", { action: "list", resource: "posts" });
//
(() => {
  window.Goop = window.Goop || {};

  var apiBase = "/api/data";
  var m = window.location.pathname.match(/^\/p\/([^/]+)\//);
  var hostId = m ? m[1] : null;
  if (m) {
    apiBase = "/api/p/" + m[1] + "/data";
  }

  // ── Shared utilities ──

  var _escEl = document.createElement("div");
  window.Goop.esc = function(s) {
    _escEl.textContent = s == null ? "" : String(s);
    return _escEl.innerHTML;
  };

  window.Goop.date = function(ts, opts) {
    if (!ts) return "";
    var d = new Date(String(ts).replace(" ", "T") + "Z");
    if (isNaN(d)) return String(ts);
    return d.toLocaleDateString(undefined, opts || { year: "numeric", month: "long", day: "numeric" });
  };

  // Lazy peer context — resolves identity + group membership once.
  var _peerCtx = null;
  window.Goop.peer = async function() {
    if (_peerCtx) return _peerCtx;
    _peerCtx = { myId: "", hostId: hostId || "", isOwner: false, isGroup: false, label: "" };
    try {
      var me = await window.Goop.identity.get();
      _peerCtx.myId = me.id;
      _peerCtx.label = me.label || "";
      _peerCtx.isOwner = !!(hostId && hostId === me.id);
      if (!_peerCtx.isOwner && hostId && window.Goop.group) {
        var subs = await window.Goop.group.subscriptions();
        var list = (subs && subs.subscriptions) || [];
        _peerCtx.isGroup = list.some(function(s) {
          return s.host_peer_id === hostId && s.app_type === "template";
        });
      }
    } catch (_) {}
    return _peerCtx;
  };

  // ── List renderer ──

  window.Goop.list = function(el, rows, renderFn, opts) {
    opts = opts || {};
    if (!rows || rows.length === 0) {
      el.innerHTML = '<div class="empty-msg">' + (opts.empty || "<p>Nothing here yet.</p>") + '</div>';
      return;
    }
    el.innerHTML = rows.map(renderFn).join("");
    if (opts.actions) {
      for (var action in opts.actions) {
        (function(act, cb) {
          el.querySelectorAll('[data-action="' + act + '"]').forEach(function(btn) {
            btn.addEventListener("click", function(e) {
              e.stopPropagation();
              var id = btn.dataset.id ? parseInt(btn.dataset.id, 10) : null;
              var row = id != null ? rows.find(function(r) { return r._id == id; }) : null;
              cb(id, row, btn);
            });
          });
        })(action, opts.actions[action]);
      }
    }
    if (opts.onRow) {
      el.querySelectorAll("[data-id]").forEach(function(item) {
        item.addEventListener("click", function(e) {
          if (e.target.tagName === "BUTTON") return;
          var id = parseInt(item.dataset.id, 10);
          opts.onRow(id, rows.find(function(r) { return r._id == id; }));
        });
      });
    }
  };

  // ── Overlay helper ──

  window.Goop.overlay = function(id) {
    var el = typeof id === "string" ? document.getElementById(id) : id;
    if (!el) return { open: function(){}, close: function(){} };

    function close() { el.classList.add("hidden"); }
    function open() {
      el.classList.remove("hidden");
      var first = el.querySelector("input, textarea, select");
      if (first) first.focus();
    }

    el.addEventListener("mousedown", function(e) {
      if (e.target === el) close();
    });

    el.querySelectorAll("[data-close]").forEach(function(btn) {
      btn.addEventListener("click", close);
    });

    document.addEventListener("keydown", function(e) {
      if (e.key === "Escape" && !el.classList.contains("hidden")) close();
    });

    return { open: open, close: close, el: el };
  };

  // ── Schema-driven UI helpers (used by orm handle) ──

  var SYS = { _id: 1, _owner: 1, _owner_email: 1, _created_at: 1, _updated_at: 1 };

  var _ormComponents = [];

  function ormFormCollect(el) {
    var data = {};
    el.querySelectorAll("[data-col]").forEach(function(inp) {
      data[inp.getAttribute("data-col")] = inp.value;
    });
    for (var i = 0; i < _ormComponents.length; i++) {
      var c = _ormComponents[i];
      if (el.contains(c.el)) data[c.name] = c.instance.getValue();
    }
    return data;
  }

  function ormFormUpgrade(el, cols, values) {
    var ui = window.Goop && window.Goop.ui;
    if (!ui) return;
    el.querySelectorAll("[data-goop-upgrade]").forEach(function(ph) {
      var type = ph.getAttribute("data-goop-upgrade");
      var name = ph.getAttribute("data-col");
      var val = values[name] != null ? values[name] : "";
      var col = null;
      for (var i = 0; i < cols.length; i++) { if (cols[i].name === name) { col = cols[i]; break; } }
      var inst = null;
      if (type === "datepicker" && ui.datepicker) {
        inst = ui.datepicker(ph, { value: val, name: name });
      } else if (type === "select" && ui.select && col && col.values) {
        inst = ui.select(ph, {
          name: name,
          value: val,
          options: col.values.map(function(v) { return { value: v.key, label: v.label }; }),
        });
      } else if (type === "stepper" && ui.stepper) {
        inst = ui.stepper(ph, { value: parseFloat(val) || 0, name: name });
      } else if (type === "toggle" && ui.toggle) {
        inst = ui.toggle(ph, { checked: val === "1" || val === 1 || val === true, name: name });
      } else if (type === "colorpicker" && ui.colorpicker) {
        inst = ui.colorpicker(ph, { value: val || "#7aa2ff", name: name });
      }
      if (inst) _ormComponents.push({ name: name, instance: inst, el: inst.el });
    });
  }

  function ormForm(el, cols, opts) {
    opts = opts || {};
    var esc = window.Goop.esc;
    var exclude = {};
    (opts.exclude || []).forEach(function(n) { exclude[n] = 1; });
    var values = opts.values || {};
    var hasComponents = window.Goop && window.Goop.ui && window.Goop.ui.datepicker;
    var html = '<div class="orm-form">';
    for (var i = 0; i < cols.length; i++) {
      var c = cols[i];
      if (SYS[c.name] || exclude[c.name] || c.auto) continue;
      var id = "orm-f-" + c.name;
      var val = values[c.name] != null ? values[c.name] : (c["default"] != null ? c["default"] : "");
      var req = c.required ? " required" : "";
      html += '<div class="orm-field">';
      html += '<label for="' + esc(id) + '">' + esc(opts.labels && opts.labels[c.name] || c.name) + '</label>';
      if (hasComponents && (c.type === "date" || c.type === "datetime")) {
        html += '<div data-goop-upgrade="datepicker" data-col="' + esc(c.name) + '"></div>';
      } else if (hasComponents && c.type === "color") {
        html += '<div data-goop-upgrade="colorpicker" data-col="' + esc(c.name) + '"></div>';
      } else if (hasComponents && c.type === "boolean") {
        html += '<div data-goop-upgrade="toggle" data-col="' + esc(c.name) + '"></div>';
      } else if (c.values && c.values.length) {
        if (hasComponents) {
          html += '<div data-goop-upgrade="select" data-col="' + esc(c.name) + '"></div>';
        } else {
          html += '<select id="' + esc(id) + '" data-col="' + esc(c.name) + '"' + req + '>';
          html += '<option value="">— select —</option>';
          for (var j = 0; j < c.values.length; j++) {
            var v = c.values[j];
            var sel = String(val) === v.key ? " selected" : "";
            html += '<option value="' + esc(v.key) + '"' + sel + '>' + esc(v.label) + '</option>';
          }
          html += '</select>';
        }
      } else if (c.type === "integer" || c.type === "real") {
        if (hasComponents) {
          html += '<div data-goop-upgrade="stepper" data-col="' + esc(c.name) + '"></div>';
        } else {
          html += '<input id="' + esc(id) + '" data-col="' + esc(c.name) + '" type="number" value="' + esc(val) + '"' + req + '>';
        }
      } else {
        html += '<input id="' + esc(id) + '" data-col="' + esc(c.name) + '" type="text" value="' + esc(val) + '"' + req + '>';
      }
      html += '</div>';
    }
    if (opts.submitLabel !== false) {
      html += '<div class="orm-btns"><button class="orm-submit">' + esc(opts.submitLabel || "Save") + '</button></div>';
    }
    html += '</div>';
    el.innerHTML = html;

    _ormComponents = _ormComponents.filter(function(c) { return document.body.contains(c.el); });
    if (hasComponents) ormFormUpgrade(el, cols, values);

    if (opts.onSave) {
      var btn = el.querySelector(".orm-submit");
      if (btn) btn.addEventListener("click", function() {
        opts.onSave(ormFormCollect(el));
      });
    }
  }

  function ormTable(el, cols, rows, opts) {
    opts = opts || {};
    var esc = window.Goop.esc;
    var exclude = {};
    (opts.exclude || []).forEach(function(n) { exclude[n] = 1; });
    var showCols = [];
    for (var i = 0; i < cols.length; i++) {
      if (!SYS[cols[i].name] && !exclude[cols[i].name]) showCols.push(cols[i]);
    }
    if (opts.showId) showCols.unshift({ name: "_id", type: "integer" });

    var html = '<table class="orm-table"><thead><tr>';
    for (var i = 0; i < showCols.length; i++) {
      html += '<th>' + esc(opts.labels && opts.labels[showCols[i].name] || showCols[i].name) + '</th>';
    }
    if (opts.actions) html += '<th></th>';
    html += '</tr></thead><tbody>';

    for (var r = 0; r < rows.length; r++) {
      var row = rows[r];
      html += '<tr data-id="' + (row._id || "") + '">';
      for (var c = 0; c < showCols.length; c++) {
        html += '<td>' + esc(row[showCols[c].name]) + '</td>';
      }
      if (opts.actions) {
        html += '<td class="orm-actions">';
        if (opts.actions.edit) html += '<button data-action="edit" data-id="' + row._id + '">Edit</button>';
        if (opts.actions.remove) html += '<button data-action="remove" data-id="' + row._id + '">Delete</button>';
        html += '</td>';
      }
      html += '</tr>';
    }
    html += '</tbody></table>';
    el.innerHTML = html;

    if (opts.actions) {
      if (opts.actions.edit) {
        el.querySelectorAll('[data-action="edit"]').forEach(function(btn) {
          btn.addEventListener("click", function() { opts.actions.edit(parseInt(btn.dataset.id, 10), rows.find(function(r) { return r._id == btn.dataset.id; })); });
        });
      }
      if (opts.actions.remove) {
        el.querySelectorAll('[data-action="remove"]').forEach(function(btn) {
          btn.addEventListener("click", function() { opts.actions.remove(parseInt(btn.dataset.id, 10), rows.find(function(r) { return r._id == btn.dataset.id; })); });
        });
      }
    }
    if (opts.onRow) {
      el.querySelectorAll("tbody tr").forEach(function(tr) {
        tr.addEventListener("click", function(e) {
          if (e.target.tagName === "BUTTON") return;
          var id = parseInt(tr.dataset.id, 10);
          opts.onRow(id, rows.find(function(r) { return r._id == tr.dataset.id; }));
        });
      });
    }
  }

  function resolveAccess(policy, peerCtx) {
    if (!policy) return false;
    switch (policy) {
      case "open": return true;
      case "owner": return peerCtx.isOwner;
      case "group": return peerCtx.isOwner || peerCtx.isGroup;
      case "local": return !hostId;
      default: return false;
    }
  }

  async function request(url, opts) {
    var res = await fetch(url, opts);
    if (!res.ok) {
      var text = await res.text();
      throw new Error(text || res.statusText);
    }
    return res.json();
  }

  function post(url, body) {
    return request(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    });
  }

  window.Goop.data = {

    // ── Reads ──

    find(table, opts) {
      return post(apiBase + "/find", Object.assign({ table: table }, opts || {}));
    },

    findOne(table, opts) {
      return post(apiBase + "/find-one", Object.assign({ table: table }, opts || {}));
    },

    getBy(table, column, value) {
      return post(apiBase + "/get-by", { table: table, column: column, value: value });
    },

    get(table, id) {
      return post(apiBase + "/find-one", { table: table, where: "_id = ?", args: [id] });
    },

    list(table, limit) {
      return post(apiBase + "/find", { table: table, limit: limit || 0 });
    },

    pluck(table, column, opts) {
      return post(apiBase + "/pluck", Object.assign({ table: table, column: column }, opts || {}));
    },

    distinct(table, column, opts) {
      return post(apiBase + "/distinct", Object.assign({ table: table, column: column }, opts || {}));
    },

    exists(table, opts) {
      return post(apiBase + "/exists", Object.assign({ table: table }, opts || {}))
        .then(function(r) { return r.exists; });
    },

    count(table, opts) {
      return post(apiBase + "/count", Object.assign({ table: table }, opts || {}))
        .then(function(r) { return r.count; });
    },

    aggregate(table, expr, opts) {
      return post(apiBase + "/aggregate", Object.assign({ table: table, expr: expr }, opts || {}));
    },

    // ── Writes ──

    insert(table, data) {
      return post(apiBase + "/insert", { table: table, data: data });
    },

    update(table, id, data) {
      return post(apiBase + "/update", { table: table, id: id, data: data });
    },

    remove(table, id) {
      return post(apiBase + "/delete", { table: table, id: id });
    },

    updateWhere(table, data, opts) {
      return post(apiBase + "/update-where", Object.assign({ table: table, data: data }, opts || {}));
    },

    deleteWhere(table, opts) {
      return post(apiBase + "/delete-where", Object.assign({ table: table }, opts || {}));
    },

    upsert(table, keyCol, data) {
      return post(apiBase + "/upsert", { table: table, key_col: keyCol, data: data });
    },

    // ── Schema ──

    tables() {
      return request(apiBase + "/tables");
    },

    describe(table) {
      return post(apiBase + "/tables/describe", { table: table });
    },

    schemas() {
      return request(apiBase + "/orm-schema");
    },

    // ── ORM DSL ──

    orm: async function(table) {
      var info = await this.describe(table);
      var s = (info && info.schema) || {};
      var cols = s.columns || [];
      var acc = s.access || {};
      var self = this;
      var peerCtx = await window.Goop.peer();

      var handle = {
        name: s.name || table,
        columns: cols,
        access: acc,
        system_key: s.system_key || false,
        context: s.context || false,

        canInsert: resolveAccess(acc.insert, peerCtx),
        canUpdate: resolveAccess(acc.update, peerCtx),
        canDelete: resolveAccess(acc.delete, peerCtx),

        validate: function(data) {
          var errors = [];
          for (var i = 0; i < cols.length; i++) {
            var c = cols[i];
            if (c.required && (data[c.name] == null || data[c.name] === "")) {
              errors.push(c.name + " is required");
            }
            if (data[c.name] != null && c.type === "integer" && typeof data[c.name] !== "number") {
              errors.push(c.name + " must be a number");
            }
          }
          return { valid: errors.length === 0, errors: errors };
        },

        form: function(el, opts) { ormForm(el, cols, opts); },
        table: function(el, rows, opts) { ormTable(el, cols, rows, opts); },

        find: function(opts) { return self.find(table, opts); },
        findOne: function(opts) { return self.findOne(table, opts); },
        get: function(id) { return self.get(table, id); },
        getBy: function(col, val) { return self.getBy(table, col, val); },
        list: function(limit) { return self.list(table, limit); },
        pluck: function(col, opts) { return self.pluck(table, col, opts); },
        exists: function(opts) { return self.exists(table, opts); },
        count: function(opts) { return self.count(table, opts); },
        distinct: function(col, opts) { return self.distinct(table, col, opts); },
        aggregate: function(expr, opts) { return self.aggregate(table, expr, opts); },
        insert: function(data) { return self.insert(table, data); },
        update: function(id, data) { return self.update(table, id, data); },
        remove: function(id) { return self.remove(table, id); },
        updateWhere: function(data, opts) { return self.updateWhere(table, data, opts); },
        deleteWhere: function(opts) { return self.deleteWhere(table, opts); },
        upsert: function(keyCol, data) { return self.upsert(table, keyCol, data); },
      };
      return handle;
    },

    // ── API caller ──

    api: function(fn) {
      var self = this;
      return function(action, params) {
        return self.call(fn, Object.assign({ action: action }, params || {}));
      };
    },

    // ── Config helper ──

    config: async function(table, defaults) {
      defaults = defaults || {};
      var info = await this.describe(table);
      var s = (info && info.schema) || {};
      var cols = s.columns || [];
      var self = this;

      var isKV = cols.some(function(c) { return c.name === "key"; }) &&
                 cols.some(function(c) { return c.name === "value"; });

      var values = {};
      for (var k in defaults) values[k] = defaults[k];

      if (isKV) {
        try {
          var rows = await self.find(table);
          (rows || []).forEach(function(r) {
            if (r.key) values[r.key] = r.value;
          });
        } catch (_) {}
      } else {
        try {
          var rows = await self.find(table, { order: "_id DESC", limit: 1 });
          if (rows && rows.length > 0) {
            var row = rows[0];
            for (var c = 0; c < cols.length; c++) {
              var cn = cols[c].name;
              if (row[cn] != null) values[cn] = row[cn];
            }
          }
        } catch (_) {}
      }

      var cfg = {
        _table: table,
        _isKV: isKV,
        _db: self,

        get: function(key) { return values[key]; },

        set: async function(key, value) {
          values[key] = value;
          if (isKV) {
            await self.upsert(table, "key", { key: key, value: value });
          } else {
            try {
              var rows = await self.find(table, { order: "_id DESC", limit: 1, fields: ["_id"] });
              if (rows && rows.length > 0) {
                var data = {}; data[key] = value;
                await self.update(table, rows[0]._id, data);
              } else {
                var data = {}; data[key] = value;
                await self.insert(table, data);
              }
            } catch (_) {}
          }
        },
      };

      for (var k in values) {
        if (k !== "get" && k !== "set" && k !== "_table" && k !== "_isKV" && k !== "_db") {
          (function(key) {
            Object.defineProperty(cfg, key, {
              get: function() { return values[key]; },
              enumerable: true,
            });
          })(k);
        }
      }

      return cfg;
    },

    // ── Legacy (kept for backward compat) ──

    query(table, opts) {
      return post(apiBase + "/query", Object.assign({ table: table }, opts || {}));
    },

    createTable(name, columns) {
      return post(apiBase + "/tables/create", { name: name, columns: columns });
    },

    dropTable(table) {
      return post(apiBase + "/tables/delete", { table: table });
    },

    addColumn(table, column) {
      return post(apiBase + "/tables/add-column", { table: table, column: column });
    },

    // ── Lua ──

    call(fn, params) {
      return post(apiBase + "/lua/call", { "function": fn, params: params || {} });
    },

    functions() {
      return request(apiBase + "/lua/list")
        .then(function(r) { return r ? (r.functions || []) : []; });
    },
  };
})();
