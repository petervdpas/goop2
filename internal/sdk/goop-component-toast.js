(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _ss = Goop.ui._setStyles || function(el, s) { for (var k in s) el.style[k] = s[k]; };
  var _rt = Goop.ui._resolveTheme || function() { return { panel: "#151924", text: "#e6e9ef", line: "rgba(255,255,255,.10)", shadow: "0 18px 46px rgba(0,0,0,.34)" }; };
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };

  var toastWrap = null;

  function ensureWrap() {
    if (!toastWrap) {
      toastWrap = document.createElement("div");
      _ss(toastWrap, {
        position: "fixed", top: "1rem", right: "1rem", zIndex: "99999",
        display: "flex", flexDirection: "column", gap: "0.5rem", pointerEvents: "none"
      });
      document.body.appendChild(toastWrap);
    }
    return toastWrap;
  }

  function toast(opts) {
    if (typeof opts === "string") opts = { message: opts };
    opts = opts || {};
    var t = _rt();
    var wrap = ensureWrap();
    var el = document.createElement("div");
    _ss(el, {
      pointerEvents: "auto", background: t.panel, color: t.text,
      border: "1px solid " + t.line, borderRadius: "8px",
      padding: "0.6rem 1rem", font: "14px/1.4 system-ui,sans-serif",
      boxShadow: t.shadow, opacity: "0", transform: "translateY(-8px)",
      transition: "opacity 0.2s ease, transform 0.2s ease"
    });
    var title = opts.title ? '<div style="font-weight:600;margin-bottom:0.15rem">' + _e(opts.title) + "</div>" : "";
    el.innerHTML = title + "<div>" + _e(opts.message || "") + "</div>";
    wrap.appendChild(el);
    requestAnimationFrame(function() {
      requestAnimationFrame(function() { el.style.opacity = "1"; el.style.transform = "none"; });
    });
    var dur = opts.duration != null ? opts.duration : 4000;
    if (dur > 0) {
      setTimeout(function() {
        el.style.opacity = "0";
        el.style.transform = "translateY(-8px)";
        setTimeout(function() { el.remove(); }, 300);
      }, dur);
    }
    return el;
  }

  Goop.ui.toast = toast;
  Goop.ui.toast.success = function(msg) { return toast({ title: "\u2713 Success", message: msg, duration: 3000 }); };
  Goop.ui.toast.error = function(msg) { return toast({ title: "\u2717 Error", message: msg, duration: 5000 }); };
  Goop.ui.toast.warning = function(msg) { return toast({ title: "\u26A0 Warning", message: msg, duration: 4000 }); };
  Goop.ui.toast.info = function(msg) { return toast({ title: "\u2139 Info", message: msg, duration: 4000 }); };
})();
