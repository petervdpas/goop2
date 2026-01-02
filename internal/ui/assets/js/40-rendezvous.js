// internal/ui/assets/js/40-rendezvous.js
(() => {
  const { onReady, qs, setHidden } = window.Goop.core;

  onReady(() => {
    const host = qs("#rv_host");
    const port = qs("#rv_port");
    const open = qs("#rv-open");
    const link = qs("#rv-open-link");

    if (!host || !port || !open) return;

    function normalizePort(v) {
      v = String(v || "").trim();
      if (!v) return "8787";
      if (!/^\d+$/.test(v)) return "8787";
      return v;
    }

    function sync() {
      const on = !!host.checked;

      port.disabled = !on;
      port.classList.toggle("rv-disabled", !on);

      setHidden(open, !on);

      if (link) {
        const p = normalizePort(port.value);
        link.href = `http://127.0.0.1:${p}/`;
      }
    }

    host.addEventListener("change", sync);
    port.addEventListener("input", sync);
    sync();
  });
})();
