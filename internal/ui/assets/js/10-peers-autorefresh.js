(() => {
  const { onReady } = window.Goop.core;

  onReady(() => {
    const url = new URL(window.location.href);
    if (url.pathname !== "/peers") return;
    if (url.searchParams.get("autorefresh") !== "1") return;

    setInterval(() => {
      if (!document.hasFocus()) return;
      if (Goop.call && Goop.call.activeCalls().length > 0) return;
      window.location.reload();
    }, 5000);
  });
})();
