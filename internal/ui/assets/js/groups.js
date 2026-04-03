// Goop.groups — shared module for hosted groups and subscriptions.
// Loaded on every page; provides group rendering and API calls.
// Listen player UI (audio, visualizer, renderHostPlayer/renderListenerPlayer) lives in listen.js.
// Page-specific initialization lives in pages/groups.js and pages/create_groups.js.
(function() {
  window.Goop = window.Goop || {};
  var core = window.Goop.core;
  if (!core) return;

  var escapeHtml = core.escapeHtml;
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

  function typeBadge(groupType) {
    if (!groupType) return '';
    return ' <span class="badge badge-' + escapeHtml(groupType) + '">' + escapeHtml(groupType) + '</span>';
  }

  function formatEventPayload(evt) {
    if (!evt) return '';
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
    popup.className = 'groups-invite-popup scroll-bounded';
    popup.innerHTML = '<div class="groups-invite-loading">Loading peers...</div>';

    var rect = btnEl.getBoundingClientRect();
    popup.style.top = (rect.bottom + 6) + 'px';
    popup.style.right = (window.innerWidth - rect.right) + 'px';
    document.body.appendChild(popup);

    Goop.api.peers.list().then(function(peers) {
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
          Goop.api.groups.invite({ group_id: groupId, peer_id: peerId }).then(function() {
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
    Goop.api.groups.list().then(function(groups) {
      if (!groups || groups.length === 0) {
        containerEl.innerHTML = '<p class="empty-state">' +
          (showMgmt ? 'No hosted groups. Go to Create &gt; Groups to create one.' : 'No hosted groups yet.') +
          '</p>';
        return;
      }

      var html = '';
      var hasListen = false;

      groups.forEach(function(g) {
        var isListen = g.group_type === 'listen';
        if (isListen) hasListen = true;
        var doWrap = showMgmt || isListen;
        var joinBtn = '';
        if (g.host_can_join !== false) {
          joinBtn = g.host_in_group
            ? '<button class="groups-action-btn groups-btn-danger grph-leaveown-btn" data-id="' + escapeHtml(g.id) + '">Leave</button>'
            : '<button class="groups-action-btn groups-btn-primary grph-joinown-btn" data-id="' + escapeHtml(g.id) + '">Join</button>';
        }
        var closeAttr = isListen ? ' data-listen="1"' : '';

        html += '<div class="' + (doWrap ? 'groups-card-wrap' : '') + '">' +
          '<div class="groups-card">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(g.name) +
                typeBadge(g.group_type) +
                (g.host_in_group ? ' <span class="badge badge-connected">joined</span>' : '') +
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
          var roles = g.roles || [];
          var roleOpts = roles.map(function(r) { return escapeHtml(r); });
          var hasRoles = g.group_type === 'template' || g.group_type === 'general' || g.group_type === '' || g.group_type === 'message';
          var gid = escapeHtml(g.id);

          html += '<div class="groups-card-mgmt">';

          html += '<div class="groups-settings-toggle section-toggle" data-panel-open="0" data-panel-remember="grp-' + gid + '">&#9881; Settings</div>' +
            '<div class="groups-settings">' +
              '<div class="groups-settings-row">' +
                '<span class="groups-settings-label">Max members</span>' +
                '<input type="number" class="groups-maxmembers-input" data-id="' + gid + '" value="' + (g.max_members || 0) + '" min="0" title="0 = unlimited">' +
              '</div>';

          if (hasRoles) {
            html += '<div class="groups-settings-row">' +
                '<span class="groups-settings-label">Default role</span>' +
                (roleOpts.length > 0
                  ? '<select class="groups-default-role-select" data-id="' + gid + '">' +
                      roleOpts.map(function(r) {
                        return '<option value="' + r + '"' + (r === escapeHtml(g.default_role || 'viewer') ? ' selected' : '') + '>' + r + '</option>';
                      }).join('') +
                    '</select>'
                  : '<input type="text" class="groups-default-role-input" data-id="' + gid + '" value="' + escapeHtml(g.default_role || 'viewer') + '" placeholder="viewer">') +
              '</div>' +
              '<div class="groups-settings-row">' +
                '<span class="groups-settings-label">Roles</span>' +
                '<div class="groups-role-tags" data-id="' + gid + '">' +
                  roles.map(function(r) {
                    return '<span class="groups-role-tag">' + escapeHtml(r) +
                      '<button class="groups-role-tag-rm" data-id="' + gid + '" data-role="' + escapeHtml(r) + '">&#10005;</button></span>';
                  }).join('') +
                  '<input class="groups-role-tag-add" data-id="' + gid + '" placeholder="+ add" size="6">' +
                '</div>' +
              '</div>';
          }

          html += '</div>';

          // Members
          if (g.members && g.members.length > 0) {
            html += '<table class="groups-member-table">' +
              '<tbody>' +
              g.members.map(function(m) {
                var isSelf = m.peer_id === selfId;
                var label = m.name || shortId(m.peer_id);
                var roleCell = '';
                if (hasRoles) {
                  if (isSelf) {
                    roleCell = '<td class="gmt-role"><span class="badge badge-role">' + escapeHtml(m.role || 'owner') + '</span></td>';
                  } else if (roleOpts.length > 0) {
                    roleCell = '<td class="gmt-role"><select class="groups-member-role-select" data-group="' + gid + '" data-peer="' + escapeHtml(m.peer_id) + '">' +
                      roleOpts.map(function(r) {
                        return '<option value="' + r + '"' + (r === escapeHtml(m.role || '') ? ' selected' : '') + '>' + r + '</option>';
                      }).join('') +
                    '</select></td>';
                  } else {
                    roleCell = '<td class="gmt-role"><span class="badge badge-role">' + escapeHtml(m.role || 'viewer') + '</span></td>';
                  }
                }
                return '<tr>' +
                  '<td class="gmt-avatar"><img class="groups-member-avatar" src="/api/avatar/peer/' + encodeURIComponent(m.peer_id) + '"></td>' +
                  '<td class="gmt-name">' + escapeHtml(label) + '</td>' +
                  roleCell +
                  '<td class="gmt-actions">' + (!isSelf ? '<button class="groups-kick-btn" data-group="' + gid + '" data-peer="' + escapeHtml(m.peer_id) + '" title="Remove">&#10005;</button>' : '') + '</td>' +
                '</tr>';
              }).join('') +
              '</tbody></table>';
          }

          html += '</div>';
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
          Goop.api.groups.joinOwn({ group_id: btn.getAttribute('data-id') }).then(function() {
            toast('Joined group');
            renderHostedGroups(containerEl, opts);
          }).catch(function(err) { toast('Failed to join: ' + err.message, true); });
        });
      });
      containerEl.querySelectorAll('.grph-leaveown-btn').forEach(function(btn) {
        on(btn, 'click', function() {
          Goop.api.groups.leaveOwn({ group_id: btn.getAttribute('data-id') }).then(function() {
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
              : Goop.api.groups.close({ group_id: id });
            p.then(function() {
              toast('Group closed');
              renderHostedGroups(containerEl, opts);
            }).catch(function(err) { toast('Failed to close: ' + err.message, true); });
          }
          Goop.dialog.confirm('Close group "' + id + '"? All members will be disconnected.', 'Close Group').then(function(ok) {
            if (ok) doClose();
          });
        });
      });

      // Bind management controls (groups page only)
      if (showMgmt) {
        Goop.panel.initAll(containerEl);

        // Max members — auto-save on change
        containerEl.querySelectorAll('.groups-maxmembers-input').forEach(function(input) {
          on(input, 'change', function() {
            var max = parseInt(input.value, 10);
            if (isNaN(max) || max < 0) max = 0;
            Goop.api.groups.setMaxMembers({ group_id: input.getAttribute('data-id'), max_members: max }).then(function() {
              toast('Max members updated');
            }).catch(function(err) { toast('Update failed: ' + err.message, true); });
          });
        });

        // Default role — auto-save on change
        containerEl.querySelectorAll('.groups-default-role-select').forEach(function(sel) {
          on(sel, 'change', function() {
            Goop.api.groups.setDefaultRole({ group_id: sel.getAttribute('data-id'), default_role: sel.value }).catch(function(err) { toast('Update failed: ' + err.message, true); });
          });
        });
        containerEl.querySelectorAll('.groups-default-role-input').forEach(function(input) {
          on(input, 'change', function() {
            var role = input.value.trim();
            if (!role) return;
            Goop.api.groups.setDefaultRole({ group_id: input.getAttribute('data-id'), default_role: role }).catch(function(err) { toast('Update failed: ' + err.message, true); });
          });
        });

        // Role tags — remove
        containerEl.querySelectorAll('.groups-role-tag-rm').forEach(function(btn) {
          on(btn, 'click', function() {
            var gid = btn.getAttribute('data-id');
            var role = btn.getAttribute('data-role');
            var tagsEl = containerEl.querySelector('.groups-role-tags[data-id="' + gid + '"]');
            var current = [];
            tagsEl.querySelectorAll('.groups-role-tag').forEach(function(t) {
              var r = t.textContent.replace('\u2715', '').trim();
              if (r && r !== role) current.push(r);
            });
            Goop.api.groups.setGroupRoles({ group_id: gid, roles: current }).then(function() {
              renderHostedGroups(containerEl, opts);
            }).catch(function(err) { toast('Update failed: ' + err.message, true); });
          });
        });

        // Role tags — add on Enter
        containerEl.querySelectorAll('.groups-role-tag-add').forEach(function(input) {
          on(input, 'keydown', function(e) {
            if (e.key !== 'Enter') return;
            e.preventDefault();
            var role = input.value.trim();
            if (!role) return;
            var gid = input.getAttribute('data-id');
            var tagsEl = containerEl.querySelector('.groups-role-tags[data-id="' + gid + '"]');
            var current = [];
            tagsEl.querySelectorAll('.groups-role-tag').forEach(function(t) {
              current.push(t.textContent.replace('\u2715', '').trim());
            });
            if (current.indexOf(role) !== -1) { input.value = ''; return; }
            current.push(role);
            Goop.api.groups.setGroupRoles({ group_id: gid, roles: current }).then(function() {
              renderHostedGroups(containerEl, opts);
            }).catch(function(err) { toast('Update failed: ' + err.message, true); });
          });
        });

        // Member role — auto-save on change
        containerEl.querySelectorAll('.groups-member-role-select').forEach(function(sel) {
          on(sel, 'change', function() {
            Goop.api.groups.setRole({ group_id: sel.getAttribute('data-group'), peer_id: sel.getAttribute('data-peer'), role: sel.value }).then(function() {
              toast('Role updated');
            }).catch(function(err) { toast('Set role failed: ' + err.message, true); });
          });
        });

        // Kick
        containerEl.querySelectorAll('.groups-kick-btn').forEach(function(btn) {
          on(btn, 'click', function() {
            Goop.api.groups.kick({ group_id: btn.getAttribute('data-group'), peer_id: btn.getAttribute('data-peer') }).then(function() {
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
      containerEl.innerHTML = '<p class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</p>';
    });
  }

  // -------- renderSubscriptions --------
  function renderSubscriptions(containerEl) {
    Goop.api.groups.subscriptions().then(function(data) {
      var subs = data.subscriptions;
      if (!subs || subs.length === 0) {
        containerEl.innerHTML = '<p class="empty-state">No subscriptions. Use Goop.group.join() from a peer\'s site to join a group.</p>';
        return;
      }
      var activeGroupIds = {};
      (data.active_groups || []).forEach(function(ag) { activeGroupIds[ag.group_id] = true; });

      var html = '';
      var hasListenSub = false;

      subs.forEach(function(s) {
        var displayName = s.group_name || s.group_id;
        var isActive = !!activeGroupIds[s.group_id];
        var isListen = s.group_type === 'listen';
        var isFiles = s.group_type === 'files';
        if (isListen && isActive) hasListenSub = true;
        var cardDisabled = !isActive && !s.host_reachable;

        html += '<div class="' + (isListen && isActive ? 'groups-card-wrap' : '') + '">' +
          '<div class="groups-card' + (cardDisabled ? ' dimmed' : '') + '">' +
            '<div class="groups-card-info">' +
              '<div class="groups-card-name">' + escapeHtml(displayName) +
                typeBadge(s.group_type) +
                (isActive ? ' <span class="badge badge-connected">connected</span>' : '') +
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
          Goop.api.groups.leave({ group_id: groupId }).then(function() {
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
          Goop.api.groups.rejoin({
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
          Goop.api.groups.removeSubscription({
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
      containerEl.innerHTML = '<p class="empty-state">Failed to load: ' + escapeHtml(err.message) + '</p>';
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
