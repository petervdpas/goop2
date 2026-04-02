(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.toolbar = function(el, opts) {
    opts = opts || {};
    var multi = !!opts.multi;
    var activeClass = opts.activeClass || "";
    var activeAttr = opts.activeAttr || "";
    var idAttr = opts.idAttr || "data-id";

    function getButtons() {
      return Array.from(el.querySelectorAll("button[" + idAttr + "]"));
    }

    function setActive(btn, on) {
      if (activeClass) { if (on) btn.classList.add(activeClass); else btn.classList.remove(activeClass); }
      if (activeAttr) { if (on) btn.setAttribute(activeAttr, ""); else btn.removeAttribute(activeAttr); }
    }

    function getActiveIds() {
      var ids = [];
      getButtons().forEach(function(btn) {
        var isActive = (activeClass && btn.classList.contains(activeClass)) || (activeAttr && btn.hasAttribute(activeAttr));
        if (isActive) ids.push(btn.getAttribute(idAttr));
      });
      return ids;
    }

    getButtons().forEach(function(btn) {
      btn.addEventListener("click", function() {
        if (btn.disabled) return;
        var id = btn.getAttribute(idAttr);
        if (multi) {
          var isActive = (activeClass && btn.classList.contains(activeClass)) || (activeAttr && btn.hasAttribute(activeAttr));
          setActive(btn, !isActive);
          _f(el, "change", { value: getActiveIds() });
          if (opts.onChange) opts.onChange(getActiveIds());
        } else {
          getButtons().forEach(function(b) { setActive(b, false); });
          setActive(btn, true);
          _f(el, "change", { value: id });
          if (opts.onChange) opts.onChange(id);
        }
      });
    });

    if (opts.active) {
      var initActive = Array.isArray(opts.active) ? opts.active : [opts.active];
      getButtons().forEach(function(btn) {
        if (initActive.indexOf(btn.getAttribute(idAttr)) >= 0) setActive(btn, true);
      });
    }

    return {
      getValue: function() { return multi ? getActiveIds() : (getActiveIds()[0] || ""); },
      setValue: function(v) {
        var ids = Array.isArray(v) ? v : [v];
        getButtons().forEach(function(btn) { setActive(btn, ids.indexOf(btn.getAttribute(idAttr)) >= 0); });
      },
      destroy: function() {},
      el: el,
    };
  };
})();
