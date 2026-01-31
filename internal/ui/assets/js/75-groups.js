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
    return id.substring(0, 8) + "...";
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
          html += '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) + '</div>' +
              '<div class="groups-card-meta">ID: <code>' + escapeHtml(g.id) + '</code>' +
                (g.app_type ? ' &middot; ' + escapeHtml(g.app_type) : '') +
                (g.max_members > 0 ? ' &middot; max: ' + g.max_members : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + (g.member_count || 0) + ' members</div>' +
            '<div class="groups-card-actions">' +
              '<button class="groups-action-btn groups-btn-danger groups-close-btn" data-id="' + escapeHtml(g.id) + '">Close</button>' +
            '</div>' +
          '</div>';
        });
        hostedListEl.innerHTML = html;

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
        var html = "";
        subs.forEach(function(s) {
          html += '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(s.group_id) + '</div>' +
              '<div class="groups-card-meta">Host: <code>' + escapeHtml(shortId(s.host_peer_id)) + '</code>' +
                (s.role ? ' &middot; ' + escapeHtml(s.role) : '') +
              '</div>' +
            '</div>' +
          '</div>';
        });
        subListEl.innerHTML = html;
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
      var payload = "";
      try {
        payload = typeof evt.payload === "string" ? evt.payload : JSON.stringify(evt.payload);
      } catch (_) {}

      div.innerHTML = '<span class="evt-time">' + escapeHtml(time) + '</span>' +
        '<span class="evt-type">' + escapeHtml(evt.type) + '</span>' +
        (evt.from ? '<span class="evt-from">' + escapeHtml(shortId(evt.from)) + '</span>' : '') +
        (payload ? '<span>' + escapeHtml(payload).substring(0, 200) + '</span>' : '');

      eventsEl.insertBefore(div, eventsEl.firstChild);

      // Keep max 100 events
      while (eventsEl.children.length > 100) {
        eventsEl.removeChild(eventsEl.lastChild);
      }
    }
  }

  // -------- Create Groups page logic --------
  function initCreateGroupsPage() {
    var idInput = qs("#cg-id");
    var nameInput = qs("#cg-name");
    var appTypeInput = qs("#cg-apptype");
    var maxMembersInput = qs("#cg-maxmembers");
    var createBtn = qs("#cg-create-btn");
    var hostedListEl = qs("#cg-hosted-list");

    loadHostedList();

    on(createBtn, "click", function() {
      var id = (idInput.value || "").trim();
      var name = (nameInput.value || "").trim();
      var appType = (appTypeInput.value || "").trim();
      var maxMembers = parseInt(maxMembersInput.value, 10) || 0;

      if (!id) { toast("Group ID is required", true); return; }
      if (!name) { toast("Group name is required", true); return; }

      api("/api/groups", { id: id, name: name, app_type: appType, max_members: maxMembers }).then(function() {
        toast("Group created: " + name);
        idInput.value = "";
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
          html += '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) + '</div>' +
              '<div class="groups-card-meta">ID: <code>' + escapeHtml(g.id) + '</code>' +
                (g.app_type ? ' &middot; ' + escapeHtml(g.app_type) : '') +
                (g.max_members > 0 ? ' &middot; max: ' + g.max_members : '') +
              '</div>' +
            '</div>' +
            '<div class="groups-card-members">' + (g.member_count || 0) + ' members</div>' +
            '<div class="groups-card-actions">' +
              '<button class="groups-action-btn groups-btn-danger cg-close-btn" data-id="' + escapeHtml(g.id) + '">Close</button>' +
            '</div>' +
          '</div>';
        });
        hostedListEl.innerHTML = html;

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
