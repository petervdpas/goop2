//
// Data SDK for peer site pages — ORM-first API.
// Usage (in any site page):
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="app.js"></script>
//
// Then in app.js:
//
//   const db = Goop.data;
//
//   // ── Reads ──
//   const rows = await db.find("posts", { where: "published = ?", args: [1], order: "_id DESC", limit: 10 });
//   const post = await db.findOne("posts", { where: "slug = ?", args: ["hello"] });
//   const post = await db.getBy("posts", "slug", "hello");
//   const row  = await db.get("posts", 42);
//   const all  = await db.list("posts", 50);
//   const titles = await db.pluck("posts", "title", { where: "published = 1" });
//   const cats = await db.distinct("notes", "category");
//   const yes  = await db.exists("posts", { where: "slug = ?", args: ["hello"] });
//   const n    = await db.count("posts", { where: "published = 1" });
//   const stats = await db.aggregate("scores", "SUM(score) as total, COUNT(*) as n");
//   const grouped = await db.aggregate("scores", "player, SUM(score) as total", { group_by: "player" });
//
//   // ── Writes ──
//   const {id} = await db.insert("posts", { title: "Hello", body: "World" });
//   await db.update("posts", 1, { title: "Updated" });
//   await db.remove("posts", 1);
//   await db.updateWhere("cards", { position: 0 }, { where: "column_id = ?", args: [3] });
//   await db.deleteWhere("cards", { where: "column_id = ?", args: [3] });
//   await db.upsert("config", "key", { key: "title", value: "My Blog" });
//
//   // ── Schema ──
//   const tables = await db.tables();
//   const schema = await db.describe("posts");
//
//   // ── Lua functions ──
//   const result = await db.call("api", { action: "list", resource: "posts" });
//
(() => {
  window.Goop = window.Goop || {};

  var apiBase = "/api/data";
  var m = window.location.pathname.match(/^\/p\/([^/]+)\//);
  if (m) {
    apiBase = "/api/p/" + m[1] + "/data";
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
