//
// Simple data API client for peer site pages.
// Usage (in any site page):
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="app.js"></script>
//
// Then in app.js:
//
//   const db = Goop.data;
//
//   // list tables
//   const tables = await db.tables();
//
//   // query rows
//   const rows = await db.query("posts");
//   const recent = await db.query("posts", { limit: 10, where: "title LIKE ?", args: ["%hello%"] });
//
//   // insert
//   await db.insert("posts", { title: "Hello", body: "World" });
//
//   // update
//   await db.update("posts", 1, { title: "Updated" });
//
//   // delete
//   await db.remove("posts", 1);
//
//   // describe table schema
//   const cols = await db.describe("posts");
//
(() => {
  window.Goop = window.Goop || {};

  // Detect remote peer context from URL path: /p/<peerID>/...
  var apiBase = "/api/data";
  var m = window.location.pathname.match(/^\/p\/([^/]+)\//);
  if (m) {
    apiBase = "/api/p/" + m[1] + "/data";
  }

  async function request(url, opts) {
    const res = await fetch(url, opts);
    if (!res.ok) {
      const text = await res.text();
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
    /** List all tables. Returns [{name, created_at}] */
    tables() {
      return request(apiBase + "/tables");
    },

    /** Describe a table's columns. Returns [{cid, name, type, not_null, default, pk}] */
    describe(table) {
      return post(apiBase + "/tables/describe", { table });
    },

    /** Query rows from a table. Options: {columns, where, args, limit, offset} */
    query(table, opts) {
      return post(apiBase + "/query", Object.assign({ table }, opts || {}));
    },

    /** Insert a row. data is {col: value, ...}. Returns {status, id} */
    insert(table, data) {
      return post(apiBase + "/insert", { table, data });
    },

    /** Update a row by _id. data is {col: value, ...}. Returns {status} */
    update(table, id, data) {
      return post(apiBase + "/update", { table, id, data });
    },

    /** Delete a row by _id. Returns {status} */
    remove(table, id) {
      return post(apiBase + "/delete", { table, id });
    },

    /** Create a new table. columns is [{name, type, not_null, default}] */
    createTable(name, columns) {
      return post(apiBase + "/tables/create", { name, columns });
    },

    /** Drop a table entirely */
    dropTable(table) {
      return post(apiBase + "/tables/delete", { table });
    },

    /** Add a column to an existing table */
    addColumn(table, column) {
      return post(apiBase + "/tables/add-column", { table, column });
    },

    /** Call a Lua data function. Returns the function's result. */
    async call(fn, params) {
      return post(apiBase + "/lua/call", { "function": fn, params: params || {} });
    },

    /** List available Lua data functions. Returns [{name, description}] */
    async functions() {
      var resp = await request(apiBase + "/lua/list");
      return resp ? (resp.functions || []) : [];
    },
  };
})();
