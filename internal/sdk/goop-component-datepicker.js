(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var MONTHS = ["January","February","March","April","May","June","July","August","September","October","November","December"];
  var ALLDAYS = ["Su","Mo","Tu","We","Th","Fr","Sa"];

  Goop.ui.datepicker = function(el, opts) {
    opts = opts || {};
    var includeTime = !!opts.time;
    var isDisabled = !!opts.disabled;
    var minDate = opts.min ? new Date(opts.min) : null;
    var maxDate = opts.max ? new Date(opts.max) : null;
    var firstDay = opts.firstDay || 0;
    var formatFn = typeof opts.format === "function" ? opts.format : null;
    var current = opts.value ? new Date(opts.value) : null;
    var viewYear, viewMonth;

    if (minDate && isNaN(minDate)) minDate = null;
    if (maxDate && isNaN(maxDate)) maxDate = null;
    var orderedDays = ALLDAYS.slice(firstDay).concat(ALLDAYS.slice(0, firstDay));

    if (current && !isNaN(current)) { viewYear = current.getFullYear(); viewMonth = current.getMonth(); }
    else { var now = new Date(); viewYear = now.getFullYear(); viewMonth = now.getMonth(); current = null; }

    var input = opts.input ? el.querySelector(opts.input) : el.querySelector("input");
    var popup = opts.popup ? el.querySelector(opts.popup) : null;
    var openAttr = opts.openAttr || "";
    var openClass = opts.openClass || "";
    var disabledAttr = opts.disabledAttr || "";
    var todayAttr = opts.todayAttr || "data-today";
    var selectedAttr = opts.selectedAttr || "data-selected";
    var outsideAttr = opts.outsideAttr || "data-outside";
    var navClass = opts.navClass || "";
    var gridClass = opts.gridClass || "";
    var hdrClass = opts.hdrClass || "";
    var timeClass = opts.timeClass || "";
    var dayAttr = opts.dayAttr || "data-day";
    var monthAttr = opts.monthAttr || "data-month";

    function dateOnly(d) { return new Date(d.getFullYear(), d.getMonth(), d.getDate()); }
    function outOfRange(d) {
      var day = dateOnly(d);
      if (minDate && day < dateOnly(minDate)) return true;
      if (maxDate && day > dateOnly(maxDate)) return true;
      return false;
    }

    function formatDate(d) {
      if (!d) return "";
      if (formatFn) return formatFn(d);
      var y = d.getFullYear(), m = String(d.getMonth() + 1).padStart(2, "0"), dy = String(d.getDate()).padStart(2, "0");
      var r = y + "-" + m + "-" + dy;
      if (includeTime) r += " " + String(d.getHours()).padStart(2, "0") + ":" + String(d.getMinutes()).padStart(2, "0");
      return r;
    }

    function renderCalendar() {
      if (!popup) return;
      popup.innerHTML = "";

      var nav = document.createElement("div");
      if (navClass) nav.className = navClass;
      var prevBtn = document.createElement("button"); prevBtn.type = "button"; prevBtn.textContent = "\u25C0";
      var label = document.createElement("span"); label.textContent = MONTHS[viewMonth] + " " + viewYear;
      var nextBtn = document.createElement("button"); nextBtn.type = "button"; nextBtn.textContent = "\u25B6";
      nav.appendChild(prevBtn); nav.appendChild(label); nav.appendChild(nextBtn);
      popup.appendChild(nav);

      prevBtn.addEventListener("click", function(e) {
        e.stopPropagation(); viewMonth--; if (viewMonth < 0) { viewMonth = 11; viewYear--; } renderCalendar();
      });
      nextBtn.addEventListener("click", function(e) {
        e.stopPropagation(); viewMonth++; if (viewMonth > 11) { viewMonth = 0; viewYear++; } renderCalendar();
      });

      var grid = document.createElement("div");
      if (gridClass) grid.className = gridClass;

      for (var i = 0; i < 7; i++) {
        var hdr = document.createElement("div");
        if (hdrClass) hdr.className = hdrClass;
        hdr.textContent = orderedDays[i];
        grid.appendChild(hdr);
      }

      var first = new Date(viewYear, viewMonth, 1);
      var startDay = (first.getDay() - firstDay + 7) % 7;
      var dim = new Date(viewYear, viewMonth + 1, 0).getDate();
      var prevDim = new Date(viewYear, viewMonth, 0).getDate();
      var today = new Date();
      var todayStr = today.getFullYear() + "-" + (today.getMonth() + 1) + "-" + today.getDate();

      function makeDay(day, mo, outside) {
        var btn = document.createElement("button"); btn.type = "button"; btn.textContent = day;
        var actualMonth = mo != null ? mo : viewMonth;
        var actualYear = viewYear;
        if (mo != null) {
          if (mo < 0) { actualMonth = 11; actualYear--; }
          if (mo > 11) { actualMonth = 0; actualYear++; }
        }
        btn.setAttribute(dayAttr, day);
        if (mo != null) btn.setAttribute(monthAttr, mo);
        if (outside) btn.setAttribute(outsideAttr, "");
        var ds = actualYear + "-" + (actualMonth + 1) + "-" + day;
        if (ds === todayStr) btn.setAttribute(todayAttr, "");
        if (current && current.getFullYear() === actualYear && current.getMonth() === actualMonth && current.getDate() === day) btn.setAttribute(selectedAttr, "");
        if (outOfRange(new Date(actualYear, actualMonth, day))) btn.disabled = true;
        btn.addEventListener("click", function(e) {
          e.stopPropagation();
          if (mo != null) { viewMonth = mo; if (viewMonth < 0) { viewMonth = 11; viewYear--; } if (viewMonth > 11) { viewMonth = 0; viewYear++; } }
          var h = 0, mi = 0;
          if (includeTime && current) { h = current.getHours(); mi = current.getMinutes(); }
          current = new Date(viewYear, viewMonth, day, h, mi);
          if (input) input.value = formatDate(current);
          _f(el, "change", { value: formatDate(current) }); _f(el, "input", { value: formatDate(current) });
          if (opts.onChange) opts.onChange(formatDate(current));
          if (!includeTime) closePopup(); else renderCalendar();
        });
        grid.appendChild(btn);
      }

      for (var d = startDay - 1; d >= 0; d--) makeDay(prevDim - d, viewMonth - 1, true);
      for (var d = 1; d <= dim; d++) makeDay(d, null, false);
      var rem = 42 - (startDay + dim);
      for (var d = 1; d <= rem; d++) makeDay(d, viewMonth + 1, true);

      popup.appendChild(grid);

      if (includeTime) {
        var timeRow = document.createElement("div");
        if (timeClass) timeRow.className = timeClass;
        var tLabel = document.createElement("span"); tLabel.textContent = "Time:";
        var hInp = document.createElement("input"); hInp.type = "text"; hInp.maxLength = 2; hInp.value = current ? String(current.getHours()).padStart(2, "0") : "00";
        var sep = document.createElement("span"); sep.textContent = ":";
        var mInp = document.createElement("input"); mInp.type = "text"; mInp.maxLength = 2; mInp.value = current ? String(current.getMinutes()).padStart(2, "0") : "00";
        timeRow.appendChild(tLabel); timeRow.appendChild(hInp); timeRow.appendChild(sep); timeRow.appendChild(mInp);
        popup.appendChild(timeRow);

        function onTimeChange(inp, type) {
          inp.addEventListener("change", function() {
            if (!current) current = new Date(viewYear, viewMonth, 1);
            var val = parseInt(inp.value, 10) || 0;
            if (type === "h") { val = Math.max(0, Math.min(23, val)); current.setHours(val); }
            if (type === "m") { val = Math.max(0, Math.min(59, val)); current.setMinutes(val); }
            inp.value = String(val).padStart(2, "0");
            if (input) input.value = formatDate(current);
            _f(el, "change", { value: formatDate(current) });
            if (opts.onChange) opts.onChange(formatDate(current));
          });
        }
        onTimeChange(hInp, "h"); onTimeChange(mInp, "m");
      }
    }

    function openPopup() {
      if (isDisabled || !popup) return;
      renderCalendar();
      if (openClass) popup.classList.add(openClass);
      if (openAttr) popup.setAttribute(openAttr, "");
    }

    function closePopup() {
      if (!popup) return;
      if (openClass) popup.classList.remove(openClass);
      if (openAttr) popup.removeAttribute(openAttr);
    }

    if (input) {
      input.readOnly = true;
      input.addEventListener("click", function(e) {
        e.stopPropagation();
        var isOpen = popup && ((openClass && popup.classList.contains(openClass)) || (openAttr && popup.hasAttribute(openAttr)));
        if (isOpen) closePopup(); else openPopup();
      });
    }

    function onDocClick(e) { if (!el.contains(e.target)) closePopup(); }
    document.addEventListener("click", onDocClick);
    if (current && input) input.value = formatDate(current);

    return {
      getValue: function() { return input ? input.value : ""; },
      setValue: function(v) {
        current = v ? new Date(v) : null;
        if (current && !isNaN(current)) { viewYear = current.getFullYear(); viewMonth = current.getMonth(); if (input) input.value = formatDate(current); }
        else { current = null; if (input) input.value = ""; }
      },
      setDisabled: function(v) {
        isDisabled = !!v;
        if (input) input.disabled = isDisabled;
        if (disabledAttr) { if (isDisabled) el.setAttribute(disabledAttr, ""); else el.removeAttribute(disabledAttr); }
      },
      destroy: function() { document.removeEventListener("click", onDocClick); },
      el: el,
    };
  };
})();
