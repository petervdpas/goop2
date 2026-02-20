(() => {
  const { onReady, qsa } = window.Goop.core;

  function autoDismiss({
    selector = ".banner.ok",
    afterMs = 3500,
    removeAfterMs = 260,
    persistAttr = "data-persist",
  } = {}) {
    qsa(selector).forEach((el) => {
      if (el.hasAttribute(persistAttr)) return;
      if (el.dataset.autohideWired === "1") return;
      el.dataset.autohideWired = "1";

      el.classList.add("banner-autohide");

      window.setTimeout(() => {
        el.classList.add("is-gone");
        window.setTimeout(() => {
          if (el && el.parentNode) el.parentNode.removeChild(el);
        }, removeAfterMs);
      }, afterMs);
    });
  }

  window.Goop.banners = { autoDismiss };

  onReady(() => autoDismiss());
})();
