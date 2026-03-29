// Unified scroll pane utility.
// Exposes Goop.scroll = { init, toBottom, savePos, restorePos }
//
// CSS classes (defined in components.css):
//   .scroll-pane    — flex child that fills space and scrolls vertically
//   .scroll-bounded — constrained scroll with --scroll-max (default 40%)
//
// JS helpers for DOM manipulation:
//   init(el)          — add scroll-pane class to element
//   toBottom(el)      — scroll element to bottom
//   savePos(el, key)  — save scroll position to sessionStorage
//   restorePos(el, key) — restore saved scroll position
//
(() => {
  window.Goop = window.Goop || {};

  function init(el) {
    if (!el) return;
    el.classList.add("scroll-pane");
  }

  function toBottom(el) {
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }

  function savePos(el, key) {
    if (!el || !key) return;
    try { sessionStorage.setItem("scroll:" + key, el.scrollTop); } catch (e) {}
  }

  function restorePos(el, key) {
    if (!el || !key) return;
    try {
      var pos = sessionStorage.getItem("scroll:" + key);
      if (pos !== null) el.scrollTop = parseInt(pos, 10) || 0;
    } catch (e) {}
  }

  Goop.scroll = { init: init, toBottom: toBottom, savePos: savePos, restorePos: restorePos };
})();
