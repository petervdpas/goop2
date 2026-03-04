// Kanban app.js
(async function () {
  var db = Goop.data;
  var root = document.getElementById("kanban-root");
  var gate = document.getElementById("kanban-gate");
  var subtitle = document.getElementById("subtitle");
  var columns = [];

  var myId = await Goop.identity.id();
  var myLabel = await Goop.identity.label();

  // Detect owner vs visitor
  var match = window.location.pathname.match(/\/p\/([^/]+)/);
  var ownerPeerId = match ? match[1] : null;
  var isOwner = !ownerPeerId || ownerPeerId === myId;

  // Check template group membership for non-owners
  var isMember = false;
  if (!isOwner && Goop.group) {
    try {
      var subs = await Goop.group.subscriptions();
      var list = (subs && subs.subscriptions) || [];
      isMember = list.some(function (s) {
        return s.host_peer_id === ownerPeerId && s.app_type === "template";
      });
    } catch (_) {}
  }

  // Find template group ID (owner needs this for approvals)
  var templateGroupId = null;

  if (isOwner) {
    // Owner: show board + config gear
    subtitle.textContent = "Shared team kanban board";
    gate.style.display = "none";
    root.style.display = "block";
    loadConfig();
    loadBoard();
    initOwnerConfig();
  } else if (isMember) {
    // Member: show board (no config gear)
    subtitle.textContent = "Shared team kanban board";
    gate.style.display = "none";
    root.style.display = "block";
    loadConfig();
    loadBoard();
  } else {
    // Non-member: show gate
    loadConfig();
    subtitle.textContent = "Join to collaborate on this board";
    gate.style.display = "flex";
    root.style.display = "none";
    initGate();
  }

  // --- Gate logic ---

  async function initGate() {
    var gateRequest = document.getElementById("gate-request");
    var gatePending = document.getElementById("gate-pending");
    var gateApproved = document.getElementById("gate-approved");
    var gateDenied = document.getElementById("gate-denied");

    try {
      var res = await db.call("kanban", { action: "get_my_request" });
      if (res.status === "pending") {
        gateRequest.classList.add("hidden");
        gatePending.classList.remove("hidden");
      } else if (res.status === "approved") {
        gateRequest.classList.add("hidden");
        gateApproved.classList.remove("hidden");
      } else if (res.status === "dismissed") {
        gateRequest.classList.add("hidden");
        gateDenied.classList.remove("hidden");
      }
    } catch (_) {}

    document.getElementById("btn-request").onclick = async function () {
      var btn = this;
      btn.disabled = true;
      btn.textContent = "Sending...";
      try {
        var msg = document.getElementById("request-message").value.trim();
        var res = await db.call("kanban", {
          action: "request_join",
          peer_name: myLabel || myId,
          message: msg
        });
        if (res.status === "pending") {
          gateRequest.classList.add("hidden");
          gatePending.classList.remove("hidden");
        } else {
          // Already requested
          gateRequest.classList.add("hidden");
          if (res.status === "dismissed") gateDenied.classList.remove("hidden");
          else if (res.status === "approved") gateApproved.classList.remove("hidden");
          else gatePending.classList.remove("hidden");
        }
      } catch (e) {
        Goop.ui.toast({ title: "Error", message: e.message || "Failed to send request" });
        btn.disabled = false;
        btn.textContent = "Request to Join";
      }
    };
  }

  // --- Owner config ---

  function initOwnerConfig() {
    var configModal = document.getElementById("config-modal");
    var configBtn = document.getElementById("config-btn");
    configBtn.classList.remove("hidden");

    configBtn.onclick = async function () {
      document.getElementById("cfg-title").value = document.getElementById("board-title").textContent;
      document.getElementById("cfg-subtitle").value = subtitle.textContent;
      configModal.classList.remove("hidden");
      document.getElementById("cfg-title").focus();
      await loadRequests();
    };

    configModal.onclick = function (e) {
      if (e.target === configModal) configModal.classList.add("hidden");
    };

    document.getElementById("cancel-config").onclick = function () {
      configModal.classList.add("hidden");
    };

    document.getElementById("save-config").onclick = async function () {
      var t = document.getElementById("cfg-title").value.trim();
      var s = document.getElementById("cfg-subtitle").value.trim();
      try {
        await db.call("kanban", {
          action: "save_config",
          title: t || "Kanban Board",
          subtitle: s || "Shared team kanban board"
        });
        document.getElementById("board-title").textContent = t || "Kanban Board";
        subtitle.textContent = s || "Shared team kanban board";
        configModal.classList.add("hidden");
      } catch (e) {
        Goop.ui.toast({ title: "Error", message: e.message || "Failed to save config" });
      }
    };
  }

  async function findTemplateGroupId() {
    if (templateGroupId) return templateGroupId;
    try {
      var groups = await Goop.group.list();
      for (var i = 0; i < groups.length; i++) {
        if (groups[i].app_type === "template") {
          templateGroupId = groups[i].id;
          return templateGroupId;
        }
      }
    } catch (_) {}
    return null;
  }

  async function loadRequests() {
    var container = document.getElementById("cfg-requests-list");
    var noRequests = document.getElementById("cfg-no-requests");
    var section = document.getElementById("cfg-requests");
    section.style.display = "block";

    try {
      var res = await db.call("kanban", { action: "get_requests" });
      var requests = res.requests || [];
      if (requests.length === 0) {
        container.innerHTML = "";
        noRequests.style.display = "block";
        return;
      }
      noRequests.style.display = "none";
      var html = "";
      for (var i = 0; i < requests.length; i++) {
        var r = requests[i];
        html += '<div class="request-item" data-request-id="' + r._id + '" data-peer-id="' + esc(r._owner) + '">';
        html += '<div class="request-info">';
        html += '<span class="request-name">' + esc(r.peer_name || r._owner) + '</span>';
        if (r.message) html += '<span class="request-msg">' + esc(r.message) + '</span>';
        html += '</div>';
        html += '<div class="request-actions">';
        html += '<button class="btn btn-primary btn-sm btn-approve">Approve</button>';
        html += '<button class="btn btn-secondary btn-sm btn-dismiss">Dismiss</button>';
        html += '</div>';
        html += '</div>';
      }
      container.innerHTML = html;

      container.querySelectorAll(".btn-approve").forEach(function (btn) {
        btn.onclick = async function () {
          var item = btn.closest(".request-item");
          var reqId = parseInt(item.getAttribute("data-request-id"));
          var peerId = item.getAttribute("data-peer-id");
          btn.disabled = true;
          btn.textContent = "Approving...";
          try {
            var res = await db.call("kanban", { action: "approve_request", request_id: reqId });
            if (res.error) throw new Error(res.error);
            // Invite the peer to the template group
            var groupId = await findTemplateGroupId();
            if (groupId && peerId) {
              await fetch("/api/groups/invite", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ group_id: groupId, peer_id: peerId })
              });
            }
            item.remove();
            if (!container.querySelector(".request-item")) {
              noRequests.style.display = "block";
            }
          } catch (e) {
            Goop.ui.toast({ title: "Error", message: e.message || "Failed to approve" });
            btn.disabled = false;
            btn.textContent = "Approve";
          }
        };
      });

      container.querySelectorAll(".btn-dismiss").forEach(function (btn) {
        btn.onclick = async function () {
          var item = btn.closest(".request-item");
          var reqId = parseInt(item.getAttribute("data-request-id"));
          btn.disabled = true;
          try {
            await db.call("kanban", { action: "dismiss_request", request_id: reqId });
            item.remove();
            if (!container.querySelector(".request-item")) {
              noRequests.style.display = "block";
            }
          } catch (e) {
            Goop.ui.toast({ title: "Error", message: e.message || "Failed to dismiss" });
            btn.disabled = false;
          }
        };
      });
    } catch (e) {
      container.innerHTML = '';
      noRequests.style.display = "block";
    }
  }

  // --- Board ---

  async function loadConfig() {
    try {
      var cfg = await db.call("kanban", { action: "get_config" });
      if (cfg.title) document.getElementById("board-title").textContent = cfg.title;
      if (cfg.subtitle && (isOwner || isMember)) subtitle.textContent = cfg.subtitle;
    } catch (_) {}
  }

  async function loadBoard() {
    try {
      var result = await db.call("kanban", { action: "get_board" });
      columns = result.columns || [];
      renderBoard();
    } catch (e) {
      root.innerHTML = '<p class="loading">Failed to load board: ' + esc(e.message) + '</p>';
    }
  }

  function renderBoard() {
    var html = '<div class="board">';

    for (var i = 0; i < columns.length; i++) {
      var col = columns[i];
      var cards = Array.isArray(col.cards) ? col.cards : [];

      html += '<div class="column" data-column-id="' + col._id + '">';
      html += '<div class="column-header">';
      html += '<span class="column-title">';
      html += '<span class="column-dot" style="background:' + esc(col.color || '#5b6abf') + '"></span>';
      html += esc(col.name);
      html += '</span>';
      html += '<span class="column-count">' + cards.length + (cards.length === 1 ? ' item' : ' items') + '</span>';
      if (isOwner) {
        html += '<span class="column-actions">';
        if (i > 0) {
          html += '<button class="col-action-btn col-move-left" data-col-id="' + col._id + '" title="Move left">&#9664;</button>';
        }
        if (i < columns.length - 1) {
          html += '<button class="col-action-btn col-move-right" data-col-id="' + col._id + '" title="Move right">&#9654;</button>';
        }
        html += '<button class="col-action-btn col-delete" data-col-id="' + col._id + '" title="Delete column">&#10005;</button>';
        html += '</span>';
      }
      html += '</div>';

      html += '<div class="column-cards">';
      if (cards.length === 0) {
        html += '<div class="empty-column">No cards</div>';
      } else {
        for (var j = 0; j < cards.length; j++) {
          html += renderCard(cards[j], col._id);
        }
      }
      html += '</div>';

      html += '<div class="column-add">';
      html += '<button class="btn-add" data-add-column="' + col._id + '">+ Add card</button>';
      html += '</div>';

      html += '</div>';
    }

    // Owner can add columns
    if (isOwner) {
      html += '<div class="column" style="background:transparent;border:1px dashed var(--border)">';
      html += '<button class="btn-add" id="add-column-btn" style="margin:1rem">+ Add Column</button>';
      html += '</div>';
    }

    html += '</div>';
    root.innerHTML = html;

    // Wire up add card buttons
    root.querySelectorAll("[data-add-column]").forEach(function (btn) {
      btn.onclick = function () {
        showAddCardModal(btn.getAttribute("data-add-column"));
      };
    });

    // Wire up card clicks
    root.querySelectorAll(".card").forEach(function (card) {
      card.onclick = function (e) {
        if (e.target.closest(".card-action-btn") || e.target.closest(".move-dropdown")) return;
        var cardId = card.getAttribute("data-card-id");
        showEditCardModal(cardId);
      };
    });

    // Wire up move buttons
    root.querySelectorAll(".move-btn").forEach(function (btn) {
      btn.onclick = function (e) {
        e.stopPropagation();
        var dropdown = btn.nextElementSibling;
        root.querySelectorAll(".move-dropdown.open").forEach(function (d) {
          if (d !== dropdown) d.classList.remove("open");
        });
        dropdown.classList.toggle("open");
      };
    });

    root.querySelectorAll(".move-option").forEach(function (opt) {
      opt.onclick = async function (e) {
        e.stopPropagation();
        var cardId = parseInt(opt.getAttribute("data-card-id"));
        var toColumn = parseInt(opt.getAttribute("data-to-column"));
        await moveCard(cardId, toColumn);
      };
    });

    // Wire up column action buttons
    root.querySelectorAll(".col-move-left").forEach(function (btn) {
      btn.onclick = function (e) {
        e.stopPropagation();
        moveColumn(parseInt(btn.getAttribute("data-col-id")), "left");
      };
    });

    root.querySelectorAll(".col-move-right").forEach(function (btn) {
      btn.onclick = function (e) {
        e.stopPropagation();
        moveColumn(parseInt(btn.getAttribute("data-col-id")), "right");
      };
    });

    root.querySelectorAll(".col-delete").forEach(function (btn) {
      btn.onclick = function (e) {
        e.stopPropagation();
        deleteColumn(parseInt(btn.getAttribute("data-col-id")));
      };
    });

    // Add column button
    var addColBtn = document.getElementById("add-column-btn");
    if (addColBtn) {
      addColBtn.onclick = function () {
        var name = prompt("Column name:");
        if (name && name.trim()) {
          addColumn(name.trim());
        }
      };
    }

    // Close dropdowns on click outside
    document.addEventListener("click", function () {
      root.querySelectorAll(".move-dropdown.open").forEach(function (d) {
        d.classList.remove("open");
      });
    });
  }

  function renderCard(card, currentColumnId) {
    var html = '<div class="card" data-card-id="' + card._id + '">';

    if (card.color) {
      html += '<div class="card-color-bar" style="background:' + esc(card.color) + '"></div>';
    }

    html += '<div class="card-actions">';
    html += '<button class="card-action-btn move-btn" title="Move">&#8644;</button>';
    html += '<div class="move-dropdown">';
    for (var i = 0; i < columns.length; i++) {
      var col = columns[i];
      if (col._id !== currentColumnId) {
        html += '<button class="move-option" data-card-id="' + card._id + '" data-to-column="' + col._id + '">';
        html += esc(col.name);
        html += '</button>';
      }
    }
    html += '</div>';
    html += '</div>';

    html += '<div class="card-title">' + esc(card.title) + '</div>';

    if (card.description) {
      html += '<div class="card-desc">' + esc(card.description) + '</div>';
    }

    // Attribution: created by / moved by
    var attrs = [];
    if (card.created_by) attrs.push('added by ' + esc(card.created_by));
    if (card.moved_by) attrs.push('moved by ' + esc(card.moved_by));
    if (attrs.length > 0) {
      html += '<div class="card-meta">' + attrs.join(' &middot; ') + '</div>';
    }

    html += '</div>';
    return html;
  }

  // --- Add card modal ---

  var addCardModal = document.getElementById("add-card-modal");
  var selectedColor = "";

  function showAddCardModal(columnId) {
    document.getElementById("card-column-id").value = columnId;
    document.getElementById("card-title").value = "";
    document.getElementById("card-desc").value = "";
    document.getElementById("card-color-value").value = "";
    selectedColor = "";

    addCardModal.querySelectorAll(".color-btn").forEach(function (btn) {
      btn.classList.remove("selected");
      if (btn.getAttribute("data-color") === "") btn.classList.add("selected");
    });

    addCardModal.classList.remove("hidden");
    document.getElementById("card-title").focus();
  }

  addCardModal.querySelectorAll(".color-btn").forEach(function (btn) {
    btn.onclick = function () {
      addCardModal.querySelectorAll(".color-btn").forEach(function (b) { b.classList.remove("selected"); });
      btn.classList.add("selected");
      selectedColor = btn.getAttribute("data-color");
      document.getElementById("card-color-value").value = selectedColor;
    };
  });

  document.getElementById("cancel-card").onclick = function () {
    addCardModal.classList.add("hidden");
  };

  addCardModal.onclick = function (e) {
    if (e.target === addCardModal) addCardModal.classList.add("hidden");
  };

  document.getElementById("save-card").onclick = async function () {
    var title = document.getElementById("card-title").value.trim();
    var columnId = parseInt(document.getElementById("card-column-id").value);
    var desc = document.getElementById("card-desc").value.trim();
    var color = document.getElementById("card-color-value").value;

    if (!title) {
      document.getElementById("card-title").focus();
      return;
    }

    try {
      await db.call("kanban", {
        action: "add_card",
        column_id: columnId,
        title: title,
        description: desc,
        color: color,
        peer_name: myLabel || myId
      });
      addCardModal.classList.add("hidden");
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to add card" });
    }
  };

  // --- Edit card modal ---

  var editCardModal = document.getElementById("edit-card-modal");
  var editingCardId = null;

  function showEditCardModal(cardId) {
    editingCardId = parseInt(cardId);

    var card = null;
    for (var i = 0; i < columns.length; i++) {
      var cards = Array.isArray(columns[i].cards) ? columns[i].cards : [];
      for (var j = 0; j < cards.length; j++) {
        if (cards[j]._id === editingCardId) {
          card = cards[j];
          break;
        }
      }
      if (card) break;
    }

    if (!card) return;

    document.getElementById("edit-card-id").value = card._id;
    document.getElementById("edit-card-title").value = card.title || "";
    document.getElementById("edit-card-desc").value = card.description || "";
    document.getElementById("edit-card-color-value").value = card.color || "";

    var editColorPicker = document.getElementById("edit-color-picker");
    editColorPicker.querySelectorAll(".color-btn").forEach(function (btn) {
      btn.classList.remove("selected");
      if (btn.getAttribute("data-color") === (card.color || "")) btn.classList.add("selected");
    });

    editCardModal.classList.remove("hidden");
    document.getElementById("edit-card-title").focus();
  }

  document.getElementById("edit-color-picker").querySelectorAll(".color-btn").forEach(function (btn) {
    btn.onclick = function () {
      document.getElementById("edit-color-picker").querySelectorAll(".color-btn").forEach(function (b) { b.classList.remove("selected"); });
      btn.classList.add("selected");
      document.getElementById("edit-card-color-value").value = btn.getAttribute("data-color");
    };
  });

  document.getElementById("cancel-edit-card").onclick = function () {
    editCardModal.classList.add("hidden");
  };

  editCardModal.onclick = function (e) {
    if (e.target === editCardModal) editCardModal.classList.add("hidden");
  };

  document.getElementById("save-edit-card").onclick = async function () {
    var cardId = parseInt(document.getElementById("edit-card-id").value);
    var title = document.getElementById("edit-card-title").value.trim();
    var desc = document.getElementById("edit-card-desc").value.trim();
    var color = document.getElementById("edit-card-color-value").value;

    if (!title) {
      document.getElementById("edit-card-title").focus();
      return;
    }

    try {
      await db.call("kanban", {
        action: "update_card",
        card_id: cardId,
        title: title,
        description: desc,
        color: color
      });
      editCardModal.classList.add("hidden");
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to update card" });
    }
  };

  document.getElementById("delete-card").onclick = async function () {
    var cardId = parseInt(document.getElementById("edit-card-id").value);

    if (Goop.ui && Goop.ui.confirm) {
      var ok = await Goop.ui.confirm("Delete this card?");
      if (!ok) return;
    }

    try {
      await db.call("kanban", {
        action: "delete_card",
        card_id: cardId
      });
      editCardModal.classList.add("hidden");
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to delete card" });
    }
  };

  async function moveCard(cardId, toColumn) {
    try {
      await db.call("kanban", {
        action: "move_card",
        card_id: cardId,
        to_column: toColumn,
        peer_name: myLabel || myId
      });
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to move card" });
    }
  }

  async function deleteColumn(columnId) {
    if (!confirm("Delete this column?")) return;
    try {
      var res = await db.call("kanban", { action: "delete_column", column_id: columnId });
      if (res.error) throw new Error(res.error);
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to delete column" });
    }
  }

  async function moveColumn(columnId, direction) {
    try {
      var res = await db.call("kanban", { action: "move_column", column_id: columnId, direction: direction });
      if (res.error) throw new Error(res.error);
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to move column" });
    }
  }

  async function addColumn(name) {
    try {
      await db.call("kanban", {
        action: "add_column",
        name: name
      });
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to add column" });
    }
  }

  function esc(s) {
    if (!s) return "";
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }
})();
