// Corkboard app.js
(async function () {
  var db = Goop.data;
  var board = document.getElementById("board");
  var addForm = document.getElementById("add-form");

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // seed sample notes on first run
  async function seed() {
    var tables = await db.tables();
    if (tables && tables.length > 0) return;

    await db.createTable("notes", [
      { name: "title", type: "TEXT", not_null: true },
      { name: "description", type: "TEXT" },
      { name: "category", type: "TEXT", default: "'general'" },
      { name: "contact", type: "TEXT" },
      { name: "color", type: "TEXT", default: "'yellow'" },
    ]);

    await db.insert("notes", { title: "Old bicycle for sale", description: "Blue 21-speed, good condition. Pick up only.", category: "selling", contact: "Knock on door #12" });
    await db.insert("notes", { title: "Cat sitter needed!", description: "Away Aug 10-17. Two friendly cats. Will pay.", category: "looking", contact: "Leave a note at #7" });
    await db.insert("notes", { title: "Free piano", description: "Upright piano, needs tuning. You arrange transport.", category: "offering" });
    await db.insert("notes", { title: "Block party Saturday", description: "BBQ at the park, 4pm. Bring a dish to share!", category: "event" });
  }

  async function loadNotes() {
    try {
      var notes = await db.query("notes", { limit: 100 });
      renderBoard(notes || []);
    } catch (err) {
      board.innerHTML = '<p class="loading">Failed to load: ' + esc(err.message) + "</p>";
    }
  }

  function renderBoard(notes) {
    if (notes.length === 0) {
      board.innerHTML = '<p class="loading">No notes pinned yet. Click "+ Pin a Note" to get started!</p>';
      return;
    }

    board.innerHTML = notes.map(function (n) {
      var cat = n.category || "general";
      var html = '<div class="note ' + esc(cat) + '">';
      html += '<div class="pin"></div>';
      html += '<button class="note-delete" data-id="' + n._id + '" title="Remove">&times;</button>';
      html += '<div class="note-cat">' + esc(cat) + "</div>";
      html += '<div class="note-title">' + esc(n.title) + "</div>";
      if (n.description) html += '<div class="note-desc">' + esc(n.description) + "</div>";
      if (n.contact) html += '<div class="note-contact">' + esc(n.contact) + "</div>";
      html += "</div>";
      return html;
    }).join("");

    // wire delete buttons
    board.querySelectorAll(".note-delete").forEach(function (btn) {
      btn.addEventListener("click", async function () {
        var id = parseInt(btn.getAttribute("data-id"), 10);
        if (Goop.ui) {
          var ok = await Goop.ui.confirm("Remove this note from the board?");
          if (!ok) return;
        }
        await db.remove("notes", id);
        loadNotes();
      });
    });
  }

  // add form
  document.getElementById("btn-add").addEventListener("click", function () {
    addForm.classList.remove("hidden");
    document.getElementById("f-title").focus();
  });

  document.getElementById("btn-cancel").addEventListener("click", function () {
    addForm.classList.add("hidden");
  });

  addForm.addEventListener("mousedown", function (e) {
    if (e.target === addForm) addForm.classList.add("hidden");
  });

  document.getElementById("btn-submit").addEventListener("click", async function () {
    var title = document.getElementById("f-title").value.trim();
    if (!title) return;

    await db.insert("notes", {
      title: title,
      description: document.getElementById("f-desc").value.trim(),
      category: document.getElementById("f-cat").value,
      contact: document.getElementById("f-contact").value.trim(),
    });

    // clear form
    document.getElementById("f-title").value = "";
    document.getElementById("f-desc").value = "";
    document.getElementById("f-cat").value = "general";
    document.getElementById("f-contact").value = "";
    addForm.classList.add("hidden");

    loadNotes();
  });

  // init
  await seed();
  loadNotes();
})();
