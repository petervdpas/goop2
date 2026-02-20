// Admin viewer JS for the Groups and Create>Groups pages.
(() => {
  var core = window.Goop && window.Goop.core;
  if (!core) return;

  var qs = core.qs;
  var on = core.on;
  var escapeHtml = core.escapeHtml;
  var api = core.api;
  var toast = core.toast;

  // -------- Listen player helpers --------
  function formatTime(s) {
    if (!s || s < 0) s = 0;
    var m = Math.floor(s / 60);
    var sec = Math.floor(s % 60);
    return m + ":" + (sec < 10 ? "0" : "") + sec;
  }

  // Active listen subscriptions and timers, keyed by group id
  var listenSubs = {};
  var listenTimers = {};
  var listenAudioEl = null; // single shared <audio> element for listener mode

  // Web Audio API state — created once, reused across re-renders
  var listenAudioCtx    = null;
  var listenAnalyser    = null;
  var listenAudioSource = null; // MediaElementSource (can only be created once per element)
  var listenAnimFrame   = null;

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

    // Create AudioContext once
    if (!listenAudioCtx) {
      var AC = window.AudioContext || window.webkitAudioContext;
      if (!AC) return; // not supported
      listenAudioCtx = new AC();
    }
    listenAudioCtx.resume().catch(function() {});

    // Connect audio element → analyser → destination (once only)
    if (!listenAnalyser) {
      try {
        listenAudioSource = listenAudioCtx.createMediaElementSource(audio);
        listenAnalyser    = listenAudioCtx.createAnalyser();
        listenAnalyser.fftSize               = 256; // 128 frequency bins
        listenAnalyser.smoothingTimeConstant = 0.75;
        listenAudioSource.connect(listenAnalyser);
        listenAnalyser.connect(listenAudioCtx.destination);
      } catch(e) {
        console.warn("LISTEN: visualizer setup failed:", e);
        return;
      }
    }

    var bufLen = listenAnalyser.frequencyBinCount; // 128
    var data   = new Uint8Array(bufLen);

    function draw() {
      listenAnimFrame = requestAnimationFrame(draw);

      // Keep canvas pixel dimensions in sync with CSS layout size
      var w = canvasEl.offsetWidth;
      var h = canvasEl.offsetHeight;
      if (!w || !h) return;
      if (canvasEl.width  !== w) canvasEl.width  = w;
      if (canvasEl.height !== h) canvasEl.height = h;

      listenAnalyser.getByteFrequencyData(data);

      var ctx2d = canvasEl.getContext("2d");
      ctx2d.clearRect(0, 0, w, h);

      var bins = 64;
      var slotW = w / bins;
      var barW  = Math.max(1, slotW - 1);

      for (var i = 0; i < bins; i++) {
        var v  = data[Math.floor(i * bufLen / bins)] / 255;
        var bh = Math.max(2, v * h);
        ctx2d.fillStyle = "rgba(92,158,237," + (0.3 + v * 0.7) + ")";
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

  function ensureAudioEl() {
    if (!listenAudioEl) {
      listenAudioEl = document.createElement("audio");
      listenAudioEl.preload = "none";
      listenAudioEl.classList.add('hidden');
      document.body.appendChild(listenAudioEl);
    }
    return listenAudioEl;
  }

  // Render host player section inside a group card wrapper
  function renderHostPlayer(wrapperEl, groupState) {
    var g = groupState;
    // Always cancel any running timer before re-rendering
    var gid = g && g.id;
    if (gid && listenTimers[gid]) { clearInterval(listenTimers[gid]); delete listenTimers[gid]; }

    var html = '';
    var bridgeURL = window.Goop && window.Goop.bridgeURL || '';

    // Track loader — browse button when bridge is available, text input fallback
    html += '<div class="groups-listen-loader">';
    if (bridgeURL) {
      html += '<button class="groups-action-btn groups-btn-primary glisten-add-btn">&#128193; Add Files</button>';
    } else {
      html += '<input type="text" class="glisten-file" placeholder="/path/to/track.mp3" />' +
        '<button class="groups-action-btn groups-btn-primary glisten-load-btn">Load</button>';
    }
    html += '</div>';

    // Queue list
    if (g && g.queue_total > 0 && g.queue && g.queue.length > 0) {
      html += '<div class="glisten-queue">';
      g.queue.forEach(function(name, i) {
        var isCurrent = i === g.queue_index;
        html += '<div class="glisten-queue-item' + (isCurrent ? ' current' : '') + '">' +
          '<span class="glisten-queue-num">' + (i + 1) + '</span>' +
          '<span class="glisten-queue-name">' + escapeHtml(name) + '</span>' +
          '</div>';
      });
      html += '</div>';
    }

    if (g && g.track) {
      html += '<div class="listen-player" style="margin-bottom:0">' +
        '<div class="listen-track-info">' +
          '<span class="listen-track-name">' + escapeHtml(g.track.name) + '</span>' +
          '<span class="listen-track-meta muted small">' +
            Math.round(g.track.bitrate / 1000) + ' kbps &middot; ' + formatTime(g.track.duration) +
          '</span>' +
        '</div>' +
        '<div class="listen-progress">' +
          '<div class="listen-progress-bar glisten-progress-bar">' +
            '<div class="listen-progress-fill glisten-progress-fill"></div>' +
          '</div>' +
          '<div class="listen-time">' +
            '<span class="glisten-time-current">0:00</span>' +
            '<span class="glisten-time-total">' + formatTime(g.track.duration) + '</span>' +
          '</div>' +
        '</div>' +
        '<div class="listen-controls">';

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

      // Listeners
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

      html += '</div>'; // close listen-player
    }

    wrapperEl.innerHTML = html;

    // Initialise progress bar to current position (covers paused state)
    if (g && g.play_state && g.track && g.track.duration > 0) {
      var fillEl = wrapperEl.querySelector(".glisten-progress-fill");
      var curEl  = wrapperEl.querySelector(".glisten-time-current");
      var pos = g.play_state.position;
      if (fillEl) fillEl.style.width = Math.min(100, (pos / g.track.duration) * 100) + "%";
      if (curEl)  curEl.textContent  = formatTime(pos);
    }

    // Bind "Add Files" button (bridge mode) — appends to existing queue
    var addBtn = wrapperEl.querySelector(".glisten-add-btn");
    if (addBtn && bridgeURL) {
      on(addBtn, "click", function() {
        fetch(bridgeURL + '/select-files?title=' + encodeURIComponent('Choose audio files'), { method: 'POST' })
          .then(function(r) { return r.json(); })
          .then(function(data) {
            if (!data.cancelled && data.paths && data.paths.length > 0) {
              window.Goop.listen.addToQueue(data.paths).catch(function(e) {
                toast("Add failed: " + e.message, true);
              });
            }
          })
          .catch(function(e) { toast("Browse failed: " + e.message, true); });
      });
    }

    // Bind text-input load button (non-bridge fallback) — appends to queue
    var loadBtn = wrapperEl.querySelector(".glisten-load-btn");
    var fileInput = wrapperEl.querySelector(".glisten-file");
    if (loadBtn && fileInput) {
      on(loadBtn, "click", function() {
        var path = fileInput.value.trim();
        if (!path) return;
        window.Goop.listen.addToQueue([path]).then(function() {
          fileInput.value = "";
        }).catch(function(e) {
          toast("Add failed: " + e.message, true);
        });
      });
    }

    // Volume
    var volEl = wrapperEl.querySelector(".glisten-volume");
    if (volEl) {
      on(volEl, "input", function() {
        var audio = ensureAudioEl();
        audio.volume = volEl.value / 100;
      });
    }

    // Bind prev/next
    var prevBtn = wrapperEl.querySelector(".glisten-prev-btn");
    var nextBtn = wrapperEl.querySelector(".glisten-next-btn");
    if (prevBtn) {
      on(prevBtn, "click", function() {
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = ""; // disconnect pipe so blocked write unblocks immediately
        window.Goop.listen.control("prev").catch(function(e) { toast("Prev failed: " + e.message, true); });
      });
    }
    if (nextBtn) {
      on(nextBtn, "click", function() {
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = ""; // disconnect pipe so blocked write unblocks immediately
        window.Goop.listen.control("next").catch(function(e) { toast("Next failed: " + e.message, true); });
      });
    }

    // Bind play/pause — also manage host self-playback audio
    var playBtn = wrapperEl.querySelector(".glisten-play-btn");
    var pauseBtn = wrapperEl.querySelector(".glisten-pause-btn");
    if (playBtn) {
      on(playBtn, "click", function() {
        // Call Play API first so m.paused=false before AudioReader() is called.
        // AudioReader's goroutine checks !m.paused — if we set audio.src first
        // it runs while still paused and returns without writing anything.
        window.Goop.listen.control("play").then(function() {
          var audio = ensureAudioEl();
          audio.src = "/api/listen/stream";
          audio.volume = volEl ? volEl.value / 100 : 0.8;
          audio.load();
          audio.play().catch(function(e) { console.warn("LISTEN host play:", e); });
        });
      });
    }
    if (pauseBtn) {
      on(pauseBtn, "click", function() {
        var audio = ensureAudioEl();
        audio.pause();
        audio.src = ""; // disconnect so blocked pipe write unblocks immediately
        window.Goop.listen.control("pause");
      });
    }

    // If track is playing and audio isn't connected yet, connect it
    if (g && g.play_state && g.play_state.playing) {
      var audio = ensureAudioEl();
      if (audio.paused || !audio.src) {
        audio.src = "/api/listen/stream";
        audio.volume = volEl ? volEl.value / 100 : 0.8;
        audio.play().catch(function(e) { console.warn("LISTEN host autoplay:", e); });
      }
    }

    // Bind progress bar seeking
    var progressBar = wrapperEl.querySelector(".glisten-progress-bar");
    if (progressBar && g && g.track) {
      on(progressBar, "click", function(e) {
        var rect = progressBar.getBoundingClientRect();
        var pct = (e.clientX - rect.left) / rect.width;
        var pos = pct * g.track.duration;
        window.Goop.listen.control("seek", pos);
      });
    }

    // Start progress timer if playing
    if (g && g.play_state && g.play_state.playing && g.track) {
      var fillEl = wrapperEl.querySelector(".glisten-progress-fill");
      var curEl = wrapperEl.querySelector(".glisten-time-current");
      if (fillEl && curEl) {
        listenTimers[gid] = setInterval(function() {
          if (!g.play_state || !g.play_state.playing) return;
          var elapsed = (Date.now() - g.play_state.updated_at) / 1000;
          var pos = g.play_state.position + elapsed;
          var dur = g.track.duration;
          if (pos >= dur) {
            pos = dur;
            clearInterval(listenTimers[gid]);
            delete listenTimers[gid];
          }
          var pct = Math.min(100, (pos / dur) * 100);
          fillEl.style.width = pct + "%";
          curEl.textContent = formatTime(pos);
        }, 250);
      }
    }
  }

  // Render listener player section inside a subscription card wrapper
  function renderListenerPlayer(wrapperEl, groupState) {
    var g = groupState;
    // Always cancel any running timer before re-rendering, regardless of play state
    var gid = (g && g.id) || "listener";
    if (listenTimers[gid]) { clearInterval(listenTimers[gid]); delete listenTimers[gid]; }

    var html = '';

    if (!g || !g.track) {
      html = '<div class="groups-listen-waiting">Waiting for host to play a track...</div>';
      wrapperEl.innerHTML = html;
      stopVisualizer();
      return;
    }

    html += '<div class="listen-player" style="margin-bottom:0">' +
      '<div class="listen-track-info">' +
        '<span class="listen-track-name">' + escapeHtml(g.track.name) + '</span>' +
        '<span class="listen-track-meta muted small">' +
          Math.round(g.track.bitrate / 1000) + ' kbps &middot; ' + formatTime(g.track.duration) +
        '</span>' +
      '</div>' +
      '<canvas class="glisten-wave"></canvas>' +
      '<div class="listen-progress">' +
        '<div class="listen-progress-bar">' +
          '<div class="listen-progress-fill glisten-progress-fill"></div>' +
        '</div>' +
        '<div class="listen-time">' +
          '<span class="glisten-time-current">0:00</span>' +
          '<span class="glisten-time-total">' + formatTime(g.track.duration) + '</span>' +
        '</div>' +
      '</div>' +
      '<div class="listen-controls">' +
        '<button class="glisten-play-fallback groups-action-btn groups-btn-primary" style="display:none">&#9654; Click to play</button>' +
        '<div class="listen-volume">' +
          '<label class="muted small">Volume</label>' +
          '<input type="range" class="glisten-volume" min="0" max="100" value="80" />' +
        '</div>' +
      '</div>' +
    '</div>';

    wrapperEl.innerHTML = html;

    // Update progress
    if (g.play_state) {
      var fillEl = wrapperEl.querySelector(".glisten-progress-fill");
      var curEl = wrapperEl.querySelector(".glisten-time-current");
      if (fillEl && curEl && g.track.duration > 0) {
        var pos = g.play_state.position;
        var dur = g.track.duration;
        fillEl.style.width = Math.min(100, (pos / dur) * 100) + "%";
        curEl.textContent = formatTime(pos);
      }

      // Start timer if playing
      if (g.play_state.playing && g.track) {
        listenTimers[gid] = setInterval(function() {
          if (!g.play_state || !g.play_state.playing) return;
          var elapsed = (Date.now() - g.play_state.updated_at) / 1000;
          var p = g.play_state.position + elapsed;
          var d = g.track.duration;
          if (p >= d) {
            p = d;
            clearInterval(listenTimers[gid]);
            delete listenTimers[gid];
          }
          if (fillEl) fillEl.style.width = Math.min(100, (p / d) * 100) + "%";
          if (curEl) curEl.textContent = formatTime(p);
        }, 250);

        // Connect audio — reconnect if not actively loading/playing
        var audio = ensureAudioEl();
        var playFallback = wrapperEl.querySelector(".glisten-play-fallback");
        if (!audio.src || audio.networkState !== 2 /* NETWORK_LOADING */) {
          audio.src = "";
          audio.src = "/api/listen/stream";
          audio.volume = 0.8;
          audio.play().catch(function(e) {
            console.warn("LISTEN autoplay blocked:", e);
            if (playFallback) playFallback.classList.remove('hidden');
          });
        }
        if (playFallback) {
          on(playFallback, "click", function() {
            playFallback.classList.add('hidden');
            audio.play().catch(function(e) { console.warn("LISTEN manual play failed:", e); });
          });
        }

        // Start frequency visualizer
        startVisualizer(wrapperEl.querySelector(".glisten-wave"));
      } else {
        stopVisualizer();
      }
    }

    // Volume control — restore actual audio volume to the slider so
    // periodic re-renders (e.g. sync pulse) don't reset it to 80%.
    var volEl = wrapperEl.querySelector(".glisten-volume");
    if (volEl) {
      var audio = ensureAudioEl();
      volEl.value = Math.round(audio.volume * 100);
      on(volEl, "input", function() {
        ensureAudioEl().volume = volEl.value / 100;
      });
    }
  }

  // -------- Groups page (/self/groups) --------
  var groupsPage = qs("#groups-page");
  if (groupsPage) {
    initGroupsPage();
  }

  // -------- Create Groups page (/create/groups) --------
  var createPage = qs("#cg-create-form");
  if (createPage) {
    initCreateGroupsPage();
  }

  function shortId(id) {
    if (!id || id.length <= 12) return id || "";
    return id.substring(0, 8) + "\u2026";
  }

  function memberLabel(n) {
    return n === 1 ? "1 member" : (n || 0) + " members";
  }

  function typeBadge(appType) {
    if (!appType) return "";
    return ' <span class="groups-type-badge groups-type-' + escapeHtml(appType) + '">' + escapeHtml(appType) + '</span>';
  }

  // Format event payload for human-readable display
  function formatEventPayload(evt) {
    var p = evt.payload;
    if (!p) return "";
    if (evt.type === "members" && p.members) {
      return p.members.length + " member" + (p.members.length !== 1 ? "s" : "") + " in group";
    }
    if (evt.type === "welcome" && p.group_name) {
      return "Joined " + p.group_name + " (" + (p.members ? p.members.length : 0) + " members)";
    }
    if (evt.type === "msg") {
      try {
        var s = typeof p === "string" ? p : JSON.stringify(p);
        return s.length > 120 ? s.substring(0, 120) + "\u2026" : s;
      } catch (_) {}
    }
    return "";
  }

  // -------- Invite peer to group --------
  function showInvitePopup(groupId, btnEl) {
    // Remove any existing popup
    var existing = document.querySelector(".groups-invite-popup");
    if (existing) existing.remove();

    var popup = document.createElement("div");
    popup.className = "groups-invite-popup";
    popup.innerHTML = '<div class="groups-invite-loading">Loading peers...</div>';

    // Append to body with fixed positioning to escape overflow:hidden card containers
    var rect = btnEl.getBoundingClientRect();
    popup.style.top = (rect.bottom + 6) + "px";
    popup.style.right = (window.innerWidth - rect.right) + "px";
    document.body.appendChild(popup);

    // Fetch peer list
    api("/api/peers").then(function(peers) {
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

      popup.querySelectorAll(".groups-invite-peer").forEach(function(peerBtn) {
        on(peerBtn, "click", function() {
          var peerId = peerBtn.getAttribute("data-peer");
          peerBtn.textContent = "Inviting...";
          peerBtn.disabled = true;
          api("/api/groups/invite", { group_id: groupId, peer_id: peerId }).then(function() {
            toast("Invite sent to " + shortId(peerId));
            popup.remove();
          }).catch(function(err) {
            toast("Invite failed: " + err.message, true);
            popup.remove();
          });
        });
      });
    }).catch(function(err) {
      popup.innerHTML = '<div class="groups-invite-empty">Failed: ' + escapeHtml(err.message) + '</div>';
    });

    // Close on outside click
    function closePopup(e) {
      if (!popup.contains(e.target) && e.target !== btnEl) {
        popup.remove();
        document.removeEventListener("click", closePopup);
      }
    }
    setTimeout(function() { document.addEventListener("click", closePopup); }, 0);
  }

  // -------- Groups page logic --------
  function initGroupsPage() {
    var hostedListEl = qs("#groups-hosted-list");
    var subListEl = qs("#groups-sub-list");
    var refreshBtn = qs("#groups-refresh");
    var eventsEl = qs("#groups-events");
    var clearEventsBtn = qs("#groups-clear-events");

    loadHostedGroups();
    loadSubscriptions();
    startEventStream();

    on(refreshBtn, "click", function() {
      loadHostedGroups();
      loadSubscriptions();
    });

    on(clearEventsBtn, "click", function() {
      eventsEl.innerHTML = '<p class="groups-empty">Waiting for events...</p>';
    });

    function loadHostedGroups() {
      cleanupListenSubs();
      api("/api/groups").then(function(groups) {
        if (!groups || groups.length === 0) {
          hostedListEl.innerHTML = '<p class="groups-empty">No hosted groups. Go to Create &gt; Groups to create one.</p>';
          return;
        }
        var html = "";
        var hasListen = false;
        groups.forEach(function(g) {
          var isListen = g.app_type === "listen";
          if (isListen) hasListen = true;
          var joinBtn = g.host_in_group
            ? '<button class="groups-action-btn groups-btn-danger groups-leaveown-btn" data-id="' + escapeHtml(g.id) + '">Leave</button>'
            : '<button class="groups-action-btn groups-btn-primary groups-joinown-btn" data-id="' + escapeHtml(g.id) + '">Join</button>';
          var closeAttr = isListen ? ' data-listen="1"' : '';
          html += '<div class="groups-card-wrap">' +
            '<div class="groups-card">' +
              '<div class="groups-card-info">' +
                '<div class="groups-card-name">' + escapeHtml(g.name) +
                  typeBadge(g.app_type) +
                  (g.host_in_group ? ' <span class="groups-status-connected">joined</span>' : '') +
                '</div>' +
                '<div class="groups-card-meta"><code>' + escapeHtml(shortId(g.id)) + '</code>' +
                  (g.max_members > 0 ? ' &middot; max ' + g.max_members : '') +
                '</div>' +
              '</div>' +
              '<div class="groups-card-members">' + memberLabel(g.member_count) + '</div>' +
              '<div class="groups-card-actions">' +
                joinBtn +
                '<button class="groups-action-btn groups-invite-btn" data-id="' + escapeHtml(g.id) + '">Invite</button>' +
                '<button class="groups-action-btn groups-btn-danger groups-close-btn" data-id="' + escapeHtml(g.id) + '"' + closeAttr + '>Close</button>' +
              '</div>' +
            '</div>' +
            '<div class="groups-card-mgmt">' +
              '<div class="groups-mgmt-row">' +
                '<label class="groups-mgmt-label">Max <span class="muted small">(0=unlimited)</span></label>' +
                '<input type="number" class="groups-maxmembers-input" data-id="' + escapeHtml(g.id) + '" value="' + (g.max_members || 0) + '" min="0">' +
                '<button class="groups-action-btn groups-maxmembers-btn" data-id="' + escapeHtml(g.id) + '">Set</button>' +
              '</div>' +
              (g.members && g.members.length > 0
                ? '<div class="groups-member-list">' +
                    g.members.map(function(m) {
                      var selfId = document.body.dataset.selfId || '';
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
            '</div>' +
            (isListen ? '<div class="groups-listen-player" data-group-id="' + escapeHtml(g.id) + '"></div>' : '') +
          '</div>';
        });
        hostedListEl.innerHTML = html;

        // Bind join-own buttons
        hostedListEl.querySelectorAll(".groups-joinown-btn").forEach(function(btn) {
          on(btn, "click", function() {
            api("/api/groups/join-own", { group_id: btn.getAttribute("data-id") }).then(function() {
              toast("Joined group");
              loadHostedGroups();
            }).catch(function(err) {
              toast("Failed to join: " + err.message, true);
            });
          });
        });

        // Bind leave-own buttons
        hostedListEl.querySelectorAll(".groups-leaveown-btn").forEach(function(btn) {
          on(btn, "click", function() {
            api("/api/groups/leave-own", { group_id: btn.getAttribute("data-id") }).then(function() {
              toast("Left group");
              loadHostedGroups();
            }).catch(function(err) {
              toast("Failed to leave: " + err.message, true);
            });
          });
        });

        // Bind invite buttons
        hostedListEl.querySelectorAll(".groups-invite-btn").forEach(function(btn) {
          on(btn, "click", function(e) {
            e.stopPropagation();
            showInvitePopup(btn.getAttribute("data-id"), btn);
          });
        });

        // Bind close buttons
        var closeBtns = hostedListEl.querySelectorAll(".groups-close-btn");
        closeBtns.forEach(function(btn) {
          on(btn, "click", function() {
            var id = btn.getAttribute("data-id");
            var isListenClose = btn.hasAttribute("data-listen");
            if (window.Goop && window.Goop.ui && window.Goop.ui.confirm) {
              window.Goop.ui.confirm('Close group "' + id + '"? All members will be disconnected.', 'Close Group').then(function(ok) {
                if (ok) closeGroup(id, isListenClose);
              });
            } else if (confirm('Close group "' + id + '"?')) {
              closeGroup(id, isListenClose);
            }
          });
        });

        // Bind kick buttons
        hostedListEl.querySelectorAll(".groups-kick-btn").forEach(function(btn) {
          on(btn, "click", function() {
            var groupId = btn.getAttribute("data-group");
            var peerId = btn.getAttribute("data-peer");
            api("/api/groups/kick", { group_id: groupId, peer_id: peerId }).then(function() {
              toast("Member removed");
              loadHostedGroups();
            }).catch(function(err) { toast("Kick failed: " + err.message, true); });
          });
        });

        // Bind max-members set buttons
        hostedListEl.querySelectorAll(".groups-maxmembers-btn").forEach(function(btn) {
          on(btn, "click", function() {
            var groupId = btn.getAttribute("data-id");
            var input = hostedListEl.querySelector('.groups-maxmembers-input[data-id="' + groupId + '"]');
            var max = input ? parseInt(input.value, 10) : 0;
            if (isNaN(max) || max < 0) max = 0;
            api("/api/groups/max-members", { group_id: groupId, max_members: max }).then(function() {
              toast("Max members updated");
              loadHostedGroups();
            }).catch(function(err) { toast("Update failed: " + err.message, true); });
          });
        });

        // Initialize listen players
        if (hasListen && window.Goop && window.Goop.listen) {
          window.Goop.listen.state().then(function(data) {
            var grp = data.group;
            if (grp) grp.listener_names = data.listener_names || {};
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, grp);
            });
          });
          var sub = window.Goop.listen.subscribe(function(g) {
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, g);
            });
          });
          if (listenSubs["groups-host"]) listenSubs["groups-host"].close();
          listenSubs["groups-host"] = sub;
        }
      }).catch(function(err) {
        hostedListEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
      });
    }

    function closeGroup(id, isListen) {
      var p = isListen && window.Goop && window.Goop.listen
        ? window.Goop.listen.close()
        : api("/api/groups/close", { group_id: id });
      p.then(function() {
        toast("Group closed");
        loadHostedGroups();
      }).catch(function(err) {
        toast("Failed to close group: " + err.message, true);
      });
    }

    function loadSubscriptions() {
      api("/api/groups/subscriptions").then(function(data) {
        // Subscription list
        var subs = data.subscriptions;
        if (!subs || subs.length === 0) {
          subListEl.innerHTML = '<p class="groups-empty">No subscriptions. Use Goop.group.join() from a peer\'s site to join a group.</p>';
          return;
        }
        var activeGroupIds = {};
        (data.active_groups || []).forEach(function(ag) { activeGroupIds[ag.group_id] = true; });
        var html = "";
        var hasListenSub = false;
        subs.forEach(function(s) {
          var displayName = s.group_name || s.group_id;
          var isActive = !!activeGroupIds[s.group_id];
          var isListen = s.app_type === "listen";
          var isFiles = s.app_type === "files";
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
              (isFiles ? '<a class="groups-action-btn groups-btn-primary" href="/documents?group_id=' + encodeURIComponent(s.group_id) + '">Browse Files</a>' : '') +
              (isActive
                ? '<button class="groups-action-btn groups-btn-danger groups-leave-sub-btn" data-group="' + escapeHtml(s.group_id) + '">Leave</button>'
                : '<button class="groups-action-btn groups-btn-primary groups-rejoin-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '"' + (s.host_reachable ? '' : ' disabled title="Host is offline"') + '>Rejoin</button>') +
              '<button class="groups-action-btn groups-btn-danger groups-remove-sub-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '">Remove</button>' +
            '</div>' +
            '</div>' +
            (isListen && isActive ? '<div class="groups-listen-player groups-listen-listener" data-group-id="' + escapeHtml(s.group_id) + '"></div>' : '') +
          '</div>';
        });
        subListEl.innerHTML = html;

        // Bind leave buttons (active group)
        subListEl.querySelectorAll(".groups-leave-sub-btn").forEach(function(btn) {
          on(btn, "click", function() {
            var groupId = btn.getAttribute("data-group");
            btn.textContent = "Leaving...";
            btn.disabled = true;
            api("/api/groups/leave", { group_id: groupId }).then(function() {
              toast("Left group");
              loadSubscriptions();
            }).catch(function(err) {
              toast("Failed to leave: " + err.message, true);
              btn.textContent = "Leave";
              btn.disabled = false;
            });
          });
        });

        // Bind rejoin buttons
        subListEl.querySelectorAll(".groups-rejoin-btn").forEach(function(btn) {
          on(btn, "click", function() {
            btn.textContent = "Joining...";
            btn.disabled = true;
            api("/api/groups/rejoin", {
              host_peer_id: btn.getAttribute("data-host"),
              group_id: btn.getAttribute("data-group")
            }).then(function() {
              toast("Rejoined group");
              loadSubscriptions();
            }).catch(function(err) {
              toast("Failed to rejoin: " + err.message, true);
              btn.textContent = "Rejoin";
              btn.disabled = false;
            });
          });
        });

        // Bind remove buttons
        subListEl.querySelectorAll(".groups-remove-sub-btn").forEach(function(btn) {
          on(btn, "click", function() {
            api("/api/groups/subscriptions/remove", {
              host_peer_id: btn.getAttribute("data-host"),
              group_id: btn.getAttribute("data-group")
            }).then(function() {
              toast("Subscription removed");
              loadSubscriptions();
            }).catch(function(err) {
              toast("Failed to remove: " + err.message, true);
            });
          });
        });

        // Initialize listener player for listen-type subscriptions
        if (hasListenSub && window.Goop && window.Goop.listen) {
          window.Goop.listen.state().then(function(data) {
            var grp = data.group;
            if (grp) grp.listener_names = data.listener_names || {};
            subListEl.querySelectorAll(".groups-listen-listener").forEach(function(el) {
              renderListenerPlayer(el, grp);
            });
          });
          var sub = window.Goop.listen.subscribe(function(g) {
            subListEl.querySelectorAll(".groups-listen-listener").forEach(function(el) {
              renderListenerPlayer(el, g);
            });
          });
          if (listenSubs["groups-listener"]) listenSubs["groups-listener"].close();
          listenSubs["groups-listener"] = sub;
        }
      }).catch(function(err) {
        subListEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
      });
    }

    function startEventStream() {
      if (!window.Goop || !window.Goop.group) {
        setTimeout(startEventStream, 200);
        return;
      }
      window.Goop.group.subscribe(function(evt) {
        addEventToLog(evt);
        // Refresh lists on membership or close changes
        if (evt.type === "members" || evt.type === "close" || evt.type === "welcome" || evt.type === "leave") {
          loadHostedGroups();
          loadSubscriptions();
        }
        // When an invite arrives, refresh subscriptions so the new entry appears.
        // The global notifier (06-notify.js) handles the toast on all pages.
        if (evt.type === "invite") {
          loadSubscriptions();
        }
      });
    }

    function addEventToLog(evt) {
      // Clear placeholder
      var placeholder = qs(".groups-empty", eventsEl);
      if (placeholder) placeholder.remove();

      var div = document.createElement("div");
      div.className = "groups-event-item";

      var time = new Date().toLocaleTimeString();
      var payload = formatEventPayload(evt);
      if (!payload) {
        try {
          payload = typeof evt.payload === "string" ? evt.payload : JSON.stringify(evt.payload);
          if (payload && payload.length > 120) payload = payload.substring(0, 120) + "\u2026";
        } catch (_) {}
      }

      div.innerHTML = '<span class="evt-time">' + escapeHtml(time) + '</span>' +
        '<span class="evt-type">' + escapeHtml(evt.type) + '</span>' +
        (evt.from ? '<span class="evt-from">' + escapeHtml(shortId(evt.from)) + '</span>' : '') +
        (payload ? '<span>' + escapeHtml(payload) + '</span>' : '');

      eventsEl.insertBefore(div, eventsEl.firstChild);

      // Keep max 100 events
      while (eventsEl.children.length > 100) {
        eventsEl.removeChild(eventsEl.lastChild);
      }
    }
  }

  // -------- Create Groups page logic --------
  function initCreateGroupsPage() {
    var gsel = window.Goop.select;
    var nameInput = qs("#cg-name");
    var appTypeSelect = qs("#cg-apptype");
    var maxMembersInput = qs("#cg-maxmembers");
    var createBtn = qs("#cg-create-btn");
    var hostedListEl = qs("#cg-hosted-list");

    gsel.init(appTypeSelect);
    loadHostedList();

    on(createBtn, "click", function() {
      var name = (nameInput.value || "").trim();
      var appType = (gsel.val(appTypeSelect) || "general").trim();
      var maxMembers = parseInt(maxMembersInput.value, 10) || 0;

      if (!name) { toast("Group name is required", true); return; }

      if (appType === "listen" && window.Goop && window.Goop.listen) {
        window.Goop.listen.create(name).then(function() {
          toast("Listen group created: " + name);
          nameInput.value = "";
          gsel.setVal(appTypeSelect, "general");
          maxMembersInput.value = "0";
          loadHostedList();
        }).catch(function(err) {
          toast("Failed to create listen group: " + err.message, true);
        });
      } else {
        api("/api/groups", { name: name, app_type: appType, max_members: maxMembers }).then(function() {
          toast("Group created: " + name);
          nameInput.value = "";
          gsel.setVal(appTypeSelect, "general");
          maxMembersInput.value = "0";
          loadHostedList();
        }).catch(function(err) {
          toast("Failed to create group: " + err.message, true);
        });
      }
    });

    function loadHostedList() {
      cleanupListenSubs();
      api("/api/groups").then(function(groups) {
        if (!groups || groups.length === 0) {
          hostedListEl.innerHTML = '<p class="groups-empty">No hosted groups yet.</p>';
          return;
        }
        var html = "";
        var hasListen = false;
        groups.forEach(function(g) {
          var isListen = g.app_type === "listen";
          if (isListen) hasListen = true;
          var joinBtn = g.host_in_group
            ? '<button class="groups-action-btn groups-btn-danger cg-leaveown-btn" data-id="' + escapeHtml(g.id) + '">Leave</button>'
            : '<button class="groups-action-btn groups-btn-primary cg-joinown-btn" data-id="' + escapeHtml(g.id) + '">Join</button>';
          var closeAttr = isListen ? ' data-listen="1"' : '';
          html += '<div class="' + (isListen ? 'groups-card-wrap' : '') + '">' +
            '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) +
                typeBadge(g.app_type) +
                (g.host_in_group ? ' <span class="groups-status-connected">joined</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta">ID: <code>' + escapeHtml(shortId(g.id)) + '</code>' +
                (g.max_members > 0 ? ' &middot; max: ' + g.max_members : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + memberLabel(g.member_count) + '</div>' +
            '<div class="groups-card-actions">' +
              joinBtn +
              '<button class="groups-action-btn cg-invite-btn" data-id="' + escapeHtml(g.id) + '">Invite</button>' +
              '<button class="groups-action-btn groups-btn-danger cg-close-btn" data-id="' + escapeHtml(g.id) + '"' + closeAttr + '>Close</button>' +
            '</div>' +
            '</div>' +
            (isListen ? '<div class="groups-listen-player" data-group-id="' + escapeHtml(g.id) + '"></div>' : '') +
          '</div>';
        });
        hostedListEl.innerHTML = html;

        // Bind join-own buttons
        hostedListEl.querySelectorAll(".cg-joinown-btn").forEach(function(btn) {
          on(btn, "click", function() {
            api("/api/groups/join-own", { group_id: btn.getAttribute("data-id") }).then(function() {
              toast("Joined group");
              loadHostedList();
            }).catch(function(err) {
              toast("Failed to join: " + err.message, true);
            });
          });
        });

        // Bind leave-own buttons
        hostedListEl.querySelectorAll(".cg-leaveown-btn").forEach(function(btn) {
          on(btn, "click", function() {
            api("/api/groups/leave-own", { group_id: btn.getAttribute("data-id") }).then(function() {
              toast("Left group");
              loadHostedList();
            }).catch(function(err) {
              toast("Failed to leave: " + err.message, true);
            });
          });
        });

        // Bind invite buttons
        hostedListEl.querySelectorAll(".cg-invite-btn").forEach(function(btn) {
          on(btn, "click", function(e) {
            e.stopPropagation();
            showInvitePopup(btn.getAttribute("data-id"), btn);
          });
        });

        // Bind close buttons
        var closeBtns = hostedListEl.querySelectorAll(".cg-close-btn");
        closeBtns.forEach(function(btn) {
          on(btn, "click", function() {
            var id = btn.getAttribute("data-id");
            var isListenClose = btn.hasAttribute("data-listen");
            if (window.Goop && window.Goop.ui && window.Goop.ui.confirm) {
              window.Goop.ui.confirm('Close group "' + id + '"? All members will be disconnected.', 'Close Group').then(function(ok) {
                if (ok) doClose(id, isListenClose);
              });
            } else if (confirm('Close group "' + id + '"?')) {
              doClose(id, isListenClose);
            }
          });
        });

        // Initialize listen players
        if (hasListen && window.Goop && window.Goop.listen) {
          window.Goop.listen.state().then(function(data) {
            var grp = data.group;
            if (grp) grp.listener_names = data.listener_names || {};
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, grp);
            });
          });
          var sub = window.Goop.listen.subscribe(function(g) {
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, g);
            });
          });
          if (listenSubs["cg-host"]) listenSubs["cg-host"].close();
          listenSubs["cg-host"] = sub;
        }
      }).catch(function(err) {
        hostedListEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
      });
    }

    function doClose(id, isListen) {
      var p = isListen && window.Goop && window.Goop.listen
        ? window.Goop.listen.close()
        : api("/api/groups/close", { group_id: id });
      p.then(function() {
        toast("Group closed");
        loadHostedList();
      }).catch(function(err) {
        toast("Failed to close group: " + err.message, true);
      });
    }
  }
})();
