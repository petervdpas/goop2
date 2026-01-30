// Photobook app.js
(async function () {
  var db = Goop.data;
  var site = Goop.site;
  var galleryEl = document.getElementById("gallery");
  var btnUpload = document.getElementById("btn-upload");
  var uploadOverlay = document.getElementById("upload-overlay");
  var lightbox = document.getElementById("lightbox");
  var isOwner = false;
  var photos = [];
  var lbIndex = 0;

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // ── Owner detection ──
  // Compare our peer ID with the peer ID in the URL path /p/{id}/
  try {
    var myId = await Goop.identity.id();
    var match = window.location.pathname.match(/\/p\/([^/]+)/);
    if (match && match[1] === myId) {
      isOwner = true;
      btnUpload.classList.remove("hidden");
    }
  } catch (_) {}

  // ── Seed on first run ──
  async function seed() {
    var tables = await db.tables();
    if (tables && tables.length > 0) return;

    await db.createTable("photos", [
      { name: "filename", type: "TEXT", not_null: true },
      { name: "caption", type: "TEXT", default: "''" },
    ]);
  }

  // ── Load & render ──
  async function loadPhotos() {
    try {
      var rows = await db.query("photos", { limit: 200 });
      photos = rows || [];
      // newest first
      photos.sort(function (a, b) { return b._id - a._id; });
      renderGallery(photos);
    } catch (err) {
      galleryEl.innerHTML =
        '<div class="empty-msg"><p>Could not load photos.</p>' +
        '<p class="loading">' + esc(err.message) + "</p></div>";
    }
  }

  function renderGallery(list) {
    if (list.length === 0) {
      galleryEl.innerHTML =
        '<div class="empty-msg"><p>No photos yet.</p>' +
        (isOwner
          ? '<p class="loading">Click &ldquo;+ Upload Photo&rdquo; to add your first one.</p>'
          : "") +
        "</div>";
      return;
    }

    galleryEl.innerHTML = list
      .map(function (p, idx) {
        var html = '<div class="photo-card" data-idx="' + idx + '">';
        html +=
          '<img src="images/' +
          esc(p.filename) +
          '" alt="' +
          esc(p.caption || p.filename) +
          '">';
        if (p.caption) {
          html += '<div class="photo-caption">' + esc(p.caption) + "</div>";
        }
        if (isOwner) {
          html += '<div class="photo-actions">';
          html +=
            '<button data-action="edit-caption" data-id="' +
            p._id +
            '" title="Edit caption">&#9998;</button>';
          html +=
            '<button data-action="delete-photo" data-id="' +
            p._id +
            '" data-file="' +
            esc(p.filename) +
            '" title="Delete">&times;</button>';
          html += "</div>";
        }
        html += "</div>";
        return html;
      })
      .join("");

    wireGalleryActions();
  }

  function wireGalleryActions() {
    // Click card to open lightbox
    galleryEl.querySelectorAll(".photo-card").forEach(function (card) {
      card.addEventListener("click", function (e) {
        if (e.target.closest(".photo-actions")) return;
        var idx = parseInt(card.getAttribute("data-idx"), 10);
        openLightbox(idx);
      });
    });

    if (!isOwner) return;

    // Edit caption
    galleryEl
      .querySelectorAll("[data-action=edit-caption]")
      .forEach(function (btn) {
        btn.addEventListener("click", async function (e) {
          e.stopPropagation();
          var id = parseInt(btn.getAttribute("data-id"), 10);
          var photo = photos.find(function (p) {
            return p._id === id;
          });
          var caption = photo ? photo.caption : "";
          if (Goop.ui) {
            var newCaption = await Goop.ui.prompt("Edit caption", caption || "");
            if (newCaption === null) return;
            await db.update("photos", id, { caption: newCaption });
            loadPhotos();
          }
        });
      });

    // Delete photo
    galleryEl
      .querySelectorAll("[data-action=delete-photo]")
      .forEach(function (btn) {
        btn.addEventListener("click", async function (e) {
          e.stopPropagation();
          var id = parseInt(btn.getAttribute("data-id"), 10);
          var filename = btn.getAttribute("data-file");
          var ok = true;
          if (Goop.ui) ok = await Goop.ui.confirm("Delete this photo?");
          if (!ok) return;
          try {
            await site.remove("images/" + filename);
          } catch (_) {} // file may already be gone
          await db.remove("photos", id);
          loadPhotos();
        });
      });
  }

  // ── Lightbox ──
  function openLightbox(idx) {
    lbIndex = idx;
    updateLightbox();
    lightbox.classList.remove("hidden");
  }

  function closeLightbox() {
    lightbox.classList.add("hidden");
  }

  function updateLightbox() {
    var p = photos[lbIndex];
    if (!p) return;
    document.getElementById("lb-img").src = "images/" + p.filename;
    document.getElementById("lb-caption").textContent = p.caption || "";
  }

  document.getElementById("lb-close").addEventListener("click", closeLightbox);

  document.getElementById("lb-prev").addEventListener("click", function () {
    lbIndex = (lbIndex - 1 + photos.length) % photos.length;
    updateLightbox();
  });

  document.getElementById("lb-next").addEventListener("click", function () {
    lbIndex = (lbIndex + 1) % photos.length;
    updateLightbox();
  });

  lightbox.addEventListener("mousedown", function (e) {
    if (e.target === lightbox) closeLightbox();
  });

  document.addEventListener("keydown", function (e) {
    if (lightbox.classList.contains("hidden")) return;
    if (e.key === "Escape") closeLightbox();
    if (e.key === "ArrowLeft") {
      lbIndex = (lbIndex - 1 + photos.length) % photos.length;
      updateLightbox();
    }
    if (e.key === "ArrowRight") {
      lbIndex = (lbIndex + 1) % photos.length;
      updateLightbox();
    }
  });

  // ── Upload flow ──
  btnUpload.addEventListener("click", function () {
    document.getElementById("f-file").value = "";
    document.getElementById("f-caption").value = "";
    uploadOverlay.classList.remove("hidden");
  });

  document
    .getElementById("btn-cancel-upload")
    .addEventListener("click", function () {
      uploadOverlay.classList.add("hidden");
    });

  uploadOverlay.addEventListener("mousedown", function (e) {
    if (e.target === uploadOverlay) uploadOverlay.classList.add("hidden");
  });

  document
    .getElementById("btn-submit-upload")
    .addEventListener("click", async function () {
      var fileInput = document.getElementById("f-file");
      var caption = document.getElementById("f-caption").value.trim();

      if (!fileInput.files || !fileInput.files[0]) return;

      var file = fileInput.files[0];
      // Generate safe filename: timestamp + sanitized original name
      var ext = file.name.split(".").pop().toLowerCase();
      var safeName =
        Date.now() +
        "-" +
        file.name
          .replace(/\.[^.]+$/, "")
          .replace(/[^a-zA-Z0-9_-]/g, "_")
          .substring(0, 60) +
        "." +
        ext;

      try {
        await site.upload("images/" + safeName, file);
        await db.insert("photos", { filename: safeName, caption: caption });
        uploadOverlay.classList.add("hidden");
        if (Goop.ui) Goop.ui.toast("Photo uploaded!");
        loadPhotos();
      } catch (err) {
        if (Goop.ui)
          Goop.ui.toast({ title: "Upload failed", message: err.message });
      }
    });

  // ── Init ──
  await seed();
  loadPhotos();
})();
