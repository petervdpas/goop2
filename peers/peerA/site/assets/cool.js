(() => {
  const $ = (sel, root = document) => root.querySelector(sel);
  const $$ = (sel, root = document) => Array.from(root.querySelectorAll(sel));

  // ------- Toast -------
  const toastEl = $(".toast");
  let toastTimer = null;

  function toast(msg) {
    if (!toastEl) return;
    toastEl.textContent = msg;
    toastEl.hidden = false;
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => (toastEl.hidden = true), 1800);
  }

  // ------- Theme -------
  const THEME_KEY = "goop.theme";
  function applyTheme(t) {
    if (t === "light") document.documentElement.setAttribute("data-theme", "light");
    else document.documentElement.removeAttribute("data-theme");
  }
  function loadTheme() {
    const saved = localStorage.getItem(THEME_KEY);
    if (saved === "light" || saved === "dark") return saved;
    // default: dark
    return "dark";
  }
  function toggleTheme() {
    const cur = loadTheme();
    const next = cur === "dark" ? "light" : "dark";
    localStorage.setItem(THEME_KEY, next);
    applyTheme(next);
    toast(`Theme: ${next}`);
  }

  // ------- Page "soft transition" -------
  // A tiny polish: fade in main content on load.
  function fadeIn() {
    const main = $("main");
    if (!main) return;
    main.style.opacity = "0";
    main.style.transform = "translateY(6px)";
    main.style.transition = "opacity 220ms ease, transform 220ms ease";
    requestAnimationFrame(() => {
      main.style.opacity = "1";
      main.style.transform = "translateY(0)";
    });
  }

  // ------- Session id -------
  function makeSession() {
    const chars = "abcdef0123456789";
    let s = "";
    for (let i = 0; i < 8; i++) s += chars[(Math.random() * chars.length) | 0];
    return s;
  }

  // ------- Optional status polling -------
  // If you later expose /__goop/status, this will populate without breaking anything today.
  async function refreshStatus() {
    const latencyEl = $$("[data-latency]");
    const presenceEl = $("[data-presence]");
    const peerLabelEl = $("[data-peer-label]");

    // Always show a client session (even without any server endpoint)
    const sessionEl = $$("[data-session]");
    const existing = sessionEl[0]?.textContent?.trim();
    if (!existing || existing === "—") {
      const s = makeSession();
      sessionEl.forEach((n) => (n.textContent = s));
    }

    // Attempt status endpoint; if missing, keep placeholders.
    const t0 = performance.now();
    try {
      const res = await fetch("/__goop/status", { cache: "no-store" });
      const t1 = performance.now();
      const rtt = Math.max(0, Math.round(t1 - t0));
      latencyEl.forEach((n) => (n.textContent = String(rtt)));

      if (!res.ok) throw new Error(`status ${res.status}`);
      const json = await res.json();

      if (peerLabelEl && json.peerLabel) peerLabelEl.textContent = String(json.peerLabel);
      if (presenceEl) {
        const online = !!json.online;
        presenceEl.textContent = online ? "Online" : "Offline";
        presenceEl.classList.toggle("ok", online);
        presenceEl.classList.toggle("bad", !online);
      }
      if (typeof json.latencyMs === "number") latencyEl.forEach((n) => (n.textContent = String(json.latencyMs)));
    } catch {
      // Fallback: show ping-like value only if we can fetch *anything* (this JS file already loaded)
      // Keep presence as Online by default; you can flip it when you implement status.
      const t1 = performance.now();
      const rtt = Math.max(0, Math.round(t1 - t0));
      if (Number.isFinite(rtt)) latencyEl.forEach((n) => (n.textContent = String(rtt)));
    }
  }

  // ------- Share link copy -------
  async function copyText(text) {
    try {
      await navigator.clipboard.writeText(text);
      toast("Copied link to clipboard");
    } catch {
      // Fallback
      const ta = document.createElement("textarea");
      ta.value = text;
      ta.style.position = "fixed";
      ta.style.left = "-9999px";
      document.body.appendChild(ta);
      ta.select();
      document.execCommand("copy");
      ta.remove();
      toast("Copied link");
    }
  }

  // ------- Confetti (cheap, no deps) -------
  function confetti() {
    const count = 26;
    for (let i = 0; i < count; i++) {
      const s = document.createElement("span");
      s.textContent = ["◆", "✦", "✶", "•"][i % 4];
      s.style.position = "fixed";
      s.style.left = `${Math.random() * 100}vw`;
      s.style.top = `-10px`;
      s.style.zIndex = "50";
      s.style.fontSize = `${10 + Math.random() * 18}px`;
      s.style.opacity = "0.9";
      s.style.transform = `rotate(${Math.random() * 360}deg)`;
      s.style.pointerEvents = "none";
      s.style.filter = "drop-shadow(0 6px 18px rgba(0,0,0,0.25))";
      document.body.appendChild(s);

      const dx = (Math.random() - 0.5) * 140;
      const dy = 110 + Math.random() * 260;
      const dur = 800 + Math.random() * 900;
      const start = performance.now();

      const tick = (now) => {
        const t = Math.min(1, (now - start) / dur);
        const ease = 1 - Math.pow(1 - t, 3);
        s.style.transform = `translate(${dx * ease}px, ${dy * ease}px) rotate(${360 * ease}deg)`;
        s.style.opacity = String(0.9 * (1 - t));
        if (t < 1) requestAnimationFrame(tick);
        else s.remove();
      };
      requestAnimationFrame(tick);
    }
    toast("✨ goop!");
  }

  // ------- Konami easter egg -------
  const konami = ["ArrowUp","ArrowUp","ArrowDown","ArrowDown","ArrowLeft","ArrowRight","ArrowLeft","ArrowRight","b","a"];
  let k = 0;
  function onKey(e) {
    const key = e.key;
    const expected = konami[k];
    const match = (expected.length === 1) ? (key.toLowerCase() === expected) : (key === expected);
    if (match) {
      k++;
      if (k === konami.length) {
        k = 0;
        confetti();
      }
    } else {
      k = 0;
    }
  }

  // ------- Actions -------
  function bindActions() {
    $$("[data-action]").forEach((el) => {
      el.addEventListener("click", async () => {
        const action = el.getAttribute("data-action");
        if (action === "toggle-theme") return toggleTheme();
        if (action === "confetti") return confetti();
        if (action === "toast") return toast(el.getAttribute("data-toast") || "OK");
        if (action === "copy-link") {
          const p = el.getAttribute("data-copy") || location.href;
          // If you later replace PEER_ID server-side, this becomes a real share link automatically.
          return copyText(p.replace("PEER_ID", "peerA"));
        }
        if (action === "ping") {
          const t0 = performance.now();
          // Try a cheap endpoint; if you don't have it, this still resolves quickly as a network error.
          try { await fetch("/__goop/ping", { cache: "no-store" }); } catch {}
          const t1 = performance.now();
          toast(`Ping: ${Math.max(0, Math.round(t1 - t0))}ms`);
          return;
        }
      });
    });
  }

  // ------- Boot -------
  function boot() {
    applyTheme(loadTheme());
    fadeIn();
    bindActions();
    refreshStatus();
    setInterval(refreshStatus, 3000);
    window.addEventListener("keydown", onKey);
  }

  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", boot);
  else boot();
})();
