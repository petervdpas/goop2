// app.js — peerC site logic
// Uses Goop.data to query the local database and render a data list.

(async function () {
  var db = Goop.data;
  var root = document.getElementById("data-root");

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // Seed a demo table if no tables exist yet
  async function seed() {
    var tables = await db.tables();
    if (tables && tables.length > 0) return;

    await db.createTable("bookmarks", [
      { name: "title", type: "TEXT", not_null: true },
      { name: "url", type: "TEXT", not_null: true },
      { name: "tags", type: "TEXT" },
    ]);

    await db.insert("bookmarks", { title: "Goop² Repo", url: "https://github.com/example/goop2", tags: "p2p, go" });
    await db.insert("bookmarks", { title: "libp2p Docs", url: "https://docs.libp2p.io", tags: "networking, p2p" });
    await db.insert("bookmarks", { title: "SQLite Docs", url: "https://sqlite.org/docs.html", tags: "database" });
    await db.insert("bookmarks", { title: "MDN Web Docs", url: "https://developer.mozilla.org", tags: "reference, web" });
    await db.insert("bookmarks", { title: "Go by Example", url: "https://gobyexample.com", tags: "go, tutorial" });
  }

  try {
    await seed();
    var tables = await db.tables();

    if (!tables || tables.length === 0) {
      root.innerHTML = '<p class="muted">No tables.</p>';
      return;
    }

    var html = "";

    for (var i = 0; i < tables.length; i++) {
      var t = tables[i];
      var cols = await db.describe(t.name);
      var rows = await db.query(t.name, { limit: 50 });

      // skip internal columns for display
      var displayCols = cols.filter(function (c) {
        return c.name !== "_owner" && c.name !== "_created_at";
      });

      html += '<section class="data-table">';
      html += "<h2>" + esc(t.name) + "</h2>";

      if (!rows || rows.length === 0) {
        html += '<p class="muted">Empty table</p>';
      } else {
        html += '<table><thead><tr>';
        displayCols.forEach(function (c) {
          html += "<th>" + esc(c.name) + "</th>";
        });
        html += "</tr></thead><tbody>";

        rows.forEach(function (row) {
          html += "<tr>";
          displayCols.forEach(function (c) {
            var v = row[c.name];
            html += "<td>" + (v == null ? '<span class="muted">null</span>' : esc(v)) + "</td>";
          });
          html += "</tr>";
        });

        html += "</tbody></table>";
      }

      html += "</section>";
    }

    root.innerHTML = html;
  } catch (err) {
    root.innerHTML = '<p class="error">Failed to load data: ' + esc(err.message) + "</p>";
  }
})();
