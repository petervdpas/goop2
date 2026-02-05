// Chess app.js
(async function () {
  var db = Goop.data;
  var root = document.getElementById("chess-root");
  var subtitle = document.getElementById("subtitle");
  var isOwner = false;
  var myId = await Goop.identity.id();
  var pollTimer = null;
  var currentGameId = null;
  var selectedSquare = null;
  var legalMoves = [];
  var pendingPromotion = null;

  // Unicode chess pieces
  var PIECES = {
    K: "\u2654", Q: "\u2655", R: "\u2656", B: "\u2657", N: "\u2658", P: "\u2659",
    k: "\u265A", q: "\u265B", r: "\u265C", b: "\u265D", n: "\u265E", p: "\u265F"
  };

  // Detect owner vs visitor
  var match = window.location.pathname.match(/\/p\/([^/]+)/);
  if (!match || match[1] === myId) {
    isOwner = true;
  }

  subtitle.textContent = "Find an opponent or play against the computer.";

  showLobby();

  // -- Polling --
  function startPolling(gameId) {
    stopPolling();
    pollTimer = setInterval(async function () {
      try {
        var state = await db.call("chess", { action: "state", game_id: gameId });
        if (state.status !== "playing" && state.status !== "waiting") {
          stopPolling();
        }
        renderGame(state);
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

  // -- Lobby --
  async function showLobby() {
    stopPolling();
    currentGameId = null;
    selectedSquare = null;
    legalMoves = [];

    var lobby;
    try {
      lobby = await db.call("chess", { action: "lobby" });
    } catch (e) {
      root.innerHTML = '<p class="chess-empty">Could not load lobby.</p>';
      return;
    }

    var games = lobby.games || [];
    var waiting = lobby.waiting || [];
    var stats = lobby.stats || {};

    // Check if I have an active game
    for (var i = 0; i < games.length; i++) {
      var g = games[i];
      if ((g.status === "playing" || g.status === "waiting") &&
          (g._owner === myId || g.challenger === myId)) {
        showGame(g._id);
        return;
      }
    }

    var html = '<div class="chess-lobby">';

    // Action buttons
    html += '<div class="chess-actions">';
    html += '<button class="btn btn-primary" id="btn-find">Find Opponent</button>';
    html += '<button class="btn btn-secondary" id="btn-pve">Play vs Computer</button>';
    html += '</div>';

    // Stats
    html += '<div class="chess-stats">';
    html += '<span><span class="stat-val">' + (stats.wins || 0) + '</span> wins</span>';
    html += '<span><span class="stat-val">' + (stats.losses || 0) + '</span> losses</span>';
    html += '<span><span class="stat-val">' + (stats.draws || 0) + '</span> draws</span>';
    html += '</div>';

    // Waiting players (others looking for a game)
    if (waiting.length > 0) {
      html += '<div class="chess-panel">';
      html += '<h2>Players Looking for Game</h2>';
      html += '<ul class="chess-challenges">';
      for (var k = 0; k < waiting.length; k++) {
        var wg = waiting[k];
        var label = esc(wg._owner_label || 'Anonymous');
        html += '<li class="chess-challenge-item">';
        html += '<span><span class="name">' + label + '</span>';
        html += '<span class="time">' + timeAgo(wg._created_at) + '</span></span>';
        html += '<button class="btn-sm btn-join" data-join="' + wg._id + '">Play</button>';
        html += '</li>';
      }
      html += '</ul></div>';
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
      html += '<div class="chess-panel">';
      html += '<h2>Recent Games</h2>';
      html += '<table class="chess-history"><thead><tr>';
      html += '<th>Opponent</th><th>Result</th><th>Mode</th><th></th>';
      html += '</tr></thead><tbody>';
      for (var n = 0; n < finished.length && n < 10; n++) {
        var hg = finished[n];
        var opponent = hg.mode === "pve" ? "Computer"
          : esc(hg.challenger_label || hg.challenger.substring(0, 12) + '...');
        var resultCls = "", resultTxt = "";
        if (hg.status === "draw") {
          resultCls = "result-draw";
          resultTxt = "draw";
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
      html += '<div class="chess-panel"><p class="chess-empty">No games played yet.</p></div>';
    }

    html += '</div>';
    root.innerHTML = html;

    // Button handlers
    var btnPve = document.getElementById("btn-pve");
    if (btnPve) {
      btnPve.onclick = async function () {
        btnPve.disabled = true;
        try {
          var result = await db.call("chess", { action: "new_pve" });
          if (result.error) {
            Goop.ui.toast({ title: "Error", message: result.error });
            btnPve.disabled = false;
          } else {
            currentGameId = result.game_id;
            renderGame(result);
          }
        } catch (e) {
          Goop.ui.toast({ title: "Error", message: e.message || "Error starting game." });
          btnPve.disabled = false;
        }
      };
    }

    var btnFind = document.getElementById("btn-find");
    if (btnFind) {
      btnFind.onclick = async function () {
        btnFind.disabled = true;
        try {
          var result = await db.call("chess", { action: "wait_for_game" });
          if (result.error) {
            Goop.ui.toast({ title: "Error", message: result.error });
            if (result.game_id) {
              showGame(result.game_id);
              return;
            }
            btnFind.disabled = false;
          } else {
            showGame(result.game_id);
          }
        } catch (e) {
          Goop.ui.toast({ title: "Error", message: e.message || "Error finding game." });
          btnFind.disabled = false;
        }
      };
    }

    // Join waiting game handlers
    root.querySelectorAll("[data-join]").forEach(function (btn) {
      btn.onclick = async function () {
        btn.disabled = true;
        var gid = parseInt(btn.getAttribute("data-join"));
        try {
          var result = await db.call("chess", { action: "join_game", game_id: gid });
          if (result.error) {
            Goop.ui.toast({ title: "Error", message: result.error });
            if (result.game_id) {
              showGame(result.game_id);
              return;
            }
            btn.disabled = false;
          } else {
            showGame(result.game_id);
          }
        } catch (e) {
          Goop.ui.toast({ title: "Error", message: e.message || "Error joining game." });
          btn.disabled = false;
        }
      };
    });
  }

  // -- Game view --
  async function showGame(gameId) {
    stopPolling();
    currentGameId = gameId;
    selectedSquare = null;
    legalMoves = [];

    try {
      var state = await db.call("chess", { action: "state", game_id: gameId });
      renderGame(state);

      if (state.mode === "pvp" && (state.status === "playing" || state.status === "waiting")) {
        startPolling(gameId);
      }
    } catch (e) {
      root.innerHTML = '<p class="chess-empty">Could not load game.</p>';
    }
  }

  function renderGame(state) {
    var fen = state.fen || "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1";
    var board = parseFen(fen);
    var yourColor = state.your_color;
    var turn = state.turn || "w";
    var isYourTurn = (state.status === "playing" && turn === yourColor);
    var gameOver = (state.status !== "playing" && state.status !== "waiting");
    var inCheck = state.in_check;

    var html = '<div class="chess-game">';

    // Waiting state
    if (state.status === "waiting") {
      html += '<div class="chess-waiting">';
      html += '<div class="spinner"></div>';
      html += '<p>Waiting for opponent&hellip;</p>';
      html += '<button class="btn-cancel" id="btn-cancel">Cancel</button>';
      html += '</div>';
      root.innerHTML = html;
      var btnCancel = document.getElementById("btn-cancel");
      if (btnCancel) {
        btnCancel.onclick = async function () {
          await db.call("chess", { action: "resign", game_id: state.game_id });
          showLobby();
        };
      }
      return;
    }

    // Result banner
    if (gameOver) {
      var resultCls = "draw";
      var resultMsg = "Draw!";
      if (state.winner === myId) {
        resultCls = "win";
        resultMsg = "You won!";
      } else if (state.winner && state.winner !== myId) {
        resultCls = "loss";
        var opponentName = (state.mode === "pve") ? "Computer" : "Opponent";
        resultMsg = opponentName + " won!";
      }
      html += '<div class="chess-result ' + resultCls + '">' + resultMsg + '</div>';
    } else {
      // Turn indicator
      var opLabel = (state.mode === "pve") ? "Computer" : "Opponent";
      var turnLabel = isYourTurn ? "Your turn" : opLabel + "'s turn";
      var colorLabel = yourColor === "w" ? "White" : "Black";
      html += '<div class="chess-status">';
      html += 'You: ' + colorLabel + ' &mdash; ' + turnLabel;
      if (inCheck) html += ' <strong style="color:#dc2626">(Check!)</strong>';
      html += '</div>';
    }

    // Board - render from white's perspective if playing white, otherwise flip
    var flipped = (yourColor === "b");
    html += '<div class="chess-board">';

    for (var row = 0; row < 8; row++) {
      for (var col = 0; col < 8; col++) {
        var rank = flipped ? (row + 1) : (8 - row);
        var file = flipped ? (8 - col) : (col + 1);
        var sq = String.fromCharCode(96 + file) + rank;
        var piece = board[sq];
        var isLight = (rank + file) % 2 === 0;
        var cls = "chess-cell " + (isLight ? "light" : "dark");

        if (sq === selectedSquare) cls += " selected";
        if (legalMoves.indexOf(sq) !== -1) {
          cls += piece ? " legal-capture" : " legal-move";
        }

        // Highlight king in check
        if (inCheck && piece && piece.toLowerCase() === "k" && pieceColor(piece) === turn) {
          cls += " in-check";
        }

        var pieceHtml = "";
        if (piece) {
          var pColor = pieceColor(piece) === "w" ? "white" : "black";
          pieceHtml = '<span class="piece ' + pColor + '">' + PIECES[piece] + '</span>';
        }

        html += '<div class="' + cls + '" data-sq="' + sq + '">' + pieceHtml + '</div>';
      }
    }
    html += '</div>';

    // Actions
    if (gameOver) {
      html += '<div class="chess-game-actions">';
      if (state.mode === "pve") {
        html += '<button class="btn btn-primary" id="btn-rematch-pve">Play Again</button>';
      }
      html += '<button class="btn btn-secondary" id="btn-back">Back to Lobby</button>';
      html += '</div>';
    } else {
      html += '<div class="chess-game-actions">';
      html += '<button class="btn btn-danger btn-sm" id="btn-resign">Resign</button>';
      html += '</div>';
    }

    // Move history
    if (state.moves) {
      html += '<div class="chess-moves">' + esc(state.moves) + '</div>';
    }

    html += '</div>';
    root.innerHTML = html;

    // Click handlers for cells
    if (!gameOver && isYourTurn) {
      root.querySelectorAll(".chess-cell").forEach(function (cell) {
        cell.onclick = function () {
          var sq = cell.getAttribute("data-sq");
          handleCellClick(sq, board, yourColor, state);
        };
      });
    }

    // Button handlers
    var btnResign = document.getElementById("btn-resign");
    if (btnResign) {
      btnResign.onclick = async function () {
        if (Goop.ui && Goop.ui.confirm) {
          var ok = await Goop.ui.confirm("Are you sure you want to resign?");
          if (!ok) return;
        }
        await db.call("chess", { action: "resign", game_id: state.game_id || currentGameId });
        showLobby();
      };
    }

    var btnRematchPve = document.getElementById("btn-rematch-pve");
    if (btnRematchPve) {
      btnRematchPve.onclick = async function () {
        btnRematchPve.disabled = true;
        try {
          var result = await db.call("chess", { action: "new_pve" });
          if (result.error) {
            Goop.ui.toast({ title: "Error", message: result.error });
            btnRematchPve.disabled = false;
          } else {
            currentGameId = result.game_id;
            renderGame(result);
          }
        } catch (e) {
          Goop.ui.toast({ title: "Error", message: e.message || "Error starting game." });
          btnRematchPve.disabled = false;
        }
      };
    }

    var btnBack = document.getElementById("btn-back");
    if (btnBack) {
      btnBack.onclick = function () { showLobby(); };
    }
  }

  async function handleCellClick(sq, board, yourColor, state) {
    var piece = board[sq];

    if (selectedSquare) {
      // Check if clicking on a legal move
      if (legalMoves.indexOf(sq) !== -1) {
        // Check for pawn promotion
        var movingPiece = board[selectedSquare];
        var rank = parseInt(sq[1]);
        if (movingPiece && movingPiece.toLowerCase() === "p" &&
            ((yourColor === "w" && rank === 8) || (yourColor === "b" && rank === 1))) {
          showPromotionDialog(selectedSquare, sq, yourColor, state);
          return;
        }

        await makeMove(selectedSquare, sq, null, state);
        return;
      }

      // Clicking on own piece - select it instead
      if (piece && pieceColor(piece) === yourColor) {
        selectedSquare = sq;
        legalMoves = await getLegalMoves(sq, state);
        renderGame(state);
        return;
      }

      // Clicking elsewhere - deselect
      selectedSquare = null;
      legalMoves = [];
      renderGame(state);
      return;
    }

    // No selection - select own piece
    if (piece && pieceColor(piece) === yourColor) {
      selectedSquare = sq;
      legalMoves = await getLegalMoves(sq, state);
      renderGame(state);
    }
  }

  function showPromotionDialog(from, to, color, state) {
    var overlay = document.createElement("div");
    overlay.className = "promo-overlay";

    var pieces = color === "w" ? ["Q", "R", "B", "N"] : ["q", "r", "b", "n"];
    var promoTypes = ["q", "r", "b", "n"];

    var html = '<div class="promo-dialog">';
    html += '<h3>Promote to:</h3>';
    html += '<div class="promo-pieces">';
    for (var i = 0; i < pieces.length; i++) {
      var pColor = color === "w" ? "white" : "black";
      html += '<div class="promo-piece" data-promo="' + promoTypes[i] + '">';
      html += '<span class="piece ' + pColor + '">' + PIECES[pieces[i]] + '</span>';
      html += '</div>';
    }
    html += '</div></div>';
    overlay.innerHTML = html;
    document.body.appendChild(overlay);

    overlay.querySelectorAll(".promo-piece").forEach(function (el) {
      el.onclick = async function () {
        var promo = el.getAttribute("data-promo");
        overlay.remove();
        await makeMove(from, to, promo, state);
      };
    });
  }

  async function makeMove(from, to, promotion, state) {
    selectedSquare = null;
    legalMoves = [];

    try {
      var result = await db.call("move", {
        game_id: state.game_id || currentGameId,
        from: from,
        to: to,
        promotion: promotion
      });

      if (result.error) {
        Goop.ui.toast({ title: "Error", message: result.error });
        renderGame(state);
        return;
      }

      if (!result.game_id) result.game_id = state.game_id || currentGameId;
      if (!result.challenger_label) result.challenger_label = state.challenger_label;
      renderGame(result);

      // Start polling for opponent's move in PvP
      if (result.mode === "pvp" && result.status === "playing") {
        startPolling(result.game_id);
      }
    } catch (e) {
      Goop.ui.toast({ title: "Error", message: e.message || "Error making move." });
      renderGame(state);
    }
  }

  async function getLegalMoves(sq, state) {
    // For now, we'll calculate legal moves client-side based on piece type
    // This is a simplified version - the server validates actual moves
    var fen = state.fen;
    var board = parseFen(fen);
    var piece = board[sq];
    if (!piece) return [];

    var moves = [];
    var color = pieceColor(piece);
    var pieceType = piece.toLowerCase();
    var file = sq.charCodeAt(0) - 96;
    var rank = parseInt(sq[1]);

    if (pieceType === "p") {
      var dir = color === "w" ? 1 : -1;
      var startRank = color === "w" ? 2 : 7;

      // Forward
      var fwd = toSq(file, rank + dir);
      if (fwd && !board[fwd]) {
        moves.push(fwd);
        // Double push
        if (rank === startRank) {
          var fwd2 = toSq(file, rank + 2 * dir);
          if (fwd2 && !board[fwd2]) moves.push(fwd2);
        }
      }
      // Captures
      [-1, 1].forEach(function (df) {
        var cap = toSq(file + df, rank + dir);
        if (cap && board[cap] && pieceColor(board[cap]) !== color) {
          moves.push(cap);
        }
        // En passant (simplified check)
        var fenParts = fen.split(" ");
        if (fenParts[3] && fenParts[3] === cap) {
          moves.push(cap);
        }
      });
    } else if (pieceType === "n") {
      [[-2,-1],[-2,1],[-1,-2],[-1,2],[1,-2],[1,2],[2,-1],[2,1]].forEach(function (d) {
        var nsq = toSq(file + d[0], rank + d[1]);
        if (nsq && (!board[nsq] || pieceColor(board[nsq]) !== color)) {
          moves.push(nsq);
        }
      });
    } else if (pieceType === "b") {
      [[1,1],[1,-1],[-1,1],[-1,-1]].forEach(function (d) {
        addSliding(board, file, rank, d[0], d[1], color, moves);
      });
    } else if (pieceType === "r") {
      [[1,0],[-1,0],[0,1],[0,-1]].forEach(function (d) {
        addSliding(board, file, rank, d[0], d[1], color, moves);
      });
    } else if (pieceType === "q") {
      [[1,0],[-1,0],[0,1],[0,-1],[1,1],[1,-1],[-1,1],[-1,-1]].forEach(function (d) {
        addSliding(board, file, rank, d[0], d[1], color, moves);
      });
    } else if (pieceType === "k") {
      [[-1,-1],[-1,0],[-1,1],[0,-1],[0,1],[1,-1],[1,0],[1,1]].forEach(function (d) {
        var ksq = toSq(file + d[0], rank + d[1]);
        if (ksq && (!board[ksq] || pieceColor(board[ksq]) !== color)) {
          moves.push(ksq);
        }
      });
      // Castling (simplified)
      var fenParts = fen.split(" ");
      var castling = fenParts[2] || "";
      var backRank = color === "w" ? 1 : 8;
      if (rank === backRank && file === 5) {
        if ((color === "w" && castling.indexOf("K") !== -1) ||
            (color === "b" && castling.indexOf("k") !== -1)) {
          if (!board[toSq(6, backRank)] && !board[toSq(7, backRank)]) {
            moves.push(toSq(7, backRank));
          }
        }
        if ((color === "w" && castling.indexOf("Q") !== -1) ||
            (color === "b" && castling.indexOf("q") !== -1)) {
          if (!board[toSq(2, backRank)] && !board[toSq(3, backRank)] && !board[toSq(4, backRank)]) {
            moves.push(toSq(3, backRank));
          }
        }
      }
    }

    return moves;
  }

  function addSliding(board, file, rank, df, dr, color, moves) {
    var f = file + df;
    var r = rank + dr;
    while (f >= 1 && f <= 8 && r >= 1 && r <= 8) {
      var sq = toSq(f, r);
      var target = board[sq];
      if (target) {
        if (pieceColor(target) !== color) moves.push(sq);
        break;
      }
      moves.push(sq);
      f += df;
      r += dr;
    }
  }

  function toSq(file, rank) {
    if (file < 1 || file > 8 || rank < 1 || rank > 8) return null;
    return String.fromCharCode(96 + file) + rank;
  }

  function parseFen(fen) {
    var board = {};
    var parts = fen.split(" ");
    var position = parts[0];
    var ranks = position.split("/");

    for (var i = 0; i < 8; i++) {
      var rankData = ranks[i];
      var file = 1;
      for (var j = 0; j < rankData.length; j++) {
        var c = rankData[j];
        if (c >= "1" && c <= "8") {
          file += parseInt(c);
        } else {
          var sq = String.fromCharCode(96 + file) + (8 - i);
          board[sq] = c;
          file++;
        }
      }
    }

    return board;
  }

  function pieceColor(piece) {
    if (!piece) return null;
    return piece === piece.toUpperCase() ? "w" : "b";
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
