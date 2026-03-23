//
// Virtual REST-like API for site templates.
// Provides clean CRUD operations backed by a server-side Lua function.
//
// Usage:
//
//   <script src="/sdk/goop-data.js"></script>
//   <script src="/sdk/goop-api.js"></script>
//
//   // Get a single record by slug or id
//   var post = await Goop.api.get("posts", { slug: "hello-world" });
//   var post = await Goop.api.get("posts", { id: 42 });
//
//   // List records (paginated)
//   var result = await Goop.api.list("posts");
//   var result = await Goop.api.list("posts", { limit: 10, offset: 20 });
//
//   // Insert a new record
//   var result = await Goop.api.insert("posts", { title: "New", body: "Content" });
//
//   // Update a record by id
//   var result = await Goop.api.update("posts", 42, { title: "Updated" });
//
//   // Delete a record by id
//   var result = await Goop.api.delete("posts", 42);
//
//   // Get a config table as a key-value map
//   var config = await Goop.api.map("config");
//
// The API reads endpoint declarations from api.json in the site root.
// If no api.json exists, all tables are exposed with default CRUD.
//
(() => {
  window.Goop = window.Goop || {};

  function call(action, params) {
    return window.Goop.data.call("api", Object.assign({ action: action }, params));
  }

  window.Goop.api = {
    get(resource, params) {
      return call("get", Object.assign({ resource: resource }, params || {}));
    },

    list(resource, params) {
      return call("list", Object.assign({ resource: resource }, params || {}));
    },

    insert(resource, data) {
      return call("insert", { resource: resource, data: data });
    },

    update(resource, id, data) {
      return call("update", { resource: resource, id: id, data: data });
    },

    delete(resource, id) {
      return call("delete", { resource: resource, id: id });
    },

    map(resource) {
      return call("map", { resource: resource });
    }
  };
})();
