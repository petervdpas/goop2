// Space Invaders
(async function() {
  var db = Goop.data;
  var canvas = document.getElementById('game-canvas');
  var startBtn = document.getElementById('btn-start');
  var leaderboardBtn = document.getElementById('btn-leaderboard');
  var leaderboardModal = document.getElementById('leaderboard-modal');
  var leaderboardList = document.getElementById('leaderboard-list');
  var closeLeaderboard = document.getElementById('close-leaderboard');

  // Engine components
  var renderer = new Renderer(canvas);
  var input = new Input();
  var loop = new GameLoop(60);
  var scenes = new SceneManager();

  // Game constants
  var CANVAS_W = 480;
  var CANVAS_H = 400;
  var PLAYER_W = 40;
  var PLAYER_H = 20;
  var PLAYER_SPEED = 250;
  var BULLET_SPEED = 400;
  var ALIEN_ROWS = 5;
  var ALIEN_COLS = 8;
  var ALIEN_W = 32;
  var ALIEN_H = 24;
  var ALIEN_GAP_X = 12;
  var ALIEN_GAP_Y = 10;

  // Game state
  var player = null;
  var aliens = [];
  var playerBullets = [];
  var alienBullets = [];
  var score = 0;
  var lives = 3;
  var wave = 1;
  var alienDir = 1;
  var alienSpeed = 30;
  var alienDropAmount = 20;
  var alienShootChance = 0.002;
  var gameStartTime = 0;

  // Colors
  var COLORS = {
    bg: '#0a0a15',
    player: '#16c784',
    bullet: '#fff',
    alienBullet: '#ff6b6b',
    alien1: '#ff6b6b',
    alien2: '#ffd93d',
    alien3: '#6bcb77',
    text: '#e0e0e0',
    muted: '#7a8194'
  };

  // ========================================================================
  // Player
  // ========================================================================
  function createPlayer() {
    return {
      x: CANVAS_W / 2 - PLAYER_W / 2,
      y: CANVAS_H - PLAYER_H - 20,
      w: PLAYER_W,
      h: PLAYER_H,
      shootCooldown: 0
    };
  }

  function updatePlayer(dt) {
    // Movement
    if (input.isDown('ArrowLeft') || input.isDown('KeyA')) {
      player.x -= PLAYER_SPEED * dt;
    }
    if (input.isDown('ArrowRight') || input.isDown('KeyD')) {
      player.x += PLAYER_SPEED * dt;
    }

    // Clamp to screen
    player.x = Utils.clamp(player.x, 0, CANVAS_W - PLAYER_W);

    // Shooting
    player.shootCooldown -= dt;
    if ((input.isDown('Space') || input.isDown('KeyW') || input.isDown('ArrowUp')) && player.shootCooldown <= 0) {
      playerBullets.push({
        x: player.x + PLAYER_W / 2 - 2,
        y: player.y,
        w: 4,
        h: 12
      });
      player.shootCooldown = 0.3;
    }
  }

  function drawPlayer() {
    var ctx = renderer.ctx;
    var x = player.x;
    var y = player.y;

    // Ship body
    ctx.fillStyle = COLORS.player;
    ctx.beginPath();
    ctx.moveTo(x + PLAYER_W / 2, y);
    ctx.lineTo(x + PLAYER_W, y + PLAYER_H);
    ctx.lineTo(x, y + PLAYER_H);
    ctx.closePath();
    ctx.fill();

    // Cockpit
    ctx.fillStyle = '#0f9960';
    ctx.fillRect(x + PLAYER_W / 2 - 4, y + 6, 8, 8);
  }

  // ========================================================================
  // Aliens
  // ========================================================================
  function createAliens() {
    aliens = [];
    var startX = (CANVAS_W - (ALIEN_COLS * (ALIEN_W + ALIEN_GAP_X) - ALIEN_GAP_X)) / 2;
    var startY = 50;

    for (var row = 0; row < ALIEN_ROWS; row++) {
      for (var col = 0; col < ALIEN_COLS; col++) {
        aliens.push({
          x: startX + col * (ALIEN_W + ALIEN_GAP_X),
          y: startY + row * (ALIEN_H + ALIEN_GAP_Y),
          w: ALIEN_W,
          h: ALIEN_H,
          row: row,
          alive: true,
          animFrame: 0
        });
      }
    }
  }

  function updateAliens(dt) {
    // Find leftmost and rightmost alive aliens
    var leftMost = CANVAS_W;
    var rightMost = 0;
    var lowestY = 0;

    for (var i = 0; i < aliens.length; i++) {
      var a = aliens[i];
      if (!a.alive) continue;
      if (a.x < leftMost) leftMost = a.x;
      if (a.x + a.w > rightMost) rightMost = a.x + a.w;
      if (a.y + a.h > lowestY) lowestY = a.y + a.h;
    }

    // Check if need to reverse direction
    var needDrop = false;
    if (alienDir > 0 && rightMost >= CANVAS_W - 10) {
      alienDir = -1;
      needDrop = true;
    } else if (alienDir < 0 && leftMost <= 10) {
      alienDir = 1;
      needDrop = true;
    }

    // Move aliens
    for (var j = 0; j < aliens.length; j++) {
      var alien = aliens[j];
      if (!alien.alive) continue;

      alien.x += alienDir * alienSpeed * dt;
      if (needDrop) {
        alien.y += alienDropAmount;
      }

      // Animation
      alien.animFrame += dt * 3;

      // Random shooting
      if (Math.random() < alienShootChance) {
        alienBullets.push({
          x: alien.x + ALIEN_W / 2 - 2,
          y: alien.y + ALIEN_H,
          w: 4,
          h: 10
        });
      }

      // Check if reached player
      if (alien.y + alien.h >= player.y) {
        gameOver();
        return;
      }
    }

    // Check win condition
    var aliveCount = aliens.filter(function(a) { return a.alive; }).length;
    if (aliveCount === 0) {
      nextWave();
    }
  }

  function drawAliens() {
    var ctx = renderer.ctx;

    for (var i = 0; i < aliens.length; i++) {
      var a = aliens[i];
      if (!a.alive) continue;

      // Color based on row
      var color;
      if (a.row === 0) color = COLORS.alien1;
      else if (a.row < 3) color = COLORS.alien2;
      else color = COLORS.alien3;

      ctx.fillStyle = color;

      // Simple alien shape with animation
      var wobble = Math.sin(a.animFrame) * 2;
      var x = a.x;
      var y = a.y;

      // Body
      ctx.fillRect(x + 4, y + 4, ALIEN_W - 8, ALIEN_H - 8);

      // Eyes
      ctx.fillStyle = '#000';
      ctx.fillRect(x + 8, y + 8, 4, 4);
      ctx.fillRect(x + ALIEN_W - 12, y + 8, 4, 4);

      // Legs/tentacles
      ctx.fillStyle = color;
      ctx.fillRect(x + 2 + wobble, y + ALIEN_H - 6, 6, 6);
      ctx.fillRect(x + ALIEN_W - 8 - wobble, y + ALIEN_H - 6, 6, 6);
    }
  }

  // ========================================================================
  // Bullets
  // ========================================================================
  function updateBullets(dt) {
    // Player bullets
    for (var i = playerBullets.length - 1; i >= 0; i--) {
      var b = playerBullets[i];
      b.y -= BULLET_SPEED * dt;

      // Off screen
      if (b.y + b.h < 0) {
        playerBullets.splice(i, 1);
        continue;
      }

      // Hit alien
      for (var j = 0; j < aliens.length; j++) {
        var a = aliens[j];
        if (!a.alive) continue;

        if (Collision.rectRect(b, a)) {
          a.alive = false;
          playerBullets.splice(i, 1);

          // Score based on row (top = more points)
          var points = (ALIEN_ROWS - a.row) * 10;
          score += points;
          break;
        }
      }
    }

    // Alien bullets
    for (var k = alienBullets.length - 1; k >= 0; k--) {
      var ab = alienBullets[k];
      ab.y += BULLET_SPEED * 0.6 * dt;

      // Off screen
      if (ab.y > CANVAS_H) {
        alienBullets.splice(k, 1);
        continue;
      }

      // Hit player
      if (Collision.rectRect(ab, player)) {
        alienBullets.splice(k, 1);
        lives--;

        if (lives <= 0) {
          gameOver();
        }
      }
    }
  }

  function drawBullets() {
    var ctx = renderer.ctx;

    // Player bullets
    ctx.fillStyle = COLORS.bullet;
    for (var i = 0; i < playerBullets.length; i++) {
      var b = playerBullets[i];
      ctx.fillRect(b.x, b.y, b.w, b.h);
    }

    // Alien bullets
    ctx.fillStyle = COLORS.alienBullet;
    for (var j = 0; j < alienBullets.length; j++) {
      var ab = alienBullets[j];
      ctx.fillRect(ab.x, ab.y, ab.w, ab.h);
    }
  }

  // ========================================================================
  // Game flow
  // ========================================================================
  function nextWave() {
    wave++;
    alienSpeed += 10;
    alienShootChance += 0.001;
    playerBullets = [];
    alienBullets = [];
    createAliens();
  }

  function gameOver() {
    scenes.switch('gameover');
  }

  function resetGame() {
    score = 0;
    lives = 3;
    wave = 1;
    alienSpeed = 30 + (wave - 1) * 10;
    alienShootChance = 0.002;
    alienDir = 1;
    playerBullets = [];
    alienBullets = [];
    player = createPlayer();
    createAliens();
    gameStartTime = Date.now();
  }

  // ========================================================================
  // UI Drawing
  // ========================================================================
  function drawUI() {
    // Score
    renderer.drawText('SCORE: ' + score, 10, 10, '14px monospace', COLORS.text);

    // Wave
    renderer.drawText('WAVE ' + wave, CANVAS_W / 2 - 30, 10, '14px monospace', COLORS.muted);

    // Lives
    var livesText = 'LIVES: ';
    for (var i = 0; i < lives; i++) {
      livesText += '\u2665 ';
    }
    renderer.drawText(livesText, CANVAS_W - 100, 10, '14px monospace', COLORS.player);
  }

  // ========================================================================
  // Scenes
  // ========================================================================
  scenes.add('title', {
    enter: function() {
      startBtn.textContent = 'Start Game';
      startBtn.disabled = false;
    },
    update: function(dt) {
      if (input.justPressed('Space') || input.justPressed('Enter')) {
        scenes.switch('game');
      }
    },
    render: function(r) {
      r.clear(COLORS.bg);

      r.drawTextCentered('SPACE INVADERS', 80, 'bold 36px monospace', COLORS.player);
      r.drawTextCentered('Defend Earth!', 120, '14px monospace', COLORS.muted);

      // Draw sample aliens
      var ctx = r.ctx;
      var startX = CANVAS_W / 2 - 80;
      ctx.fillStyle = COLORS.alien1;
      ctx.fillRect(startX, 160, 28, 20);
      ctx.fillStyle = COLORS.alien2;
      ctx.fillRect(startX + 50, 160, 28, 20);
      ctx.fillStyle = COLORS.alien3;
      ctx.fillRect(startX + 100, 160, 28, 20);

      r.drawTextCentered('= 50    = 30    = 10', 200, '12px monospace', COLORS.text);

      r.drawTextCentered('Arrow keys to move', 260, '12px monospace', COLORS.muted);
      r.drawTextCentered('SPACE to shoot', 280, '12px monospace', COLORS.muted);

      r.drawTextCentered('Press SPACE to start', 340, '14px monospace', COLORS.player);
    }
  });

  scenes.add('game', {
    enter: function() {
      startBtn.textContent = 'Restart';
      resetGame();
    },
    update: function(dt) {
      updatePlayer(dt);
      updateAliens(dt);
      updateBullets(dt);
    },
    render: function(r) {
      r.clear(COLORS.bg);
      drawAliens();
      drawPlayer();
      drawBullets();
      drawUI();
    }
  });

  scenes.add('gameover', {
    finalScore: 0,
    finalWave: 1,
    submitted: false,

    enter: async function() {
      this.finalScore = score;
      this.finalWave = wave;
      this.submitted = false;
      startBtn.textContent = 'Play Again';

      // Submit score
      if (this.finalScore > 0) {
        try {
          await db.call('arcade', {
            action: 'submit_score',
            score: this.finalScore,
            level: this.finalWave,
            time_ms: Date.now() - gameStartTime
          });
          this.submitted = true;
        } catch (e) {
          console.log('Failed to submit score:', e);
        }
      }
    },
    update: function(dt) {
      if (input.justPressed('Space') || input.justPressed('Enter')) {
        scenes.switch('game');
      }
    },
    render: function(r) {
      r.clear(COLORS.bg);

      r.drawTextCentered('GAME OVER', 100, 'bold 32px monospace', COLORS.alien1);
      r.drawTextCentered('Score: ' + this.finalScore, 160, '20px monospace', COLORS.text);
      r.drawTextCentered('Wave: ' + this.finalWave, 190, '16px monospace', COLORS.muted);

      if (this.submitted) {
        r.drawTextCentered('Score submitted!', 240, '12px monospace', COLORS.player);
      }

      r.drawTextCentered('Press SPACE to play again', 300, '14px monospace', COLORS.muted);
    }
  });

  // ========================================================================
  // Leaderboard
  // ========================================================================
  async function showLeaderboard() {
    leaderboardModal.classList.remove('hidden');

    try {
      var result = await db.call('arcade', { action: 'get_leaderboard', limit: 10 });
      var scores = result.scores || [];

      if (scores.length === 0) {
        leaderboardList.innerHTML = '<p class="empty">No scores yet!</p>';
      } else {
        var html = '<ol class="scores-list">';
        for (var i = 0; i < scores.length; i++) {
          var s = scores[i];
          html += '<li><span class="name">' + esc(s.player_label || 'Anonymous') + '</span>';
          html += '<span class="score">' + s.score + '</span>';
          html += '<span class="time">Wave ' + (s.level || 1) + '</span></li>';
        }
        html += '</ol>';
        leaderboardList.innerHTML = html;
      }
    } catch (e) {
      leaderboardList.innerHTML = '<p class="empty">Failed to load scores</p>';
    }
  }

  function esc(s) {
    if (!s) return '';
    var d = document.createElement('div');
    d.appendChild(document.createTextNode(s));
    return d.innerHTML;
  }

  // ========================================================================
  // UI Event handlers
  // ========================================================================
  startBtn.onclick = function() {
    scenes.switch('game');
    canvas.focus();
  };

  leaderboardBtn.onclick = function() {
    showLeaderboard();
  };

  closeLeaderboard.onclick = function() {
    leaderboardModal.classList.add('hidden');
  };

  leaderboardModal.onclick = function(e) {
    if (e.target === leaderboardModal) {
      leaderboardModal.classList.add('hidden');
    }
  };

  // ========================================================================
  // Initialize
  // ========================================================================
  function init() {
    input.bind(canvas);
    canvas.focus();

    scenes.switch('title');

    loop.start(
      function(dt) {
        scenes.update(dt);
        input.update();
      },
      function() {
        scenes.render(renderer);
      }
    );
  }

  init();
})();
