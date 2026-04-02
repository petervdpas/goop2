(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.carousel = function(el, opts) {
    opts = opts || {};
    var slides = opts.slides || [];
    var current = opts.start || 0;
    var autoplay = opts.autoplay || 0;
    var loop = opts.loop !== false;
    var showDots = opts.dots !== false;
    var showArrows = opts.arrows !== false;
    var autoTimer = null;

    var wrap = document.createElement("div");
    wrap.className = opts.class || "gc-carousel";
    wrap.setAttribute("data-goop-component", "carousel");

    var track = document.createElement("div"); track.className = "gc-carousel-track";
    var prevBtn = document.createElement("button"); prevBtn.type = "button"; prevBtn.className = "gc-carousel-btn"; prevBtn.setAttribute("data-goop-dir", "prev"); prevBtn.textContent = "\u2039";
    var nextBtn = document.createElement("button"); nextBtn.type = "button"; nextBtn.className = "gc-carousel-btn"; nextBtn.setAttribute("data-goop-dir", "next"); nextBtn.textContent = "\u203A";
    var dots = document.createElement("div"); dots.className = "gc-carousel-dots";

    wrap.appendChild(track);
    if (slides.length > 1 && showArrows) { wrap.appendChild(prevBtn); wrap.appendChild(nextBtn); }
    if (slides.length > 1 && showDots) wrap.appendChild(dots);
    el.appendChild(wrap);

    function renderSlides() {
      track.innerHTML = "";
      for (var i = 0; i < slides.length; i++) {
        var slide = document.createElement("div"); slide.className = "gc-carousel-slide";
        var sv = slides[i];
        if (typeof sv === "string") slide.innerHTML = '<img src="' + _e(sv) + '" alt="">';
        else if (sv.html) slide.innerHTML = sv.html;
        else if (sv.src) slide.innerHTML = '<img src="' + _e(sv.src) + '" alt="' + _e(sv.alt || "") + '">';
        track.appendChild(slide);
      }
      renderDots(); goTo(current);
    }

    function renderDots() {
      dots.innerHTML = "";
      for (var i = 0; i < slides.length; i++) {
        var dot = document.createElement("button"); dot.type = "button"; dot.className = "gc-carousel-dot";
        if (i === current) dot.setAttribute("data-goop-active", "");
        (function(idx) { dot.addEventListener("click", function() { goTo(idx); }); })(i);
        dots.appendChild(dot);
      }
    }

    function goTo(idx) {
      if (loop) { current = ((idx % slides.length) + slides.length) % slides.length; }
      else { current = Math.max(0, Math.min(slides.length - 1, idx)); }
      track.style.transform = "translateX(-" + (current * 100) + "%)";
      dots.querySelectorAll(".gc-carousel-dot").forEach(function(d, i) { if (i === current) d.setAttribute("data-goop-active", ""); else d.removeAttribute("data-goop-active"); });
      if (!loop) { prevBtn.disabled = current <= 0; nextBtn.disabled = current >= slides.length - 1; }
      _f(wrap, "change", { index: current, slide: slides[current] });
      if (opts.onChange) opts.onChange(current);
    }

    prevBtn.addEventListener("click", function() { goTo(current - 1); resetAuto(); });
    nextBtn.addEventListener("click", function() { goTo(current + 1); resetAuto(); });

    function resetAuto() { if (autoTimer) clearInterval(autoTimer); if (autoplay > 0) autoTimer = setInterval(function() { goTo(current + 1); }, autoplay); }

    renderSlides();
    if (autoplay > 0) resetAuto();

    return {
      getValue: function() { return current; },
      goTo: goTo,
      next: function() { goTo(current + 1); },
      prev: function() { goTo(current - 1); },
      setSlides: function(s) { slides = s; renderSlides(); },
      destroy: function() { if (autoTimer) clearInterval(autoTimer); wrap.remove(); },
      el: wrap,
    };
  };
})();
