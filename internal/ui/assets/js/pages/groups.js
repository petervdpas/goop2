// Groups tabs: hosted, joined, events pages.
// Detects which elements are present and initializes accordingly.
(function() {
  var g = window.Goop && window.Goop.groups;
  var core = window.Goop && window.Goop.core;
  if (!g || !core) return;

  var escapeHtml = core.escapeHtml;

  // ── Hosted tab (/groups/hosted) ──────────────────────────────────────────
  var hostedListEl = document.getElementById('cg-hosted-list');
  var refreshBtn = document.getElementById('groups-refresh');

  if (hostedListEl) {
    var hostedOpts = { showMgmt: true };

    function refreshHosted() {
      g.renderHostedGroups(hostedListEl, hostedOpts);
    }

    refreshHosted();

    if (refreshBtn) {
      refreshBtn.addEventListener('click', refreshHosted);
    }

    // Auto-refresh on group events
    function startHostedStream() {
      if (!window.Goop || !window.Goop.mq) { setTimeout(startHostedStream, 100); return; }
      Goop.mq.onGroup(function(from, topic, payload, ack) {
        var type = payload && payload.type;
        if (type === 'members' || type === 'close' || type === 'welcome' || type === 'leave') {
          refreshHosted();
        }
        ack();
      });
    }
    startHostedStream();
  }

  // ── Joined tab (/groups/joined) ─────────────────────────────────────────
  var subListEl = document.getElementById('groups-sub-list');
  var joinedRefreshBtn = document.getElementById('groups-joined-refresh');

  if (subListEl) {
    function refreshJoined() {
      g.renderSubscriptions(subListEl);
    }

    refreshJoined();

    if (joinedRefreshBtn) {
      joinedRefreshBtn.addEventListener('click', refreshJoined);
    }

    function startJoinedStream() {
      if (!window.Goop || !window.Goop.mq) { setTimeout(startJoinedStream, 100); return; }
      Goop.mq.onGroupInvite(function(from, topic, payload, ack) {
        refreshJoined();
        ack();
      });
      Goop.mq.onGroup(function(from, topic, payload, ack) {
        var type = payload && payload.type;
        if (type === 'members' || type === 'close' || type === 'welcome' || type === 'leave') {
          refreshJoined();
        }
        ack();
      });
    }
    startJoinedStream();
  }

  // ── Events tab (/groups/events) ─────────────────────────────────────────
  var eventsEl = document.getElementById('groups-events');
  var clearBtn = document.getElementById('groups-clear-events');

  if (eventsEl) {
    if (clearBtn) {
      clearBtn.addEventListener('click', function() {
        eventsEl.innerHTML = '<p class="empty-state">Waiting for events...</p>';
      });
    }

    function addEventToLog(evt) {
      if (!evt) return;
      var placeholder = eventsEl.querySelector('.empty-state');
      if (placeholder) placeholder.remove();

      var div = document.createElement('div');
      div.className = 'groups-event-item';

      var time = new Date().toLocaleTimeString();
      var payload = g.formatEventPayload(evt);
      if (!payload) {
        try {
          payload = typeof evt.payload === 'string' ? evt.payload : JSON.stringify(evt.payload);
          if (payload && payload.length > 120) payload = payload.substring(0, 120) + '\u2026';
        } catch (_) {}
      }

      div.innerHTML =
        '<span class="evt-time">' + escapeHtml(time) + '</span>' +
        '<span class="evt-type">' + escapeHtml(evt.type) + '</span>' +
        (evt.from ? '<span class="evt-from">' + escapeHtml(g.shortId(evt.from)) + '</span>' : '') +
        (payload ? '<span>' + escapeHtml(payload) + '</span>' : '');

      eventsEl.insertBefore(div, eventsEl.firstChild);
      while (eventsEl.children.length > 100) {
        eventsEl.removeChild(eventsEl.lastChild);
      }
    }

    function startEventStream() {
      if (!window.Goop || !window.Goop.mq) { setTimeout(startEventStream, 100); return; }
      Goop.mq.onGroup(function(from, topic, payload, ack) {
        addEventToLog(payload);
        ack();
      });
      Goop.mq.onGroupInvite(function(from, topic, payload, ack) {
        addEventToLog(payload);
        ack();
      });
    }
    startEventStream();
  }
})();
