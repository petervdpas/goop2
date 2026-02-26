// api.js — Goop.api: typed HTTP client derived from the OpenAPI spec.
// One named function per endpoint. No magic URL strings anywhere else.
// Load order: immediately after core.js, before everything else.
//
// Usage:  Goop.api.mq.send({peer_id, topic, payload})
//         Goop.api.call.start({channel_id, remote_peer})
//         Goop.api.groups.invite({group_id, peer_id})
//         etc.
//
// For SSE / WebSocket endpoints the methods return a URL string or a
// constructed EventSource — never a Promise.

(function () {
  'use strict';

  // Re-use core GET/POST wrapper. Resolved at call time so load order
  // of core.js before api.js is the only requirement.
  function _get(url) {
    return window.Goop.core.api(url);
  }
  function _post(url, body) {
    return window.Goop.core.api(url, body !== undefined ? body : {});
  }
  function _delete(url) {
    return fetch(url, { method: 'DELETE' }).then(function (r) {
      if (!r.ok) return r.text().then(function (t) { throw new Error(t || r.statusText); });
      var ct = r.headers.get('Content-Type') || '';
      return ct.includes('application/json') ? r.json() : null;
    });
  }
  function _wsUrl(path) {
    var proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    return proto + '//' + window.location.host + path;
  }
  function _upload(url, formData) {
    return new Promise(function (resolve, reject) {
      var xhr = new XMLHttpRequest();
      xhr.open('POST', url);
      xhr.onload = function () {
        if (xhr.status >= 200 && xhr.status < 300) {
          try { resolve(JSON.parse(xhr.responseText)); } catch (_) { resolve(null); }
        } else {
          reject(new Error(xhr.responseText || xhr.statusText));
        }
      };
      xhr.onerror = function () { reject(new Error('upload failed')); };
      xhr.send(formData);
    });
  }

  window.Goop = window.Goop || {};
  window.Goop.api = {

    // ── MQ ─────────────────────────────────────────────────────────────────────
    // Transport layer: send, ack, and the SSE event bus.
    // For P2P messaging use Goop.mq.send() (full stack with retry/sequence).
    // POST /api/mq/send and /api/mq/ack are the raw HTTP contract.
    mq: {
      send:   function (p) { return _post('/api/mq/send', p); },
      ack:    function (p) { return _post('/api/mq/ack', p); },
      events: function ()  { return new EventSource('/api/mq/events'); },
    },

    // ── Call ───────────────────────────────────────────────────────────────────
    call: {
      mode:          function ()       { return _get('/api/call/mode'); },
      active:        function ()       { return _get('/api/call/active'); },
      debug:         function ()       { return _get('/api/call/debug'); },
      start:         function (p)      { return _post('/api/call/start', p); },
      accept:        function (p)      { return _post('/api/call/accept', p); },
      hangup:        function (p)      { return _post('/api/call/hangup', p); },
      toggleAudio:   function (p)      { return _post('/api/call/toggle-audio', p); },
      toggleVideo:   function (p)      { return _post('/api/call/toggle-video', p); },
      loopbackOffer: function (ch, p)  { return _post('/api/call/loopback/' + ch + '/offer', p); },
      loopbackIce:   function (ch, p)  { return _post('/api/call/loopback/' + ch + '/ice', p); },
      // WebSocket URLs — pass to new WebSocket(url) or MSE adapter
      mediaWsUrl:    function (ch)     { return _wsUrl('/api/call/media/' + ch); },
      selfWsUrl:     function (ch)     { return _wsUrl('/api/call/self/' + ch); },
    },

    // ── Groups ─────────────────────────────────────────────────────────────────
    groups: {
      list:               function ()  { return _get('/api/groups'); },
      create:             function (p) { return _post('/api/groups', p); },
      close:              function (p) { return _post('/api/groups/close', p); },
      invite:             function (p) { return _post('/api/groups/invite', p); },
      join:               function (p) { return _post('/api/groups/join', p); },
      joinOwn:            function (p) { return _post('/api/groups/join-own', p); },
      kick:               function (p) { return _post('/api/groups/kick', p); },
      leave:              function (p) { return _post('/api/groups/leave', p); },
      leaveOwn:           function (p) { return _post('/api/groups/leave-own', p); },
      setMaxMembers:      function (p) { return _post('/api/groups/max-members', p); },
      setMeta:            function (p) { return _post('/api/groups/meta', p); },
      rejoin:             function (p) { return _post('/api/groups/rejoin', p); },
      send:               function (p) { return _post('/api/groups/send', p); },
      subscriptions:      function ()  { return _get('/api/groups/subscriptions'); },
      removeSubscription: function (p) { return _post('/api/groups/subscriptions/remove', p); },
    },

    // ── Listen ─────────────────────────────────────────────────────────────────
    listen: {
      state:     function ()  { return _get('/api/listen/state'); },
      create:    function (p) { return _post('/api/listen/create', p); },
      close:     function ()  { return _post('/api/listen/close'); },
      load:      function (p) { return _post('/api/listen/load', p); },
      queueAdd:  function (p) { return _post('/api/listen/queue/add', p); },
      control:   function (p) { return _post('/api/listen/control', p); },
      join:      function (p) { return _post('/api/listen/join', p); },
      leave:     function ()  { return _post('/api/listen/leave'); },
      // Audio stream URL — assign directly to <audio>.src
      streamUrl: function ()  { return '/api/listen/stream'; },
    },

    // ── Peers ──────────────────────────────────────────────────────────────────
    peers: {
      list:     function ()    { return _get('/api/peers'); },
      self:     function ()    { return _get('/api/self'); },
      content:  function (id)  { return _get('/api/peer/content?id=' + encodeURIComponent(id)); },
      favorite: function (p)   { return _post('/api/peers/favorite', p); },
      probe:    function ()    { return _post('/api/peers/probe'); },
    },

    // ── Settings ───────────────────────────────────────────────────────────────
    settings: {
      get:            function ()           { return _get('/api/settings/quick/get'); },
      save:           function (p)          { return _post('/api/settings/quick', p); },
      servicesHealth: function ()           { return _get('/api/services/health'); },
      checkService:   function (url, type)  {
        var qs = '?url=' + encodeURIComponent(url) + (type ? '&type=' + encodeURIComponent(type) : '');
        return _get('/api/services/check' + qs);
      },
    },

    // ── Logs ───────────────────────────────────────────────────────────────────
    logs: {
      snapshot: function ()  { return _get('/api/logs'); },
      client:   function (p) { return _post('/api/logs/client', p); },
      stream:   function ()  { return new EventSource('/api/logs/stream'); },
    },

    // ── Docs ───────────────────────────────────────────────────────────────────
    docs: {
      my:      function (groupId) {
        return _get('/api/docs/my?group_id=' + encodeURIComponent(groupId));
      },
      browse:  function (groupId) {
        return _get('/api/docs/browse?group_id=' + encodeURIComponent(groupId));
      },
      delete:  function (p) { return _post('/api/docs/delete', p); },
      // Returns a URL — pass to <a href> or fetch() for download
      downloadUrl: function (groupId, file, peerId, inline) {
        var qs = '?group_id=' + encodeURIComponent(groupId) + '&file=' + encodeURIComponent(file);
        if (peerId) qs += '&peer_id=' + encodeURIComponent(peerId);
        if (inline) qs += '&inline=1';
        return '/api/docs/download' + qs;
      },
      upload: function (groupId, file) {
        var fd = new FormData();
        fd.append('group_id', groupId);
        fd.append('file', file);
        return _upload('/api/docs/upload', fd);
      },
    },

    // ── Data ───────────────────────────────────────────────────────────────────
    data: {
      tables:        function ()  { return _get('/api/data/tables'); },
      createTable:   function (p) { return _post('/api/data/tables/create', p); },
      dropTable:     function (p) { return _post('/api/data/tables/delete', p); },
      describeTable: function (p) { return _post('/api/data/tables/describe', p); },
      renameTable:   function (p) { return _post('/api/data/tables/rename', p); },
      addColumn:     function (p) { return _post('/api/data/tables/add-column', p); },
      dropColumn:    function (p) { return _post('/api/data/tables/drop-column', p); },
      setPolicy:     function (p) { return _post('/api/data/tables/set-policy', p); },
      query:         function (p) { return _post('/api/data/query', p); },
      insert:        function (p) { return _post('/api/data/insert', p); },
      update:        function (p) { return _post('/api/data/update', p); },
      delete:        function (p) { return _post('/api/data/delete', p); },
    },

    // ── Avatar ─────────────────────────────────────────────────────────────────
    avatar: {
      // URL helpers — assign to <img>.src, never fetch
      url:     function ()    { return '/api/avatar'; },
      peerUrl: function (id)  { return '/api/avatar/peer/' + encodeURIComponent(id); },
      delete:  function ()    { return _delete('/api/avatar/delete'); },
      upload:  function (file) {
        var fd = new FormData();
        fd.append('file', file);
        return _upload('/api/avatar/upload', fd);
      },
    },
  };
})();
