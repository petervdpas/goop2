// internal/ui/assets/js/00-core.js
(() => {
  function onReady(fn) {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", fn, { once: true });
    } else {
      fn();
    }
  }

  function safeLocalStorageGet(key) {
    try { return localStorage.getItem(key); } catch { return null; }
  }

  function safeLocalStorageSet(key, value) {
    try { localStorage.setItem(key, value); } catch {}
  }

  function qs(sel, root = document) {
    return root.querySelector(sel);
  }

  function qsa(sel, root = document) {
    return Array.from(root.querySelectorAll(sel));
  }

  function on(el, evt, fn, opts) {
    if (!el) return;
    el.addEventListener(evt, fn, opts);
  }

  function closest(el, sel) {
    return el ? el.closest(sel) : null;
  }

  function escapeCss(s) {
    return CSS && CSS.escape ? CSS.escape(String(s)) : String(s).replace(/["\\]/g, "\\$&");
  }

  function setHidden(el, hidden) {
    if (!el) return;
    el.classList.toggle("hidden", !!hidden);
  }

  function escapeHtml(str) {
    var d = document.createElement("div");
    d.textContent = String(str == null ? "" : str);
    return d.innerHTML;
  }

  async function api(url, body) {
    var resp = await fetch(url, {
      method: body !== undefined ? "POST" : "GET",
      headers: body !== undefined ? { "Content-Type": "application/json" } : {},
      body: body !== undefined ? JSON.stringify(body) : undefined,
    });
    if (!resp.ok) {
      var text = await resp.text();
      throw new Error(text || resp.statusText);
    }
    var ct = resp.headers.get("Content-Type") || "";
    if (ct.includes("application/json")) {
      return resp.json();
    }
    return null;
  }

  function toast(msg, isError) {
    if (window.Goop && window.Goop.toast) {
      window.Goop.toast({
        icon: isError ? "!" : "ok",
        title: isError ? "Error" : "Success",
        message: msg,
        duration: isError ? 6000 : 3000,
      });
    }
  }

  // Lightweight namespace
  window.Goop = window.Goop || {};
  window.Goop.core = {
    onReady,
    qs,
    qsa,
    on,
    closest,
    escapeCss,
    setHidden,
    safeLocalStorageGet,
    safeLocalStorageSet,
    escapeHtml,
    api,
    toast,
  };
})();
