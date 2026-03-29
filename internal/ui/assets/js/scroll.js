// Unified scroll pane component.
// Exposes Goop.scroll = { init }
//
// Marks elements with class "scroll-pane" as scrollable flex children.
// Apply the class in HTML or add it dynamically, then call init() to
// ensure the element participates in flex layout correctly.
//
// Usage:
//   <div class="scroll-pane">...long content...</div>
//
//   // After dynamically adding scroll-pane elements:
//   Goop.scroll.init(container)
//
(() => {
  window.Goop = window.Goop || {};

  var STYLE_ID = "goop-scroll-style";

  function injectStyle() {
    if (document.getElementById(STYLE_ID)) return;
    var style = document.createElement("style");
    style.id = STYLE_ID;
    style.textContent =
      ".scroll-pane{flex:1 1 0;min-height:0;overflow-y:auto}" +
      ".scroll-bounded{overflow-y:auto;max-height:var(--scroll-max,40%)}";
    document.head.appendChild(style);
  }

  function init(container) {
    if (!container) container = document;
    injectStyle();
  }

  injectStyle();

  Goop.scroll = { init: init };
})();
