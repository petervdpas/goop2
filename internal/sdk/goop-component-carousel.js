(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.carousel = function(el, opts) {
    opts = opts || {};
    var current = opts.start || 0;
    var autoplay = opts.autoplay || 0;
    var loop = opts.loop !== false;
    var autoTimer = null;
    var activeAttr = opts.activeAttr || "";
    var activeClass = opts.activeClass || "";
    var slideVar = opts.slideVar || "--gc-slide";

    var track = opts.track ? el.querySelector(opts.track) : null;
    var prevBtn = opts.prev ? el.querySelector(opts.prev) : null;
    var nextBtn = opts.next ? el.querySelector(opts.next) : null;
    var dotsContainer = opts.dots ? el.querySelector(opts.dots) : null;

    function getSlides() {
      return track ? Array.from(track.children) : [];
    }

    function getDots() {
      return dotsContainer ? Array.from(dotsContainer.children) : [];
    }

    function goTo(idx) {
      var slides = getSlides();
      if (!slides.length) return;
      if (loop) current = ((idx % slides.length) + slides.length) % slides.length;
      else current = Math.max(0, Math.min(slides.length - 1, idx));

      if (track) track.style.setProperty(slideVar, "-" + (current * 100) + "%");

      getDots().forEach(function(d, i) {
        if (activeAttr) { if (i === current) d.setAttribute(activeAttr, ""); else d.removeAttribute(activeAttr); }
        if (activeClass) { if (i === current) d.classList.add(activeClass); else d.classList.remove(activeClass); }
      });

      if (!loop) {
        if (prevBtn) prevBtn.disabled = current <= 0;
        if (nextBtn) nextBtn.disabled = current >= slides.length - 1;
      }

      _f(el, "change", { index: current });
      if (opts.onChange) opts.onChange(current);
    }

    if (prevBtn) prevBtn.addEventListener("click", function() { goTo(current - 1); resetAuto(); });
    if (nextBtn) nextBtn.addEventListener("click", function() { goTo(current + 1); resetAuto(); });

    getDots().forEach(function(dot, i) {
      dot.addEventListener("click", function() { goTo(i); resetAuto(); });
    });

    function resetAuto() { if (autoTimer) clearInterval(autoTimer); if (autoplay > 0) autoTimer = setInterval(function() { goTo(current + 1); }, autoplay); }

    goTo(current);
    if (autoplay > 0) resetAuto();

    return {
      getValue: function() { return current; },
      goTo: goTo,
      next: function() { goTo(current + 1); },
      prev: function() { goTo(current - 1); },
      destroy: function() { if (autoTimer) clearInterval(autoTimer); },
      el: el,
    };
  };
})();
