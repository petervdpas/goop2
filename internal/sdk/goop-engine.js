// Arcade Engine - Lightweight 2D game engine
// No dependencies, pure Canvas API

// ============================================================================
// Game Loop - Fixed timestep with variable rendering
// ============================================================================
class GameLoop {
  constructor(tickRate = 60) {
    this.tickRate = tickRate;
    this.frameTime = 1000 / tickRate;
    this.running = false;
    this.lastTime = 0;
    this.accumulator = 0;
    this.updateFn = null;
    this.renderFn = null;
  }

  start(updateFn, renderFn) {
    this.updateFn = updateFn;
    this.renderFn = renderFn;
    this.running = true;
    this.lastTime = performance.now();
    this.accumulator = 0;
    requestAnimationFrame(this._tick.bind(this));
  }

  stop() {
    this.running = false;
  }

  _tick(currentTime) {
    if (!this.running) return;

    var deltaTime = currentTime - this.lastTime;
    this.lastTime = currentTime;

    // Cap delta to prevent spiral of death
    if (deltaTime > 250) deltaTime = 250;

    this.accumulator += deltaTime;

    // Fixed timestep updates
    var dt = this.frameTime / 1000;
    while (this.accumulator >= this.frameTime) {
      this.updateFn(dt);
      this.accumulator -= this.frameTime;
    }

    // Render
    this.renderFn();

    requestAnimationFrame(this._tick.bind(this));
  }
}

// ============================================================================
// Renderer - Canvas drawing utilities
// ============================================================================
class Renderer {
  constructor(canvas) {
    this.canvas = canvas;
    this.ctx = canvas.getContext('2d');
    this.ctx.imageSmoothingEnabled = false; // Pixel art friendly
    this.width = canvas.width;
    this.height = canvas.height;
  }

  clear(color) {
    this.ctx.fillStyle = color || '#000';
    this.ctx.fillRect(0, 0, this.width, this.height);
  }

  drawRect(x, y, w, h, color) {
    this.ctx.fillStyle = color;
    this.ctx.fillRect(Math.floor(x), Math.floor(y), w, h);
  }

  drawRectOutline(x, y, w, h, color, lineWidth) {
    this.ctx.strokeStyle = color;
    this.ctx.lineWidth = lineWidth || 1;
    this.ctx.strokeRect(Math.floor(x), Math.floor(y), w, h);
  }

  drawCircle(x, y, radius, color) {
    this.ctx.fillStyle = color;
    this.ctx.beginPath();
    this.ctx.arc(Math.floor(x), Math.floor(y), radius, 0, Math.PI * 2);
    this.ctx.fill();
  }

  drawSprite(image, sx, sy, sw, sh, dx, dy, dw, dh) {
    if (!image || !image.complete) return;
    this.ctx.drawImage(
      image,
      sx, sy, sw, sh,
      Math.floor(dx), Math.floor(dy), dw || sw, dh || sh
    );
  }

  drawText(text, x, y, font, color, align) {
    this.ctx.font = font || '16px monospace';
    this.ctx.fillStyle = color || '#fff';
    this.ctx.textAlign = align || 'left';
    this.ctx.textBaseline = 'top';
    this.ctx.fillText(text, Math.floor(x), Math.floor(y));
  }

  drawTextCentered(text, y, font, color) {
    this.drawText(text, this.width / 2, y, font, color, 'center');
  }
}

// ============================================================================
// SpriteSheet - Manage sprite atlases
// ============================================================================
class SpriteSheet {
  constructor(imageSrc, tileWidth, tileHeight) {
    this.image = new Image();
    this.image.src = imageSrc;
    this.tw = tileWidth;
    this.th = tileHeight;
    this.ready = false;
    this.cols = 0;
    this.rows = 0;

    var self = this;
    this.image.onload = function() {
      self.ready = true;
      self.cols = Math.floor(self.image.width / self.tw);
      self.rows = Math.floor(self.image.height / self.th);
    };
  }

  getTile(col, row) {
    return {
      sx: col * this.tw,
      sy: row * this.th,
      sw: this.tw,
      sh: this.th
    };
  }

  draw(renderer, col, row, x, y, scale) {
    if (!this.ready) return;
    scale = scale || 1;
    var tile = this.getTile(col, row);
    renderer.drawSprite(
      this.image,
      tile.sx, tile.sy, tile.sw, tile.sh,
      x, y, this.tw * scale, this.th * scale
    );
  }
}

// ============================================================================
// Input - Keyboard and touch handling
// ============================================================================
class Input {
  constructor() {
    this.keys = {};
    this.keysJustPressed = {};
    this.keysJustReleased = {};
    this.touches = [];
    this.mouseX = 0;
    this.mouseY = 0;
    this.mouseDown = false;
    this.mouseJustPressed = false;
  }

