// Admin viewer JS for the Documents page.
(() => {
  var core = window.Goop && window.Goop.core;
  if (!core) return;

  var qs = core.qs;
  var on = core.on;
  var escapeHtml = core.escapeHtml;
  var setHidden = core.setHidden;
  var api = core.api;
  var toast = core.toast;

  var docsPage = qs("#docs-page");
  if (!docsPage) return;

  var gsel = window.Goop.select;
  var groupSelect = qs("#docs-group-select");
  var refreshBtn = qs("#docs-refresh-btn");
  var uploadArea = qs("#docs-upload-area");
  var fileInput = qs("#docs-file-input");
  var uploadBtn = qs("#docs-upload-btn");
  var uploadProgress = qs("#docs-upload-progress");
  var progressText = qs("#docs-progress-text");
  var mySection = qs("#docs-my-section");
  var myList = qs("#docs-my-list");
  var peersSection = qs("#docs-peers-section");
  var peersList = qs("#docs-peers-list");

  var previewEmpty = qs("#docs-preview-empty");
  var previewContent = qs("#docs-preview-content");

  var urlParams = new URLSearchParams(window.location.search);
  var currentGroupID = urlParams.get("group_id") || "";

  // File type detection
  var IMAGE_RE  = /\.(png|jpe?g|gif|webp|svg|bmp|ico|tiff?)$/i;
  var TEXT_RE   = /\.(txt|md|json|js|ts|go|py|html?|css|xml|ya?ml|toml|sh|bash|log|csv|sql|rs|c|cpp|h|java|rb|php|kt|swift|dart|lua|ini|conf|env)$/i;
  var AUDIO_RE  = /\.(mp3|ogg|wav|m4a|flac|aac|opus)$/i;
  var VIDEO_RE  = /\.(mp4|webm|ogv|mov|avi|mkv)$/i;
  var PDF_RE    = /\.pdf$/i;
  var TEXT_SIZE_LIMIT = 512 * 1024; // 512 KB

  // Init custom select with change handler
  gsel.init(groupSelect, function(newVal) {
    currentGroupID = newVal;
    if (currentGroupID) {
      setHidden(uploadArea, false);
      setHidden(mySection, false);
      setHidden(peersSection, false);
      loadBrowse();
    } else {
      setHidden(uploadArea, true);
      setHidden(mySection, true);
      setHidden(peersSection, true);
    }
  });

  // Load groups into selector
  loadGroups();

  on(refreshBtn, "click", function() {
    if (currentGroupID) loadBrowse();
  });

  on(fileInput, "change", function() {
    uploadBtn.disabled = !fileInput.files || fileInput.files.length === 0;
  });

  function doUpload(files) {
    if (!files || files.length === 0) return;
    if (!currentGroupID) return;

    for (var i = 0; i < files.length; i++) {
      if (files[i].size > 50 * 1024 * 1024) {
        toast('"' + files[i].name + '" exceeds 50 MB limit', true);
        return;
      }
    }

    uploadBtn.disabled = true;
    setHidden(uploadProgress, false);

    var total = files.length;

    function uploadNext(idx) {
      if (idx >= total) {
        fileInput.value = "";
        uploadBtn.disabled = true;
        setHidden(uploadProgress, true);
        loadBrowse();
        return;
      }

      if (progressText) {
        progressText.textContent = total > 1
          ? "Uploading " + (idx + 1) + " of " + total + "..."
          : "Uploading...";
      }

      var formData = new FormData();
      formData.append("group_id", currentGroupID);
      formData.append("file", files[idx]);

      fetch("/api/docs/upload", {
        method: "POST",
        body: formData
      }).then(function(r) {
        if (!r.ok) return r.text().then(function(t) { throw new Error(t); });
        return r.json();
      }).then(function(data) {
        toast("Uploaded: " + data.filename);
        uploadNext(idx + 1);
      }).catch(function(err) {
        toast("Upload failed: " + err.message, true);
        uploadBtn.disabled = false;
        setHidden(uploadProgress, true);
      });
    }

    uploadNext(0);
  }

  on(uploadBtn, "click", function() {
    doUpload(Array.from(fileInput.files));
  });

  // Drag and drop
  on(uploadArea, "dragenter", function(e) {
    e.preventDefault();
    if (currentGroupID) uploadArea.classList.add("drag-over");
  });

  on(uploadArea, "dragover", function(e) {
    e.preventDefault();
    if (currentGroupID) uploadArea.classList.add("drag-over");
  });

  on(uploadArea, "dragleave", function(e) {
    if (!uploadArea.contains(e.relatedTarget)) {
      uploadArea.classList.remove("drag-over");
    }
  });

  on(uploadArea, "drop", function(e) {
    e.preventDefault();
    uploadArea.classList.remove("drag-over");
    if (!currentGroupID) return;
    var files = Array.from(e.dataTransfer.files);
    if (files.length > 0) doUpload(files);
  });

  // Listen for group events to auto-refresh on doc changes (via MQ)
  function startEventListener() {
    if (!window.Goop || !window.Goop.mq) { setTimeout(startEventListener, 100); return; }
    Goop.mq.onGroup( function(from, topic, payload, ack) {
      if (payload && payload.type === 'msg' && currentGroupID && payload.group === currentGroupID) {
        var p = payload.payload;
        if (p && (p.action === "doc-added" || p.action === "doc-removed")) {
          loadBrowse();
        }
      }
      ack();
    });
  }
  startEventListener();

  function loadGroups() {
    // Fetch both hosted groups and subscriptions, filter to "files" type only
    Promise.all([
      api("/api/groups").catch(function() { return []; }),
      api("/api/groups/subscriptions").catch(function() { return { subscriptions: [] }; })
    ]).then(function(results) {
      var hosted = (results[0] || []).filter(function(g) { return g.app_type === "files"; });
      var subsData = results[1] || {};
      var subs = (subsData.subscriptions || []).filter(function(s) { return s.app_type === "files"; });

      var groups = [];

      if (hosted.length > 0) {
        var hostedItems = [];
        hosted.forEach(function(g) {
          hostedItems.push({ value: g.id, label: g.name + " (hosted)" });
        });
        groups.push({ label: "My Groups", items: hostedItems });
      }

      if (subs.length > 0) {
        var subItems = [];
        subs.forEach(function(s) {
          var label = s.group_name || s.group_id;
          subItems.push({ value: s.group_id, label: label + " (joined)" });
        });
        groups.push({ label: "Joined Groups", items: subItems });
      }

      if (hosted.length === 0 && subs.length === 0) {
        gsel.setOpts(groupSelect, {
          options: [{ value: "", label: 'No file-sharing groups. Create one with type "Files".', disabled: true }]
        }, "");
      } else {
        gsel.setOpts(groupSelect, { groups: groups }, currentGroupID || "");
        // setOpts does not fire the change callback — trigger manually if pre-selected
        if (currentGroupID) {
          setHidden(uploadArea, false);
          setHidden(mySection, false);
          setHidden(peersSection, false);
          loadBrowse();
        }
      }
    });
  }

  function loadBrowse() {
    if (!currentGroupID) return;

    clearPreview();
    myList.innerHTML = '<p class="docs-empty">Loading...</p>';
    peersList.innerHTML = '<p class="docs-empty">Loading...</p>';

    api("/api/docs/browse?group_id=" + encodeURIComponent(currentGroupID))
      .then(function(data) {
        var peers = data.peers || [];

        // Separate self from others
        var selfPeer = null;
        var otherPeers = [];
        peers.forEach(function(p) {
          if (p.self) {
            selfPeer = p;
          } else {
            otherPeers.push(p);
          }
        });

        // Render my files
        if (selfPeer && selfPeer.files && selfPeer.files.length > 0) {
          myList.innerHTML = renderFileTable(selfPeer.files, selfPeer.peer_id, true);
          bindDeleteButtons(myList);
          bindRowClicks(myList);
        } else {
          myList.innerHTML = '<p class="docs-empty">No files shared yet. Use the upload form above.</p>';
        }

        // Render peer files
        if (otherPeers.length > 0) {
          var html = "";
          otherPeers.forEach(function(p) {
            if (p.error) {
              html += '<div class="docs-peer-block">' +
                '<div class="docs-peer-label">' + escapeHtml(shortLabel(p.label, p.peer_id)) +
                ' <span class="docs-status-offline">offline</span></div>' +
                '</div>';
              return;
            }
            if (p.files && p.files.length > 0) {
              html += '<div class="docs-peer-block">' +
                '<div class="docs-peer-label">' + escapeHtml(shortLabel(p.label, p.peer_id)) +
                ' <span class="docs-status-online">online</span>' +
                ' <span class="muted small">(' + p.files.length + ' file' + (p.files.length !== 1 ? 's' : '') + ')</span></div>' +
                renderFileTable(p.files, p.peer_id, false) +
                '</div>';
            } else {
              html += '<div class="docs-peer-block">' +
                '<div class="docs-peer-label">' + escapeHtml(shortLabel(p.label, p.peer_id)) +
                ' <span class="docs-status-online">online</span>' +
                ' <span class="muted small">(no files)</span></div>' +
                '</div>';
            }
          });
          if (html) {
            peersList.innerHTML = html;
          } else {
            peersList.innerHTML = '<p class="docs-empty">No other members in this group.</p>';
          }
          bindRowClicks(peersList);
        } else {
          peersList.innerHTML = '<p class="docs-empty">No other members in this group.</p>';
        }
      })
      .catch(function(err) {
        myList.innerHTML = '<p class="docs-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
        peersList.innerHTML = '';
      });
  }

  function renderFileTable(files, peerID, isSelf) {
    var html = '<table class="docs-table"><thead><tr>' +
      '<th>Name</th><th>Size</th><th>Actions</th>' +
      '</tr></thead><tbody>';

    files.forEach(function(f) {
      var downloadUrl = '/api/docs/download?peer_id=' + encodeURIComponent(peerID) +
        '&group_id=' + encodeURIComponent(currentGroupID) +
        '&file=' + encodeURIComponent(f.name);

      html += '<tr class="docs-data-row"' +
        ' data-filename="' + escapeHtml(f.name) + '"' +
        ' data-url="' + escapeHtml(downloadUrl) + '"' +
        ' data-size="' + f.size + '"' +
        ' data-self="' + isSelf + '">' +
        '<td class="docs-file-name">' + escapeHtml(f.name) + '</td>' +
        '<td class="docs-file-size">' + formatSize(f.size) + '</td>' +
        '<td class="docs-file-actions">' +
        '<a href="' + downloadUrl + '" class="docs-action-btn docs-btn-small" download>Download</a>';

      if (isSelf) {
        html += '<button class="docs-action-btn docs-btn-small docs-btn-danger docs-delete-btn" ' +
          'data-file="' + escapeHtml(f.name) + '">Delete</button>';
      }

      html += '</td></tr>';
    });

    html += '</tbody></table>';
    return html;
  }

  // ---- Row click → preview ----

  function bindRowClicks(container) {
    container.querySelectorAll(".docs-data-row").forEach(function(row) {
      on(row, "click", function(e) {
        // Don't intercept clicks on buttons/links (let them work normally)
        if (e.target && e.target.closest("a, button")) return;

        // Deselect all rows across both lists
        docsPage.querySelectorAll(".docs-data-row.docs-row-selected").forEach(function(r) {
          r.classList.remove("docs-row-selected");
        });

        row.classList.add("docs-row-selected");

        showPreview({
          name:        row.getAttribute("data-filename"),
          downloadUrl: row.getAttribute("data-url"),
          size:        parseInt(row.getAttribute("data-size"), 10) || 0,
          isSelf:      row.getAttribute("data-self") === "true"
        });
      });
    });
  }

  // ---- Preview rendering ----

  function clearPreview() {
    docsPage.querySelectorAll(".docs-data-row.docs-row-selected").forEach(function(r) {
      r.classList.remove("docs-row-selected");
    });
    previewContent.innerHTML = "";
    setHidden(previewContent, true);
    setHidden(previewEmpty, false);
  }

  function showPreview(info) {
    var name = info.name;
    var downloadUrl = info.downloadUrl;
    var size = info.size;
    var isSelf = info.isSelf;

    var headerHtml =
      '<div class="docs-preview-header">' +
        '<div class="docs-preview-filename">' + escapeHtml(name) + '</div>' +
        '<div class="docs-preview-meta">' + formatSize(size) + (isSelf ? ' &middot; mine' : ' &middot; peer') + '</div>' +
      '</div>';

    var bodyHtml = buildPreviewBody(name, size, downloadUrl);

    var footerHtml =
      '<div class="docs-preview-footer">' +
        '<a href="' + escapeHtml(downloadUrl) + '" class="docs-action-btn docs-btn-primary docs-btn-small" download>Download</a>' +
      '</div>';

    previewContent.innerHTML = headerHtml +
      '<div class="docs-preview-body">' + bodyHtml + '</div>' +
      footerHtml;

    setHidden(previewEmpty, true);
    setHidden(previewContent, false);

    // For text files: fetch content asynchronously
    if (TEXT_RE.test(name) && size > 0 && size <= TEXT_SIZE_LIMIT) {
      var loadingEl = previewContent.querySelector(".docs-preview-loading");
      if (loadingEl) {
        fetch(downloadUrl)
          .then(function(r) {
            if (!r.ok) throw new Error("HTTP " + r.status);
            return r.text();
          })
          .then(function(text) {
            if (!loadingEl.parentNode) return; // panel was replaced
            var wrap = document.createElement("div");

            // Check if this is a markdown file
            if (/\.md$/i.test(name)) {
              try {
                wrap.className = "docs-preview-markdown-wrap";
                var mdDiv = document.createElement("div");
                mdDiv.className = "docs-preview-markdown";

                // marked.js can be accessed as window.marked or window.marked.marked
                var markedFn = window.marked;
                if (typeof markedFn !== 'function') {
                  if (window.marked && typeof window.marked.parse === 'function') {
                    markedFn = window.marked.parse;
                  } else {
                    throw new Error("marked.js not available (window.marked=" + typeof window.marked + ")");
                  }
                }
                mdDiv.innerHTML = markedFn(text);
                wrap.appendChild(mdDiv);
              } catch (e) {
                throw new Error("Markdown render error: " + e.message);
              }
            } else {
              wrap.className = "docs-preview-text-wrap";
              var pre = document.createElement("pre");
              pre.className = "docs-preview-text";
              pre.textContent = text;
              wrap.appendChild(pre);
            }

            loadingEl.parentNode.replaceChild(wrap, loadingEl);
          })
          .catch(function(err) {
            if (!loadingEl.parentNode) return;
            loadingEl.className = "docs-preview-error";
            loadingEl.textContent = "Could not load preview: " + (err.message || err);
          });
      }
    }

    // For images: handle load error gracefully
    if (IMAGE_RE.test(name)) {
      var img = previewContent.querySelector(".docs-preview-image");
      if (img) {
        img.onerror = function() {
          var wrap = img.parentNode;
          if (wrap) {
            var err = document.createElement("div");
            err.className = "docs-preview-error";
            err.textContent = "Could not load image — peer may be offline.";
            wrap.parentNode.replaceChild(err, wrap);
          }
        };
      }
    }
  }

  function buildPreviewBody(name, size, downloadUrl) {
    if (IMAGE_RE.test(name)) {
      return '<div class="docs-preview-image-wrap">' +
        '<img class="docs-preview-image" src="' + escapeHtml(downloadUrl) + '" alt="' + escapeHtml(name) + '" />' +
        '</div>';
    }

    if (TEXT_RE.test(name)) {
      if (size > TEXT_SIZE_LIMIT) {
        return '<div class="docs-preview-unavailable">File too large to preview (' + formatSize(size) + ')</div>';
      }
      if (size === 0) {
        return '<div class="docs-preview-unavailable">Empty file</div>';
      }
      return '<div class="docs-preview-loading">Loading preview\u2026</div>';
    }

    if (AUDIO_RE.test(name)) {
      return '<audio class="docs-preview-audio" src="' + escapeHtml(downloadUrl) + '" controls></audio>';
    }

    if (VIDEO_RE.test(name)) {
      return '<video class="docs-preview-video" src="' + escapeHtml(downloadUrl) + '" controls></video>';
    }

    if (PDF_RE.test(name)) {
      var inlineUrl = escapeHtml(downloadUrl + "&inline=1");
      return '<iframe class="docs-preview-pdf" src="' + inlineUrl + '" title="' + escapeHtml(name) + '"></iframe>';
    }

    return '<div class="docs-preview-unavailable">No preview available<br>' +
      '<span style="font-size:11px">Use the Download button to save this file.</span></div>';
  }

  // ---- Delete ----

  function bindDeleteButtons(container) {
    container.querySelectorAll(".docs-delete-btn").forEach(function(btn) {
      on(btn, "click", function() {
        var filename = btn.getAttribute("data-file");
        Goop.dialogs.confirm('Delete "' + filename + '"? Other peers will no longer be able to download it.', 'Delete File').then(function(ok) {
          if (ok) deleteFile(filename);
        });
      });
    });
  }

  function deleteFile(filename) {
    api("/api/docs/delete", { group_id: currentGroupID, filename: filename })
      .then(function() {
        toast("Deleted: " + filename);
        loadBrowse();
      })
      .catch(function(err) {
        toast("Failed to delete: " + err.message, true);
      });
  }

  // ---- Helpers ----

  function shortLabel(label, id) {
    if (label && label !== id) return label;
    if (!id || id.length <= 12) return id || "Unknown";
    return id.substring(0, 8) + "\u2026";
  }

  function formatSize(bytes) {
    if (bytes === 0) return "0 B";
    var units = ["B", "KB", "MB", "GB"];
    var i = 0;
    var b = bytes;
    while (b >= 1024 && i < units.length - 1) {
      b /= 1024;
      i++;
    }
    return (i === 0 ? b : b.toFixed(1)) + " " + units[i];
  }
})();
