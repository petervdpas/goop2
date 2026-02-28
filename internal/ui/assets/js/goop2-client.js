// goop2-client.js — standalone API client for goop2 peers.
// Self-contained: no dependency on Goop.core or any internal module.
//
// Usage (external site):
//   <script src="http://peer:8787/assets/js/goop2-client.js"></script>
//   const client = Goop2Client('http://peer:8787');
//   client.peers.list().then(peers => console.log(peers));
//   client.mq.send({ peer_id, topic, payload });
//   const es = client.mq.events();   // EventSource
//   es.addEventListener('message', ev => console.log(JSON.parse(ev.data)));

(function (global) {
  'use strict';

  function Goop2Client(baseURL) {
    baseURL = (baseURL || '').replace(/\/$/, '');

    function _get(path) {
      return fetch(baseURL + path).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t.trim() || r.statusText); });
        var ct = r.headers.get('Content-Type') || '';
        return ct.includes('application/json') ? r.json() : null;
      });
    }

    function _post(path, body) {
      return fetch(baseURL + path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body !== undefined ? body : {}),
      }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t.trim() || r.statusText); });
        var ct = r.headers.get('Content-Type') || '';
        return ct.includes('application/json') ? r.json() : null;
      });
    }

    function _delete(path) {
      return fetch(baseURL + path, { method: 'DELETE' }).then(function (r) {
        if (!r.ok) return r.text().then(function (t) { throw new Error(t.trim() || r.statusText); });
        var ct = r.headers.get('Content-Type') || '';
        return ct.includes('application/json') ? r.json() : null;
      });
    }

    function _upload(path, formData) {
      return new Promise(function (resolve, reject) {
        var xhr = new XMLHttpRequest();
        xhr.open('POST', baseURL + path);
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

    return {

      // ── MQ ───────────────────────────────────────────────────────────────────
      mq: {
        send:   function (p) { return _post('/api/mq/send', p); },
        ack:    function (p) { return _post('/api/mq/ack', p); },
        events: function ()  { return new EventSource(baseURL + '/api/mq/events'); },
      },

      // ── Call ─────────────────────────────────────────────────────────────────
      call: {
        mode:          function ()      { return _get('/api/call/mode'); },
        active:        function ()      { return _get('/api/call/active'); },
        start:         function (p)     { return _post('/api/call/start', p); },
        accept:        function (p)     { return _post('/api/call/accept', p); },
        hangup:        function (p)     { return _post('/api/call/hangup', p); },
        toggleAudio:   function (p)     { return _post('/api/call/toggle-audio', p); },
        toggleVideo:   function (p)     { return _post('/api/call/toggle-video', p); },
        loopbackOffer: function (ch, p) { return _post('/api/call/loopback/' + ch + '/offer', p); },
        loopbackIce:   function (ch, p) { return _post('/api/call/loopback/' + ch + '/ice', p); },
        mediaWsUrl:    function (ch)    { return baseURL + '/api/call/media/' + ch; },
        selfWsUrl:     function (ch)    { return baseURL + '/api/call/self/' + ch; },
      },

      // ── Groups ───────────────────────────────────────────────────────────────
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

      // ── Listen ───────────────────────────────────────────────────────────────
      listen: {
        state:     function ()  { return _get('/api/listen/state'); },
        create:    function (p) { return _post('/api/listen/create', p); },
        close:     function ()  { return _post('/api/listen/close'); },
        load:      function (p) { return _post('/api/listen/load', p); },
        queueAdd:  function (p) { return _post('/api/listen/queue/add', p); },
        control:   function (p) { return _post('/api/listen/control', p); },
        join:      function (p) { return _post('/api/listen/join', p); },
        leave:     function ()  { return _post('/api/listen/leave'); },
        streamUrl: function ()  { return baseURL + '/api/listen/stream'; },
        // Subscribe to listen state changes via MQ SSE
        subscribe: function (callback) {
          var es = new EventSource(baseURL + '/api/mq/events');
          es.addEventListener('message', function (ev) {
            try {
              var msg = JSON.parse(ev.data);
              if (msg && msg.msg && msg.msg.topic && msg.msg.topic.startsWith('listen:')) {
                callback(msg.msg.payload && msg.msg.payload.group);
              }
            } catch (_) {}
          });
          return { close: function () { es.close(); } };
        },
      },

      // ── Peers ─────────────────────────────────────────────────────────────────
      peers: {
        list:     function ()   { return _get('/api/peers'); },
        self:     function ()   { return _get('/api/self'); },
        content:  function (id) { return _get('/api/peer/content?id=' + encodeURIComponent(id)); },
        favorite: function (p)  { return _post('/api/peers/favorite', p); },
        probe:    function ()   { return _post('/api/peers/probe'); },
      },

      // ── Settings ──────────────────────────────────────────────────────────────
      settings: {
        get:            function ()          { return _get('/api/settings/quick/get'); },
        save:           function (p)         { return _post('/api/settings/quick', p); },
        servicesHealth: function ()          { return _get('/api/services/health'); },
        checkService:   function (url, type) {
          var qs = '?url=' + encodeURIComponent(url) + (type ? '&type=' + encodeURIComponent(type) : '');
          return _get('/api/services/check' + qs);
        },
      },

      // ── Logs ──────────────────────────────────────────────────────────────────
      logs: {
        snapshot: function ()  { return _get('/api/logs'); },
        client:   function (p) { return _post('/api/logs/client', p); },
        stream:   function ()  { return new EventSource(baseURL + '/api/logs/stream'); },
      },

      // ── Docs ──────────────────────────────────────────────────────────────────
      docs: {
        my:     function (groupId) {
          return _get('/api/docs/my?group_id=' + encodeURIComponent(groupId));
        },
        browse: function (groupId) {
          return _get('/api/docs/browse?group_id=' + encodeURIComponent(groupId));
        },
        delete: function (p) { return _post('/api/docs/delete', p); },
        downloadUrl: function (groupId, file, peerId, inline) {
          var qs = '?group_id=' + encodeURIComponent(groupId) + '&file=' + encodeURIComponent(file);
          if (peerId) qs += '&peer_id=' + encodeURIComponent(peerId);
          if (inline) qs += '&inline=1';
          return baseURL + '/api/docs/download' + qs;
        },
        upload: function (groupId, file) {
          var fd = new FormData();
          fd.append('group_id', groupId);
          fd.append('file', file);
          return _upload('/api/docs/upload', fd);
        },
      },

      // ── Data ──────────────────────────────────────────────────────────────────
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

      // ── Chat ──────────────────────────────────────────────────────────────────
      chat: {
        history: function (peerId) { return _get('/api/chat/history?peer_id=' + encodeURIComponent(peerId)); },
        clear:   function (peerId) { return _delete('/api/chat/history?peer_id=' + encodeURIComponent(peerId)); },
      },

      // ── Avatar ────────────────────────────────────────────────────────────────
      avatar: {
        url:     function ()   { return baseURL + '/api/avatar'; },
        peerUrl: function (id) { return baseURL + '/api/avatar/peer/' + encodeURIComponent(id); },
        delete:  function ()   { return _delete('/api/avatar/delete'); },
        upload:  function (file) {
          var fd = new FormData();
          fd.append('file', file);
          return _upload('/api/avatar/upload', fd);
        },
      },
    };
  }

  global.Goop2Client = Goop2Client;

})(typeof window !== 'undefined' ? window : this);
