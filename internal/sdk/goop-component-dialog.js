(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _ss = Goop.ui._setStyles || function(el, s) { for (var k in s) el.style[k] = s[k]; };
  var _rt = Goop.ui._resolveTheme || function() { return { panel: "#151924", text: "#e6e9ef", muted: "#9aa3b2", line: "rgba(255,255,255,.10)", accent: "#7aa2ff", field: "rgba(0,0,0,.25)", shadow: "0 18px 46px rgba(0,0,0,.34)" }; };
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };

  function hasViewer() { return window.Goop.dialog && typeof window.Goop.dialog === "function"; }

  function fallbackDialog(opts) {
    opts = opts || {};
    var title      = opts.title || "";
    var message    = opts.message || "";
    var input      = opts.input || null;
    var okLabel    = (opts.ok && opts.ok.label) || opts.okText || "OK";
    var okDanger   = (opts.ok && opts.ok.danger) || opts.dangerOk || false;
    var cancelLabel = opts.cancel === false ? null : ((opts.cancel && opts.cancel.label) || opts.cancelText || "Cancel");
    var hideOk     = opts.ok === false;
    var hasInput   = !!input;
    var match      = hasInput && input.match ? input.match : null;

    return new Promise(function(resolve) {
      var t = _rt();

      var bg = document.createElement("div");
      _ss(bg, {
        position: "fixed", top: "0", left: "0", right: "0", bottom: "0",
        background: "rgba(0,0,0,0.55)", zIndex: "99998",
        display: "flex", alignItems: "center", justifyContent: "center",
        backdropFilter: "blur(6px)"
      });

      var dlg = document.createElement("div");
      _ss(dlg, {
        background: t.panel, color: t.text, border: "1px solid " + t.line,
        borderRadius: "16px", minWidth: "300px", maxWidth: "420px",
        font: "14px/1.5 system-ui,sans-serif", boxShadow: t.shadow, overflow: "hidden"
      });

      var head = document.createElement("div");
      head.textContent = title;
      _ss(head, { padding: "12px 14px", borderBottom: "1px solid " + t.line, fontWeight: "750" });

      var body = document.createElement("div");
      _ss(body, { padding: "14px", display: "flex", flexDirection: "column", gap: "10px" });

      var msg = document.createElement("div");
      msg.textContent = message;
      _ss(msg, { color: t.muted, fontSize: "13px", whiteSpace: "pre-wrap" });
      body.appendChild(msg);

      var inp = null;
      if (hasInput) {
        inp = document.createElement("input");
        inp.autocomplete = "off"; inp.spellcheck = false;
        inp.placeholder = input.placeholder || "";
        inp.value = input.value || "";
        if (input.type) inp.type = input.type;
        _ss(inp, {
          width: "100%", boxSizing: "border-box", padding: "10px 12px",
          border: "1px solid " + t.line, borderRadius: "999px",
          background: t.field, color: t.text, font: "inherit", outline: "none"
        });
        body.appendChild(inp);
      }

      var foot = document.createElement("div");
      _ss(foot, {
        padding: "12px 14px", borderTop: "1px solid " + t.line,
        display: "flex", gap: "8px", justifyContent: "flex-end"
      });

      function makeBtn(label, danger) {
        var btn = document.createElement("button");
        btn.type = "button"; btn.textContent = label;
        var c = danger ? "#ff5a5a" : t.accent;
        _ss(btn, {
          padding: "8px 12px", border: "1px solid " + c, borderRadius: "999px",
          cursor: "pointer", font: "inherit", color: t.text,
          background: danger ? "rgba(255,90,90,.14)" : "rgba(122,162,255,.14)",
          transition: "background 0.17s, transform 0.17s"
        });
        btn.addEventListener("mouseenter", function() { _ss(btn, { background: danger ? "rgba(255,90,90,.22)" : "rgba(122,162,255,.22)", transform: "translateY(-0.5px)" }); });
        btn.addEventListener("mouseleave", function() { _ss(btn, { background: danger ? "rgba(255,90,90,.14)" : "rgba(122,162,255,.14)", transform: "none" }); });
        return btn;
      }

      var bCancel = null, bOk = null;
      if (cancelLabel !== null) { bCancel = makeBtn(cancelLabel, false); foot.appendChild(bCancel); }
      if (!hideOk) {
        bOk = makeBtn(okLabel, okDanger);
        if (match) { bOk.disabled = true; bOk.style.opacity = "0.4"; bOk.style.cursor = "not-allowed"; }
        foot.appendChild(bOk);
      }

      if (match && inp) {
        inp.addEventListener("input", function() {
          var ok = inp.value === match;
          bOk.disabled = !ok; bOk.style.opacity = ok ? "1" : "0.4"; bOk.style.cursor = ok ? "pointer" : "not-allowed";
        });
      }

      dlg.appendChild(head); dlg.appendChild(body); dlg.appendChild(foot);
      bg.appendChild(dlg);

      function cleanup(confirmed) {
        document.removeEventListener("keydown", onKey);
        bg.remove();
        resolve(confirmed ? (hasInput ? inp.value : true) : null);
      }

      function onKey(e) {
        if (e.key === "Escape") { cleanup(false); return; }
        if (e.key === "Enter") { if (match && inp && inp.value !== match) return; cleanup(true); }
      }

      bg.addEventListener("mousedown", function(e) { if (e.target === bg) cleanup(false); });
      if (bCancel) bCancel.addEventListener("click", function() { cleanup(false); });
      if (bOk) bOk.addEventListener("click", function() { cleanup(true); });
      document.addEventListener("keydown", onKey);

      document.body.appendChild(bg);
      if (inp) { setTimeout(function() { inp.focus(); inp.select(); }, 0); }
      else if (bOk) { setTimeout(function() { bOk.focus(); }, 0); }
    });
  }

  Goop.ui.dialog = function(opts) {
    return hasViewer() ? Goop.dialog(opts) : fallbackDialog(opts);
  };

  Goop.ui.alert = function(title, message) {
    return hasViewer() ? Goop.dialog.alert(title, message) : fallbackDialog({ title: title, message: message, cancel: false });
  };

  Goop.ui.confirm = function(message, title) {
    return hasViewer() ? Goop.dialog.confirm(message, title) : fallbackDialog({ title: title || "Confirm", message: message });
  };

  Goop.ui.prompt = function(opts, defaultValue) {
    if (typeof opts === "string") opts = { message: opts, value: defaultValue || "" };
    opts = opts || {};
    if (hasViewer()) return Goop.dialog.prompt(opts);
    return fallbackDialog({
      title: opts.title || "Input", message: opts.message || "",
      input: { placeholder: opts.placeholder || "", value: opts.value || "", type: opts.type || "text" },
      okText: opts.okText || "OK", cancelText: opts.cancelText || "Cancel", dangerOk: opts.dangerOk || false,
    });
  };

  Goop.ui.confirmDanger = function(opts) {
    opts = opts || {};
    if (hasViewer()) return Goop.dialog.confirmDanger(opts);
    return fallbackDialog({
      title: opts.title || "Confirm", message: opts.message || "",
      input: { placeholder: opts.placeholder || "Type " + (opts.match || "DELETE"), match: opts.match || "DELETE" },
      okText: opts.okText || "Delete", cancelText: opts.cancelText || "Cancel", dangerOk: true,
    });
  };
})();
