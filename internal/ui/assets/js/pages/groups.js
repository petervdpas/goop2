// Groups page (/self/groups): initialize hosted groups + subscriptions + event log.
(function() {
  if (!document.querySelector('#groups-page')) return;

  var g = window.Goop && window.Goop.groups;
  var core = window.Goop && window.Goop.core;
  if (!g || !core) return;

  var escapeHtml = core.escapeHtml;

  var hostedListEl = document.getElementById('groups-hosted-list');
  var subListEl = document.getElementById('groups-sub-list');
  var eventsEl = document.getElementById('groups-events');

  var hostedOpts = { showMgmt: true };

  function refresh() {
    g.renderHostedGroups(hostedListEl, hostedOpts);
    g.renderSubscriptions(subListEl);
  }

  refresh();

  document.getElementById('groups-refresh').addEventListener('click', refresh);
  document.getElementById('groups-clear-events').addEventListener('click', function() {
    eventsEl.innerHTML = '<p class="groups-empty">Waiting for events...</p>';
  });

  // Subscribe to group events via MQ
  function startEventStream() {
    if (!window.Goop || !window.Goop.mq) { setTimeout(startEventStream, 100); return; }

    Goop.mq.subscribe('group:*', function(from, topic, payload, ack) {
      var type = payload && payload.type;
      addEventToLog(payload);
      if (type === 'members' || type === 'close' || type === 'welcome' || type === 'leave') {
        refresh();
      }
      ack();
    });

    Goop.mq.subscribe('group.invite', function(from, topic, payload, ack) {
      addEventToLog(payload);
      g.renderSubscriptions(subListEl);
      ack();
    });
  }
  startEventStream();

  function addEventToLog(evt) {
    var placeholder = eventsEl.querySelector('.groups-empty');
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
})();
