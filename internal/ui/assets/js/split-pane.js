(() => {
  const G = window.Goop || (window.Goop = {});
  const STORAGE_PREFIX = 'goop:split:';
  const DEFAULT_MIN = 150;
  const DEFAULT_MAX_PCT = 50;

  function parse(el) {
    const key = el.dataset.splitKey || '';
    const min = parseInt(el.dataset.splitMin, 10) || DEFAULT_MIN;
    let max = el.dataset.splitMax;
    if (max && max.endsWith('%')) {
      max = { pct: parseInt(max, 10) || DEFAULT_MAX_PCT };
    } else if (max) {
      max = { px: parseInt(max, 10) };
    } else {
      max = { pct: DEFAULT_MAX_PCT };
    }
    return { key, min, max };
  }

  function maxPx(cfg, containerW) {
    return cfg.max.px != null ? cfg.max.px : containerW * cfg.max.pct / 100;
  }

  function clamp(w, cfg, containerW) {
    return Math.max(cfg.min, Math.min(w, maxPx(cfg, containerW)));
  }

  function save(key, w) {
    if (!key) return;
    try { localStorage.setItem(STORAGE_PREFIX + key, w); } catch (_) {}
  }

  function load(key) {
    if (!key) return null;
    try {
      const v = localStorage.getItem(STORAGE_PREFIX + key);
      return v != null ? parseInt(v, 10) : null;
    } catch (_) { return null; }
  }

  function initOne(el) {
    if (el._splitInit) return;

    if (el.offsetParent === null) return;

    el._splitInit = true;

    const left = el.querySelector(':scope > .split-left');
    const right = el.querySelector(':scope > .split-right');
    if (!left || !right) return;

    const handle = document.createElement('div');
    handle.className = 'split-handle';
    el.insertBefore(handle, right);

    const cfg = parse(el);
    const defaultW = left.offsetWidth || cfg.min;

    function apply(w) {
      const cw = el.offsetWidth;
      if (cw === 0) return;
      const clamped = clamp(w, cfg, cw);
      left.style.width = clamped + 'px';
    }

    const saved = load(cfg.key);
    if (saved != null) apply(saved);

    let startX, startW, overlay;

    function onMove(e) {
      const dx = (e.clientX || e.touches[0].clientX) - startX;
      const w = startW + dx;
      apply(w);
    }

    function onUp() {
      handle.classList.remove('dragging');
      document.body.style.userSelect = '';
      document.body.style.cursor = '';
      if (overlay) { overlay.remove(); overlay = null; }
      save(cfg.key, left.offsetWidth);
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      document.removeEventListener('touchmove', onMove);
      document.removeEventListener('touchend', onUp);
    }

    function onDown(e) {
      e.preventDefault();
      startX = e.clientX || e.touches[0].clientX;
      startW = left.offsetWidth;
      handle.classList.add('dragging');
      document.body.style.userSelect = 'none';
      document.body.style.cursor = 'col-resize';
      overlay = document.createElement('div');
      overlay.style.cssText = 'position:fixed;inset:0;z-index:9999;cursor:col-resize;';
      document.body.appendChild(overlay);
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup', onUp);
      document.addEventListener('touchmove', onMove);
      document.addEventListener('touchend', onUp);
    }

    handle.addEventListener('mousedown', onDown);
    handle.addEventListener('touchstart', onDown, { passive: false });

    handle.addEventListener('dblclick', () => {
      apply(defaultW);
      save(cfg.key, defaultW);
    });
  }

  G.splitPane = {
    init(root) {
      const container = root || document;
      container.querySelectorAll('.split-layout').forEach(initOne);
    }
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', () => G.splitPane.init());
  } else {
    G.splitPane.init();
  }
})();
