//
// CSS hooks:
//   .gc-datepicker          — wrapper (override via opts.class)
//   .gc-datepicker-input    — text input showing selected date
//   .gc-datepicker-popup    — calendar dropdown
//   .gc-dp-nav              — month/year navigation row
//   .gc-dp-grid             — day grid (7 columns)
//   .gc-dp-hdr              — day-of-week header cells
//   .gc-dp-time             — time input row (when opts.time)
//   [data-goop-open]        — popup is visible
//   [data-goop-today]       — today cell
//   [data-goop-selected]    — selected day cell
//   [data-goop-outside]     — day from adjacent month
//   [data-goop-disabled]    — disabled state
//   :disabled               — out-of-range day (opts.min/max)
//

(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};
  var _e = Goop.ui._esc || function(s) { var d = document.createElement("div"); d.textContent = s == null ? "" : String(s); return d.innerHTML; };
  var _f = Goop.ui._fire || function(el, n, dt) { el.dispatchEvent(new CustomEvent(n, { bubbles: true, detail: dt })); };

  var MONTHS = ["January","February","March","April","May","June","July","August","September","October","November","December"];
  var ALLDAYS = ["Su","Mo","Tu","We","Th","Fr","Sa"];

  Goop.ui.datepicker = function(opts) {
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

    var wrap = document.createElement("div");
    for (var _k in opts) { if (_k.indexOf("data-") === 0) wrap.setAttribute(_k, opts[_k]); }
    wrap.className = opts.class || "gc-datepicker";
    wrap.setAttribute("data-goop-component", "datepicker");
    if (isDisabled) wrap.setAttribute("data-goop-disabled", "");

    var input = document.createElement("input");
    input.className = opts.inputClass || "gc-datepicker-input";
    input.readOnly = true;
    input.placeholder = opts.placeholder || (includeTime ? "Select date & time" : "Select date");
    if (opts.name) input.setAttribute("data-goop-name", opts.name);
    if (isDisabled) input.disabled = true;

    var popup = document.createElement("div");
    popup.className = opts.popupClass || "gc-datepicker-popup";
    wrap.appendChild(input);
    wrap.appendChild(popup);

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
      var html = '<div class="gc-dp-nav">';
      html += '<button type="button" data-goop-action="prev">\u25C0</button>';
      html += "<span>" + MONTHS[viewMonth] + " " + viewYear + "</span>";
      html += '<button type="button" data-goop-action="next">\u25B6</button></div>';
      html += '<div class="gc-dp-grid">';
      for (var i = 0; i < 7; i++) html += '<div class="gc-dp-hdr">' + orderedDays[i] + "</div>";

      var first = new Date(viewYear, viewMonth, 1);
      var startDay = (first.getDay() - firstDay + 7) % 7;
      var dim = new Date(viewYear, viewMonth + 1, 0).getDate();
      var prevDim = new Date(viewYear, viewMonth, 0).getDate();
      var today = new Date();
      var todayStr = today.getFullYear() + "-" + (today.getMonth() + 1) + "-" + today.getDate();

      for (var d = startDay - 1; d >= 0; d--) {
        var pd = prevDim - d, oor = outOfRange(new Date(viewYear, viewMonth - 1, pd));
        html += '<button type="button" data-goop-outside data-goop-day="' + pd + '" data-goop-month="' + (viewMonth - 1) + '"' + (oor ? " disabled" : "") + '>' + pd + "</button>";
      }
      for (var d = 1; d <= dim; d++) {
        var attrs = 'data-goop-day="' + d + '"';
        var ds = viewYear + "-" + (viewMonth + 1) + "-" + d;
        if (ds === todayStr) attrs += " data-goop-today";
        if (current && current.getFullYear() === viewYear && current.getMonth() === viewMonth && current.getDate() === d) attrs += " data-goop-selected";
        var oor = outOfRange(new Date(viewYear, viewMonth, d));
        html += '<button type="button" ' + attrs + (oor ? " disabled" : "") + ">" + d + "</button>";
      }
      var rem = 42 - (startDay + dim);
      for (var d = 1; d <= rem; d++) {
        var oor = outOfRange(new Date(viewYear, viewMonth + 1, d));
        html += '<button type="button" data-goop-outside data-goop-day="' + d + '" data-goop-month="' + (viewMonth + 1) + '"' + (oor ? " disabled" : "") + '>' + d + "</button>";
      }
      html += "</div>";

      if (includeTime) {
        var h = current ? String(current.getHours()).padStart(2, "0") : "00";
        var m = current ? String(current.getMinutes()).padStart(2, "0") : "00";
        html += '<div class="gc-dp-time"><span>Time:</span>';
        html += '<input type="text" data-goop-action="hours" value="' + h + '" maxlength="2"><span>:</span>';
        html += '<input type="text" data-goop-action="minutes" value="' + m + '" maxlength="2"></div>';
      }
      popup.innerHTML = html;

      popup.querySelectorAll(".gc-dp-nav button").forEach(function(btn) {
        btn.addEventListener("click", function(e) {
          e.stopPropagation();
          if (btn.getAttribute("data-goop-action") === "prev") { viewMonth--; if (viewMonth < 0) { viewMonth = 11; viewYear--; } }
          else { viewMonth++; if (viewMonth > 11) { viewMonth = 0; viewYear++; } }
          renderCalendar();
        });
      });

      popup.querySelectorAll(".gc-dp-grid button:not([disabled])").forEach(function(btn) {
        btn.addEventListener("click", function(e) {
          e.stopPropagation();
          var day = parseInt(btn.getAttribute("data-goop-day"), 10);
          var mo = btn.getAttribute("data-goop-month");
          if (mo !== null) { viewMonth = parseInt(mo, 10); if (viewMonth < 0) { viewMonth = 11; viewYear--; } if (viewMonth > 11) { viewMonth = 0; viewYear++; } }
          var h = 0, mi = 0;
          if (includeTime && current) { h = current.getHours(); mi = current.getMinutes(); }
          current = new Date(viewYear, viewMonth, day, h, mi);
          input.value = formatDate(current);
          _f(wrap, "change", { value: input.value }); _f(wrap, "input", { value: input.value });
          if (opts.onChange) opts.onChange(input.value);
          if (!includeTime) closePopup(); else renderCalendar();
        });
      });

      popup.querySelectorAll(".gc-dp-time input").forEach(function(inp) {
        inp.addEventListener("change", function() {
          if (!current) current = new Date(viewYear, viewMonth, 1);
          var action = inp.getAttribute("data-goop-action"), val = parseInt(inp.value, 10) || 0;
          if (action === "hours") { val = Math.max(0, Math.min(23, val)); current.setHours(val); inp.value = String(val).padStart(2, "0"); }
          if (action === "minutes") { val = Math.max(0, Math.min(59, val)); current.setMinutes(val); inp.value = String(val).padStart(2, "0"); }
          input.value = formatDate(current);
          _f(wrap, "change", { value: input.value });
          if (opts.onChange) opts.onChange(input.value);
        });
      });
    }

    function openPopup() { if (isDisabled) return; renderCalendar(); popup.setAttribute("data-goop-open", ""); }
    function closePopup() { popup.removeAttribute("data-goop-open"); }
    input.addEventListener("click", function(e) { e.stopPropagation(); if (popup.hasAttribute("data-goop-open")) closePopup(); else openPopup(); });
    function onDocClick(e) { if (!wrap.contains(e.target)) closePopup(); }
    document.addEventListener("click", onDocClick);
    if (current) input.value = formatDate(current);

    return {
      getValue: function() { return input.value; },
      setValue: function(v) {
        current = v ? new Date(v) : null;
        if (current && !isNaN(current)) { viewYear = current.getFullYear(); viewMonth = current.getMonth(); input.value = formatDate(current); }
        else { current = null; input.value = ""; }
      },
      setDisabled: function(v) { isDisabled = !!v; input.disabled = isDisabled; if (isDisabled) wrap.setAttribute("data-goop-disabled", ""); else wrap.removeAttribute("data-goop-disabled"); },
      destroy: function() { document.removeEventListener("click", onDocClick); wrap.remove(); },
      el: wrap,
    };
  };
})();
