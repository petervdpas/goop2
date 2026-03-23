//
// Portable UI helpers for site pages.
// When the viewer's dialogs.js is loaded, delegates to Goop.dialog.
// Otherwise uses a self-contained fallback — 100% inline styles, no injected
// stylesheet (WebKitGTK CSP blocks injected <style> tags on site pages).
//
// Usage:
//
//   <script src="/sdk/goop-ui.js"></script>
//
//   // toast notification
//   Goop.ui.toast("Saved!");
//   Goop.ui.toast({ title: "Success", message: "Row inserted", duration: 3000 });
//
//   // alert dialog
//   await Goop.ui.alert("Oops", "Something went wrong");
//
//   // confirm dialog (returns true or null)
//   if (await Goop.ui.confirm("Delete this row?")) { ... }
//
//   // prompt dialog (returns string or null)
//   const name = await Goop.ui.prompt({ title: "Rename", message: "New name:", placeholder: "untitled" });
//
//   // danger confirm (returns matched string or null)
//   const ok = await Goop.ui.confirmDanger({ title: "Drop table", message: "Type DELETE", match: "DELETE" });
//
//   // full options dialog
//   const result = await Goop.ui.dialog({ title: "...", message: "...", input: { ... }, ... });
//
//   // current theme
//   const theme = Goop.ui.theme(); // "dark" or "light"
//
(() => {
  window.Goop = window.Goop || {};

  function hasViewer() { return window.Goop.dialog && typeof window.Goop.dialog === "function"; }

  var LIGHT = {
    bg: "#ffffff", panel: "#ffffff", text: "#101325", muted: "#4a4f6b",
    line: "rgba(16,19,37,.12)", accent: "#5a3dff",
    field: "rgba(16,19,37,.04)", shadow: "0 22px 60px rgba(16,19,37,.14)"
  };
  var DARK = {
    bg: "#0f1115", panel: "#151924", text: "#e6e9ef", muted: "#9aa3b2",
    line: "rgba(255,255,255,.10)", accent: "#7aa2ff",
    field: "rgba(0,0,0,.25)", shadow: "0 18px 46px rgba(0,0,0,.34)"
  };

  function isDark() {
    var t = document.documentElement.getAttribute("data-theme") || "";
    if (t === "dark") return true;
    if (document.documentElement.classList.contains("theme-dark")) return true;
    var bg = getComputedStyle(document.documentElement).backgroundColor;
    if (bg) {
      var m = bg.match(/\d+/g);
      if (m && m.length >= 3) {
        var lum = (parseInt(m[0]) * 299 + parseInt(m[1]) * 587 + parseInt(m[2]) * 114) / 1000;
        return lum < 128;
      }
    }
    return false;
  }

  function cssVar(name) {
    var v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return v || null;
  }

  function resolveTheme() {
    var d = isDark() ? DARK : LIGHT;
    return {
      bg:     cssVar("--bg") || cssVar("--gui-bg") || d.bg,
      panel:  cssVar("--panel") || cssVar("--gui-panel") || d.panel,
      text:   cssVar("--text") || cssVar("--gui-text") || d.text,
      muted:  cssVar("--muted") || cssVar("--gui-muted") || d.muted,
      line:   cssVar("--line") || cssVar("--border") || cssVar("--gui-line") || d.line,
      accent: cssVar("--accent") || cssVar("--gui-accent") || d.accent,
      field:  cssVar("--field-bg") || cssVar("--gui-field") || d.field,
      shadow: d.shadow,
    };
  }

  function setStyles(el, styles) {
    for (var k in styles) el.style[k] = styles[k];
  }

  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // ── toast ──
  var toastWrap = null;

  function ensureToastWrap() {
    if (!toastWrap) {
      toastWrap = document.createElement("div");
      setStyles(toastWrap, {
        position: "fixed", top: "1rem", right: "1rem", zIndex: "99999",
        display: "flex", flexDirection: "column", gap: "0.5rem", pointerEvents: "none"
      });
      document.body.appendChild(toastWrap);
    }
    return toastWrap;
  }

  function toast(opts) {
    if (typeof opts === "string") opts = { message: opts };
    var t = resolveTheme();
    var wrap = ensureToastWrap();
    var el = document.createElement("div");
    setStyles(el, {
      pointerEvents: "auto", background: t.panel, color: t.text,
      border: "1px solid " + t.line, borderRadius: "8px",
      padding: "0.6rem 1rem", font: "14px/1.4 system-ui,sans-serif",
      boxShadow: t.shadow, opacity: "0", transform: "translateY(-8px)",
      transition: "opacity 0.2s ease, transform 0.2s ease"
    });
    var title = opts.title ? '<div style="font-weight:600;margin-bottom:0.15rem">' + esc(opts.title) + "</div>" : "";
    el.innerHTML = title + "<div>" + esc(opts.message || "") + "</div>";
    wrap.appendChild(el);
    requestAnimationFrame(function() {
      requestAnimationFrame(function() { el.style.opacity = "1"; el.style.transform = "none"; });
    });
    var dur = opts.duration || 4000;
    if (dur > 0) {
      setTimeout(function() {
        el.style.opacity = "0";
        el.style.transform = "translateY(-8px)";
        setTimeout(function() { el.remove(); }, 300);
      }, dur);
    }
    return el;
  }

  // ── fallback dialog (100% inline styles, CSP-safe) ──
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
      var t = resolveTheme();

      var bg = document.createElement("div");
      setStyles(bg, {
        position: "fixed", top: "0", left: "0", right: "0", bottom: "0",
        background: "rgba(0,0,0,0.55)", zIndex: "99998",
        display: "flex", alignItems: "center", justifyContent: "center",
        backdropFilter: "blur(6px)"
      });

      var dlg = document.createElement("div");
      setStyles(dlg, {
        background: t.panel, color: t.text, border: "1px solid " + t.line,
        borderRadius: "16px", minWidth: "300px", maxWidth: "420px",
        font: "14px/1.5 system-ui,sans-serif", boxShadow: t.shadow, overflow: "hidden"
      });

      var head = document.createElement("div");
      head.textContent = title;
      setStyles(head, {
        padding: "12px 14px", borderBottom: "1px solid " + t.line, fontWeight: "750"
      });

      var body = document.createElement("div");
      setStyles(body, { padding: "14px", display: "flex", flexDirection: "column", gap: "10px" });

      var msg = document.createElement("div");
      msg.textContent = message;
      setStyles(msg, { color: t.muted, fontSize: "13px", whiteSpace: "pre-wrap" });
      body.appendChild(msg);

      var inp = null;
      if (hasInput) {
        inp = document.createElement("input");
        inp.autocomplete = "off";
        inp.spellcheck = false;
        inp.placeholder = input.placeholder || "";
        inp.value = input.value || "";
        if (input.type) inp.type = input.type;
        setStyles(inp, {
          width: "100%", boxSizing: "border-box", padding: "10px 12px",
          border: "1px solid " + t.line, borderRadius: "999px",
          background: t.field, color: t.text, font: "inherit", outline: "none"
        });
        body.appendChild(inp);
      }

      var foot = document.createElement("div");
      setStyles(foot, {
        padding: "12px 14px", borderTop: "1px solid " + t.line,
        display: "flex", gap: "8px", justifyContent: "flex-end"
      });

      function makeBtn(label, danger) {
        var btn = document.createElement("button");
        btn.type = "button";
        btn.textContent = label;
        var c = danger ? "#ff5a5a" : t.accent;
        setStyles(btn, {
          padding: "8px 12px", border: "1px solid " + c, borderRadius: "999px",
          cursor: "pointer", font: "inherit", color: t.text,
          background: "color-mix(in srgb," + c + " 14%,transparent)",
          transition: "background 0.17s, transform 0.17s"
        });
        btn.addEventListener("mouseenter", function() {
          btn.style.background = "color-mix(in srgb," + c + " 22%,transparent)";
          btn.style.transform = "translateY(-0.5px)";
        });
        btn.addEventListener("mouseleave", function() {
          btn.style.background = "color-mix(in srgb," + c + " 14%,transparent)";
          btn.style.transform = "none";
        });
        return btn;
      }

      var bCancel = null;
      var bOk = null;

      if (cancelLabel !== null) {
        bCancel = makeBtn(cancelLabel, false);
        foot.appendChild(bCancel);
      }
      if (!hideOk) {
        bOk = makeBtn(okLabel, okDanger);
        if (match) { bOk.disabled = true; bOk.style.opacity = "0.4"; bOk.style.cursor = "not-allowed"; }
        foot.appendChild(bOk);
      }

      if (match && inp) {
        inp.addEventListener("input", function() {
          var ok = inp.value === match;
          bOk.disabled = !ok;
          bOk.style.opacity = ok ? "1" : "0.4";
          bOk.style.cursor = ok ? "pointer" : "not-allowed";
        });
      }

      dlg.appendChild(head);
      dlg.appendChild(body);
      dlg.appendChild(foot);
      bg.appendChild(dlg);

      function cleanup(confirmed) {
        document.removeEventListener("keydown", onKey);
        bg.remove();
        resolve(confirmed ? (hasInput ? inp.value : true) : null);
      }

      function onKey(e) {
        if (e.key === "Escape") { cleanup(false); return; }
        if (e.key === "Enter") {
          if (match && inp && inp.value !== match) return;
          cleanup(true);
        }
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

  // ── public API ──
  window.Goop.ui = {
    toast: toast,

    dialog: function(opts) {
      return hasViewer() ? Goop.dialog(opts) : fallbackDialog(opts);
    },

    alert: function(title, message) {
      return hasViewer() ? Goop.dialog.alert(title, message) : fallbackDialog({ title: title, message: message, cancel: false });
    },

    confirm: function(message, title) {
      return hasViewer() ? Goop.dialog.confirm(message, title) : fallbackDialog({ title: title || "Confirm", message: message });
    },

    prompt: function(opts, defaultValue) {
      if (typeof opts === "string") opts = { message: opts, value: defaultValue || "" };
      opts = opts || {};
      if (hasViewer()) return Goop.dialog.prompt(opts);
      return fallbackDialog({
        title: opts.title || "Input",
        message: opts.message || "",
        input: { placeholder: opts.placeholder || "", value: opts.value || "", type: opts.type || "text" },
        okText: opts.okText || "OK",
        cancelText: opts.cancelText || "Cancel",
        dangerOk: opts.dangerOk || false,
      });
    },

    confirmDanger: function(opts) {
      opts = opts || {};
      if (hasViewer()) return Goop.dialog.confirmDanger(opts);
      return fallbackDialog({
        title: opts.title || "Confirm",
        message: opts.message || "",
        input: { placeholder: opts.placeholder || "Type " + (opts.match || "DELETE"), match: opts.match || "DELETE" },
        okText: opts.okText || "Delete",
        cancelText: opts.cancelText || "Cancel",
        dangerOk: true,
      });
    },

    theme: function() {
      return document.documentElement.getAttribute("data-theme") || "dark";
    },
  };
})();
