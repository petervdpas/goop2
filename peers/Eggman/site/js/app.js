// Kanban app.js
(async function () {
  var h = Goop.dom;
  var db = Goop.data;
  var root = document.getElementById("kanban-root");
  var gate = document.getElementById("kanban-gate");
  var subtitle = document.getElementById("subtitle");
  var columns = [];

  var toast = Goop.ui.toast(document.getElementById("toasts"), {
    toastClass: "gc-toast",
    titleClass: "gc-toast-title",
    messageClass: "gc-toast-message",
    enterClass: "gc-toast-enter",
    exitClass: "gc-toast-exit",
  });

  Goop.ui.dialog(document.getElementById("confirm-dialog"), {
    title: ".gc-dialog-title",
    message: ".gc-dialog-message",
    inputWrap: ".gc-dialog-input-wrap",
    input: ".gc-dialog-input",
    ok: ".gc-dialog-ok",
    cancel: ".gc-dialog-cancel",
    hiddenClass: "hidden",
  });

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
    gate.classList.add("hidden");
    root.classList.remove("hidden");
    loadConfig();
    loadBoard();
    initOwnerConfig();
  } else if (isMember) {
    // Member: show board (no config gear)
    subtitle.textContent = "Shared team kanban board";
    gate.classList.add("hidden");
    root.classList.remove("hidden");
    loadConfig();
    loadBoard();
  } else {
    // Non-member: show gate
    loadConfig();
    subtitle.textContent = "Join to collaborate on this board";
    gate.classList.remove("hidden");
    root.classList.add("hidden");
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
        toast({ title: "Error", message: e.message || "Failed to send request" });
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
        toast({ title: "Error", message: e.message || "Failed to save config" });
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
    section.classList.remove("hidden");

    try {
      var res = await db.call("kanban", { action: "get_requests" });
      var requests = res.requests || [];
      if (requests.length === 0) { container.innerHTML = ""; noRequests.classList.remove("hidden"); return; }
      noRequests.classList.add("hidden");
      Goop.list(container, requests, function(r) {
        var item = h("div", { class: "request-item" },
          h("div", { class: "request-info" },
            h("span", { class: "request-name" }, r.peer_name || r._owner),
            r.message ? h("span", { class: "request-msg" }, r.message) : null
          ),
          h("div", { class: "request-actions" },
            h("button", { class: "btn btn-primary btn-sm", onclick: async function() {
              this.disabled = true; this.textContent = "Approving...";
              try {
                var res = await db.call("kanban", { action: "approve_request", request_id: r._id });
                if (res.error) throw new Error(res.error);
                var groupId = await findTemplateGroupId();
                if (groupId && r._owner) {
                  await fetch("/api/groups/invite", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ group_id: groupId, peer_id: r._owner }) });
                }
                item.remove();
                if (!container.querySelector(".gc-item")) noRequests.classList.remove("hidden");
              } catch (e) { toast.error(e.message || "Failed to approve"); this.disabled = false; this.textContent = "Approve"; }
            } }, "Approve"),
            h("button", { class: "btn btn-secondary btn-sm", onclick: async function() {
              this.disabled = true;
              try {
                await db.call("kanban", { action: "dismiss_request", request_id: r._id });
                item.remove();
                if (!container.querySelector(".request-item")) noRequests.classList.remove("hidden");
              } catch (e) { toast.error(e.message || "Failed to dismiss"); this.disabled = false; }
            } }, "Dismiss")
          )
        );
        return item;
      });
    } catch (e) { container.innerHTML = ""; noRequests.classList.remove("hidden"); }
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
      Goop.render(root, Goop.ui.empty("Failed to load board: " + (e.message || "unknown error"), { class: "loading" }));
    }
  }

  async function renderBoard() {
    var boardEl = h("div", { class: "board" },
      columns.map(function(col, i) {
        var cards = Array.isArray(col.cards) ? col.cards : [];
        return h("div", { class: "column", data: { "column-id": col._id } },
          h("div", { class: "column-header" },
            h("span", { class: "column-title" },
              h("span", { class: "column-dot", data: { color: col.color || "#5b6abf" } }),
              col.name
            ),
            h("span", { class: "column-count" }, cards.length + (cards.length === 1 ? " item" : " items")),
            isOwner ? h("span", { class: "column-actions" },
              h("button", { class: "col-action-btn", title: "Rename column", onclick: function(e) { e.stopPropagation(); renameColumn(col._id, col.name); } }, "\u270E"),
              i > 0 ? h("button", { class: "col-action-btn", title: "Move left", onclick: function(e) { e.stopPropagation(); moveColumn(col._id, "left"); } }, "\u25C4") : null,
              i < columns.length - 1 ? h("button", { class: "col-action-btn", title: "Move right", onclick: function(e) { e.stopPropagation(); moveColumn(col._id, "right"); } }, "\u25BA") : null,
              h("button", { class: "col-action-btn", title: "Delete column", onclick: function(e) { e.stopPropagation(); deleteColumn(col._id); } }, "\u2715")
            ) : null
          ),
          h("div", { class: "column-cards", data: { "col-id": col._id } },
            cards.length === 0
              ? h("div", { class: "empty-column" }, "No cards")
              : null
          ),
          h("div", { class: "column-add" },
            h("button", { class: "btn-add", onclick: function() { showAddCardModal(col._id); } }, "+ Add card")
          )
        );
      }),
      isOwner ? h("div", { class: "column add-column-placeholder" },
        h("button", { class: "btn-add", onclick: async function() {
          var name = await Goop.ui.prompt({ title: "New Column", message: "Column name:" });
          if (name && name.trim()) addColumn(name.trim());
        } }, "+ Add Column")
      ) : null
    );

    Goop.render(root, boardEl);

    // Render cards async into their columns
    for (var ci = 0; ci < columns.length; ci++) {
      var col = columns[ci];
      var cards = Array.isArray(col.cards) ? col.cards : [];
      var container = root.querySelector('[data-col-id="' + col._id + '"]');
      if (container && cards.length > 0) {
        for (var cj = 0; cj < cards.length; cj++) {
          container.appendChild(await renderCard(cards[cj], col._id));
        }
      }
    }

    document.addEventListener("click", function() {
      root.querySelectorAll(".move-dropdown.open").forEach(function(d) { d.classList.remove("open"); });
    });

    if (Goop.drag) initDrag();
  }

  async function renderCard(card, currentColumnId) {
    var metaParts = [];
    if (card.created_by) metaParts.push("added by " + card.created_by);
    if (card.moved_by) metaParts.push("moved by " + card.moved_by);

    var el = await Goop.partial("card", {
      _id: card._id, title: card.title, description: card.description,
      color: card.color || "", meta: metaParts.join(" \u00B7 ")
    });

    var moveDropdown = h("div", { class: "move-dropdown" },
      columns.filter(function(c) { return c._id !== currentColumnId; }).map(function(col) {
        return h("button", { class: "move-option", onclick: async function(e) {
          e.stopPropagation();
          await moveCard(card._id, col._id);
        } }, col.name);
      })
    );

    el.insertBefore(h("div", { class: "card-actions" },
      h("button", { class: "card-action-btn move-btn", title: "Move", onclick: function(e) {
        e.stopPropagation();
        root.querySelectorAll(".move-dropdown.open").forEach(function(d) { if (d !== moveDropdown) d.classList.remove("open"); });
        moveDropdown.classList.toggle("open");
      } }, "\u21C4"),
      moveDropdown
    ), el.firstChild);

    el.addEventListener("click", function(e) {
      if (justDragged) return;
      if (e.target.closest(".card-action-btn") || e.target.closest(".move-dropdown")) return;
      showEditCardModal(card._id);
    });

    return el;
  }

  // --- Drag-and-drop ---

  var cardSortables = [];
  var columnSortable = null;
  var justDragged = false;

  function initDrag() {
    destroyDrag();

    // Card drag (move between columns)
    document.querySelectorAll(".column-cards").forEach(function (container) {
      cardSortables.push(Goop.drag.sortable(container, {
        items: ".card",
        group: "cards",
        direction: "vertical",
        onEnd: function (evt) {
          justDragged = true;
          setTimeout(function () { justDragged = false; }, 0);
          var cardId = parseInt(evt.item.getAttribute("data-card-id"));
          var toColumn = parseInt(evt.to.closest(".column").getAttribute("data-column-id"));
          var toPosition = evt.newIndex;
          moveCardToPosition(cardId, toColumn, toPosition);
        },
        onCancel: function () {
          justDragged = true;
          setTimeout(function () { justDragged = false; }, 0);
        }
      }));
    });

    // Column drag (owner only)
    if (isOwner) {
      var board = document.querySelector(".board");
      if (board) {
        columnSortable = Goop.drag.sortable(board, {
          items: ".column[data-column-id]",
          handle: ".column-header",
          direction: "horizontal",
          onEnd: function () {
            reorderColumns();
          }
        });
      }
    }
  }

  function destroyDrag() {
    for (var i = 0; i < cardSortables.length; i++) cardSortables[i].destroy();
    cardSortables = [];
    if (columnSortable) { columnSortable.destroy(); columnSortable = null; }
  }

  async function moveCardToPosition(cardId, toColumn, toPosition) {
    try {
      await db.call("kanban", {
        action: "move_card",
        card_id: cardId,
        to_column: toColumn,
        to_position: toPosition,
        peer_name: myLabel || myId
      });
      loadBoard();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to move card" });
      loadBoard();
    }
  }

  async function reorderColumns() {
    var cols = document.querySelectorAll(".board > .column[data-column-id]");
    var ids = [];
    cols.forEach(function (col) {
      ids.push(parseInt(col.getAttribute("data-column-id")));
    });
    try {
      var res = await db.call("kanban", {
        action: "reorder_columns",
        column_ids: ids
      });
      if (res.error) throw new Error(res.error);
      loadBoard();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to reorder columns" });
      loadBoard();
    }
  }

  // --- Add card modal ---

  var addCardModal = document.getElementById("add-card-modal");
  var CARD_COLORS = ["#ef4444", "#f59e0b", "#22c55e", "#3b82f6", "#8b5cf6", "#ec4899"];
  var addColorPicker = null;

  function showAddCardModal(columnId) {
    document.getElementById("card-column-id").value = columnId;
    document.getElementById("card-title").value = "";
    document.getElementById("card-desc").value = "";

    var cpEl = document.getElementById("card-colorpicker");
    addColorPicker = Goop.ui.colorpicker(cpEl, {
      colors: CARD_COLORS,
      value: CARD_COLORS[0],
      swatch: ".color-swatch",
      popup: ".color-popup",
      openClass: "open",
      selectedAttr: "data-selected",
      colorVar: "--gc-color",
      buttonClass: "color-btn",
    });

    addCardModal.classList.remove("hidden");
    document.getElementById("card-title").focus();
  }

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
    var color = addColorPicker ? addColorPicker.getValue() : "";

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
      toast({ title: "Error", message: e.message || "Failed to add card" });
    }
  };

  // --- Edit card modal ---

  var editCardModal = document.getElementById("edit-card-modal");
  var editingCardId = null;
  var editColorPicker = null;

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

    var cpEl = document.getElementById("edit-colorpicker");
    editColorPicker = Goop.ui.colorpicker(cpEl, {
      colors: CARD_COLORS,
      value: card.color || CARD_COLORS[0],
      swatch: ".color-swatch",
      popup: ".color-popup",
      openClass: "open",
      selectedAttr: "data-selected",
      colorVar: "--gc-color",
      buttonClass: "color-btn",
    });

    editCardModal.classList.remove("hidden");
    document.getElementById("edit-card-title").focus();
  }

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
    var color = editColorPicker ? editColorPicker.getValue() : "";

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
      toast({ title: "Error", message: e.message || "Failed to update card" });
    }
  };

  document.getElementById("delete-card").onclick = async function () {
    var cardId = parseInt(document.getElementById("edit-card-id").value);

    if (!(await Goop.ui.confirm("Delete this card?"))) return;

    try {
      await db.call("kanban", {
        action: "delete_card",
        card_id: cardId
      });
      editCardModal.classList.add("hidden");
      loadBoard();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to delete card" });
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
      toast({ title: "Error", message: e.message || "Failed to move card" });
    }
  }

  async function deleteColumn(columnId) {
    if (!(await Goop.ui.confirm("Delete this column?"))) return;
    try {
      var res = await db.call("kanban", { action: "delete_column", column_id: columnId });
      if (res.error) throw new Error(res.error);
      loadBoard();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to delete column" });
    }
  }

  async function renameColumn(columnId, currentName) {
    var name = await Goop.ui.prompt({ title: "Rename Column", message: "Column name:", value: currentName });
    if (!name || !name.trim() || name.trim() === currentName) return;
    try {
      var res = await db.call("kanban", { action: "update_column", column_id: columnId, name: name.trim() });
      if (res.error) throw new Error(res.error);
      loadBoard();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to rename column" });
    }
  }

  async function moveColumn(columnId, direction) {
    try {
      var res = await db.call("kanban", { action: "move_column", column_id: columnId, direction: direction });
      if (res.error) throw new Error(res.error);
      loadBoard();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to move column" });
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
      toast({ title: "Error", message: e.message || "Failed to add column" });
    }
  }

})();
