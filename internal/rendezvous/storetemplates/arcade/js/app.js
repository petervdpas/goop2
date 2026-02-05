// Arcade - Sample Platformer Game
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
  var audio = new GameAudio();

  // Game state
  var sprites = null;
  var tilemap = null;
  var player = null;
  var coins = [];
  var score = 0;
  var startTime = 0;
  var levelData = null;

  // Constants
  var TILE_SIZE = 16;
  var SCALE = 1;
  var GRAVITY = 600;
  var JUMP_FORCE = -280;
  var MOVE_SPEED = 120;
  var PLAYER_WIDTH = 14;
  var PLAYER_HEIGHT = 16;

  // ========================================================================
  // Asset loading
  // ========================================================================
  async function loadAssets() {
    // Load sprite sheet (will be generated programmatically if not exists)
    sprites = new SpriteSheet('images/demo-sprites.png', 16, 16);

    // Wait for sprites to load or timeout
    await new Promise(function(resolve) {
      if (sprites.ready) {
        resolve();
      } else {
        sprites.image.onload = resolve;
        sprites.image.onerror = resolve; // Continue even if image fails
        setTimeout(resolve, 1000); // Timeout fallback
      }
    });
  }

  async function loadLevel(levelNum) {
    try {
      var result = await db.call('arcade', { action: 'get_level', level_num: levelNum });
      if (result.error) {
        console.log('Level not found, using default');
        return getDefaultLevel();
      }
      return JSON.parse(result.data);
    } catch (e) {
      console.log('Error loading level:', e);
      return getDefaultLevel();
    }
  }

  function getDefaultLevel() {
    return {
      width: 20,
      height: 15,
      tiles: [
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,0,0,1,1,1,0,0,0,0,1,1,1,0,0,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [0,0,0,1,1,0,0,0,0,0,0,0,0,0,0,1,1,0,0,0],
        [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
        [1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1],
        [1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1]
      ],
      spawn: { x: 32, y: 192 },
      coins: [
        { x: 96, y: 128 },
        { x: 208, y: 128 },
        { x: 152, y: 64 }
      ]
    };
  }

  // ========================================================================
  // Player
  // ========================================================================
  function createPlayer(x, y) {
    return {
      x: x,
      y: y,
      w: PLAYER_WIDTH,
      h: PLAYER_HEIGHT,
      vx: 0,
      vy: 0,
      grounded: false,
      facing: 1, // 1 = right, -1 = left
      animFrame: 0,
      animTimer: 0
    };
  }

  function updatePlayer(dt) {
    // Horizontal movement
    if (input.isDown('ArrowLeft') || input.isDown('KeyA')) {
      player.vx = -MOVE_SPEED;
      player.facing = -1;
    } else if (input.isDown('ArrowRight') || input.isDown('KeyD')) {
      player.vx = MOVE_SPEED;
      player.facing = 1;
    } else {
      player.vx = 0;
    }

    // Jump
    if ((input.justPressed('Space') || input.justPressed('ArrowUp') || input.justPressed('KeyW')) && player.grounded) {
      player.vy = JUMP_FORCE;
      player.grounded = false;
    }

    // Apply gravity
    player.vy += GRAVITY * dt;

    // Cap fall speed
    if (player.vy > 400) player.vy = 400;

    // Move and collide
    moveAndCollide(player, dt);

    // Animation
    if (player.vx !== 0) {
      player.animTimer += dt;
      if (player.animTimer > 0.1) {
        player.animTimer = 0;
        player.animFrame = (player.animFrame + 1) % 2;
      }
    } else {
      player.animFrame = 0;
    }

    // Check if fell off
    if (player.y > canvas.height + 32) {
      gameOver();
    }
  }

  function moveAndCollide(entity, dt) {
    // Move X
    entity.x += entity.vx * dt;

    // Check X collision
    var collision = tilemap.collideRect({
      x: entity.x,
      y: entity.y,
      w: entity.w,
      h: entity.h
    });

    if (collision) {
      if (entity.vx > 0) {
        entity.x = collision.x - entity.w;
      } else if (entity.vx < 0) {
        entity.x = collision.x + TILE_SIZE;
      }
      entity.vx = 0;
    }

    // Move Y
    entity.y += entity.vy * dt;

    // Check Y collision
    collision = tilemap.collideRect({
      x: entity.x,
      y: entity.y,
      w: entity.w,
      h: entity.h
    });

    if (collision) {
      if (entity.vy > 0) {
        entity.y = collision.y - entity.h;
        entity.grounded = true;
      } else if (entity.vy < 0) {
        entity.y = collision.y + TILE_SIZE;
      }
      entity.vy = 0;
    } else {
      entity.grounded = false;
    }

    // Clamp to level bounds
    entity.x = Utils.clamp(entity.x, 0, tilemap.width * TILE_SIZE - entity.w);
  }

  function drawPlayer() {
    if (sprites && sprites.ready) {
      // Draw player sprite
      var col = player.animFrame;
      var row = 0; // Player row in sprite sheet
      var x = player.x - 1; // Offset for centering
      var y = player.y;

      renderer.ctx.save();
      if (player.facing < 0) {
        renderer.ctx.scale(-1, 1);
        x = -x - PLAYER_WIDTH - 1;
      }
      sprites.draw(renderer, col, row, x, y, SCALE);
      renderer.ctx.restore();
    } else {
      // Fallback rectangle
      renderer.drawRect(player.x, player.y, player.w, player.h, '#16c784');
    }
  }

  // ========================================================================
  // Coins
  // ========================================================================
  function createCoins(coinData) {
    return coinData.map(function(c) {
      return {
        x: c.x,
        y: c.y,
        w: 12,
        h: 12,
        collected: false,
        animTimer: Math.random() * Math.PI * 2 // Random start phase
      };
    });
  }

  function updateCoins(dt) {
    for (var i = 0; i < coins.length; i++) {
      var coin = coins[i];
      if (coin.collected) continue;

      coin.animTimer += dt * 5;

      // Check collection
      if (Collision.rectRect(
        { x: player.x, y: player.y, w: player.w, h: player.h },
        { x: coin.x, y: coin.y, w: coin.w, h: coin.h }
      )) {
        coin.collected = true;
        score += 100;
        // audio.play('coin');
      }
    }

    // Check win condition
    var allCollected = coins.every(function(c) { return c.collected; });
    if (allCollected && coins.length > 0) {
      win();
    }
  }

  function drawCoins() {
    for (var i = 0; i < coins.length; i++) {
      var coin = coins[i];
      if (coin.collected) continue;

      var bounce = Math.sin(coin.animTimer) * 2;

      if (sprites && sprites.ready) {
        // Draw coin sprite (row 2, col 0)
        sprites.draw(renderer, 0, 2, coin.x, coin.y + bounce, SCALE);
      } else {
        // Fallback circle
        renderer.drawCircle(coin.x + 6, coin.y + 6 + bounce, 6, '#ffd700');
      }
    }
  }

  // ========================================================================
  // Scenes
  // ========================================================================

  // Title scene
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
      r.clear('#0f0f1a');

      r.drawTextCentered('ARCADE', 60, 'bold 32px monospace', '#16c784');
      r.drawTextCentered('Sample Platformer', 100, '14px monospace', '#7a8194');

      r.drawTextCentered('Collect all coins!', 150, '12px monospace', '#e0e0e0');
      r.drawTextCentered('Arrow keys or WASD to move', 170, '10px monospace', '#7a8194');
      r.drawTextCentered('Space to jump', 185, '10px monospace', '#7a8194');

      r.drawTextCentered('Press SPACE to start', 220, '12px monospace', '#16c784');
    }
  });

  // Game scene
  scenes.add('game', {
    enter: async function() {
      startBtn.textContent = 'Restart';
      startBtn.disabled = false;

      // Load level
      levelData = await loadLevel(1);

      // Create tilemap
      tilemap = new TileMap(levelData.tiles, TILE_SIZE, TILE_SIZE, sprites);

      // Create player
      player = createPlayer(levelData.spawn.x, levelData.spawn.y);

      // Create coins
      coins = createCoins(levelData.coins || []);

      // Reset score and timer
      score = 0;
      startTime = Date.now();
    },
    update: function(dt) {
      updatePlayer(dt);
      updateCoins(dt);
    },
    render: function(r) {
      r.clear('#1a1a2e');

      // Draw tilemap
      if (tilemap) {
        drawTilemap();
      }

      // Draw coins
      drawCoins();

      // Draw player
      if (player) {
        drawPlayer();
      }

      // Draw UI
      r.drawText('Score: ' + score, 8, 8, '12px monospace', '#fff');

      var elapsed = Math.floor((Date.now() - startTime) / 1000);
      r.drawText('Time: ' + elapsed + 's', 8, 24, '12px monospace', '#7a8194');
    }
  });

  function drawTilemap() {
    if (sprites && sprites.ready) {
      tilemap.draw(renderer, 0, 0);
    } else {
      // Fallback: draw rectangles
      for (var row = 0; row < tilemap.data.length; row++) {
        for (var col = 0; col < tilemap.data[row].length; col++) {
          if (tilemap.data[row][col] > 0) {
            renderer.drawRect(
              col * TILE_SIZE,
              row * TILE_SIZE,
              TILE_SIZE,
              TILE_SIZE,
              '#4a4a6a'
            );
          }
        }
      }
    }
  }

  // Game over scene
  scenes.add('gameover', {
    enter: function() {
      startBtn.textContent = 'Try Again';
    },
    update: function(dt) {
      if (input.justPressed('Space') || input.justPressed('Enter')) {
        scenes.switch('game');
      }
    },
    render: function(r) {
      r.clear('#0f0f1a');

      r.drawTextCentered('GAME OVER', 80, 'bold 24px monospace', '#dc2626');
      r.drawTextCentered('Score: ' + score, 120, '16px monospace', '#e0e0e0');

      r.drawTextCentered('Press SPACE to retry', 180, '12px monospace', '#7a8194');
    }
  });

  // Win scene
  scenes.add('win', {
    finalScore: 0,
    finalTime: 0,
    submitted: false,

    enter: async function() {
      this.finalScore = score;
      this.finalTime = Date.now() - startTime;
      this.submitted = false;

      startBtn.textContent = 'Play Again';

      // Submit score
      try {
        await db.call('arcade', {
          action: 'submit_score',
          score: this.finalScore,
          level: 1,
          time_ms: this.finalTime
        });
        this.submitted = true;
      } catch (e) {
        console.log('Failed to submit score:', e);
      }
    },
    update: function(dt) {
      if (input.justPressed('Space') || input.justPressed('Enter')) {
        scenes.switch('game');
      }
    },
    render: function(r) {
      r.clear('#0f0f1a');

      r.drawTextCentered('YOU WIN!', 60, 'bold 28px monospace', '#16c784');
      r.drawTextCentered('Score: ' + this.finalScore, 100, '18px monospace', '#e0e0e0');

      var seconds = (this.finalTime / 1000).toFixed(1);
      r.drawTextCentered('Time: ' + seconds + 's', 125, '14px monospace', '#7a8194');

      if (this.submitted) {
        r.drawTextCentered('Score submitted!', 160, '11px monospace', '#16c784');
      }

      r.drawTextCentered('Press SPACE to play again', 200, '12px monospace', '#7a8194');
    }
  });

  function gameOver() {
    scenes.switch('gameover');
  }

  function win() {
    scenes.switch('win');
  }

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
          var time = s.time_ms ? (s.time_ms / 1000).toFixed(1) + 's' : '-';
          html += '<li><span class="name">' + esc(s.player_label || 'Anonymous') + '</span>';
          html += '<span class="score">' + s.score + '</span>';
          html += '<span class="time">' + time + '</span></li>';
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
    if (scenes.currentName === 'title' || scenes.currentName === 'gameover' || scenes.currentName === 'win') {
      scenes.switch('game');
    } else if (scenes.currentName === 'game') {
      scenes.switch('game'); // Restart
    }
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
  async function init() {
    await loadAssets();

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
