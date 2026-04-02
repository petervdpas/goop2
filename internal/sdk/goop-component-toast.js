(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.toast = function(el, opts) {
    opts = opts || {};
    var toastClass = opts.toastClass || "";
    var titleClass = opts.titleClass || "";
    var messageClass = opts.messageClass || "";
    var enterClass = opts.enterClass || "";
    var exitClass = opts.exitClass || "";
    var defaultDuration = opts.duration != null ? opts.duration : 4000;

    function show(config) {
      if (typeof config === "string") config = { message: config };
      config = config || {};
      var toast = document.createElement("div");
      if (toastClass) toast.className = toastClass;

      if (config.title) {
        var t = document.createElement("div");
        if (titleClass) t.className = titleClass;
        t.textContent = config.title;
        toast.appendChild(t);
      }

      if (config.message) {
        var m = document.createElement("div");
        if (messageClass) m.className = messageClass;
        m.textContent = config.message;
        toast.appendChild(m);
      }

      el.appendChild(toast);

      if (enterClass) {
        requestAnimationFrame(function() {
          requestAnimationFrame(function() { toast.classList.add(enterClass); });
        });
      }

      var dur = config.duration != null ? config.duration : defaultDuration;
      if (dur > 0) {
        setTimeout(function() {
          if (exitClass) toast.classList.add(exitClass);
          if (enterClass) toast.classList.remove(enterClass);
          setTimeout(function() { toast.remove(); }, 300);
        }, dur);
      }
      return toast;
    }

    show.success = function(msg) { return show({ title: "\u2713 Success", message: msg, duration: 3000 }); };
    show.error = function(msg) { return show({ title: "\u2717 Error", message: msg, duration: 5000 }); };
    show.warning = function(msg) { return show({ title: "\u26A0 Warning", message: msg, duration: 4000 }); };
    show.info = function(msg) { return show({ title: "\u2139 Info", message: msg, duration: 4000 }); };
    show.el = el;
    show.destroy = function() { el.innerHTML = ""; };

    return show;
  };
})();
