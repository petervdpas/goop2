// Kanban app.js
(async function () {
  var db = Goop.data;
  var root = document.getElementById("kanban-root");
  var joinSection = document.getElementById("kanban-join");
  var subtitle = document.getElementById("subtitle");
  var isOwner = false;
  var myId = await Goop.identity.id();
  var myLabel = await Goop.identity.label();
  var groupConnected = false;
  var columns = [];

  // Detect owner vs visitor
  var match = window.location.pathname.match(/\/p\/([^/]+)/);
  if (!match || match[1] === myId) {
    isOwner = true;
  }

  // For owner, go directly to the board
  if (isOwner) {
    subtitle.textContent = "Shared team kanban board";
    joinSection.style.display = "none";
    root.style.display = "block";
    loadBoard();
  } else {
    // Visitor needs to join the group
    subtitle.textContent = "Join to collaborate on this board";
    joinSection.style.display = "flex";
    root.style.display = "none";

    document.getElementById("btn-join").onclick = async function () {
      this.disabled = true;
      this.textContent = "Joining...";
      await joinGroup();
    };
  }

  async function joinGroup() {
    try {
      // Try to join the kanban group
      if (Goop.group && Goop.group.join) {
        Goop.group.join("kanban", handleGroupMessage);
        groupConnected = true;
      }

      joinSection.style.display = "none";
      root.style.display = "block";
      subtitle.innerHTML = 'Shared team kanban board <span class="connection-status"><span class="status-dot"></span>connected</span>';
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: "Failed to join: " + (e.message || e) });
      document.getElementById("btn-join").disabled = false;
      document.getElementById("btn-join").textContent = "Join Board";
    }
  }

  function handleGroupMessage(msg) {
    // Real-time updates from other users
    if (msg.type === "board_update") {
      loadBoard();
    }
  }

  function broadcastUpdate() {
    if (groupConnected && Goop.group && Goop.group.send) {
      Goop.group.send({ type: "board_update" });
    }
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
      var cards = col.cards || [];

      html += '<div class="column" data-column-id="' + col._id + '">';
      html += '<div class="column-header">';
      html += '<span class="column-title">';
      html += '<span class="column-dot" style="background:' + esc(col.color || '#5b6abf') + '"></span>';
      html += esc(col.name);
      html += '</span>';
      html += '<span class="column-count">' + cards.length + '</span>';
      html += '</div>';

      html += '<div class="column-cards">';
      if (cards.length === 0) {
        html += '<div class="empty-column">No cards</div>';
      } else {
        for (var j = 0; j < cards.length; j++) {
          var card = cards[j];
          html += renderCard(card, col._id);
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
        // Close other dropdowns
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

    html += '</div>';
    return html;
  }

  // Add card modal
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
      if (btn.getAttribute("data-color") === "") {
        btn.classList.add("selected");
      }
    });

    addCardModal.classList.remove("hidden");
    document.getElementById("card-title").focus();
  }

  addCardModal.querySelectorAll(".color-btn").forEach(function (btn) {
    btn.onclick = function () {
      addCardModal.querySelectorAll(".color-btn").forEach(function (b) {
        b.classList.remove("selected");
      });
      btn.classList.add("selected");
      selectedColor = btn.getAttribute("data-color");
      document.getElementById("card-color-value").value = selectedColor;
    };
  });

  document.getElementById("cancel-card").onclick = function () {
    addCardModal.classList.add("hidden");
  };

  addCardModal.onclick = function (e) {
    if (e.target === addCardModal) {
      addCardModal.classList.add("hidden");
    }
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
        color: color
      });
      addCardModal.classList.add("hidden");
      broadcastUpdate();
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to add card" });
    }
  };

  // Edit card modal
  var editCardModal = document.getElementById("edit-card-modal");
  var editingCardId = null;

  function showEditCardModal(cardId) {
    editingCardId = parseInt(cardId);

    // Find the card data
    var card = null;
    for (var i = 0; i < columns.length; i++) {
      var cards = columns[i].cards || [];
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
      if (btn.getAttribute("data-color") === (card.color || "")) {
        btn.classList.add("selected");
      }
    });

    editCardModal.classList.remove("hidden");
    document.getElementById("edit-card-title").focus();
  }

  document.getElementById("edit-color-picker").querySelectorAll(".color-btn").forEach(function (btn) {
    btn.onclick = function () {
      document.getElementById("edit-color-picker").querySelectorAll(".color-btn").forEach(function (b) {
        b.classList.remove("selected");
      });
      btn.classList.add("selected");
      document.getElementById("edit-card-color-value").value = btn.getAttribute("data-color");
    };
  });

  document.getElementById("cancel-edit-card").onclick = function () {
    editCardModal.classList.add("hidden");
  };

  editCardModal.onclick = function (e) {
    if (e.target === editCardModal) {
      editCardModal.classList.add("hidden");
    }
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
      broadcastUpdate();
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
      broadcastUpdate();
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
        to_column: toColumn
      });
      broadcastUpdate();
      loadBoard();
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Failed to move card" });
    }
  }

  async function addColumn(name) {
    try {
      await db.call("kanban", {
        action: "add_column",
        name: name
      });
      broadcastUpdate();
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
