//
// Portable UI helpers for site pages.
// When the viewer's dialogs.js is loaded, delegates to Goop.dialog.
// Otherwise uses a self-contained fallback (inline CSS, no deps).
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

  // ── inject minimal CSS for fallback dialogs + toast (only once) ──
  const STYLE_ID = "goop-ui-style";
  if (!document.getElementById(STYLE_ID)) {
    const s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .goop-toast-wrap{position:fixed;top:1rem;right:1rem;z-index:99999;display:flex;flex-direction:column;gap:.5rem;pointer-events:none}
      .goop-toast{pointer-events:auto;border-radius:8px;padding:.6rem 1rem;font:14px/1.4 system-ui,sans-serif;animation:goop-fade-in .2s ease}
      .goop-toast.exit{opacity:0;transition:opacity .25s}
      .goop-toast-title{font-weight:600;margin-bottom:.15rem}
      @keyframes goop-fade-in{from{opacity:0;transform:translateY(-8px)}to{opacity:1;transform:none}}
      .goop-dlg-bg{position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:99998;display:flex;align-items:center;justify-content:center;backdrop-filter:blur(6px)}
      .goop-dlg{border-radius:16px;min-width:300px;max-width:420px;font:14px/1.5 system-ui,sans-serif;overflow:hidden}
      .goop-dlg-head{padding:12px 14px;font-weight:750}
      .goop-dlg-body{padding:14px;display:flex;flex-direction:column;gap:10px}
      .goop-dlg-msg{font-size:13px;white-space:pre-wrap}
      .goop-dlg input{width:100%;box-sizing:border-box;padding:10px 12px;border-radius:999px;font:inherit;outline:none;transition:border-color .17s,box-shadow .17s}
      .goop-dlg-foot{padding:12px 14px;display:flex;gap:8px;justify-content:flex-end}
      .goop-dlg-foot button{padding:8px 12px;border-radius:999px;cursor:pointer;font:inherit;transition:background .17s,transform .17s}
      .goop-dlg-foot button:hover{transform:translateY(-0.5px)}
      .goop-dlg-foot button:disabled{opacity:.4;cursor:not-allowed;transform:none}
    `;
    document.head.appendChild(s);
  }

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

  function applyThemeToEl(el, t) {
    el.style.background = t.panel;
    el.style.color = t.text;
    el.style.borderColor = t.line;
    el.style.boxShadow = t.shadow;
  }

  // ── toast ──
  let toastWrap = null;

  function ensureToastWrap() {
    if (!toastWrap) {
      toastWrap = document.createElement("div");
      toastWrap.className = "goop-toast-wrap";
      document.body.appendChild(toastWrap);
    }
    return toastWrap;
  }

  function toast(opts) {
    if (typeof opts === "string") opts = { message: opts };
    var t = resolveTheme();
    const wrap = ensureToastWrap();
    const el = document.createElement("div");
    el.className = "goop-toast";
    applyThemeToEl(el, t);
    const title = opts.title ? '<div class="goop-toast-title">' + esc(opts.title) + "</div>" : "";
    el.innerHTML = title + "<div>" + esc(opts.message || "") + "</div>";
    wrap.appendChild(el);
    const dur = opts.duration || 4000;
    if (dur > 0) {
      setTimeout(() => {
        el.classList.add("exit");
        setTimeout(() => el.remove(), 300);
      }, dur);
    }
    return el;
  }

  // ── fallback dialog (self-contained, no external deps) ──
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
      bg.className = "goop-dlg-bg";

      var bodyHtml = '<div class="goop-dlg-msg"></div>';
      if (hasInput) bodyHtml += '<input autocomplete="off" spellcheck="false" />';

      var footHtml = '';
      if (cancelLabel !== null) footHtml += '<button class="cancel"></button>';
      if (!hideOk) footHtml += '<button class="ok' + (okDanger ? ' danger' : '') + '"></button>';

      bg.innerHTML =
        '<div class="goop-dlg">' +
          '<div class="goop-dlg-head"></div>' +
          '<div class="goop-dlg-body">' + bodyHtml + '</div>' +
          '<div class="goop-dlg-foot">' + footHtml + '</div>' +
        '</div>';

      var dlg = bg.querySelector(".goop-dlg");
      applyThemeToEl(dlg, t);

      var head = dlg.querySelector(".goop-dlg-head");
      head.textContent = title;
      head.style.borderBottomColor = t.line;

      var msg = dlg.querySelector(".goop-dlg-msg");
      msg.textContent = message;
      msg.style.color = t.muted;

      var foot = dlg.querySelector(".goop-dlg-foot");
      foot.style.borderTopColor = t.line;

      var inp = hasInput ? dlg.querySelector("input") : null;
      if (inp) {
        inp.placeholder = input.placeholder || "";
        inp.value = input.value || "";
        if (input.type) inp.type = input.type;
        inp.style.border = "1px solid " + t.line;
        inp.style.background = t.field;
        inp.style.color = t.text;
      }

      var bCancel = dlg.querySelector("button.cancel");
      var bOk = dlg.querySelector("button.ok");

      function styleBtn(btn, danger) {
        var c = danger ? "#ff5a5a" : t.accent;
        btn.style.border = "1px solid " + c;
        btn.style.background = "color-mix(in srgb," + c + " 14%,transparent)";
        btn.style.color = t.text;
      }

      if (bCancel) { bCancel.textContent = cancelLabel; styleBtn(bCancel, false); }
      if (bOk) {
        bOk.textContent = okLabel;
        styleBtn(bOk, okDanger);
        if (match) bOk.disabled = true;
      }

      if (match && inp) {
        inp.addEventListener("input", function() { bOk.disabled = inp.value !== match; });
      }

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

  // ── helpers ──
  function esc(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  // ── public API ──
  window.Goop.ui = {
    toast,

    dialog: function(opts) {
      return hasViewer() ? Goop.dialog(opts) : fallbackDialog(opts);
    },

    alert: function(title, message) {
      return hasViewer() ? Goop.dialog.alert(title, message) : fallbackDialog({ title: title, message: message, cancel: false });
    },

    confirm: function(message, title) {
      return hasViewer() ? Goop.dialog.confirm(message, title) : fallbackDialog({ title: title || "Confirm", message: message });
    },

    prompt: function(opts) {
      if (typeof opts === "string") opts = { message: opts };
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
