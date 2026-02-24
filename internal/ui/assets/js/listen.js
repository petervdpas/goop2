// Listen group — API layer + audio player UI (host & listener).
(function () {
  if (!window.Goop) window.Goop = {};

  var core = window.Goop.core || {};
  var escapeHtml = core.escapeHtml || function(s) { return String(s).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;'); };
  var on = core.on || function(el, ev, fn) { el.addEventListener(ev, fn); };
  var toast = core.toast || function(msg) { console.log('[listen]', msg); };

  function log(level, msg) {
    if (window.Goop && window.Goop.log && window.Goop.log[level]) {
      window.Goop.log[level]('listen', msg);
    } else {
      (console[level] || console.log)('[listen]', msg);
    }
  }

  // ── HTTP API ─────────────────────────────────────────────────────────────────

  var api = {
    create: function (name) {
      return fetch("/api/listen/create", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: name }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    close: function () {
      return fetch("/api/listen/close", { method: "POST" }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    load: function (filePath) {
      return fetch("/api/listen/load", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_path: filePath }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    loadQueue: function (filePaths) {
      return fetch("/api/listen/load", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_paths: filePaths }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    addToQueue: function (filePaths) {
      return fetch("/api/listen/queue/add", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ file_paths: filePaths }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    control: function (action, position, index) {
      return fetch("/api/listen/control", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: action, position: position || 0, index: index || 0 }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    join: function (hostPeerId, groupId) {
      return fetch("/api/listen/join", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host_peer_id: hostPeerId, group_id: groupId }),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    leave: function () {
      return fetch("/api/listen/leave", { method: "POST" }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t); });
        return r.json();
      });
    },
    state: function () {
      return fetch("/api/listen/state").then(function (r) { return r.json(); });
    },

    // subscribe(callback) — MQ subscription for peer sites.
    // callback receives the group object (or null) on every state change.
    // Returns { close: function } to stop the subscription.
    subscribe: function (callback) {
      var unsub = null;
      function init() {
        if (!window.Goop || !window.Goop.mq) { setTimeout(init, 100); return; }
        unsub = Goop.mq.onListen( function(from, topic, payload, ack) {
          callback(payload && payload.group);
          ack();
        });
        api.state().then(function(data) { callback(data.group); });
      }
      init();
      return {
        close: function() { if (unsub) { unsub(); unsub = null; } },
      };
    },

    // streamURL() — returns the audio stream URL for use in <audio> elements.
    streamURL: function () {
      return "/api/listen/stream";
    },
  };

  // ── Audio / visualizer state ─────────────────────────────────────────────────

  var listenTimers = {};
  var listenAudioEl = null;
  var listenAudioCtx = null;
  var listenAnalyser = null;
  var listenAudioSource = null;
  var listenAnimFrame = null;

  var listenStallCheckInterval = null;
  var listenLastProgressTime = null;
  var listenLastProgressPos = -1;

  function formatTime(s) {
    if (!s || s < 0) s = 0;
    var m = Math.floor(s / 60);
    var sec = Math.floor(s % 60);
    return m + ':' + (sec < 10 ? '0' : '') + sec;
  }

  function ensureAudioEl() {
    if (!listenAudioEl) {
      listenAudioEl = document.createElement('audio');
      listenAudioEl.preload = 'none';
      listenAudioEl.classList.add('hidden');
      document.body.appendChild(listenAudioEl);
    }
    return listenAudioEl;
  }

  // Monitor audio stream for stalls: if playback doesn't advance for 3 seconds, close stream
  function startStallMonitor() {
    stopStallMonitor();
    var audio = listenAudioEl;
    if (!audio || audio.paused || !audio.src) return;

    listenStallCheckInterval = setInterval(function() {
      if (!audio || audio.paused || !audio.src) {
        stopStallMonitor();
        return;
      }

      var currentTime = audio.currentTime || 0;
      var now = Date.now();

      if (currentTime !== listenLastProgressPos) {
        listenLastProgressPos = currentTime;
        listenLastProgressTime = now;
        return;
      }

      if (listenLastProgressTime && (now - listenLastProgressTime) > 3000) {
        log('warn', 'stream stalled for 3s, closing connection');
        audio.pause();
        audio.src = '';
        stopStallMonitor();
      }
    }, 500);
  }

  function stopStallMonitor() {
    if (listenStallCheckInterval) {
      clearInterval(listenStallCheckInterval);
      listenStallCheckInterval = null;
    }
    listenLastProgressTime = null;
    listenLastProgressPos = -1;
  }

  function stopVisualizer() {
    if (listenAnimFrame) {
      cancelAnimationFrame(listenAnimFrame);
      listenAnimFrame = null;
    }
  }

  function startVisualizer(canvasEl) {
    stopVisualizer();
    if (!canvasEl) return;
    var audio = ensureAudioEl();
    if (!listenAudioCtx) {
      var AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return;
      listenAudioCtx = new AC();
    }
    listenAudioCtx.resume().catch(function() {});
    if (!listenAnalyser) {
      try {
        listenAudioSource = listenAudioCtx.createMediaElementSource(audio);
        listenAnalyser = listenAudioCtx.createAnalyser();
        listenAnalyser.fftSize = 256;
        listenAnalyser.smoothingTimeConstant = 0.75;
        listenAudioSource.connect(listenAnalyser);
        listenAnalyser.connect(listenAudioCtx.destination);
      } catch (e) {
        log('warn', 'visualizer setup failed: ' + e);
        return;
      }
    }
    var bufLen = listenAnalyser.frequencyBinCount;
    var data = new Uint8Array(bufLen);
    function draw() {
      listenAnimFrame = requestAnimationFrame(draw);
      var w = canvasEl.offsetWidth;
      var h = canvasEl.offsetHeight;
      if (!w || !h) return;
      if (canvasEl.width !== w) canvasEl.width = w;
      if (canvasEl.height !== h) canvasEl.height = h;
      listenAnalyser.getByteFrequencyData(data);
      var ctx2d = canvasEl.getContext('2d');
      ctx2d.clearRect(0, 0, w, h);
      var bins = 64;
      var slotW = w / bins;
      var barW = Math.max(1, slotW - 1);
      for (var i = 0; i < bins; i++) {
        var v = data[Math.floor(i * bufLen / bins)] / 255;
        var bh = Math.max(2, v * h);
        ctx2d.fillStyle = 'rgba(92,158,237,' + (0.3 + v * 0.7) + ')';
        ctx2d.fillRect(i * slotW, h - bh, barW, bh);
      }
    }
    draw();
  }

  // cleanupPlayerTimers — stops visualizer, audio element, and all progress timers.
  // Called by groups.js before re-rendering the groups list.
  function cleanupPlayerTimers() {
    stopVisualizer();
    stopStallMonitor();
    Object.keys(listenTimers).forEach(function(k) {
      clearInterval(listenTimers[k]);
      delete listenTimers[k];
    });
    if (listenAudioEl) {
      listenAudioEl.pause();
      listenAudioEl.src = '';
    }
  }

  // ── Host player renderer ─────────────────────────────────────────────────────

  function renderHostPlayer(wrapperEl, groupState) {
    var g = groupState;
    var gid = g && g.id;
    if (gid && listenTimers[gid]) { clearInterval(listenTimers[gid]); delete listenTimers[gid]; }

    var html = '';
    var bridgeURL = window.Goop && window.Goop.bridgeURL || '';

    html += '<div class="groups-listen-loader">';
    if (bridgeURL) {
      html += '<button class="groups-action-btn groups-btn-primary glisten-add-btn">&#128193; Add Files</button>';
    } else {
      html += '<input type="text" class="glisten-file" placeholder="/path/to/track.mp3" />' +
        '<button class="groups-action-btn groups-btn-primary glisten-load-btn">Load</button>';
    }
    html += '<input type="text" class="glisten-stream-url" placeholder="https://..." />' +
      '<button class="groups-action-btn groups-btn-secondary glisten-add-stream-btn">&#128225; Add Stream</button>';
    html += '</div>';

    if (g && g.queue_total > 0 && g.queue && g.queue.length > 0) {
      html += '<div class="glisten-queue">';
      g.queue.forEach(function(name, i) {
        var isCurrent = i === g.queue_index;
        var typeIcon = (g.queue_types && g.queue_types[i] === 'stream') ? '📡 ' : '';
        html += '<div class="glisten-queue-item' + (isCurrent ? ' current' : '') + '" data-queue-idx="' + i + '">' +
          '<span class="glisten-queue-num">' + (i + 1) + '</span>' +
          '<span class="glisten-queue-name">' + typeIcon + escapeHtml(name) + '</span>' +
          '<button class="glisten-queue-remove" title="Remove from queue">×</button>' +
          '</div>';
      });
      html += '</div>';
    }

    if (g && g.track) {
      var trackMeta = g.track.is_stream
        ? '<span class="glisten-live-badge">● LIVE</span>'
        : Math.round(g.track.bitrate / 1000) + ' kbps &middot; ' + formatTime(g.track.duration);
      html += '<div class="listen-player" style="margin-bottom:0">' +
        '<div class="listen-track-info">' +
          '<span class="listen-track-name">' + escapeHtml(g.track.name) + '</span>' +
          '<span class="listen-track-meta muted small">' +
            trackMeta +
          '</span>' +
        '</div>';
      if (!g.track.is_stream) {
        html += '<div class="listen-progress">' +
          '<div class="listen-progress-bar glisten-progress-bar">' +
            '<div class="listen-progress-fill glisten-progress-fill"></div>' +
          '</div>' +
          '<div class="listen-time">' +
            '<span class="glisten-time-current">0:00</span>' +
            '<span class="glisten-time-total">' + formatTime(g.track.duration) + '</span>' +
          '</div>' +
        '</div>';
      }
      html += '<div class="listen-controls">';

      var hasPrev = g.queue_total > 1;
      var hasNext = g.queue_total > 1 && g.queue_index < g.queue_total - 1;
      html += '<button class="listen-control-btn glisten-prev-btn" title="Previous"' + (hasPrev ? '' : ' disabled') + '>&#9664;&#9664;</button>';
      if (g.play_state && g.play_state.playing) {
        html += '<button class="listen-control-btn glisten-pause-btn" title="Pause">&#9646;&#9646;</button>';
      } else {
        html += '<button class="listen-control-btn glisten-play-btn" title="Play">&#9654;</button>';
      }
      html += '<button class="listen-control-btn glisten-next-btn" title="Next"' + (hasNext ? '' : ' disabled') + '>&#9654;&#9654;</button>';
      html += '<div class="listen-volume">' +
        '<label class="muted small">Volume</label>' +
        '<input type="range" class="glisten-volume" min="0" max="100" value="80" />' +
        '</div>';
      html += '</div>';

      if (g.listeners && g.listeners.length > 0) {
        html += '<div class="listen-listeners"><span class="listen-section-subtitle">Listeners</span>' +
          '<div class="listen-listener-list">';
        g.listeners.forEach(function(pid) {
          var label = (g.listener_names && g.listener_names[pid]) || pid.substring(0, 8) + '\u2026';
          html += '<span class="listen-listener-chip">' +
            '<img src="/api/avatar/peer/' + encodeURIComponent(pid) + '">' +
            '<span>' + escapeHtml(label) + '</span></span>';
        });
        html += '</div></div>';
      }
      html += '</div>';
    }

    wrapperEl.innerHTML = html;

    if (g && g.play_state && g.track && g.track.duration > 0 && !g.track.is_stream) {
      var fillEl = wrapperEl.querySelector('.glisten-progress-fill');
      var curEl = wrapperEl.querySelector('.glisten-time-current');
      var pos = g.play_state.position;
      if (fillEl) fillEl.style.width = Math.min(100, (pos / g.track.duration) * 100) + '%';
      if (curEl) curEl.textContent = formatTime(pos);
    }

    var addBtn = wrapperEl.querySelector('.glisten-add-btn');
    if (addBtn && bridgeURL) {
      on(addBtn, 'click', function() {
        fetch(bridgeURL + '/select-files?title=' + encodeURIComponent('Choose audio files'), { method: 'POST' })
          .then(function(r) { return r.json(); })
          .then(function(data) {
            if (!data.cancelled && data.paths && data.paths.length > 0) {
              api.addToQueue(data.paths).catch(function(e) {
                toast('Add failed: ' + e.message, true);
              });
            }
          })
          .catch(function(e) { toast('Browse failed: ' + e.message, true); });
      });
    }

    var loadBtn = wrapperEl.querySelector('.glisten-load-btn');
    var fileInput = wrapperEl.querySelector('.glisten-file');
    if (loadBtn && fileInput) {
      on(loadBtn, 'click', function() {
        var path = fileInput.value.trim();
        if (!path) return;
        api.addToQueue([path]).then(function() {
          fileInput.value = '';
        }).catch(function(e) { toast('Add failed: ' + e.message, true); });
      });
    }

    var streamInput = wrapperEl.querySelector('.glisten-stream-url');
    var addStreamBtn = wrapperEl.querySelector('.glisten-add-stream-btn');
    if (addStreamBtn && streamInput) {
      on(addStreamBtn, 'click', function() {
        var url = streamInput.value.trim();
        if (!url) return;
        api.addToQueue([url]).then(function() {
          streamInput.value = '';
        }).catch(function(e) { toast('Add stream failed: ' + e.message, true); });
      });
    }

    var volEl = wrapperEl.querySelector('.glisten-volume');
    if (volEl) {
      on(volEl, 'input', function() {
        ensureAudioEl().volume = volEl.value / 100;
      });
    }

    var prevBtn = wrapperEl.querySelector('.glisten-prev-btn');
    var nextBtn = wrapperEl.querySelector('.glisten-next-btn');
    if (prevBtn) {
      on(prevBtn, 'click', function() {
        stopStallMonitor();
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = '';
        api.control('prev').catch(function(e) { toast('Prev failed: ' + e.message, true); });
      });
    }
    if (nextBtn) {
      on(nextBtn, 'click', function() {
        stopStallMonitor();
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = '';
        api.control('next').catch(function(e) { toast('Next failed: ' + e.message, true); });
      });
    }

    var playBtn = wrapperEl.querySelector('.glisten-play-btn');
    var pauseBtn = wrapperEl.querySelector('.glisten-pause-btn');
    if (playBtn) {
      on(playBtn, 'click', function() {
        var audio = ensureAudioEl();
        audio.src = '/api/listen/stream';
        audio.volume = volEl ? volEl.value / 100 : 0.8;
        audio.load();
        audio.play().catch(function(e) { log('warn', 'host play failed: ' + e); });
        startStallMonitor();
        api.control('play').catch(function(e) {
          toast('Play failed: ' + e.message, true);
        });
      });
    }
    if (pauseBtn) {
      on(pauseBtn, 'click', function() {
        stopStallMonitor();
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = '';
        audio.load();
        api.control('pause').catch(function(e) { toast('Pause failed: ' + e.message, true); });
      });
    }

    wrapperEl.querySelectorAll('.glisten-queue-item').forEach(function(item) {
      var idx = parseInt(item.getAttribute('data-queue-idx'), 10);
      on(item, 'click', function(e) {
        if (e.target && e.target.classList.contains('glisten-queue-remove')) return;
        if (idx !== g.queue_index) {
          stopStallMonitor();
          var audio = ensureAudioEl();
          audio.pause();
          audio.src = '';
          api.control('skip', 0, idx).catch(function(e) { toast('Skip failed: ' + e.message, true); });
        }
      });
    });

    wrapperEl.querySelectorAll('.glisten-queue-remove').forEach(function(btn) {
      var item = btn.closest('.glisten-queue-item');
      var idx = parseInt(item.getAttribute('data-queue-idx'), 10);
      on(btn, 'click', function(e) {
        e.stopPropagation();
        api.control('remove', 0, idx).catch(function(e) { toast('Remove failed: ' + e.message, true); });
      });
    });

    if (g && g.play_state && g.play_state.playing) {
      var audio = ensureAudioEl();
      if (audio.paused || !audio.src) {
        audio.src = '/api/listen/stream';
        audio.volume = volEl ? volEl.value / 100 : 0.8;
        audio.play().catch(function(e) { log('warn', 'host autoplay failed: ' + e); });
        startStallMonitor();
      } else if (audio.src === '/api/listen/stream') {
        startStallMonitor();
      }
    }

    var progressBar = wrapperEl.querySelector('.glisten-progress-bar');
    if (progressBar && g && g.track && !g.track.is_stream) {
      on(progressBar, 'click', function(e) {
        var rect = progressBar.getBoundingClientRect();
        var pct = (e.clientX - rect.left) / rect.width;
        api.control('seek', pct * g.track.duration);
      });
    }

    if (g && g.play_state && g.play_state.playing && g.track && !g.track.is_stream) {
      var fillEl = wrapperEl.querySelector('.glisten-progress-fill');
      var curEl = wrapperEl.querySelector('.glisten-time-current');
      if (fillEl && curEl) {
        listenTimers[gid] = setInterval(function() {
          if (!g.play_state || !g.play_state.playing) return;
          var elapsed = (Date.now() - g.play_state.updated_at) / 1000;
          var pos = g.play_state.position + elapsed;
          var dur = g.track.duration;
          if (pos >= dur) { pos = dur; clearInterval(listenTimers[gid]); delete listenTimers[gid]; }
          fillEl.style.width = Math.min(100, (pos / dur) * 100) + '%';
          curEl.textContent = formatTime(pos);
        }, 250);
      }
    }
  }

  // ── Listener player renderer ─────────────────────────────────────────────────

  function renderListenerPlayer(wrapperEl, groupState) {
    var g = groupState;
    var gid = (g && g.id) || 'listener';
    if (listenTimers[gid]) { clearInterval(listenTimers[gid]); delete listenTimers[gid]; }

    if (!g || !g.track) {
      wrapperEl.innerHTML = '<div class="groups-listen-waiting">Waiting for host to play a track...</div>';
      stopVisualizer();
      return;
    }

    var trackMeta = g.track.is_stream
      ? '<span class="glisten-live-badge">● LIVE</span>'
      : Math.round(g.track.bitrate / 1000) + ' kbps &middot; ' + formatTime(g.track.duration);
    var html = '<div class="listen-player" style="margin-bottom:0">' +
      '<div class="listen-track-info">' +
        '<span class="listen-track-name">' + escapeHtml(g.track.name) + '</span>' +
        '<span class="listen-track-meta muted small">' +
          trackMeta +
        '</span>' +
      '</div>' +
      '<canvas class="glisten-wave"></canvas>';
    if (!g.track.is_stream) {
      html += '<div class="listen-progress">' +
        '<div class="listen-progress-bar">' +
          '<div class="listen-progress-fill glisten-progress-fill"></div>' +
        '</div>' +
        '<div class="listen-time">' +
          '<span class="glisten-time-current">0:00</span>' +
          '<span class="glisten-time-total">' + formatTime(g.track.duration) + '</span>' +
        '</div>' +
      '</div>';
    }
    html += '<div class="listen-controls">' +
      '<button class="glisten-play-fallback groups-action-btn groups-btn-primary" style="display:none">&#9654; Click to play</button>' +
      '<div class="listen-volume">' +
        '<label class="muted small">Volume</label>' +
        '<input type="range" class="glisten-volume" min="0" max="100" value="80" />' +
      '</div>' +
    '</div>' +
    '</div>';

    wrapperEl.innerHTML = html;

    if (g.play_state) {
      var fillEl = wrapperEl.querySelector('.glisten-progress-fill');
      var curEl = wrapperEl.querySelector('.glisten-time-current');
      if (fillEl && curEl && g.track.duration > 0 && !g.track.is_stream) {
        fillEl.style.width = Math.min(100, (g.play_state.position / g.track.duration) * 100) + '%';
        curEl.textContent = formatTime(g.play_state.position);
      }

      if (g.play_state.playing && g.track) {
        if (!g.track.is_stream) {
          listenTimers[gid] = setInterval(function() {
            if (!g.play_state || !g.play_state.playing) return;
            var elapsed = (Date.now() - g.play_state.updated_at) / 1000;
            var p = g.play_state.position + elapsed;
            var d = g.track.duration;
            if (p >= d) { p = d; clearInterval(listenTimers[gid]); delete listenTimers[gid]; }
            if (fillEl) fillEl.style.width = Math.min(100, (p / d) * 100) + '%';
            if (curEl) curEl.textContent = formatTime(p);
          }, 250);
        }

        var audio = ensureAudioEl();
        var playFallback = wrapperEl.querySelector('.glisten-play-fallback');
        if (!audio.src || audio.networkState !== 2) {
          audio.src = '';
          audio.src = '/api/listen/stream';
          audio.volume = 0.8;
          audio.play().catch(function(e) {
            log('warn', 'autoplay blocked: ' + e);
            if (playFallback) playFallback.classList.remove('hidden');
          });
        }
        if (playFallback) {
          on(playFallback, 'click', function() {
            playFallback.classList.add('hidden');
            audio.play().catch(function(e) { log('warn', 'manual play failed: ' + e); });
          });
        }
        startVisualizer(wrapperEl.querySelector('.glisten-wave'));
      } else {
        stopVisualizer();
        stopStallMonitor();
        var audioElement = ensureAudioEl();
        audioElement.pause();
        audioElement.src = '';
      }
    }

    var volEl = wrapperEl.querySelector('.glisten-volume');
    if (volEl) {
      var audio = ensureAudioEl();
      volEl.value = Math.round(audio.volume * 100);
      on(volEl, 'input', function() {
        ensureAudioEl().volume = volEl.value / 100;
      });
    }
  }

  // ── Public API ───────────────────────────────────────────────────────────────

  Goop.listen = Object.assign(api, {
    renderHostPlayer: renderHostPlayer,
    renderListenerPlayer: renderListenerPlayer,
    cleanupPlayerTimers: cleanupPlayerTimers,
  });
})();
