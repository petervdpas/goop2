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

  function cleanupListenSubs() {
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
      listenAudioEl.style.display = "none";
      document.body.appendChild(listenAudioEl);
    }
    return listenAudioEl;
  }

  // Render host player section inside a group card wrapper
  function renderHostPlayer(wrapperEl, groupState) {
    var g = groupState;
    var html = '';

    // Track loader
    var bridgeURL = window.Goop && window.Goop.bridgeURL || '';
    html += '<div class="groups-listen-loader">' +
      '<input type="text" class="glisten-file" placeholder="/path/to/track.mp3" />';
    if (bridgeURL) {
      html += '<button class="groups-action-btn glisten-browse-btn" title="Browse for file">&#128193;</button>';
    }
    html += '<button class="groups-action-btn groups-btn-primary glisten-load-btn">Load</button>' +
    '</div>';

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

      if (g.play_state && g.play_state.playing) {
        html += '<button class="listen-control-btn glisten-pause-btn" title="Pause">&#9646;&#9646;</button>';
      } else {
        html += '<button class="listen-control-btn glisten-play-btn" title="Play">&#9654;</button>';
      }
      html += '</div>';

      // Listeners
      if (g.listeners && g.listeners.length > 0) {
        html += '<div class="listen-listeners"><span class="listen-section-subtitle">Listeners</span>' +
          '<div class="listen-listener-list">';
        g.listeners.forEach(function(pid) {
          html += '<span class="listen-listener-chip">' +
            '<img src="/api/avatar/peer/' + encodeURIComponent(pid) + '">' +
            '<span>' + escapeHtml(pid.substring(0, 12)) + '...</span></span>';
        });
        html += '</div></div>';
      }

      html += '</div>'; // close listen-player
    }

    wrapperEl.innerHTML = html;

    // Bind load button
    var loadBtn = wrapperEl.querySelector(".glisten-load-btn");
    var fileInput = wrapperEl.querySelector(".glisten-file");
    if (loadBtn && fileInput) {
      var browseBtn = wrapperEl.querySelector(".glisten-browse-btn");
      if (browseBtn && bridgeURL) {
        on(browseBtn, "click", function() {
          fetch(bridgeURL + '/select-file?title=' + encodeURIComponent('Choose audio file'), { method: 'POST' })
            .then(function(r) { return r.json(); })
            .then(function(data) {
              if (!data.cancelled && data.path) {
                fileInput.value = data.path;
              }
            })
            .catch(function() {});
        });
      }

      on(loadBtn, "click", function() {
        var path = fileInput.value.trim();
        if (!path) return;
        try {
          window.Goop.listen.load(path).catch(function(e) {
            toast("Load failed: " + e.message, true);
          });
        } catch(e) {
          toast("Load failed: " + e.message, true);
        }
      });
    }

    // Bind play/pause
    var playBtn = wrapperEl.querySelector(".glisten-play-btn");
    var pauseBtn = wrapperEl.querySelector(".glisten-pause-btn");
    if (playBtn) on(playBtn, "click", function() { window.Goop.listen.control("play"); });
    if (pauseBtn) on(pauseBtn, "click", function() { window.Goop.listen.control("pause"); });

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
        var gid = g.id;
        if (listenTimers[gid]) clearInterval(listenTimers[gid]);
        listenTimers[gid] = setInterval(function() {
          if (!g.play_state || !g.play_state.playing) return;
          var elapsed = (Date.now() - g.play_state.updated_at) / 1000;
          var pos = g.play_state.position + elapsed;
          var dur = g.track.duration;
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
    var html = '';

    if (!g || !g.track) {
      html = '<div class="groups-listen-waiting">Waiting for host to play a track...</div>';
      wrapperEl.innerHTML = html;
      return;
    }

    html += '<div class="listen-player" style="margin-bottom:0">' +
      '<div class="listen-track-info">' +
        '<span class="listen-track-name">' + escapeHtml(g.track.name) + '</span>' +
        '<span class="listen-track-meta muted small">' +
          Math.round(g.track.bitrate / 1000) + ' kbps &middot; ' + formatTime(g.track.duration) +
        '</span>' +
      '</div>' +
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
        var gid = g.id || "listener";
        if (listenTimers[gid]) clearInterval(listenTimers[gid]);
        listenTimers[gid] = setInterval(function() {
          if (!g.play_state || !g.play_state.playing) return;
          var elapsed = (Date.now() - g.play_state.updated_at) / 1000;
          var p = g.play_state.position + elapsed;
          var d = g.track.duration;
          if (fillEl) fillEl.style.width = Math.min(100, (p / d) * 100) + "%";
          if (curEl) curEl.textContent = formatTime(p);
        }, 250);

        // Connect audio
        var audio = ensureAudioEl();
        if (audio.paused || !audio.src) {
          audio.src = "/api/listen/stream";
          audio.volume = 0.8;
          audio.play().catch(function(e) { console.warn("LISTEN autoplay blocked:", e); });
        }
      }
    }

    // Volume control
    var volEl = wrapperEl.querySelector(".glisten-volume");
    if (volEl) {
      on(volEl, "input", function() {
        var audio = ensureAudioEl();
        audio.volume = volEl.value / 100;
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

    // Position relative to button
    btnEl.parentNode.style.position = "relative";
    btnEl.parentNode.appendChild(popup);

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
    var activeConnEl = qs("#groups-active-conn");
    var activeGroupEl = qs("#groups-active-group");
    var activeHostEl = qs("#groups-active-host");
    var leaveBtn = qs("#groups-leave-btn");
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

    on(leaveBtn, "click", function() {
      api("/api/groups/leave", {}).then(function() {
        toast("Left group");
        loadSubscriptions();
      }).catch(function(err) {
        toast("Failed to leave: " + err.message, true);
      });
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
          html += '<div class="' + (isListen ? 'groups-card-wrap' : '') + '">' +
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

        // Initialize listen players
        if (hasListen && window.Goop && window.Goop.listen) {
          window.Goop.listen.state().then(function(data) {
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, data.group);
            });
          });
          var sub = window.Goop.listen.subscribe(function(g) {
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, g);
            });
          });
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
        // Active connection
        if (data.active && data.active.connected) {
          activeConnEl.style.display = "flex";
          activeGroupEl.textContent = data.active.group_id;
          activeHostEl.textContent = shortId(data.active.host_peer_id);
        } else {
          activeConnEl.style.display = "none";
        }

        // Subscription list
        var subs = data.subscriptions;
        if (!subs || subs.length === 0) {
          subListEl.innerHTML = '<p class="groups-empty">No subscriptions. Use Goop.group.join() from a peer\'s site to join a group.</p>';
          return;
        }
        var activeGroupId = (data.active && data.active.connected) ? data.active.group_id : null;
        var html = "";
        var hasListenSub = false;
        subs.forEach(function(s) {
          var displayName = s.group_name || s.group_id;
          var isActive = activeGroupId && s.group_id === activeGroupId;
          var isListen = s.app_type === "listen";
          if (isListen && isActive) hasListenSub = true;
          html += '<div class="' + (isListen && isActive ? 'groups-card-wrap' : '') + '">' +
            '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(displayName) +
                typeBadge(s.app_type) +
                (isActive ? ' <span class="groups-status-connected">connected</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta">Host: <code>' + escapeHtml(shortId(s.host_peer_id)) + '</code>' +
                (s.role ? ' &middot; ' + escapeHtml(s.role) : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-actions">' +
              (!isActive ? '<button class="groups-action-btn groups-btn-primary groups-rejoin-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '">Rejoin</button>' : '') +
              '<button class="groups-action-btn groups-btn-danger groups-remove-sub-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '">Remove</button>' +
            '</div>' +
            '</div>' +
            (isListen && isActive ? '<div class="groups-listen-player groups-listen-listener" data-group-id="' + escapeHtml(s.group_id) + '"></div>' : '') +
          '</div>';
        });
        subListEl.innerHTML = html;

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
            subListEl.querySelectorAll(".groups-listen-listener").forEach(function(el) {
              renderListenerPlayer(el, data.group);
            });
          });
          var sub = window.Goop.listen.subscribe(function(g) {
            subListEl.querySelectorAll(".groups-listen-listener").forEach(function(el) {
              renderListenerPlayer(el, g);
            });
          });
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
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, data.group);
            });
          });
          var sub = window.Goop.listen.subscribe(function(g) {
            hostedListEl.querySelectorAll(".groups-listen-player").forEach(function(el) {
              renderHostPlayer(el, g);
            });
          });
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
