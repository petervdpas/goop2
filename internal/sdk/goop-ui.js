//
// Portable UI helpers for site pages.
// Works standalone — does not depend on the viewer's CSS.
//
// Usage:
//
//   <script src="/sdk/goop-ui.js"></script>
//
//   // toast notification
//   Goop.ui.toast("Saved!");
//   Goop.ui.toast({ title: "Success", message: "Row inserted", duration: 3000 });
//
//   // confirm dialog (returns true/false)
//   if (await Goop.ui.confirm("Delete this row?")) { ... }
//
//   // prompt dialog (returns string or null)
//   const name = await Goop.ui.prompt("Enter a name", "default");
//
//   // current theme
//   const theme = Goop.ui.theme(); // "dark" or "light"
//
(() => {
  window.Goop = window.Goop || {};

  // ── inject minimal CSS (only once) ──
  const STYLE_ID = "goop-ui-style";
  if (!document.getElementById(STYLE_ID)) {
    const s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent = `
      .goop-toast-wrap{position:fixed;top:1rem;right:1rem;z-index:99999;display:flex;flex-direction:column;gap:.5rem;pointer-events:none}
      .goop-toast{pointer-events:auto;background:#1e2433;color:#e6e9ef;border:1px solid #2a3142;border-radius:8px;padding:.6rem 1rem;font:14px/1.4 system-ui,sans-serif;box-shadow:0 4px 12px rgba(0,0,0,.4);animation:goop-fade-in .2s ease}
      .goop-toast.exit{opacity:0;transition:opacity .25s}
      .goop-toast-title{font-weight:600;margin-bottom:.15rem}
      @keyframes goop-fade-in{from{opacity:0;transform:translateY(-8px)}to{opacity:1;transform:none}}
      .goop-dlg-bg{position:fixed;inset:0;background:rgba(0,0,0,.55);z-index:99998;display:flex;align-items:center;justify-content:center}
      .goop-dlg{background:#151924;color:#e6e9ef;border:1px solid #2a3142;border-radius:12px;padding:1.25rem;min-width:300px;max-width:420px;font:14px/1.5 system-ui,sans-serif;box-shadow:0 8px 24px rgba(0,0,0,.5)}
      .goop-dlg h3{margin:0 0 .5rem;font-size:1rem}
      .goop-dlg p{margin:0 0 .75rem;color:#9aa3b2}
      .goop-dlg input{width:100%;box-sizing:border-box;padding:.45rem .6rem;border:1px solid #2a3142;border-radius:6px;background:#0f1115;color:#e6e9ef;font:inherit;margin-bottom:.75rem}
      .goop-dlg-btns{display:flex;gap:.5rem;justify-content:flex-end}
      .goop-dlg-btns button{padding:.4rem .9rem;border:1px solid #2a3142;border-radius:6px;cursor:pointer;font:inherit;background:#1e2433;color:#e6e9ef}
      .goop-dlg-btns button.primary{background:#7aa2ff;color:#0f1115;border-color:#7aa2ff}
    `;
    document.head.appendChild(s);
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
    const wrap = ensureToastWrap();
    const el = document.createElement("div");
    el.className = "goop-toast";
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

  // ── dialogs ──
  function dialog(title, message, hasInput, defaultValue) {
    return new Promise((resolve) => {
      const bg = document.createElement("div");
      bg.className = "goop-dlg-bg";

      const inputHtml = hasInput
        ? '<input class="goop-dlg-inp" value="' + escAttr(defaultValue || "") + '" />'
        : "";

      bg.innerHTML =
        '<div class="goop-dlg">' +
        "<h3>" + esc(title || "") + "</h3>" +
        "<p>" + esc(message || "") + "</p>" +
        inputHtml +
        '<div class="goop-dlg-btns">' +
        '<button class="cancel">Cancel</button>' +
        '<button class="primary ok">OK</button>' +
        "</div></div>";

      const inp = bg.querySelector(".goop-dlg-inp");

      function done(val) {
        document.removeEventListener("keydown", onKey);
        bg.remove();
        resolve(val);
      }

      function onKey(e) {
        if (e.key === "Escape") done(null);
        if (e.key === "Enter") done(inp ? inp.value : true);
      }

      bg.querySelector(".cancel").onclick = () => done(null);
      bg.querySelector(".ok").onclick = () => done(inp ? inp.value : true);
      bg.addEventListener("mousedown", (e) => { if (e.target === bg) done(null); });
      document.addEventListener("keydown", onKey);

      document.body.appendChild(bg);
      if (inp) { setTimeout(() => { inp.focus(); inp.select(); }, 0); }
    });
  }

  function esc(s) {
    const d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function escAttr(s) {
    return esc(s).replace(/"/g, "&quot;");
  }

  window.Goop.ui = {
    toast,

    /** Show a confirm dialog. Resolves to true or null. */
    confirm(message, title) {
      return dialog(title || "Confirm", message, false, null);
    },

    /** Show a prompt dialog. Resolves to string or null. */
    prompt(message, defaultValue, title) {
      return dialog(title || "Input", message, true, defaultValue);
    },

    /** Get the current theme ("dark" or "light") */
    theme() {
      return document.documentElement.getAttribute("data-theme") || "dark";
    },
  };
})();
