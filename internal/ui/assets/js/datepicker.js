// Custom date picker component — themed to match goop2 design system.
// Exposes Goop.datepicker = { attach, detach, val, setVal }
(() => {
  window.Goop = window.Goop || {};

  var DAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];
  var MONTHS = ["January", "February", "March", "April", "May", "June",
    "July", "August", "September", "October", "November", "December"];

  var activePopup = null;

  function pad(n) { return n < 10 ? "0" + n : "" + n; }

  function formatDate(y, m, d) {
    return y + "-" + pad(m + 1) + "-" + pad(d);
  }

  function parseDate(s) {
    if (!s) return null;
    var parts = s.split("-");
    if (parts.length !== 3) return null;
    return { year: parseInt(parts[0], 10), month: parseInt(parts[1], 10) - 1, day: parseInt(parts[2], 10) };
  }

  function daysInMonth(year, month) {
    return new Date(year, month + 1, 0).getDate();
  }

  function firstDayOfWeek(year, month) {
    return new Date(year, month, 1).getDay();
  }

  function closePopup() {
    if (activePopup) {
      activePopup.remove();
      activePopup = null;
    }
  }

  function buildCalendar(input, year, month) {
    closePopup();

    var selected = parseDate(input.value);
    var today = new Date();
    var todayStr = formatDate(today.getFullYear(), today.getMonth(), today.getDate());

    var popup = document.createElement("div");
    popup.className = "gdp-popup";
    activePopup = popup;

    var header = '<div class="gdp-header">' +
      '<button class="gdp-nav" data-dir="-1" data-scope="month">&#8249;</button>' +
      '<span class="gdp-month">' + MONTHS[month] + '</span>' +
      '<button class="gdp-nav" data-dir="1" data-scope="month">&#8250;</button>' +
      '<button class="gdp-nav" data-dir="-1" data-scope="year">&#8249;</button>' +
      '<span class="gdp-year">' + year + '</span>' +
      '<button class="gdp-nav" data-dir="1" data-scope="year">&#8250;</button>' +
    '</div>';

    var dayHeaders = '<div class="gdp-days">';
    DAYS.forEach(function(d) { dayHeaders += '<span class="gdp-day-name">' + d + '</span>'; });
    dayHeaders += '</div>';

    var grid = '<div class="gdp-grid">';
    var dim = daysInMonth(year, month);
    var start = firstDayOfWeek(year, month);

    var prevDim = daysInMonth(year, month === 0 ? 11 : month - 1);
    for (var i = start - 1; i >= 0; i--) {
      grid += '<span class="gdp-cell gdp-other">' + (prevDim - i) + '</span>';
    }

    for (var d = 1; d <= dim; d++) {
      var dateStr = formatDate(year, month, d);
      var cls = "gdp-cell";
      if (selected && selected.year === year && selected.month === month && selected.day === d) cls += " gdp-selected";
      if (dateStr === todayStr) cls += " gdp-today";
      grid += '<span class="' + cls + '" data-date="' + dateStr + '">' + d + '</span>';
    }

    var totalCells = start + dim;
    var remaining = (7 - (totalCells % 7)) % 7;
    for (var r = 1; r <= remaining; r++) {
      grid += '<span class="gdp-cell gdp-other">' + r + '</span>';
    }
    grid += '</div>';

    var footer = '<div class="gdp-footer">' +
      '<button class="gdp-today-btn">Today</button>' +
      '<button class="gdp-clear-btn">Clear</button>' +
    '</div>';

    popup.innerHTML = header + dayHeaders + grid + footer;

    var rect = input.getBoundingClientRect();
    popup.style.top = (rect.bottom + window.scrollY + 4) + "px";
    popup.style.left = (rect.left + window.scrollX) + "px";
    document.body.appendChild(popup);

    var popupRect = popup.getBoundingClientRect();
    if (popupRect.right > window.innerWidth - 8) {
      popup.style.left = (rect.right + window.scrollX - popupRect.width) + "px";
    }

    popup.querySelectorAll(".gdp-nav").forEach(function(btn) {
      btn.addEventListener("click", function(e) {
        e.preventDefault();
        e.stopPropagation();
        var dir = parseInt(btn.dataset.dir, 10);
        var scope = btn.dataset.scope;
        if (scope === "year") {
          buildCalendar(input, year + dir, month);
        } else {
          var nm = month + dir;
          var ny = year;
          if (nm < 0) { nm = 11; ny--; }
          if (nm > 11) { nm = 0; ny++; }
          buildCalendar(input, ny, nm);
        }
      });
    });

    popup.querySelectorAll(".gdp-cell[data-date]").forEach(function(cell) {
      cell.addEventListener("click", function(e) {
        e.preventDefault();
        e.stopPropagation();
        input.value = cell.dataset.date;
        input.dispatchEvent(new Event("change", { bubbles: true }));
        closePopup();
      });
    });

    var todayBtn = popup.querySelector(".gdp-today-btn");
    if (todayBtn) {
      todayBtn.addEventListener("click", function(e) {
        e.preventDefault();
        e.stopPropagation();
        input.value = todayStr;
        input.dispatchEvent(new Event("change", { bubbles: true }));
        closePopup();
      });
    }

    var clearBtn = popup.querySelector(".gdp-clear-btn");
    if (clearBtn) {
      clearBtn.addEventListener("click", function(e) {
        e.preventDefault();
        e.stopPropagation();
        input.value = "";
        input.dispatchEvent(new Event("change", { bubbles: true }));
        closePopup();
      });
    }
  }

  function attach(input) {
    if (input.dataset.gdpBound) return;
    input.dataset.gdpBound = "1";
    input.setAttribute("readonly", "");
    input.style.cursor = "pointer";

    input.addEventListener("click", function(e) {
      e.stopPropagation();
      if (activePopup) {
        closePopup();
        return;
      }
      var parsed = parseDate(input.value);
      var now = new Date();
      var year = parsed ? parsed.year : now.getFullYear();
      var month = parsed ? parsed.month : now.getMonth();
      buildCalendar(input, year, month);
    });
  }

  document.addEventListener("click", function(e) {
    if (activePopup && !activePopup.contains(e.target)) {
      closePopup();
    }
  });

  document.addEventListener("keydown", function(e) {
    if (e.key === "Escape" && activePopup) {
      closePopup();
    }
  });

  Goop.datepicker = {
    attach: attach,
    val: function(input) { return input.value || ""; },
    setVal: function(input, v) { input.value = v || ""; },
  };
})();
