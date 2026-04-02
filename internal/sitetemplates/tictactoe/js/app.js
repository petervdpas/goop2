// Tic-Tac-Toe app.js — PvP and PvE with server-side validation via Lua
(async function () {
  var toast = Goop.ui.toast(document.getElementById("toasts"), {
    toastClass: "gc-toast",
    titleClass: "gc-toast-title",
    messageClass: "gc-toast-message",
    enterClass: "gc-toast-enter",
    exitClass: "gc-toast-exit",
  });

  var h = Goop.dom;
  var db = Goop.data;
  var root = document.getElementById("ttt-root");
  var subtitle = document.getElementById("subtitle");
  var isOwner = false;
  var myId = await Goop.identity.id();
  var myLabel = await Goop.identity.label();
  var pollTimer = null;
  var currentGameId = null;

  var match = window.location.pathname.match(/\/p\/([^/]+)/);
  if (!match || match[1] === myId) isOwner = true;

  subtitle.textContent = isOwner
    ? "Challenge visitors or play against the computer."
    : "Challenge the host or play against the computer.";

  showLobby();

  function startPolling(gameId) {
    stopPolling();
    pollTimer = setInterval(async function () {
      try {
        var state = await db.call("ttt", { action: "state", game_id: gameId });
        renderBoard(state);
        if (state.status !== "playing" && state.status !== "waiting") stopPolling();
      } catch (e) {}
    }, 2000);
  }

  function stopPolling() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null; } }

  async function showLobby() {
    stopPolling();
    currentGameId = null;

    var lobby;
    try { lobby = await db.call("ttt", { action: "lobby" }); }
    catch (e) { Goop.render(root, Goop.ui.empty("Could not load lobby.")); return; }

    var games = lobby.games || [];
    var stats = lobby.stats || {};

    for (var i = 0; i < games.length; i++) {
      var g = games[i];
      if (isOwner && g.status === "waiting" && g.mode === "pvp") continue;
      if ((g.status === "playing" || g.status === "waiting") && (g._owner === myId || g.challenger === myId)) {
        showGame(g._id);
        return;
      }
    }

    var pending = isOwner ? games.filter(function(g) { return g.status === "waiting" && g.mode === "pvp"; }) : [];
    var finished = games.filter(function(g) { return g.status !== "waiting" && g.status !== "playing"; }).slice(0, 10);

    Goop.render(root, h("div", { class: "ttt-lobby" },
      h("div", { class: "ttt-actions" },
        !isOwner ? h("button", { class: "btn btn-primary", onclick: async function() {
          this.disabled = true;
          try {
            var result = await db.call("ttt", { action: "new" });
            if (result.error) { toast(result.error); if (result.game_id) { showGame(result.game_id); return; } this.disabled = false; }
            else showGame(result.game_id);
          } catch (e) { toast(e.message); this.disabled = false; }
        } }, "Challenge Host") : null,
        h("button", { class: "btn " + (isOwner ? "btn-primary" : "btn-secondary"), onclick: async function() {
          this.disabled = true;
          try {
            var result = await db.call("ttt", { action: "new_pve" });
            if (result.error) { toast(result.error); this.disabled = false; }
            else { currentGameId = result.game_id; renderBoard(result); }
          } catch (e) { toast(e.message); this.disabled = false; }
        } }, "Play vs Computer")
      ),

      h("div", { class: "ttt-stats" },
        h("span", {}, h("span", { class: "stat-val" }, String(stats.wins || 0)), " wins"),
        h("span", {}, h("span", { class: "stat-val" }, String(stats.losses || 0)), " losses"),
        h("span", {}, h("span", { class: "stat-val" }, String(stats.draws || 0)), " draws")
      ),

      pending.length > 0 ? h("div", { class: "ttt-panel" },
        h("h2", {}, "Pending Challenges"),
        h("ul", { class: "ttt-challenges" }, pending.map(function(pg) {
          return h("li", { class: "ttt-challenge-item" },
            h("span", {}, h("span", { class: "name" }, pg.challenger_label || pg.challenger.substring(0, 12) + "..."), h("span", { class: "time" }, Goop.ui.time(pg._created_at))),
            h("button", { class: "btn-sm", onclick: async function() {
              this.disabled = true;
              try { await db.call("ttt", { action: "accept", game_id: pg._id }); } catch (e) {}
              showGame(pg._id);
            } }, "Play")
          );
        }))
      ) : null,

      h("div", { class: "ttt-panel" },
        h("h2", {}, "Recent Games"),
        finished.length === 0
          ? h("p", { class: "ttt-empty" }, "No games played yet.")
          : h("table", { class: "ttt-history" },
              h("thead", {}, h("tr", {}, h("th", {}, "Opponent"), h("th", {}, "Result"), h("th", {}, "Mode"), h("th", {}))),
              h("tbody", {}, finished.map(function(fg) {
                var opponent = fg.mode === "pve" ? "Computer" : (fg.challenger_label || fg.challenger.substring(0, 12) + "...");
                var result = fg.status === "draw" || fg.status === "cancelled" ? (fg.status === "cancelled" ? "cancelled" : "draw") : (fg.winner === myId ? "won" : "lost");
                var resultCls = fg.winner === myId ? "result-win" : (result === "draw" || result === "cancelled" ? "result-draw" : "result-loss");
                return h("tr", {},
                  h("td", {}, opponent),
                  h("td", { class: resultCls }, result),
                  h("td", {}, fg.mode === "pve" ? "vs AI" : "PvP"),
                  h("td", {}, Goop.ui.time(fg._created_at))
                );
              }))
            )
      )
    ));
  }

  async function showGame(gameId) {
    stopPolling();
    currentGameId = gameId;
    try {
      var state = await db.call("ttt", { action: "state", game_id: gameId });
      renderBoard(state);
      if (state.mode === "pvp" && (state.status === "playing" || state.status === "waiting")) startPolling(gameId);
    } catch (e) { Goop.render(root, Goop.ui.empty("Could not load game.")); }
  }

  function symbolChar(s) { return s === "X" ? "\u2715" : s === "O" ? "\u25CB" : ""; }

  function renderBoard(state) {
    var board = state.board || "---------";
    var yourSymbol = state.your_symbol;
    var isYourTurn = (state.status === "playing" && state.turn === yourSymbol);
    var gameOver = (state.status !== "playing" && state.status !== "waiting");
    var winCells = {};
    if (state.win_line) state.win_line.forEach(function(w) { winCells[w] = true; });

    if (state.status === "waiting") {
      Goop.render(root, h("div", { class: "ttt-game" },
        h("div", { class: "ttt-waiting" },
          h("div", { class: "spinner" }),
          h("p", {}, "Waiting for host to accept\u2026"),
          h("button", { class: "btn btn-secondary btn-sm", onclick: async function() { await db.call("ttt", { action: "cancel", game_id: state.game_id }); showLobby(); } }, "Cancel")
        )
      ));
      return;
    }

    var statusEl;
    if (gameOver) {
      var cls = "draw", msg = "Draw!";
      if (state.status === "cancelled") msg = "Cancelled";
      else if (state.winner === myId) { cls = "win"; msg = "You won!"; }
      else if (state.winner) { cls = "loss"; msg = (state.mode === "pve" ? "Computer" : "Opponent") + " won!"; }
      statusEl = h("div", { class: "ttt-result " + cls }, msg);
    } else {
      var opLabel = state.mode === "pve" ? "Computer" : "Opponent";
      statusEl = h("div", { class: "ttt-status" },
        "You: ", h("span", { class: yourSymbol === "X" ? "symbol-x" : "symbol-o" }, symbolChar(yourSymbol)),
        " \u2014 " + (isYourTurn ? "Your turn" : opLabel + "'s turn")
      );
    }

    var cells = [];
    for (var i = 0; i < 9; i++) {
      var ch = board.charAt(i);
      var cls = "ttt-cell";
      if (ch === "X") cls += " taken x";
      else if (ch === "O") cls += " taken o";
      if (winCells[i]) cls += " win-cell";
      if (gameOver || !isYourTurn || ch !== "-") cls += " disabled";

      cells.push(h("div", {
        class: cls,
        data: { pos: i },
        onclick: (!gameOver && isYourTurn && ch === "-") ? (function(pos) {
          return async function() {
            this.classList.add("disabled");
            try {
              var gameId = state.game_id || currentGameId;
              var result = await db.call("move", { game_id: gameId, position: pos });
              if (result.error) { toast(result.error); renderBoard(state); return; }
              if (!result.game_id) result.game_id = gameId;
              if (!result.challenger_label) result.challenger_label = state.challenger_label;
              renderBoard(result);
              if (result.mode === "pvp" && result.status === "playing") startPolling(result.game_id);
            } catch (e) { toast(e.message); renderBoard(state); }
          };
        })(i) : null,
      }, ch !== "-" ? symbolChar(ch) : ""));
    }

    Goop.render(root, h("div", { class: "ttt-game" },
      statusEl,
      h("div", { class: "ttt-board" }, cells),
      gameOver ? h("div", { class: "ttt-game-actions" },
        state.mode === "pve" ? h("button", { class: "btn btn-primary", onclick: async function() {
          this.disabled = true;
          try {
            var result = await db.call("ttt", { action: "new_pve" });
            if (result.error) { toast(result.error); this.disabled = false; }
            else { currentGameId = result.game_id; renderBoard(result); }
          } catch (e) { toast(e.message); this.disabled = false; }
        } }, "Play Again") : null,
        h("button", { class: "btn btn-secondary", onclick: function() { showLobby(); } }, "Back to Lobby")
      ) : null
    ));
  }
})();
