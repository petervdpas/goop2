(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var SID = "gc-lightbox-style";
  if (!document.getElementById(SID)) {
    var s = document.createElement("style"); s.id = SID;
    s.textContent = `
      .gc-lightbox {
        display: none; position: fixed; top: 0; left: 0; right: 0; bottom: 0;
        z-index: 9995; background: rgba(0,0,0,.9); backdrop-filter: blur(6px);
        align-items: center; justify-content: center;
      }
      .gc-lightbox[data-goop-open] { display: flex; }
      .gc-lightbox img { max-width: 90vw; max-height: 85vh; object-fit: contain; border-radius: var(--goop-radius, 6px); }
      .gc-lightbox-close { position: absolute; top: 1rem; right: 1rem; background: none; border: none; color: #fff; font-size: 2rem; cursor: pointer; }
      .gc-lightbox-prev, .gc-lightbox-next {
        position: absolute; top: 50%; transform: translateY(-50%);
        background: none; border: none; color: #fff; font-size: 2.5rem; cursor: pointer; padding: 1rem;
      }
      .gc-lightbox-prev { left: .5rem; }
      .gc-lightbox-next { right: .5rem; }
      .gc-lightbox-caption { position: absolute; bottom: 1.5rem; left: 50%; transform: translateX(-50%); color: #fff; font-size: .9rem; text-align: center; max-width: 80vw; }
      .gc-lightbox-counter { position: absolute; top: 1rem; left: 1rem; color: rgba(255,255,255,.7); font-size: .85rem; }
    `;
    document.head.appendChild(s);
  }

  Goop.ui.lightbox = function(opts) {
    opts = opts || {};
    var items = opts.items || [];
    var current = 0;
    var showCounter = !!opts.showCounter;
    var loop = opts.loop !== false;

    var lb = document.createElement("div");
    lb.className = "gc-lightbox";
    lb.setAttribute("data-goop-component", "lightbox");

    var closeBtn = document.createElement("button"); closeBtn.type = "button"; closeBtn.className = "gc-lightbox-close"; closeBtn.textContent = "\u00D7";
    var prevBtn = document.createElement("button"); prevBtn.type = "button"; prevBtn.className = "gc-lightbox-prev"; prevBtn.textContent = "\u2039";
    var nextBtn = document.createElement("button"); nextBtn.type = "button"; nextBtn.className = "gc-lightbox-next"; nextBtn.textContent = "\u203A";
    var img = document.createElement("img");
    var caption = document.createElement("div"); caption.className = "gc-lightbox-caption";
    var counter = null;
    if (showCounter) { counter = document.createElement("div"); counter.className = "gc-lightbox-counter"; }

    lb.appendChild(closeBtn); lb.appendChild(prevBtn); lb.appendChild(img); lb.appendChild(nextBtn); lb.appendChild(caption);
    if (counter) lb.appendChild(counter);
    document.body.appendChild(lb);

    function show(idx) {
      if (loop) current = ((idx % items.length) + items.length) % items.length;
      else current = Math.max(0, Math.min(items.length - 1, idx));
      var item = items[current];
      img.src = typeof item === "string" ? item : item.src;
      caption.textContent = (typeof item === "string" ? "" : item.caption) || "";
      if (counter) counter.textContent = (current + 1) + " / " + items.length;
      if (!loop) { prevBtn.style.visibility = current <= 0 ? "hidden" : ""; nextBtn.style.visibility = current >= items.length - 1 ? "hidden" : ""; }
      lb.setAttribute("data-goop-open", "");
      if (opts.onChange) opts.onChange(current);
    }

    function hide() { lb.removeAttribute("data-goop-open"); }

    closeBtn.addEventListener("click", hide);
    prevBtn.addEventListener("click", function() { show(current - 1); });
    nextBtn.addEventListener("click", function() { show(current + 1); });
    lb.addEventListener("click", function(e) { if (e.target === lb) hide(); });

    function onKey(e) {
      if (!lb.hasAttribute("data-goop-open")) return;
      if (e.key === "Escape") hide();
      if (e.key === "ArrowLeft") show(current - 1);
      if (e.key === "ArrowRight") show(current + 1);
    }
    document.addEventListener("keydown", onKey);

    return {
      open: show,
      close: hide,
      setItems: function(i) { items = i; },
      destroy: function() { document.removeEventListener("keydown", onKey); lb.remove(); },
      el: lb,
    };
  };
})();
