// internal/ui/assets/js/10-peers-autorefresh.js
(() => {
  const { onReady } = window.Goop.core;

  onReady(() => {
    const url = new URL(window.location.href);
    if (url.pathname !== "/peers") return;
    if (url.searchParams.get("autorefresh") !== "1") return;

    setInterval(() => {
      if (document.hasFocus()) window.location.reload();
    }, 5000);
  });
})();
