// Goop.groups — shared module for hosted groups and subscriptions.
// Loaded on every page; provides rendering, API calls, and listen player logic.
// Page-specific initialization lives in pages/groups.js and pages/create_groups.js.
(function() {
  window.Goop = window.Goop || {};
  var core = window.Goop.core;
  if (!core) return;

  var escapeHtml = core.escapeHtml;
  var api = core.api;
  var toast = core.toast;
  var on = core.on;

  // -------- Listen audio state (persists across re-renders) --------
  var listenSubs = {};
  var listenTimers = {};
  var listenAudioEl = null;
  var listenAudioCtx = null;
  var listenAnalyser = null;
  var listenAudioSource = null;
  var listenAnimFrame = null;

  // -------- Utilities --------
  function formatTime(s) {
    if (!s || s < 0) s = 0;
    var m = Math.floor(s / 60);
    var sec = Math.floor(s % 60);
    return m + ':' + (sec < 10 ? '0' : '') + sec;
  }

  function shortId(id) {
    if (!id || id.length <= 12) return id || '';
    return id.substring(0, 8) + '\u2026';
  }

  function memberLabel(n) {
    return n === 1 ? '1 member' : (n || 0) + ' members';
  }

  function typeBadge(appType) {
    if (!appType) return '';
    return ' <span class="groups-type-badge groups-type-' + escapeHtml(appType) + '">' + escapeHtml(appType) + '</span>';
  }

  function formatEventPayload(evt) {
    var p = evt.payload;
    if (!p) return '';
    if (evt.type === 'members' && p.members) {
      return p.members.length + ' member' + (p.members.length !== 1 ? 's' : '') + ' in group';
    }
    if (evt.type === 'welcome' && p.group_name) {
      return 'Joined ' + p.group_name + ' (' + (p.members ? p.members.length : 0) + ' members)';
    }
    if (evt.type === 'msg') {
      try {
        var s = typeof p === 'string' ? p : JSON.stringify(p);
        return s.length > 120 ? s.substring(0, 120) + '\u2026' : s;
      } catch (_) {}
    }
    return '';
  }

  // -------- Audio element --------
  var listenStallCheckInterval = null;
  var listenLastProgressTime = null;
  var listenLastProgressPos = -1;

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

      // Check if we've made progress
      if (currentTime !== listenLastProgressPos) {
        listenLastProgressPos = currentTime;
        listenLastProgressTime = now;
        return; // Progress detected, reset timeout
      }

      // No progress; check if stalled for 3+ seconds
      if (listenLastProgressTime && (now - listenLastProgressTime) > 3000) {
        // Stream stalled - host is likely gone
        console.warn('[LISTEN] Stream stalled for 3s, closing connection');
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

  // -------- Visualizer --------
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
        console.warn('LISTEN: visualizer setup failed:', e);
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

  function cleanupListenSubs() {
    stopVisualizer();
    Object.keys(listenSubs).forEach(function(k) {
      listenSubs[k].close();
      delete listenSubs[k];
    });
    Object.keys(listenTimers).forEach(function(k) {
      clearInterval(listenTimers[k]);
      delete listenTimers[k];
    });
  }

  // -------- Host player --------
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
          var label = (g.listener_names && g.listener_names[pid]) || shortId(pid);
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
              window.Goop.listen.addToQueue(data.paths).catch(function(e) {
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
        window.Goop.listen.addToQueue([path]).then(function() {
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
        window.Goop.listen.addToQueue([url]).then(function() {
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
        window.Goop.listen.control('prev').catch(function(e) { toast('Prev failed: ' + e.message, true); });
      });
    }
    if (nextBtn) {
      on(nextBtn, 'click', function() {
        stopStallMonitor();
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = '';
        window.Goop.listen.control('next').catch(function(e) { toast('Next failed: ' + e.message, true); });
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
        audio.play().catch(function(e) { console.warn('LISTEN host play:', e); });
        startStallMonitor();
        // Tell server to play (UI will update via SSE)
        window.Goop.listen.control('play').catch(function(e) {
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
        audio.load(); // Force reload to clear any buffered data
        // Tell server to pause (UI will update via SSE)
        window.Goop.listen.control('pause').catch(function(e) { toast('Pause failed: ' + e.message, true); });
      });
    }

    // Queue item click handlers (skip to track)
    wrapperEl.querySelectorAll('.glisten-queue-item').forEach(function(item) {
      var idx = parseInt(item.getAttribute('data-queue-idx'), 10);
      on(item, 'click', function(e) {
        // Don't skip if clicking the remove button
        if (e.target && e.target.classList.contains('glisten-queue-remove')) return;
        if (idx !== g.queue_index) {
          stopStallMonitor();
          var audio = ensureAudioEl();
          audio.pause();
          audio.src = '';
          window.Goop.listen.control('skip', 0, idx).catch(function(e) { toast('Skip failed: ' + e.message, true); });
        }
      });
    });

    // Queue item remove button handlers
    wrapperEl.querySelectorAll('.glisten-queue-remove').forEach(function(btn) {
      var item = btn.closest('.glisten-queue-item');
      var idx = parseInt(item.getAttribute('data-queue-idx'), 10);
      on(btn, 'click', function(e) {
        e.stopPropagation(); // Don't trigger the skip handler
        window.Goop.listen.control('remove', 0, idx).catch(function(e) { toast('Remove failed: ' + e.message, true); });
      });
    });

    if (g && g.play_state && g.play_state.playing) {
      var audio = ensureAudioEl();
      if (audio.paused || !audio.src) {
        audio.src = '/api/listen/stream';
        audio.volume = volEl ? volEl.value / 100 : 0.8;
        audio.play().catch(function(e) { console.warn('LISTEN host autoplay:', e); });
        startStallMonitor();
      } else if (audio.src === '/api/listen/stream') {
        // Already playing the stream, ensure monitor is active
        startStallMonitor();
      }
    }

    var progressBar = wrapperEl.querySelector('.glisten-progress-bar');
    if (progressBar && g && g.track && !g.track.is_stream) {
      on(progressBar, 'click', function(e) {
        var rect = progressBar.getBoundingClientRect();
        var pct = (e.clientX - rect.left) / rect.width;
        window.Goop.listen.control('seek', pct * g.track.duration);
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

  // -------- Listener player --------
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
            console.warn('LISTEN autoplay blocked:', e);
            if (playFallback) playFallback.classList.remove('hidden');
          });
        }
        if (playFallback) {
          on(playFallback, 'click', function() {
            playFallback.classList.add('hidden');
            audio.play().catch(function(e) { console.warn('LISTEN manual play failed:', e); });
          });
        }
        startVisualizer(wrapperEl.querySelector('.glisten-wave'));
      } else {
        // Host paused — close the audio stream
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

  // -------- Invite popup --------
  function showInvitePopup(groupId, btnEl) {
    var existing = document.querySelector('.groups-invite-popup');
    if (existing) existing.remove();

    var popup = document.createElement('div');
    popup.className = 'groups-invite-popup';
    popup.innerHTML = '<div class="groups-invite-loading">Loading peers...</div>';

    var rect = btnEl.getBoundingClientRect();
    popup.style.top = (rect.bottom + 6) + 'px';
    popup.style.right = (window.innerWidth - rect.right) + 'px';
    document.body.appendChild(popup);

    api('/api/peers').then(function(peers) {
      if (!peers || peers.length === 0) {
        popup.innerHTML = '<div class="groups-invite-empty">No peers online</div>';
        return;
      }
      var html = '<div class="groups-invite-title">Invite peer</div>';
      peers.forEach(function(p) {
        var label = p.Content || shortId(p.ID);
        html += '<button class="groups-invite-peer" data-peer="' + escapeHtml(p.ID) + '">' +
          '<span class="groups-invite-peer-name">' + escapeHtml(label) + '</span>' +
          (p.Email ? '<span class="groups-invite-peer-email">' + escapeHtml(p.Email) + '</span>' : '') +
        '</button>';
      });
      popup.innerHTML = html;

      popup.querySelectorAll('.groups-invite-peer').forEach(function(peerBtn) {
        on(peerBtn, 'click', function() {
          var peerId = peerBtn.getAttribute('data-peer');
          peerBtn.textContent = 'Inviting...';
          peerBtn.disabled = true;
          api('/api/groups/invite', { group_id: groupId, peer_id: peerId }).then(function() {
            toast('Invite sent to ' + shortId(peerId));
            popup.remove();
          }).catch(function(err) {
            toast('Invite failed: ' + err.message, true);
            popup.remove();
          });
        });
      });
    }).catch(function(err) {
      popup.innerHTML = '<div class="groups-invite-empty">Failed: ' + escapeHtml(err.message) + '</div>';
    });

    function closePopup(e) {
      if (!popup.contains(e.target) && e.target !== btnEl) {
        popup.remove();
        document.removeEventListener('click', closePopup);
      }
    }
    setTimeout(function() { document.addEventListener('click', closePopup); }, 0);
  }

  // -------- renderHostedGroups --------
  // opts.showMgmt  — show max-members input and kick buttons (groups page only)
  // When showMgmt is true, all cards are wrapped; otherwise only listen-type cards are.
  function renderHostedGroups(containerEl, opts) {
    opts = opts || {};
    var showMgmt = !!opts.showMgmt;

    cleanupListenSubs();
    api('/api/groups').then(function(groups) {
      if (!groups || groups.length === 0) {
        containerEl.innerHTML = '<p class="groups-empty">' +
          (showMgmt ? 'No hosted groups. Go to Create &gt; Groups to create one.' : 'No hosted groups yet.') +
          '</p>';
        return;
      }

      var html = '';
      var hasListen = false;

      groups.forEach(function(g) {
        var isListen = g.app_type === 'listen';
        if (isListen) hasListen = true;
        var doWrap = showMgmt || isListen;
        var joinBtn = g.host_in_group
          ? '<button class="groups-action-btn groups-btn-danger grph-leaveown-btn" data-id="' + escapeHtml(g.id) + '">Leave</button>'
          : '<button class="groups-action-btn groups-btn-primary grph-joinown-btn" data-id="' + escapeHtml(g.id) + '">Join</button>';
        var closeAttr = isListen ? ' data-listen="1"' : '';

        html += '<div class="' + (doWrap ? 'groups-card-wrap' : '') + '">' +
          '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) +
                typeBadge(g.app_type) +
                (g.host_in_group ? ' <span class="groups-status-connected">joined</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta">' +
                (showMgmt ? '<code>' : 'ID: <code>') + escapeHtml(shortId(g.id)) + '</code>' +
                (g.max_members > 0 ? ' &middot; max ' + g.max_members : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + memberLabel(g.member_count) + '</div>' +
            '<div class="groups-card-actions">' +
              joinBtn +
              '<button class="groups-action-btn grph-invite-btn" data-id="' + escapeHtml(g.id) + '">Invite</button>' +
              '<button class="groups-action-btn groups-btn-danger grph-close-btn" data-id="' + escapeHtml(g.id) + '"' + closeAttr + '>Close</button>' +
            '</div>' +
          '</div>';

        if (showMgmt) {
          var selfId = document.body.dataset.selfId || '';
          html += '<div class="groups-card-mgmt">' +
            '<div class="groups-mgmt-row">' +
              '<label class="groups-mgmt-label">Max <span class="muted small">(0=unlimited)</span></label>' +
              '<input type="number" class="groups-maxmembers-input" data-id="' + escapeHtml(g.id) + '" value="' + (g.max_members || 0) + '" min="0">' +
              '<button class="groups-action-btn grph-maxmembers-btn" data-id="' + escapeHtml(g.id) + '">Set</button>' +
            '</div>' +
            (g.members && g.members.length > 0
              ? '<div class="groups-member-list">' +
                  g.members.map(function(m) {
                    var isSelf = m.peer_id === selfId;
                    var label = m.name || shortId(m.peer_id);
                    return '<span class="groups-member-chip">' +
                      '<img src="/api/avatar/peer/' + encodeURIComponent(m.peer_id) + '">' +
                      '<span>' + escapeHtml(label) + '</span>' +
                      (!isSelf ? '<button class="groups-kick-btn" data-group="' + escapeHtml(g.id) + '" data-peer="' + escapeHtml(m.peer_id) + '" title="Remove">&#10005;</button>' : '') +
                    '</span>';
                  }).join('') +
                '</div>'
              : '') +
          '</div>';
        }

        if (isListen) {
          html += '<div class="groups-listen-player" data-group-id="' + escapeHtml(g.id) + '"></div>';
        }

        html += '</div>'; // wrap div
      });

      containerEl.innerHTML = html;

      // Bind join/leave
      containerEl.querySelectorAll('.grph-joinown-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          api('/api/groups/join-own', { group_id: btn.getAttribute('data-id') }).then(function() {
            toast('Joined group');
            renderHostedGroups(containerEl, opts);
          }).catch(function(err) { toast('Failed to join: ' + err.message, true); });
        });
      });
      containerEl.querySelectorAll('.grph-leaveown-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          api('/api/groups/leave-own', { group_id: btn.getAttribute('data-id') }).then(function() {
            toast('Left group');
            renderHostedGroups(containerEl, opts);
          }).catch(function(err) { toast('Failed to leave: ' + err.message, true); });
        });
      });

      // Bind invite
      containerEl.querySelectorAll('.grph-invite-btn').forEach(function(btn) {
        on(btn, 'click', function(e) {
          e.stopPropagation();
          showInvitePopup(btn.getAttribute('data-id'), btn);
        });
      });

      // Bind close
      containerEl.querySelectorAll('.grph-close-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          var id = btn.getAttribute('data-id');
          var isListenClose = btn.hasAttribute('data-listen');
          function doClose() {
            var p = isListenClose && window.Goop && window.Goop.listen
              ? window.Goop.listen.close()
              : api('/api/groups/close', { group_id: id });
            p.then(function() {
              toast('Group closed');
              renderHostedGroups(containerEl, opts);
            }).catch(function(err) { toast('Failed to close: ' + err.message, true); });
          }
          Goop.dialogs.confirm('Close group "' + id + '"? All members will be disconnected.', 'Close Group').then(function(ok) {
            if (ok) doClose();
          });
        });
      });

      // Bind management controls (groups page only)
      if (showMgmt) {
        containerEl.querySelectorAll('.grph-maxmembers-btn').forEach(function(btn) {
          on(btn, 'click', function() {
            var groupId = btn.getAttribute('data-id');
            var input = containerEl.querySelector('.groups-maxmembers-input[data-id="' + groupId + '"]');
            var max = input ? parseInt(input.value, 10) : 0;
            if (isNaN(max) || max < 0) max = 0;
            api('/api/groups/max-members', { group_id: groupId, max_members: max }).then(function() {
              toast('Max members updated');
              renderHostedGroups(containerEl, opts);
            }).catch(function(err) { toast('Update failed: ' + err.message, true); });
          });
        });
        containerEl.querySelectorAll('.groups-kick-btn').forEach(function(btn) {
          on(btn, 'click', function() {
            api('/api/groups/kick', { group_id: btn.getAttribute('data-group'), peer_id: btn.getAttribute('data-peer') }).then(function() {
              toast('Member removed');
              renderHostedGroups(containerEl, opts);
            }).catch(function(err) { toast('Kick failed: ' + err.message, true); });
          });
        });
      }

      // Listen player subscriptions
      if (hasListen && window.Goop && window.Goop.listen) {
        var subKey = containerEl.id || 'host-' + Math.random();
        window.Goop.listen.state().then(function(data) {
          var grp = data.group;
          if (grp) grp.listener_names = data.listener_names || {};
          containerEl.querySelectorAll('.groups-listen-player').forEach(function(el) {
            renderHostPlayer(el, grp);
          });
        });
        var sub = window.Goop.listen.subscribe(function(g) {
          containerEl.querySelectorAll('.groups-listen-player').forEach(function(el) {
            renderHostPlayer(el, g);
          });
        });
        if (listenSubs[subKey]) listenSubs[subKey].close();
        listenSubs[subKey] = sub;
      }
    }).catch(function(err) {
      containerEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
    });
  }

  // -------- renderSubscriptions --------
  function renderSubscriptions(containerEl) {
    api('/api/groups/subscriptions').then(function(data) {
      var subs = data.subscriptions;
      if (!subs || subs.length === 0) {
        containerEl.innerHTML = '<p class="groups-empty">No subscriptions. Use Goop.group.join() from a peer\'s site to join a group.</p>';
        return;
      }
      var activeGroupIds = {};
      (data.active_groups || []).forEach(function(ag) { activeGroupIds[ag.group_id] = true; });

      var html = '';
      var hasListenSub = false;

      subs.forEach(function(s) {
        var displayName = s.group_name || s.group_id;
        var isActive = !!activeGroupIds[s.group_id];
        var isListen = s.app_type === 'listen';
        var isFiles = s.app_type === 'files';
        if (isListen && isActive) hasListenSub = true;
        var cardDisabled = !isActive && !s.host_reachable;

        html += '<div class="' + (isListen && isActive ? 'groups-card-wrap' : '') + '">' +
          '<div class="groups-card' + (cardDisabled ? ' dimmed' : '') + '">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(displayName) +
                typeBadge(s.app_type) +
                (isActive ? ' <span class="groups-status-connected">connected</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta">Host: <code>' + escapeHtml(s.host_name || shortId(s.host_peer_id)) + '</code>' +
                (s.role ? ' &middot; ' + escapeHtml(s.role) : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + (s.member_count > 0 ? memberLabel(s.member_count) : '') + '</div>' +
            '<div class="groups-card-actions">' +
              '<span' + (cardDisabled ? ' inert' : '') + '>' +
                (isFiles ? '<a class="groups-action-btn groups-btn-primary" href="/documents?group_id=' + encodeURIComponent(s.group_id) + '">Browse Files</a>' : '') +
                (isActive
                  ? '<button class="groups-action-btn groups-btn-danger grph-leave-sub-btn" data-group="' + escapeHtml(s.group_id) + '">Leave</button>'
                  : '<button class="groups-action-btn groups-btn-primary grph-rejoin-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '"' + (s.host_reachable ? '' : ' disabled title="Host is offline"') + '>Rejoin</button>') +
              '</span>' +
              '<button class="groups-action-btn groups-btn-danger grph-remove-sub-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '">Remove</button>' +
            '</div>' +
          '</div>' +
          (isListen && isActive ? '<div class="groups-listen-player groups-listen-listener" data-group-id="' + escapeHtml(s.group_id) + '"></div>' : '') +
        '</div>';
      });

      containerEl.innerHTML = html;

      containerEl.querySelectorAll('.grph-leave-sub-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          var groupId = btn.getAttribute('data-group');
          btn.textContent = 'Leaving...';
          btn.disabled = true;
          api('/api/groups/leave', { group_id: groupId }).then(function() {
            toast('Left group');
            renderSubscriptions(containerEl);
          }).catch(function(err) {
            toast('Failed to leave: ' + err.message, true);
            btn.textContent = 'Leave';
            btn.disabled = false;
          });
        });
      });

      containerEl.querySelectorAll('.grph-rejoin-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          btn.textContent = 'Joining...';
          btn.disabled = true;
          api('/api/groups/rejoin', {
            host_peer_id: btn.getAttribute('data-host'),
            group_id: btn.getAttribute('data-group')
          }).then(function() {
            toast('Rejoined group');
            renderSubscriptions(containerEl);
          }).catch(function(err) {
            toast('Failed to rejoin: ' + err.message, true);
            btn.textContent = 'Rejoin';
            btn.disabled = false;
          });
        });
      });

      containerEl.querySelectorAll('.grph-remove-sub-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          api('/api/groups/subscriptions/remove', {
            host_peer_id: btn.getAttribute('data-host'),
            group_id: btn.getAttribute('data-group')
          }).then(function() {
            toast('Subscription removed');
            renderSubscriptions(containerEl);
          }).catch(function(err) { toast('Failed to remove: ' + err.message, true); });
        });
      });

      if (hasListenSub && window.Goop && window.Goop.listen) {
        var subKey = containerEl.id || 'listener-' + Math.random();
        window.Goop.listen.state().then(function(data) {
          var grp = data.group;
          if (grp) grp.listener_names = data.listener_names || {};
          containerEl.querySelectorAll('.groups-listen-listener').forEach(function(el) {
            renderListenerPlayer(el, grp);
          });
        });
        var sub = window.Goop.listen.subscribe(function(g) {
          containerEl.querySelectorAll('.groups-listen-listener').forEach(function(el) {
            renderListenerPlayer(el, g);
          });
        });
        if (listenSubs[subKey]) listenSubs[subKey].close();
        listenSubs[subKey] = sub;
      }
    }).catch(function(err) {
      containerEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
    });
  }

  window.Goop.groups = {
    renderHostedGroups: renderHostedGroups,
    renderSubscriptions: renderSubscriptions,
    showInvitePopup: showInvitePopup,
    stopListenSubs: cleanupListenSubs,
    shortId: shortId,
    formatEventPayload: formatEventPayload,
  };
})();