  bind(element) {
    var self = this;

    // Keyboard
    window.addEventListener('keydown', function(e) {
      if (!self.keys[e.code]) {
        self.keysJustPressed[e.code] = true;
      }
      self.keys[e.code] = true;

      // Prevent arrow keys from scrolling
      if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Space'].indexOf(e.code) !== -1) {
        e.preventDefault();
      }
    });

    window.addEventListener('keyup', function(e) {
      self.keys[e.code] = false;
      self.keysJustReleased[e.code] = true;
    });

    // Mouse
    element.addEventListener('mousedown', function(e) {
      self.mouseDown = true;
      self.mouseJustPressed = true;
      self._updateMouse(e, element);
    });

    element.addEventListener('mouseup', function() {
      self.mouseDown = false;
    });

    element.addEventListener('mousemove', function(e) {
      self._updateMouse(e, element);
    });

    // Touch
    element.addEventListener('touchstart', function(e) {
      e.preventDefault();
      self.mouseDown = true;
      self.mouseJustPressed = true;
      self._updateTouch(e, element);
    });

    element.addEventListener('touchend', function() {
      self.mouseDown = false;
      self.touches = [];
    });

    element.addEventListener('touchmove', function(e) {
      e.preventDefault();
      self._updateTouch(e, element);
    });
  }

  _updateMouse(e, element) {
    var rect = element.getBoundingClientRect();
    var scaleX = element.width / rect.width;
    var scaleY = element.height / rect.height;
    this.mouseX = (e.clientX - rect.left) * scaleX;
    this.mouseY = (e.clientY - rect.top) * scaleY;
  }

  _updateTouch(e, element) {
    var rect = element.getBoundingClientRect();
    var scaleX = element.width / rect.width;
    var scaleY = element.height / rect.height;
    this.touches = [];
    for (var i = 0; i < e.touches.length; i++) {
      var t = e.touches[i];
      this.touches.push({
        x: (t.clientX - rect.left) * scaleX,
        y: (t.clientY - rect.top) * scaleY
      });
    }
    if (this.touches.length > 0) {
      this.mouseX = this.touches[0].x;
      this.mouseY = this.touches[0].y;
    }
  }

  isDown(key) {
    return !!this.keys[key];
  }

  justPressed(key) {
    return !!this.keysJustPressed[key];
  }

  justReleased(key) {
    return !!this.keysJustReleased[key];
  }

  update() {
    // Clear just pressed/released states
    this.keysJustPressed = {};
    this.keysJustReleased = {};
    this.mouseJustPressed = false;
  }
}

// ============================================================================
// Collision - AABB collision detection
// ============================================================================
class Collision {
  static rectRect(a, b) {
    return a.x < b.x + b.w &&
           a.x + a.w > b.x &&
           a.y < b.y + b.h &&
           a.y + a.h > b.y;
  }

  static pointRect(px, py, rect) {
    return px >= rect.x &&
           px < rect.x + rect.w &&
           py >= rect.y &&
           py < rect.y + rect.h;
  }

  static rectContains(outer, inner) {
    return inner.x >= outer.x &&
           inner.y >= outer.y &&
           inner.x + inner.w <= outer.x + outer.w &&
           inner.y + inner.h <= outer.y + outer.h;
  }

  static circleCircle(a, b) {
    var dx = a.x - b.x;
    var dy = a.y - b.y;
    var dist = Math.sqrt(dx * dx + dy * dy);
    return dist < a.r + b.r;
  }
}

// ============================================================================
// TileMap - Grid-based map with collision
// ============================================================================
class TileMap {
  constructor(data, tileWidth, tileHeight, spriteSheet) {
    this.data = data;
    this.tw = tileWidth;
    this.th = tileHeight;
    this.sheet = spriteSheet;
    this.width = data[0] ? data[0].length : 0;
    this.height = data.length;
    this.solidTiles = [1]; // Tile indices that are solid
  }

  draw(renderer, offsetX, offsetY) {
    offsetX = offsetX || 0;
    offsetY = offsetY || 0;

    for (var row = 0; row < this.data.length; row++) {
      for (var col = 0; col < this.data[row].length; col++) {
        var tile = this.data[row][col];
        if (tile > 0) {
          // Map tile index to sprite sheet position
          // Row 1 of sprite sheet = tiles
          this.sheet.draw(
            renderer,
            tile - 1, 1, // col, row in spritesheet
            col * this.tw + offsetX,
            row * this.th + offsetY,
            1
          );
        }
      }
    }
  }

