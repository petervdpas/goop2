// Tic-Tac-Toe app.js — PvP and PvE with server-side validation via Lua
(async function () {
  var db = Goop.data;
  var root = document.getElementById("ttt-root");
  var subtitle = document.getElementById("subtitle");
  var isOwner = false;
  var myId = await Goop.identity.id();
  var myLabel = await Goop.identity.label();
  var pollTimer = null;
  var currentGameId = null;

  // Detect owner vs visitor
  var match = window.location.pathname.match(/\/p\/([^/]+)/);
  if (!match || match[1] === myId) {
    isOwner = true;
  }

  subtitle.textContent = isOwner
    ? "Challenge visitors or play against the computer."
    : "Challenge the host or play against the computer.";

  showLobby();

  // ── Polling ──

  function startPolling(gameId) {
    stopPolling();
    pollTimer = setInterval(async function () {
      try {
        var state = await db.call("ttt", { action: "state", game_id: gameId });
        renderBoard(state);
        if (state.status !== "playing" && state.status !== "waiting") {
          stopPolling();
        }
      } catch (e) {
        // ignore transient errors
      }
    }, 2000);
  }

  function stopPolling() {
    if (pollTimer) {
      clearInterval(pollTimer);
      pollTimer = null;
    }
  }

  // ── Lobby ──

  async function showLobby() {
    stopPolling();
    currentGameId = null;

    var lobby;
    try {
      // Single call returns both games list and stats
      lobby = await db.call("ttt", { action: "lobby" });
    } catch (e) {
      root.innerHTML = '<p class="ttt-empty">Could not load lobby.</p>';
      return;
    }

    var games = lobby.games || [];
    var stats = lobby.stats || {};

    // Check if I have an active game — render directly from lobby data
    for (var i = 0; i < games.length; i++) {
      var g = games[i];
      if ((g.status === "playing" || g.status === "waiting") &&
          (g._owner === myId || g.challenger === myId)) {
        showGame(g._id);
        return;
      }
    }

    var html = '<div class="ttt-lobby">';

    // Action buttons
    html += '<div class="ttt-actions">';
    if (!isOwner) {
      html += '<button class="btn btn-primary" id="btn-challenge">Challenge Host</button>';
    }
    html += '<button class="btn ' + (isOwner ? 'btn-primary' : 'btn-secondary') + '" id="btn-pve">Play vs Computer</button>';
    html += '</div>';

    // Stats
    html += '<div class="ttt-stats">';
    html += '<span><span class="stat-val">' + (stats.wins || 0) + '</span> wins</span>';
    html += '<span><span class="stat-val">' + (stats.losses || 0) + '</span> losses</span>';
    html += '<span><span class="stat-val">' + (stats.draws || 0) + '</span> draws</span>';
    html += '</div>';

    // Pending challenges (owner only)
    if (isOwner) {
      var pending = [];
      for (var j = 0; j < games.length; j++) {
        if (games[j].status === "waiting" && games[j].mode === "pvp") {
          pending.push(games[j]);
        }
      }
      if (pending.length > 0) {
        html += '<div class="ttt-panel">';
        html += '<h2>Pending Challenges</h2>';
        html += '<ul class="ttt-challenges">';
        for (var k = 0; k < pending.length; k++) {
          var pg = pending[k];
          var label = esc(pg.challenger_label || pg.challenger.substring(0, 12) + '...');
          html += '<li class="ttt-challenge-item">';
          html += '<span><span class="name">' + label + '</span>';
          html += '<span class="time">' + timeAgo(pg._created_at) + '</span></span>';
          html += '<button class="btn-sm" data-accept="' + pg._id + '">Play</button>';
          html += '</li>';
        }
        html += '</ul></div>';
      }
    }

    // Recent games
    var finished = [];
    for (var m = 0; m < games.length; m++) {
      var fg = games[m];
      if (fg.status !== "waiting" && fg.status !== "playing") {
        finished.push(fg);
      }
    }

    if (finished.length > 0) {
      html += '<div class="ttt-panel">';
      html += '<h2>Recent Games</h2>';
      html += '<table class="ttt-history"><thead><tr>';
      html += '<th>Opponent</th><th>Result</th><th>Mode</th><th></th>';
      html += '</tr></thead><tbody>';
      for (var n = 0; n < finished.length && n < 10; n++) {
        var hg = finished[n];
        var opponent = hg.mode === "pve" ? "Computer"
          : esc(hg.challenger_label || hg.challenger.substring(0, 12) + '...');
        var resultCls = "", resultTxt = "";
        if (hg.status === "draw" || hg.status === "cancelled") {
          resultCls = "result-draw";
          resultTxt = hg.status === "cancelled" ? "cancelled" : "draw";
        } else if (hg.winner === myId) {
          resultCls = "result-win";
          resultTxt = "won";
        } else {
          resultCls = "result-loss";
          resultTxt = "lost";
        }
        html += '<tr>';
        html += '<td>' + opponent + '</td>';
        html += '<td class="' + resultCls + '">' + resultTxt + '</td>';
        html += '<td>' + (hg.mode === "pve" ? "vs AI" : "PvP") + '</td>';
        html += '<td>' + timeAgo(hg._created_at) + '</td>';
        html += '</tr>';
      }
      html += '</tbody></table></div>';
    } else {
      html += '<div class="ttt-panel"><p class="ttt-empty">No games played yet.</p></div>';
    }

    html += '</div>';
    root.innerHTML = html;

    // Button handlers
    var btnPve = document.getElementById("btn-pve");
    if (btnPve) {
      btnPve.onclick = async function () {
        btnPve.disabled = true;
        try {
          var result = await db.call("ttt", { action: "new_pve" });
          if (result.error) {
            Goop.ui.toast(result.error);
            btnPve.disabled = false;
          } else {
            // Render directly from the new_pve response — no extra game_state call
            currentGameId = result.game_id;
            renderBoard(result);
          }
        } catch (e) {
          Goop.ui.toast(e.message || "Error starting game.");
          btnPve.disabled = false;
        }
      };
    }

    var btnChallenge = document.getElementById("btn-challenge");
    if (btnChallenge) {
      btnChallenge.onclick = async function () {
        btnChallenge.disabled = true;
        try {
          var result = await db.call("ttt", { action: "new" });
          if (result.error) {
            Goop.ui.toast(result.error);
            if (result.game_id) {
              showGame(result.game_id);
              return;
            }
            btnChallenge.disabled = false;
          } else {
            // For PvP, we need to poll for the host's acceptance
            showGame(result.game_id);
          }
        } catch (e) {
          Goop.ui.toast(e.message || "Error creating challenge.");
          btnChallenge.disabled = false;
        }
      };
    }

    // Accept handlers
    root.querySelectorAll("[data-accept]").forEach(function (btn) {
      btn.onclick = function () {
        var gid = parseInt(btn.getAttribute("data-accept"));
        showGame(gid);
      };
    });
  }

  // ── Game view ──

  async function showGame(gameId) {
    stopPolling();
    currentGameId = gameId;

    try {
      var state = await db.call("ttt", { action: "state", game_id: gameId });
      renderBoard(state);

      // Poll for PvP games in progress or waiting
      if (state.mode === "pvp" && (state.status === "playing" || state.status === "waiting")) {
        startPolling(gameId);
      }
    } catch (e) {
      root.innerHTML = '<p class="ttt-empty">Could not load game.</p>';
    }
  }

  function renderBoard(state) {
    var board = state.board || "---------";
    var yourSymbol = state.your_symbol;
    var isYourTurn = (state.status === "playing" && state.turn === yourSymbol);
    var gameOver = (state.status !== "playing" && state.status !== "waiting");
    var winCells = {};

    if (state.win_line) {
      for (var w = 0; w < state.win_line.length; w++) {
        winCells[state.win_line[w]] = true;
      }
    }

    var html = '<div class="ttt-game">';

    // Status line
    if (state.status === "waiting") {
      html += '<div class="ttt-waiting">';
      html += '<div class="spinner"></div>';
      html += '<p>Waiting for host to make a move&hellip;</p>';
      html += '<button class="btn btn-secondary btn-sm" id="btn-cancel">Cancel</button>';
      html += '</div>';
      root.innerHTML = html;
      var btnCancel = document.getElementById("btn-cancel");
      if (btnCancel) {
        btnCancel.onclick = async function () {
          await db.call("ttt", { action: "cancel", game_id: state.game_id });
          showLobby();
        };
      }
      return;
    }

    // Result banner
    if (gameOver) {
      var resultCls = "draw";
      var resultMsg = "Draw!";
      if (state.status === "cancelled") {
        resultMsg = "Cancelled";
      } else if (state.winner === myId) {
        resultCls = "win";
        resultMsg = "You won!";
      } else if (state.winner && state.winner !== myId) {
        resultCls = "loss";
        var opponentName = (state.mode === "pve") ? "Computer" : "Opponent";
        resultMsg = opponentName + " won!";
      }
      html += '<div class="ttt-result ' + resultCls + '">' + resultMsg + '</div>';
    } else {
      // Turn indicator
      var opLabel = (state.mode === "pve") ? "Computer" : "Opponent";
      var turnLabel = isYourTurn ? "Your turn" : opLabel + "'s turn";
      html += '<div class="ttt-status">';
      html += 'You: <span class="' + (yourSymbol === "X" ? "symbol-x" : "symbol-o") + '">' + symbolChar(yourSymbol) + '</span>';
      html += ' &mdash; ' + turnLabel;
      html += '</div>';
    }

    // Board
    html += '<div class="ttt-board">';
    for (var i = 0; i < 9; i++) {
      var ch = board.charAt(i);
      var cls = "ttt-cell";
      var content = "";

      if (ch === "X") {
        cls += " taken x";
        content = symbolChar("X");
      } else if (ch === "O") {
        cls += " taken o";
        content = symbolChar("O");
      }

      if (winCells[i]) {
        cls += " win-cell";
      }

      if (gameOver || !isYourTurn || ch !== "-") {
        cls += " disabled";
      }

      html += '<div class="' + cls + '" data-pos="' + i + '">' + content + '</div>';
    }
    html += '</div>';

    // Actions
    if (gameOver) {
      html += '<div class="ttt-game-actions">';
      if (state.mode === "pve") {
        html += '<button class="btn btn-primary" id="btn-rematch-pve">Play Again</button>';
      }
      html += '<button class="btn btn-secondary" id="btn-back">Back to Lobby</button>';
      html += '</div>';
    }

    html += '</div>';
    root.innerHTML = html;

    // Click handlers for cells
    if (!gameOver && isYourTurn) {
      root.querySelectorAll(".ttt-cell:not(.taken):not(.disabled)").forEach(function (cell) {
        cell.onclick = async function () {
          var pos = parseInt(cell.getAttribute("data-pos"));
          cell.classList.add("disabled");

          try {
            var gameId = state.game_id || currentGameId;
            var result = await db.call("move", {
              game_id: gameId,
              position: pos
            });
            if (result.error) {
              Goop.ui.toast(result.error);
              renderBoard(state); // re-render previous state
              return;
            }
            if (!result.game_id) result.game_id = gameId;
            if (!result.challenger_label) result.challenger_label = state.challenger_label;
            renderBoard(result);

            // Start polling for opponent's move in PvP
            if (result.mode === "pvp" && result.status === "playing") {
              startPolling(result.game_id);
            }
          } catch (e) {
            Goop.ui.toast(e.message || "Error making move.");
            renderBoard(state);
          }
        };
      });
    }

    // Game-over button handlers
    var btnRematchPve = document.getElementById("btn-rematch-pve");
    if (btnRematchPve) {
      btnRematchPve.onclick = async function () {
        btnRematchPve.disabled = true;
        try {
          var result = await db.call("ttt", { action: "new_pve" });
          if (result.error) {
            Goop.ui.toast(result.error);
            btnRematchPve.disabled = false;
          } else {
            // Render directly — no extra game_state call needed
            currentGameId = result.game_id;
            renderBoard(result);
          }
        } catch (e) {
          Goop.ui.toast(e.message || "Error starting game.");
          btnRematchPve.disabled = false;
        }
      };
    }

    var btnBack = document.getElementById("btn-back");
    if (btnBack) {
      btnBack.onclick = function () { showLobby(); };
    }
  }

  // ── Helpers ──

  function symbolChar(s) {
    if (s === "X") return "\u2715";
    if (s === "O") return "\u25CB";
    return "";
  }

  function timeAgo(ts) {
    if (!ts) return "";
    var now = new Date();
    var then = new Date(ts.replace(" ", "T") + "Z");
    var diff = Math.floor((now - then) / 1000);
    if (diff < 60) return "just now";
    if (diff < 3600) return Math.floor(diff / 60) + "m ago";
    if (diff < 86400) return Math.floor(diff / 3600) + "h ago";
    return Math.floor(diff / 86400) + "d ago";
  }

  function esc(s) {
    if (!s) return "";
    var d = document.createElement("div");
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }
})();
