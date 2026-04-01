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

      return {
        name: s.name || table,
        columns: cols,
        access: acc,
        system_key: s.system_key || false,
        context: s.context || false,

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
