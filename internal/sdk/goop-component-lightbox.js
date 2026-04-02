//
// CSS hooks:
//   .gc-lightbox            — fullscreen overlay
//   .gc-lightbox img        — displayed image
//   .gc-lightbox-close      — close button
//   .gc-lightbox-prev       — previous button
//   .gc-lightbox-next       — next button
//   .gc-lightbox-caption    — image caption text
//   .gc-lightbox-counter    — "3 / 10" counter (when opts.showCounter)
//   [data-goop-open]        — lightbox is visible
//

(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.lightbox = function(opts) {
    opts = opts || {};
    var items = opts.items || [];
    var current = 0;
    var showCounter = !!opts.showCounter;
    var loop = opts.loop !== false;

    var lb = document.createElement("div");
    lb.className = opts.class || "gc-lightbox";
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
