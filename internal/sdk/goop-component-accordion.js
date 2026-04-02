(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  Goop.ui.accordion = function(el, opts) {
    opts = opts || {};
    var multi = opts.multi !== false;
    var sectionSelector = opts.section || "";
    var headerSelector = opts.header || "";
    var openClass = opts.openClass || "";
    var openAttr = opts.openAttr || "";
    var sectionAttr = opts.sectionAttr || "data-section";

    function getSections() {
      return sectionSelector ? Array.from(el.querySelectorAll(sectionSelector)) : Array.from(el.children);
    }

    function isOpen(sec) {
      if (openClass && sec.classList.contains(openClass)) return true;
      if (openAttr && sec.hasAttribute(openAttr)) return true;
      return false;
    }

    function setOpen(sec, open) {
      if (openClass) { if (open) sec.classList.add(openClass); else sec.classList.remove(openClass); }
      if (openAttr) { if (open) sec.setAttribute(openAttr, ""); else sec.removeAttribute(openAttr); }
    }

    function getOpenIds() {
      var ids = [];
      getSections().forEach(function(sec) {
        if (isOpen(sec)) {
          var id = sec.getAttribute(sectionAttr) || "";
          if (id) ids.push(id);
        }
      });
      return ids;
    }

    function toggle(sec) {
      if (isOpen(sec)) {
        setOpen(sec, false);
      } else {
        if (!multi) getSections().forEach(function(s) { setOpen(s, false); });
        setOpen(sec, true);
      }
      _f(el, "change", { value: getOpenIds() });
      if (opts.onChange) opts.onChange(getOpenIds());
    }

    getSections().forEach(function(sec) {
      var header = headerSelector ? sec.querySelector(headerSelector) : sec.firstElementChild;
      if (header) {
        header.addEventListener("click", function() { toggle(sec); });
      }
    });

    return {
      getValue: function() { return getOpenIds(); },
      open: function(id) {
        var sec = el.querySelector("[" + sectionAttr + '="' + id + '"]');
        if (sec) setOpen(sec, true);
      },
      close: function(id) {
        var sec = el.querySelector("[" + sectionAttr + '="' + id + '"]');
        if (sec) setOpen(sec, false);
      },
      getSection: function(id) {
        return el.querySelector("[" + sectionAttr + '="' + id + '"]');
      },
      destroy: function() {},
      el: el,
    };
  };
})();
