// Listen group — host & listener UI logic
(function () {
  if (!window.Goop) window.Goop = {};

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
    control: function (action, position) {
      return fetch("/api/listen/control", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ action: action, position: position || 0 }),
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

    // subscribe(callback) — SSE subscription for peer sites.
    // callback receives the group object (or null) on every state change.
    // Returns { close: function } to stop the subscription.
    subscribe: function (callback) {
      var es = new EventSource("/api/listen/events");
      es.addEventListener("state", function (e) {
        try {
          var data = JSON.parse(e.data);
          callback(data.group);
        } catch (err) {
          console.error("LISTEN subscribe parse error:", err);
        }
      });
      es.addEventListener("connected", function () {
        // Fetch initial state on connect
        api.state().then(function (data) {
          callback(data.group);
        });
      });
      return {
        close: function () {
          es.close();
        },
      };
    },

    // streamURL() — returns the audio stream URL for use in <audio> elements.
    streamURL: function () {
      return "/api/listen/stream";
    },
  };

  Goop.listen = api;

  // ── Only run UI logic on the listen page ──────────────────────────────────
  if (!document.getElementById("listen-page")) return;

  var idle = document.getElementById("listen-idle");
  var hostEl = document.getElementById("listen-host");
  var listenerEl = document.getElementById("listen-listener");
  var joinSection = document.getElementById("listen-join-section");

  // Host elements
  var hostName = document.getElementById("listen-host-name");
  var groupIdEl = document.getElementById("listen-group-id");
  var playerEl = document.getElementById("listen-player");
  var trackNameEl = document.getElementById("listen-track-name");
  var trackMetaEl = document.getElementById("listen-track-meta");
  var progressFill = document.getElementById("listen-progress-fill");
  var timeCurrent = document.getElementById("listen-time-current");
  var timeTotal = document.getElementById("listen-time-total");
  var playBtn = document.getElementById("listen-play-btn");
  var pauseBtn = document.getElementById("listen-pause-btn");
  var listenerList = document.getElementById("listen-listener-list");

  // Listener elements
  var lGroupName = document.getElementById("listen-listener-group-name");
  var lPlayerEl = document.getElementById("listen-listener-player");
  var lTrackName = document.getElementById("listen-listener-track-name");
  var lTrackMeta = document.getElementById("listen-listener-track-meta");
  var lProgressFill = document.getElementById("listen-listener-progress-fill");
  var lTimeCurrent = document.getElementById("listen-listener-time-current");
  var lTimeTotal = document.getElementById("listen-listener-time-total");
  var lWaiting = document.getElementById("listen-listener-waiting");
  var audioEl = document.getElementById("listen-audio");
  var volumeEl = document.getElementById("listen-volume");

  var currentGroup = null;
  var progressTimer = null;

  function formatTime(s) {
    if (!s || s < 0) s = 0;
    var m = Math.floor(s / 60);
    var sec = Math.floor(s % 60);
    return m + ":" + (sec < 10 ? "0" : "") + sec;
  }

  function showMode(mode) {
    idle.style.display = mode === "idle" ? "" : "none";
    hostEl.style.display = mode === "host" ? "" : "none";
    listenerEl.style.display = mode === "listener" ? "" : "none";
    joinSection.style.display = mode === "idle" ? "" : "none";
  }

  function updateProgress(pos, duration, fillEl, curEl, totEl) {
    if (!duration || duration <= 0) return;
    var pct = Math.min(100, (pos / duration) * 100);
    fillEl.style.width = pct + "%";
    curEl.textContent = formatTime(pos);
    totEl.textContent = formatTime(duration);
  }

  function startProgressTimer() {
    stopProgressTimer();
    progressTimer = setInterval(function () {
      if (!currentGroup || !currentGroup.play_state || !currentGroup.play_state.playing) return;
      var ps = currentGroup.play_state;
      var elapsed = (Date.now() - ps.updated_at) / 1000;
      var pos = ps.position + elapsed;
      var dur = currentGroup.track ? currentGroup.track.duration : 0;

      if (currentGroup.role === "host") {
        updateProgress(pos, dur, progressFill, timeCurrent, timeTotal);
      } else {
        updateProgress(pos, dur, lProgressFill, lTimeCurrent, lTimeTotal);
      }
    }, 250);
  }

  function stopProgressTimer() {
    if (progressTimer) {
      clearInterval(progressTimer);
      progressTimer = null;
    }
  }

  function renderGroup(g) {
    currentGroup = g;
    stopProgressTimer();

    if (!g) {
      showMode("idle");
      return;
    }

    if (g.role === "host") {
      showMode("host");
      hostName.textContent = g.name;
      groupIdEl.textContent = g.id;

      if (g.track) {
        playerEl.style.display = "";
        trackNameEl.textContent = g.track.name;
        trackMetaEl.textContent =
          Math.round(g.track.bitrate / 1000) + " kbps · " + formatTime(g.track.duration);

        if (g.play_state) {
          if (g.play_state.playing) {
            playBtn.style.display = "none";
            pauseBtn.style.display = "";
            startProgressTimer();
          } else {
            playBtn.style.display = "";
            pauseBtn.style.display = "none";
            var dur = g.track.duration;
            updateProgress(g.play_state.position, dur, progressFill, timeCurrent, timeTotal);
          }
        }
      } else {
        playerEl.style.display = "none";
      }

      // Render listeners
      if (g.listeners && g.listeners.length > 0) {
        listenerList.innerHTML = "";
        g.listeners.forEach(function (pid) {
          var chip = document.createElement("span");
          chip.className = "listen-listener-chip";
          var img = document.createElement("img");
          img.src = "/api/avatar/peer/" + encodeURIComponent(pid);
          chip.appendChild(img);
          var label = document.createElement("span");
          label.textContent = pid.substring(0, 12) + "...";
          chip.appendChild(label);
          listenerList.appendChild(chip);
        });
      } else {
        listenerList.innerHTML = '<span class="muted small">No listeners yet.</span>';
      }
    } else {
      // Listener mode
      showMode("listener");
      lGroupName.textContent = g.name;

      if (g.track) {
        lPlayerEl.style.display = "";
        lWaiting.style.display = "none";
        lTrackName.textContent = g.track.name;
        lTrackMeta.textContent =
          Math.round(g.track.bitrate / 1000) + " kbps · " + formatTime(g.track.duration);

        if (g.play_state) {
          var dur = g.track.duration;
          updateProgress(g.play_state.position, dur, lProgressFill, lTimeCurrent, lTimeTotal);
          if (g.play_state.playing) {
            startProgressTimer();
            connectAudio();
          }
        }
      } else {
        lPlayerEl.style.display = "none";
        lWaiting.style.display = "";
      }
    }
  }

  function connectAudio() {
    if (!audioEl) return;
    if (audioEl.src && !audioEl.paused) return; // already playing
    audioEl.src = "/api/listen/stream";
    audioEl.volume = (volumeEl ? volumeEl.value : 80) / 100;
    audioEl.play().catch(function (e) {
      console.warn("LISTEN: autoplay blocked:", e);
    });
  }

  function disconnectAudio() {
    if (!audioEl) return;
    audioEl.pause();
    audioEl.removeAttribute("src");
    audioEl.load();
  }

  // ── Event handlers ────────────────────────────────────────────────────────

  document.getElementById("listen-create-btn").addEventListener("click", function () {
    var name = document.getElementById("listen-group-name").value.trim() || "Listening Group";
    api.create(name).then(function (g) {
      renderGroup(g);
    }).catch(function (e) {
      if (window.Goop && Goop.toast) Goop.toast({ message: e.message, type: "error" });
    });
  });

  document.getElementById("listen-close-btn").addEventListener("click", function () {
    api.close().then(function () {
      disconnectAudio();
      renderGroup(null);
    });
  });

  document.getElementById("listen-load-btn").addEventListener("click", function () {
    var path = document.getElementById("listen-file-path").value.trim();
    if (!path) return;
    api.load(path).catch(function (e) {
      if (window.Goop && Goop.toast) Goop.toast({ message: e.message, type: "error" });
    });
  });

  playBtn.addEventListener("click", function () {
    api.control("play");
  });

  pauseBtn.addEventListener("click", function () {
    api.control("pause");
  });

  // Progress bar seeking (host only)
  document.getElementById("listen-progress-bar").addEventListener("click", function (e) {
    if (!currentGroup || currentGroup.role !== "host" || !currentGroup.track) return;
    var rect = this.getBoundingClientRect();
    var pct = (e.clientX - rect.left) / rect.width;
    var pos = pct * currentGroup.track.duration;
    api.control("seek", pos);
  });

  document.getElementById("listen-join-btn").addEventListener("click", function () {
    var hostId = document.getElementById("listen-join-host").value.trim();
    var groupId = document.getElementById("listen-join-group").value.trim();
    if (!hostId || !groupId) return;
    api.join(hostId, groupId).catch(function (e) {
      if (window.Goop && Goop.toast) Goop.toast({ message: e.message, type: "error" });
    });
  });

  document.getElementById("listen-leave-btn").addEventListener("click", function () {
    disconnectAudio();
    api.leave().then(function () {
      renderGroup(null);
    });
  });

  if (volumeEl) {
    volumeEl.addEventListener("input", function () {
      if (audioEl) audioEl.volume = this.value / 100;
    });
  }

  // ── SSE subscription ──────────────────────────────────────────────────────

  var sse = new EventSource("/api/listen/events");

  sse.addEventListener("state", function (e) {
    try {
      var data = JSON.parse(e.data);
      renderGroup(data.group);
    } catch (err) {
      console.error("LISTEN SSE parse error:", err);
    }
  });

  sse.addEventListener("connected", function () {
    // Fetch initial state
    api.state().then(function (data) {
      renderGroup(data.group);
    });
  });

  sse.onerror = function () {
    console.warn("LISTEN: SSE connection lost, will retry...");
  };

  // Initial state fetch
  api.state().then(function (data) {
    renderGroup(data.group);
  });
})();
