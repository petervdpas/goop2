(function() {
  function qs(sel, root) { return (root || document).querySelector(sel); }

  function escapeHtml(s) {
    var d = document.createElement("div");
    d.textContent = s == null ? "" : String(s);
    return d.innerHTML;
  }

  function createElement(html) {
    var t = document.createElement("template");
    t.innerHTML = html.trim();
    return t.content.firstElementChild;
  }

  function dialog(opts) {
    opts = opts || {};

    var title     = opts.title || "";
    var message   = opts.message || "";
    var input     = opts.input || null;
    var okLabel   = (opts.ok && opts.ok.label) || opts.okText || "OK";
    var okDanger  = (opts.ok && opts.ok.danger) || opts.dangerOk || false;
    var okValue   = opts.ok && opts.ok.value !== undefined ? opts.ok.value : undefined;
    var cancelLabel = opts.cancel === false ? null : ((opts.cancel && opts.cancel.label) || opts.cancelText || "Cancel");
    var cancelValue = (opts.cancel && opts.cancel.value !== undefined) ? opts.cancel.value : null;
    var hideOk    = opts.ok === false;
    var wide      = opts.wide || false;

    var hasInput  = !!input;
    var match     = hasInput && input.match ? input.match : null;

    var bodyHtml = '<div class="ed-dlg-msg"></div>';
    if (hasInput) bodyHtml += '<input class="ed-dlg-input" autocomplete="off" spellcheck="false" />';

    var footHtml = '';
    if (cancelLabel !== null) footHtml += '<button type="button" class="ed-dlg-btn cancel"></button>';
    if (!hideOk) footHtml += '<button type="button" class="ed-dlg-btn ok"></button>';

    var dlgClass = "ed-dlg" + (wide ? " ed-dlg-wide" : "");

    return new Promise(function(resolve) {
      var backdrop = createElement('<div class="ed-dlg-backdrop"></div>');
      var dlg = createElement(
        '<div class="' + dlgClass + '" role="dialog" aria-modal="true">' +
          '<div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>' +
          '<div class="ed-dlg-body">' + bodyHtml + '</div>' +
          '<div class="ed-dlg-foot">' + footHtml + '</div>' +
        '</div>'
      );

      qs(".ed-dlg-title", dlg).textContent = title;
      qs(".ed-dlg-msg", dlg).textContent = message;

      var inputEl = hasInput ? qs(".ed-dlg-input", dlg) : null;
      if (inputEl) {
        inputEl.placeholder = input.placeholder || "";
        inputEl.value = input.value || "";
        if (input.type) inputEl.type = input.type;
      }

      var bCancel = qs("button.cancel", dlg);
      var bOk = qs("button.ok", dlg);

      if (bCancel) bCancel.textContent = cancelLabel;
      if (bOk) {
        bOk.textContent = okLabel;
        if (okDanger) bOk.classList.add("danger");
        if (match) bOk.disabled = true;
      }

      if (match && inputEl) {
        inputEl.addEventListener("input", function() {
          bOk.disabled = inputEl.value !== match;
        });
      }

      function getResult(confirmed) {
        if (!confirmed) return cancelValue;
        if (hasInput) return inputEl.value;
        return okValue !== undefined ? okValue : true;
      }

      function cleanup(confirmed) {
        document.removeEventListener("keydown", handleKey);
        backdrop.remove();
        resolve(getResult(confirmed));
      }

      function handleKey(e) {
        if (e.key === "Escape") { cleanup(false); return; }
        if (e.key === "Enter") {
          if (match && inputEl && inputEl.value !== match) return;
          cleanup(true);
        }
      }

      backdrop.addEventListener("mousedown", function(e) {
        if (e.target === backdrop) cleanup(false);
      });

      if (bCancel) bCancel.addEventListener("click", function() { cleanup(false); });
      if (bOk) bOk.addEventListener("click", function() { cleanup(true); });

      backdrop.appendChild(dlg);
      document.body.appendChild(backdrop);
      document.addEventListener("keydown", handleKey);

      if (inputEl) {
        setTimeout(function() { inputEl.focus(); inputEl.select(); }, 0);
      } else if (bOk) {
        setTimeout(function() { bOk.focus(); }, 0);
      }
    });
  }

  dialog.alert = function(title, message) {
    return dialog({ title: title, message: message, cancel: false });
  };

  dialog.confirm = function(message, title) {
    return dialog({ title: title || "Confirm", message: message });
  };

  dialog.prompt = function(opts) {
    opts = opts || {};
    return dialog({
      title: opts.title || "Input",
      message: opts.message || "",
      input: { placeholder: opts.placeholder || "", value: opts.value || "", type: opts.type || "text" },
      okText: opts.okText || "OK",
      cancelText: opts.cancelText || "Cancel",
      dangerOk: opts.dangerOk || false
    });
  };

  dialog.confirmDanger = function(opts) {
    opts = opts || {};
    return dialog({
      title: opts.title || "Confirm",
      message: opts.message || "",
      input: { placeholder: opts.placeholder || "Type " + (opts.match || "DELETE"), match: opts.match || "DELETE" },
      okText: opts.okText || "Delete",
      cancelText: opts.cancelText || "Cancel",
      dangerOk: true
    });
  };

  dialog.filePicker = function(options) {
    var title = (options && options.title) || "Select File";
    var startDir = (options && options.dir) || "";
    var filter = (options && options.filter) || null;
    if (!filter && options && options.extensions) {
      var exts = options.extensions.map(function(e) { return e.toLowerCase().replace(/^\./, ""); });
      filter = function(name) {
        var dot = name.lastIndexOf(".");
        if (dot < 0) return false;
        return exts.indexOf(name.substring(dot + 1).toLowerCase()) >= 0;
      };
    }

    var browse = window.Goop && window.Goop.api && window.Goop.api.fs && window.Goop.api.fs.browse;
    if (!browse) return Promise.resolve(null);

    return new Promise(function(resolve) {
      var backdrop = createElement('<div class="ed-dlg-backdrop"></div>');
      var dlg = createElement(
        '<div class="ed-dlg ed-dlg-wide" role="dialog" aria-modal="true">' +
          '<div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>' +
          '<div class="ed-dlg-body">' +
            '<div class="fp-path muted small"></div>' +
            '<div class="fp-list"></div>' +
          '</div>' +
          '<div class="ed-dlg-foot">' +
            '<button type="button" class="ed-dlg-btn cancel">Cancel</button>' +
          '</div>' +
        '</div>'
      );

      qs(".ed-dlg-title", dlg).textContent = title;
      var pathEl = qs(".fp-path", dlg);
      var listEl = qs(".fp-list", dlg);

      function cleanup(result) {
        document.removeEventListener("keydown", handleKey);
        backdrop.remove();
        resolve(result);
      }

      function handleKey(e) {
        if (e.key === "Escape") cleanup(null);
      }

      function loadDir(dir) {
        listEl.innerHTML = '<p class="muted small">Loading...</p>';
        browse(dir).then(function(data) {
          pathEl.textContent = data.dir || "/";
          var html = "";

          if (data.parent && data.parent !== data.dir) {
            html += '<div class="fp-entry fp-dir" data-path="' + escapeHtml(data.parent) + '">' +
              '<span class="fp-icon">&#128194;</span> ..</div>';
          }

          var entries = data.entries || [];
          entries.forEach(function(e) {
            var fullPath = data.dir + "/" + e.name;
            if (data.dir === "/") fullPath = "/" + e.name;

            if (e.is_dir) {
              html += '<div class="fp-entry fp-dir" data-path="' + escapeHtml(fullPath) + '">' +
                '<span class="fp-icon">&#128194;</span> ' + escapeHtml(e.name) + '</div>';
            } else {
              if (filter && !filter(e.name)) return;
              html += '<div class="fp-entry fp-file" data-path="' + escapeHtml(fullPath) + '">' +
                '<span class="fp-icon">&#128196;</span> ' + escapeHtml(e.name) + '</div>';
            }
          });

          if (!html) html = '<p class="muted small">Empty directory</p>';
          listEl.innerHTML = html;

          listEl.querySelectorAll(".fp-dir").forEach(function(el) {
            el.addEventListener("click", function() { loadDir(el.getAttribute("data-path")); });
          });
          listEl.querySelectorAll(".fp-file").forEach(function(el) {
            el.addEventListener("click", function() { cleanup(el.getAttribute("data-path")); });
          });
        }).catch(function() {
          listEl.innerHTML = '<p class="muted small">Cannot read directory</p>';
        });
      }

      backdrop.addEventListener("mousedown", function(e) {
        if (e.target === backdrop) cleanup(null);
      });
      qs("button.cancel", dlg).addEventListener("click", function() { cleanup(null); });

      backdrop.appendChild(dlg);
      document.body.appendChild(backdrop);
      document.addEventListener("keydown", handleKey);

      loadDir(startDir);
    });
  };

  dialog.pathPicker = function(options) {
    var title = (options && options.title) || "Select Folder";
    var startDir = (options && options.dir) || "";

    var browse = window.Goop && window.Goop.api && window.Goop.api.fs && window.Goop.api.fs.browse;
    if (!browse) return Promise.resolve(null);

    return new Promise(function(resolve) {
      var backdrop = createElement('<div class="ed-dlg-backdrop"></div>');
      var dlg = createElement(
        '<div class="ed-dlg ed-dlg-wide" role="dialog" aria-modal="true">' +
          '<div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>' +
          '<div class="ed-dlg-body">' +
            '<div class="fp-path muted small"></div>' +
            '<div class="fp-list"></div>' +
          '</div>' +
          '<div class="ed-dlg-foot">' +
            '<button type="button" class="ed-dlg-btn cancel">Cancel</button>' +
            '<button type="button" class="ed-dlg-btn ok">Select</button>' +
          '</div>' +
        '</div>'
      );

      qs(".ed-dlg-title", dlg).textContent = title;
      var pathEl = qs(".fp-path", dlg);
      var listEl = qs(".fp-list", dlg);
      var currentDir = startDir;

      function cleanup(result) {
        document.removeEventListener("keydown", handleKey);
        backdrop.remove();
        resolve(result);
      }

      function handleKey(e) {
        if (e.key === "Escape") cleanup(null);
      }

      function loadDir(dir) {
        listEl.innerHTML = '<p class="muted small">Loading...</p>';
        browse(dir).then(function(data) {
          currentDir = data.dir || "/";
          pathEl.textContent = currentDir;
          var html = "";

          if (data.parent && data.parent !== data.dir) {
            html += '<div class="fp-entry fp-dir" data-path="' + escapeHtml(data.parent) + '">' +
              '<span class="fp-icon">&#128194;</span> ..</div>';
          }

          var entries = data.entries || [];
          entries.forEach(function(e) {
            if (!e.is_dir) return;
            var fullPath = data.dir + "/" + e.name;
            if (data.dir === "/") fullPath = "/" + e.name;
            html += '<div class="fp-entry fp-dir" data-path="' + escapeHtml(fullPath) + '">' +
              '<span class="fp-icon">&#128194;</span> ' + escapeHtml(e.name) + '</div>';
          });

          if (!html) html = '<p class="muted small">Empty directory</p>';
          listEl.innerHTML = html;

          listEl.querySelectorAll(".fp-dir").forEach(function(el) {
            el.addEventListener("click", function() { loadDir(el.getAttribute("data-path")); });
          });
        }).catch(function() {
          listEl.innerHTML = '<p class="muted small">Cannot read directory</p>';
        });
      }

      backdrop.addEventListener("mousedown", function(e) {
        if (e.target === backdrop) cleanup(null);
      });
      qs("button.cancel", dlg).addEventListener("click", function() { cleanup(null); });
      qs("button.ok", dlg).addEventListener("click", function() { cleanup(currentDir); });

      backdrop.appendChild(dlg);
      document.body.appendChild(backdrop);
      document.addEventListener("keydown", handleKey);

      loadDir(startDir);
    });
  };

  dialog.custom = function(opts) {
    opts = opts || {};
    var title = opts.title || "";
    var wide = opts.wide !== false;
    var okLabel = opts.okText || "OK";
    var cancelLabel = opts.cancelText || "Cancel";

    return new Promise(function(resolve) {
      var backdrop = createElement('<div class="ed-dlg-backdrop"></div>');
      var dlg = createElement(
        '<div class="ed-dlg' + (wide ? ' ed-dlg-wide' : '') + '" role="dialog" aria-modal="true">' +
          '<div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>' +
          '<div class="ed-dlg-body"></div>' +
          '<div class="ed-dlg-foot">' +
            '<button type="button" class="ed-dlg-btn cancel"></button>' +
            '<button type="button" class="ed-dlg-btn ok"></button>' +
          '</div>' +
        '</div>'
      );

      qs(".ed-dlg-title", dlg).textContent = title;
      qs("button.cancel", dlg).textContent = cancelLabel;
      qs("button.ok", dlg).textContent = okLabel;

      var bodyEl = qs(".ed-dlg-body", dlg);

      function cleanup(result) {
        document.removeEventListener("keydown", handleKey);
        backdrop.remove();
        resolve(result);
      }

      function handleKey(e) {
        if (e.key === "Escape") cleanup(null);
      }

      if (opts.build) opts.build(bodyEl, cleanup);

      qs("button.cancel", dlg).addEventListener("click", function() { cleanup(null); });
      qs("button.ok", dlg).addEventListener("click", function() {
        var result = opts.collect ? opts.collect(bodyEl) : true;
        cleanup(result);
      });

      backdrop.addEventListener("mousedown", function(e) {
        if (e.target === backdrop) cleanup(null);
      });

      backdrop.appendChild(dlg);
      document.body.appendChild(backdrop);
      document.addEventListener("keydown", handleKey);
    });
  };

  window.Goop = window.Goop || {};
  window.Goop.dialog = dialog;
})();