  getTileAt(worldX, worldY) {
    var col = Math.floor(worldX / this.tw);
    var row = Math.floor(worldY / this.th);
    if (row < 0 || row >= this.height || col < 0 || col >= this.width) {
      return -1; // Out of bounds
    }
    return this.data[row][col];
  }

  isSolid(worldX, worldY) {
    var tile = this.getTileAt(worldX, worldY);
    return this.solidTiles.indexOf(tile) !== -1;
  }

  setTile(col, row, tileIndex) {
    if (row >= 0 && row < this.height && col >= 0 && col < this.width) {
      this.data[row][col] = tileIndex;
    }
  }

  // Check collision with a rectangle against solid tiles
  collideRect(rect) {
    var left = Math.floor(rect.x / this.tw);
    var right = Math.floor((rect.x + rect.w - 1) / this.tw);
    var top = Math.floor(rect.y / this.th);
    var bottom = Math.floor((rect.y + rect.h - 1) / this.th);

    for (var row = top; row <= bottom; row++) {
      for (var col = left; col <= right; col++) {
        if (row < 0 || row >= this.height || col < 0 || col >= this.width) {
          continue;
        }
        if (this.solidTiles.indexOf(this.data[row][col]) !== -1) {
          return {
            col: col,
            row: row,
            x: col * this.tw,
            y: row * this.th
          };
        }
      }
    }
    return null;
  }
}

// ============================================================================
// Audio - Simple sound management
// ============================================================================
class GameAudio {
  constructor() {
    this.sounds = {};
    this.musicVolume = 0.5;
    this.sfxVolume = 0.7;
    this.muted = false;
  }

  load(name, src) {
    var audio = new Audio(src);
    audio.preload = 'auto';
    this.sounds[name] = audio;
  }

  play(name, loop) {
    if (this.muted) return;
    var sound = this.sounds[name];
    if (!sound) return;

    // Clone for overlapping sounds
    var instance = sound.cloneNode();
    instance.volume = loop ? this.musicVolume : this.sfxVolume;
    instance.loop = !!loop;
    instance.play().catch(function() {}); // Ignore autoplay errors
    return instance;
  }

  stop(instance) {
    if (instance) {
      instance.pause();
      instance.currentTime = 0;
    }
  }

  stopAll() {
    for (var name in this.sounds) {
      this.sounds[name].pause();
      this.sounds[name].currentTime = 0;
    }
  }

  toggleMute() {
    this.muted = !this.muted;
    return this.muted;
  }
}

// ============================================================================
// SceneManager - State/scene management
// ============================================================================
class SceneManager {
  constructor() {
    this.scenes = {};
    this.current = null;
    this.currentName = '';
  }

  add(name, scene) {
    this.scenes[name] = scene;
  }

  switch(name, data) {
    if (this.current && this.current.exit) {
      this.current.exit();
    }

    this.currentName = name;
    this.current = this.scenes[name];

    if (this.current && this.current.enter) {
      this.current.enter(data);
    }
  }

  update(dt) {
    if (this.current && this.current.update) {
      this.current.update(dt);
    }
  }

  render(renderer) {
    if (this.current && this.current.render) {
      this.current.render(renderer);
    }
  }
}

// ============================================================================
// Entity - Simple game object base
// ============================================================================
class Entity {
  constructor(x, y, w, h) {
    this.x = x || 0;
    this.y = y || 0;
    this.w = w || 16;
    this.h = h || 16;
    this.vx = 0;
    this.vy = 0;
    this.active = true;
  }

  get rect() {
    return { x: this.x, y: this.y, w: this.w, h: this.h };
  }

  get centerX() {
    return this.x + this.w / 2;
  }

  get centerY() {
    return this.y + this.h / 2;
  }

  update(dt) {
    this.x += this.vx * dt;
    this.y += this.vy * dt;
  }

  collidesWith(other) {
    return Collision.rectRect(this.rect, other.rect || other);
  }
}

// ============================================================================
// Utils - Helper functions
// ============================================================================
var Utils = {
  clamp: function(val, min, max) {
    return Math.max(min, Math.min(max, val));
  },

  lerp: function(a, b, t) {
    return a + (b - a) * t;
  },

  random: function(min, max) {
    return Math.random() * (max - min) + min;
  },

  randomInt: function(min, max) {
    return Math.floor(Math.random() * (max - min + 1)) + min;
  },

  distance: function(x1, y1, x2, y2) {
    var dx = x2 - x1;
    var dy = y2 - y1;
    return Math.sqrt(dx * dx + dy * dy);
  },

  angle: function(x1, y1, x2, y2) {
    return Math.atan2(y2 - y1, x2 - x1);
  }
};
