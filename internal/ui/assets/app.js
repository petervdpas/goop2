// internal/ui/assets/app.js
(() => {
  const BASE = "/assets/js/";

  const files = [
    "00-core.js",
    "10-peers-autorefresh.js",
    "20-theme.js",
    "30-banners.js",
    "40-rendezvous.js",
    "50-editor.js",
    "60-codemirror.js",
    "99-dialogs.js",
    "99-toast.js",
  ];

  function loadSequentially(list, i = 0) {
    if (i >= list.length) return;

    const s = document.createElement("script");
    s.src = BASE + list[i];
    s.defer = true;
    s.onload = () => loadSequentially(list, i + 1);
    s.onerror = () => {
      console.error("Failed to load", s.src);
      // Continue loading the rest so the app isn't dead if one file is missing.
      loadSequentially(list, i + 1);
    };
    document.head.appendChild(s);
  }

  loadSequentially(files);
})();
