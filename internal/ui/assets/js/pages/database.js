// Database page — coordinator (tab switching, shared init)
(() => {
  var qs = window.Goop.core.qs;
  var qsa = window.Goop.core.qsa;
  var on = window.Goop.core.on;

  var dbPage = qs("#db-page");
  if (!dbPage) return;

  var pageEl = dbPage.closest(".page");
  if (pageEl) pageEl.classList.add("page-no-scroll");

  window.Goop = window.Goop || {};
  window.Goop.database = window.Goop.database || {};

  var viewTables = qs("#db-view-tables");
  var viewSchemas = qs("#db-view-schemas");
  var viewTransforms = qs("#db-view-transforms");
  var allViews = [viewTables, viewSchemas, viewTransforms];

  qsa("[data-db-tab]").forEach(function(tab) {
    on(tab, "click", function(e) {
      e.preventDefault();
      var target = tab.dataset.dbTab;
      qsa("[data-db-tab]").forEach(function(t) { t.classList.toggle("active", t === tab); });
      allViews.forEach(function(v) { v.classList.remove("active"); });
      if (target === "tables") {
        viewTables.classList.add("active");
      } else if (target === "schemas") {
        viewSchemas.classList.add("active");
        if (Goop.database.loadSchemas) Goop.database.loadSchemas();
      } else if (target === "transforms") {
        viewTransforms.classList.add("active");
        if (Goop.database.loadTransforms) Goop.database.loadTransforms();
      }
      if (Goop.splitPane) Goop.splitPane.init();
    });
  });

  if (Goop.database.initTables) Goop.database.initTables();
})();
