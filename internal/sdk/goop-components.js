//
// Component library loader — loads all goop-component-*.js files.
// Include this single file to get every component, or pick individual ones.
//
// Usage (load everything):
//   <script src="/sdk/goop-components.js"></script>
//
// Usage (pick what you need):
//   <script src="/sdk/goop-component-base.js"></script>
//   <script src="/sdk/goop-component-toast.js"></script>
//   <script src="/sdk/goop-component-tabs.js"></script>
//
(() => {
  var files = [
    "goop-component-base.js",
    "goop-component-toast.js",
    "goop-component-dialog.js",
    "goop-component-datepicker.js",
    "goop-component-select.js",
    "goop-component-colorpicker.js",
    "goop-component-toggle.js",
    "goop-component-tabs.js",
    "goop-component-accordion.js",
    "goop-component-taginput.js",
    "goop-component-stepper.js",
    "goop-component-sidebar.js",
    "goop-component-carousel.js",
    "goop-component-lightbox.js",
    "goop-component-toolbar.js",
    "goop-component-badge.js",
    "goop-component-progress.js",
    "goop-component-pagination.js",
    "goop-component-tooltip.js",
    "goop-component-panel.js",
  ];

  var sdkBase = "/sdk/";
  var thisScript = document.currentScript;
  if (thisScript && thisScript.src) {
    var idx = thisScript.src.lastIndexOf("/");
    if (idx >= 0) sdkBase = thisScript.src.substring(0, idx + 1);
  }

  function load(src, cb) {
    var s = document.createElement("script");
    s.src = sdkBase + src;
    if (cb) s.onload = cb;
    s.onerror = function() { console.warn("goop-components: failed to load " + src); if (cb) cb(); };
    document.head.appendChild(s);
  }

  load(files[0], function() {
    for (var i = 1; i < files.length; i++) load(files[i]);
  });
})();
