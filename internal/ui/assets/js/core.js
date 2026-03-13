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
      var text = (await resp.text()).trim();
      throw new Error(text || resp.statusText);
    }
    var ct = resp.headers.get("Content-Type") || "";
    if (ct.includes("application/json")) {
      return resp.json();
    }
    return null;
  }

  function copyToClipboard(text, element) {
    navigator.clipboard.writeText(text).then(function() {
      if (element) {
        element.classList.add('copied');
        setTimeout(function() {
          element.classList.remove('copied');
        }, 1200);
      }
    });
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

  // ── Call buttons ─────────────────────────────────────────────────────────────

  function callDisabledReason(peerVideoDisabled, selfVideoDisabled) {
    if (peerVideoDisabled && selfVideoDisabled) return 'Calls disabled by you and this peer';
    if (peerVideoDisabled) return 'This peer has disabled calls';
    if (selfVideoDisabled) return 'You have calls disabled in settings';
    return '';
  }

  // opts: { cls, audioId, videoId, large }
  // cls defaults to 'peer-call-btn'; large=true uses bigger SVG sizes for the chat page
  function callButtonsHTML(peerId, disabledReason, opts) {
    opts = opts || {};
    var cls  = opts.cls || 'peer-call-btn';
    var dis  = disabledReason ? ' disabled' : '';
    var aTitle = escapeHtml(disabledReason || 'Voice call');
    var vTitle = escapeHtml(disabledReason || 'Video call');
    var aAttr  = opts.audioId ? ' id="' + opts.audioId + '"' : ' data-call-audio="' + escapeHtml(peerId) + '"';
    var vAttr  = opts.videoId ? ' id="' + opts.videoId + '"' : ' data-call-video="' + escapeHtml(peerId) + '"';
    var aW = opts.large ? '18' : '14';
    var vW = opts.large ? '20' : '16';
    return '<button class="' + cls + '"' + dis + aAttr + ' title="' + aTitle + '">' +
        '<svg width="' + aW + '" height="' + aW + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
          '<path d="M22 16.92v3a2 2 0 0 1-2.18 2 19.79 19.79 0 0 1-8.63-3.07 19.5 19.5 0 0 1-6-6 19.79 19.79 0 0 1-3.07-8.67A2 2 0 0 1 4.11 2h3a2 2 0 0 1 2 1.72c.12.8.3 1.58.52 2.34a2 2 0 0 1-.45 2.11L8.09 9.91a16 16 0 0 0 6 6l1.27-1.27a2 2 0 0 1 2.11-.45c.76.22 1.54.4 2.34.52A2 2 0 0 1 22 16.92z"/>' +
        '</svg>' +
      '</button>' +
      '<button class="' + cls + '"' + dis + vAttr + ' title="' + vTitle + '">' +
        '<svg width="' + vW + '" height="' + aW + '" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">' +
          '<polygon points="23 7 16 12 23 17 23 7"/>' +
          '<rect x="1" y="5" width="15" height="14" rx="2" ry="2"/>' +
        '</svg>' +
      '</button>';
  }

  // ── Unreachable peer guard ──────────────────────────────────────────────────
  // Any element inside a [data-unreachable] container has clicks suppressed.
  // Links, buttons, and onclick handlers are blocked with a toast notification.
  // Peer name links (a.peerid) are exempt — you can still view peer info.
  document.addEventListener('click', function(e) {
    var container = e.target.closest('[data-unreachable]');
    if (!container) return;

    // Only block elements explicitly marked as peer actions.
    // Use data-peer-action on buttons/links that require reachability,
    // or match common peer action patterns.
    var interactive = e.target.closest('[data-peer-action], [data-call-audio], [data-call-video], [onclick*="openSite"], .identity-site-btn, .peer-call-btn, .call-btn');
    if (!interactive) return;

    e.preventDefault();
    e.stopPropagation();
    if (toast) toast('Peer is unreachable', 2000);
  }, true); // capture phase so we beat all other handlers

  // Lightweight namespace
  function validateInput(value, opts) {
    opts = opts || {};
    var v = (value || '').trim();
    if (!v) return opts.allowEmpty ? { ok: true, value: '' } : { ok: false, value: v, error: 'required' };
    if (opts.lowercase) v = v.toLowerCase();
    if (opts.noSpaces && /\s/.test(v)) return { ok: false, value: v, error: 'no spaces allowed' };
    if (opts.maxLength && v.length > opts.maxLength) return { ok: false, value: v, error: 'max ' + opts.maxLength + ' characters' };
    if (opts.pattern && !opts.pattern.test(v)) return { ok: false, value: v, error: opts.patternHint || 'invalid characters' };
    return { ok: true, value: v };
  }

  function validateJSON(value) {
    var v = (value || '').trim();
    if (!v) return { ok: true, value: null };
    try { return { ok: true, value: JSON.parse(v) }; }
    catch (e) { return { ok: false, error: e.message }; }
  }

  function bindValidation(input, opts) {
    if (!input) return;
    on(input, 'blur', function () {
      var r = validateInput(input.value, opts);
      input.value = r.value;
    });
  }

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
    copyToClipboard,
    callDisabledReason,
    callButtonsHTML,
    validateInput,
    validateJSON,
    bindValidation,
  };
})();
