//
// Real-time channel API — bridged over Goop MQ.
// API surface identical to the previous /api/realtime/* version.
//
// Usage:
//   <script src="/sdk/goop-realtime.js"></script>
//
//   // Connect to a peer
//   const channel = await Goop.realtime.connect(peerId);
//   channel.send({ type: "move", from: "e2", to: "e4" });
//   channel.onMessage(function(msg) { console.log(msg); });
//   channel.close();
//
//   // Accept an incoming channel
//   Goop.realtime.onIncoming(function(info) {
//     const channel = await Goop.realtime.accept(info.channelId, info.hostPeerId);
//     channel.onMessage(function(msg) { ... });
//   });
//
//   // List active channels
//   const channels = await Goop.realtime.channels();
//
// MQ topics:
//   realtime:invite          — incoming channel invitation
//   realtime:{channelId}     — messages on a specific channel
//
(() => {
  window.Goop = window.Goop || {};

  var activeChannels  = {};  // channelId -> channel wrapper
  var messageHandlers = {};  // channelId -> [callbacks]
  var globalHandlers  = [];  // callbacks for all channels
  var incomingHandlers = []; // callbacks for incoming invitations
  var _unsub = null;
  var _initialized = false;

  function waitMQ(fn) {
    if (window.Goop && window.Goop.mq) { fn(); return; }
    var t = setInterval(function() {
      if (window.Goop && window.Goop.mq) { clearInterval(t); fn(); }
    }, 50);
  }

  function newChannelId() {
    return 'rt-' + Math.random().toString(36).slice(2, 10);
  }

  function ensureListening() {
    if (_initialized) return;
    _initialized = true;

    waitMQ(function() {
      _unsub = Goop.mq.subscribe('realtime:*', function(from, topic, payload, ack) {
        var suffix = topic.slice('realtime:'.length);

        // Incoming channel invitation
        if (suffix === 'invite') {
          var info = {
            channelId:   payload && payload.channel_id,
            hostPeerId:  from,
          };
          for (var i = 0; i < incomingHandlers.length; i++) {
            try { incomingHandlers[i](info); } catch (e) {}
          }
          ack();
          return;
        }

        // Regular message on a channel
        var channelId = suffix;
        var env = { channel: channelId, from: from };

        var cbs = messageHandlers[channelId];
        if (cbs) {
          for (var i = 0; i < cbs.length; i++) {
            try { cbs[i](payload, env); } catch (e) { console.error('realtime handler error:', e); }
          }
        }
        for (var j = 0; j < globalHandlers.length; j++) {
          try { globalHandlers[j](payload, env); } catch (e) { console.error('realtime handler error:', e); }
        }
        ack();
      });
    });
  }

  function wrapChannel(channelId, remotePeer) {
    var ch = {
      id:         channelId,
      remotePeer: remotePeer,

      send: function(payload) {
        return new Promise(function(resolve, reject) {
          waitMQ(function() {
            Goop.mq.send(remotePeer, 'realtime:' + channelId, payload)
              .then(resolve).catch(reject);
          });
        });
      },

      onMessage: function(callback) {
        if (!messageHandlers[channelId]) messageHandlers[channelId] = [];
        messageHandlers[channelId].push(callback);
      },

      offMessage: function(callback) {
        var cbs = messageHandlers[channelId];
        if (!cbs) return;
        var idx = cbs.indexOf(callback);
        if (idx >= 0) cbs.splice(idx, 1);
      },

      close: function() {
        delete messageHandlers[channelId];
        delete activeChannels[channelId];
        return Promise.resolve();
      },
    };

    activeChannels[channelId] = ch;
    return ch;
  }

  Goop.realtime = {
    /** Connect to a peer, creating a new virtual MQ channel */
    connect: async function(peerId) {
      ensureListening();
      var channelId = newChannelId();
      var channel = wrapChannel(channelId, peerId);
      // Notify the remote peer
      await new Promise(function(resolve) {
        waitMQ(function() {
          Goop.mq.send(peerId, 'realtime:invite', { channel_id: channelId })
            .then(resolve).catch(resolve);
        });
      });
      return channel;
    },

    /** Accept an incoming channel invitation */
    accept: async function(channelId, hostPeerId) {
      ensureListening();
      return wrapChannel(channelId, hostPeerId);
    },

    /** List active channels */
    channels: function() {
      return Promise.resolve(
        Object.keys(activeChannels).map(function(id) {
          return { id: id, remote_peer: activeChannels[id].remotePeer };
        })
      );
    },

    /** Register handler for incoming channel invitations */
    onIncoming: function(callback) {
      ensureListening();
      incomingHandlers.push(callback);
    },

    /** Register handler for all messages on all channels */
    onMessage: function(callback) {
      ensureListening();
      globalHandlers.push(callback);
    },

    /** Remove a global message handler */
    offMessage: function(callback) {
      var idx = globalHandlers.indexOf(callback);
      if (idx >= 0) globalHandlers.splice(idx, 1);
    },

    /** Returns true once MQ subscription is active */
    isConnected: function() {
      return _initialized;
    },
  };
})();
