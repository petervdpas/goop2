// Goop.groups — shared module for hosted groups and subscriptions.
// Loaded on every page; provides group rendering and API calls.
// Listen player UI (audio, visualizer, renderHostPlayer/renderListenerPlayer) lives in listen.js.
// Page-specific initialization lives in pages/groups.js and pages/create_groups.js.
(function() {
  window.Goop = window.Goop || {};
  var core = window.Goop.core;
  if (!core) return;

  var escapeHtml = core.escapeHtml;
  var api = core.api;
  var toast = core.toast;
  var on = core.on;

  // SSE subscriptions per container (keyed by containerEl.id).
  // Each value is a { close: fn } object returned by Goop.listen.subscribe().
  var listenSubs = {};

  // -------- Utilities --------
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

  function cleanupListenSubs() {
    if (window.Goop && window.Goop.listen) {
      window.Goop.listen.cleanupPlayerTimers();
    }
    Object.keys(listenSubs).forEach(function(k) {
      listenSubs[k].close();
      delete listenSubs[k];
    });
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
            window.Goop.listen.renderHostPlayer(el, grp);
          });
        });
        var sub = window.Goop.listen.subscribe(function(g) {
          containerEl.querySelectorAll('.groups-listen-player').forEach(function(el) {
            window.Goop.listen.renderHostPlayer(el, g);
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
            window.Goop.listen.renderListenerPlayer(el, grp);
          });
        });
        var sub = window.Goop.listen.subscribe(function(g) {
          containerEl.querySelectorAll('.groups-listen-listener').forEach(function(el) {
            window.Goop.listen.renderListenerPlayer(el, g);
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
