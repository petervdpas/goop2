// internal/ui/assets/js/75-groups.js
// Admin viewer JS for the Groups and Create>Groups pages.
(() => {
  var core = window.Goop && window.Goop.core;
  if (!core) return;

  var qs = core.qs;
  var on = core.on;

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

  function toast(msg, isError) {
    if (window.Goop && window.Goop.ui && window.Goop.ui.toast) {
      window.Goop.ui.toast({
        title: isError ? "Error" : "Success",
        message: msg,
        duration: isError ? 6000 : 3000,
      });
    }
  }

  function escapeHtml(str) {
    var d = document.createElement("div");
    d.textContent = String(str == null ? "" : str);
    return d.innerHTML;
  }

  function shortId(id) {
    if (!id || id.length <= 12) return id || "";
    return id.substring(0, 8) + "\u2026";
  }

  function memberLabel(n) {
    return n === 1 ? "1 member" : (n || 0) + " members";
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

  function api(url, body) {
    return fetch(url, {
      method: body !== undefined ? "POST" : "GET",
      headers: body !== undefined ? { "Content-Type": "application/json" } : {},
      body: body !== undefined ? JSON.stringify(body) : undefined,
    }).then(function(resp) {
      if (!resp.ok) return resp.text().then(function(t) { throw new Error(t || resp.statusText); });
      var ct = resp.headers.get("Content-Type") || "";
      if (ct.indexOf("application/json") !== -1) return resp.json();
      return null;
    });
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
      api("/api/groups").then(function(groups) {
        if (!groups || groups.length === 0) {
          hostedListEl.innerHTML = '<p class="groups-empty">No hosted groups. Go to Create &gt; Groups to create one.</p>';
          return;
        }
        var html = "";
        groups.forEach(function(g) {
          var joinBtn = g.host_in_group
            ? '<button class="groups-action-btn groups-btn-danger groups-leaveown-btn" data-id="' + escapeHtml(g.id) + '">Leave</button>'
            : '<button class="groups-action-btn groups-btn-primary groups-joinown-btn" data-id="' + escapeHtml(g.id) + '">Join</button>';
          html += '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) +
                (g.host_in_group ? ' <span class="groups-status-connected">joined</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta"><code>' + escapeHtml(shortId(g.id)) + '</code>' +
                (g.app_type ? ' &middot; ' + escapeHtml(g.app_type) : '') +
                (g.max_members > 0 ? ' &middot; max ' + g.max_members : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + memberLabel(g.member_count) + '</div>' +
            '<div class="groups-card-actions">' +
              joinBtn +
              '<button class="groups-action-btn groups-invite-btn" data-id="' + escapeHtml(g.id) + '">Invite</button>' +
              '<button class="groups-action-btn groups-btn-danger groups-close-btn" data-id="' + escapeHtml(g.id) + '">Close</button>' +
            '</div>' +
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
            if (window.Goop && window.Goop.ui && window.Goop.ui.confirm) {
              window.Goop.ui.confirm('Close group "' + id + '"? All members will be disconnected.', 'Close Group').then(function(ok) {
                if (ok) closeGroup(id);
              });
            } else if (confirm('Close group "' + id + '"?')) {
              closeGroup(id);
            }
          });
        });
      }).catch(function(err) {
        hostedListEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
      });
    }

    function closeGroup(id) {
      api("/api/groups/close", { group_id: id }).then(function() {
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
        subs.forEach(function(s) {
          var displayName = s.group_name || s.group_id;
          var isActive = activeGroupId && s.group_id === activeGroupId;
          html += '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(displayName) +
                (isActive ? ' <span class="groups-status-connected">connected</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta">Host: <code>' + escapeHtml(shortId(s.host_peer_id)) + '</code>' +
                (s.app_type ? ' &middot; ' + escapeHtml(s.app_type) : '') +
                (s.role ? ' &middot; ' + escapeHtml(s.role) : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-actions">' +
              (!isActive ? '<button class="groups-action-btn groups-btn-primary groups-rejoin-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '">Rejoin</button>' : '') +
              '<button class="groups-action-btn groups-btn-danger groups-remove-sub-btn" data-host="' + escapeHtml(s.host_peer_id) + '" data-group="' + escapeHtml(s.group_id) + '">Remove</button>' +
            '</div>' +
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
    var nameInput = qs("#cg-name");
    var appTypeInput = qs("#cg-apptype");
    var maxMembersInput = qs("#cg-maxmembers");
    var createBtn = qs("#cg-create-btn");
    var hostedListEl = qs("#cg-hosted-list");

    loadHostedList();

    on(createBtn, "click", function() {
      var name = (nameInput.value || "").trim();
      var appType = (appTypeInput.value || "").trim();
      var maxMembers = parseInt(maxMembersInput.value, 10) || 0;

      if (!name) { toast("Group name is required", true); return; }

      api("/api/groups", { name: name, app_type: appType, max_members: maxMembers }).then(function() {
        toast("Group created: " + name);
        nameInput.value = "";
        appTypeInput.value = "";
        maxMembersInput.value = "0";
        loadHostedList();
      }).catch(function(err) {
        toast("Failed to create group: " + err.message, true);
      });
    });

    function loadHostedList() {
      api("/api/groups").then(function(groups) {
        if (!groups || groups.length === 0) {
          hostedListEl.innerHTML = '<p class="groups-empty">No hosted groups yet.</p>';
          return;
        }
        var html = "";
        groups.forEach(function(g) {
          var joinBtn = g.host_in_group
            ? '<button class="groups-action-btn groups-btn-danger cg-leaveown-btn" data-id="' + escapeHtml(g.id) + '">Leave</button>'
            : '<button class="groups-action-btn groups-btn-primary cg-joinown-btn" data-id="' + escapeHtml(g.id) + '">Join</button>';
          html += '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) +
                (g.host_in_group ? ' <span class="groups-status-connected">joined</span>' : '') +
              '</div>' +
              '<div class="groups-card-meta">ID: <code>' + escapeHtml(shortId(g.id)) + '</code>' +
                (g.app_type ? ' &middot; ' + escapeHtml(g.app_type) : '') +
                (g.max_members > 0 ? ' &middot; max: ' + g.max_members : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + memberLabel(g.member_count) + '</div>' +
            '<div class="groups-card-actions">' +
              joinBtn +
              '<button class="groups-action-btn cg-invite-btn" data-id="' + escapeHtml(g.id) + '">Invite</button>' +
              '<button class="groups-action-btn groups-btn-danger cg-close-btn" data-id="' + escapeHtml(g.id) + '">Close</button>' +
            '</div>' +
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
            if (window.Goop && window.Goop.ui && window.Goop.ui.confirm) {
              window.Goop.ui.confirm('Close group "' + id + '"? All members will be disconnected.', 'Close Group').then(function(ok) {
                if (ok) doClose(id);
              });
            } else if (confirm('Close group "' + id + '"?')) {
              doClose(id);
            }
          });
        });
      }).catch(function(err) {
        hostedListEl.innerHTML = '<p class="groups-empty">Failed to load: ' + escapeHtml(err.message) + '</p>';
      });
    }

    function doClose(id) {
      api("/api/groups/close", { group_id: id }).then(function() {
        toast("Group closed");
        loadHostedList();
      }).catch(function(err) {
        toast("Failed to close group: " + err.message, true);
      });
    }
  }
})();
