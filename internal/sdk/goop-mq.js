(() => {
  window.Goop = window.Goop || {};

  let sse = null;

  window.Goop.mq = {
    subscribe(callback) {
      if (sse) sse.close();
      sse = new EventSource("/api/mq/events");

      sse.addEventListener("message", function(e) {
        try {
          if (callback) callback(JSON.parse(e.data));
        } catch (_) {}
      });

      sse.addEventListener("connected", function() {});
      sse.onerror = function() {};

      return sse;
    },

    unsubscribe() {
      if (sse) {
        sse.close();
        sse = null;
      }
    },
  };
})();
