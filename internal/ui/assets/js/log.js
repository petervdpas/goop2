//
// Client-side logging that sends to the Goop² logs page.
// Also logs to browser console for debugging.
//
// Usage:
//   Goop.log.debug("webrtc", "ICE state changed to connected");
//   Goop.log.info("call", "Call started with peer abc123");
//   Goop.log.warn("realtime", "Channel disconnected, reconnecting...");
//   Goop.log.error("webrtc", "ICE connection failed!");
//
(() => {
  window.Goop = window.Goop || {};

  var queue = [];
  var sending = false;
  var batchDelay = 100; // ms to batch logs

  function sendLogs() {
    if (sending || queue.length === 0) return;
    sending = true;

    var batch = queue.splice(0, queue.length);

    // Send each log entry
    var promises = batch.map(function(entry) {
      return fetch('/api/logs/client', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(entry)
      }).catch(function() { /* ignore send failures */ });
    });

    Promise.all(promises).then(function() {
      sending = false;
      if (queue.length > 0) {
        setTimeout(sendLogs, batchDelay);
      }
    });
  }

  function log(level, source, message) {
    // Also log to console
    var consoleFn = console.log;
    if (level === 'error') consoleFn = console.error;
    else if (level === 'warn') consoleFn = console.warn;
    else if (level === 'debug') consoleFn = console.debug;
    consoleFn('[' + source + ']', message);

    // Queue for backend
    queue.push({ level: level, source: source, message: message });

    // Debounced send
    if (!sending) {
      setTimeout(sendLogs, batchDelay);
    }
  }

  Goop.log = {
    debug: function(source, msg) { log('debug', source, msg); },
    info: function(source, msg) { log('info', source, msg); },
    warn: function(source, msg) { log('warn', source, msg); },
    error: function(source, msg) { log('error', source, msg); }
  };
})();
