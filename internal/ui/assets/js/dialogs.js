// Reusable dialog utilities
(() => {
  function createElement(html) {
    const t = document.createElement("template");
    t.innerHTML = html.trim();
    return t.content.firstElementChild;
  }

  function dlgBase({ title, message, hasInput, placeholder, value, okText, cancelText, dangerOk }) {
    return new Promise((resolve) => {
      const backdrop = createElement(`<div class="ed-dlg-backdrop"></div>`);
      const bodyContent = hasInput 
        ? `<div class="ed-dlg-msg"></div><input class="ed-dlg-input" />`
        : `<div class="ed-dlg-msg"></div>`;

      const dlg = createElement(`
        <div class="ed-dlg" role="dialog" aria-modal="true">
          <div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>
          <div class="ed-dlg-body">${bodyContent}</div>
          <div class="ed-dlg-foot">
            <button type="button" class="ed-dlg-btn cancel"></button>
            <button type="button" class="ed-dlg-btn ok"></button>
          </div>
        </div>
      `);

      const { qs } = window.Goop.core;
      qs(".ed-dlg-title", dlg).textContent = title || "Input";
      qs(".ed-dlg-msg", dlg).textContent = message || "";

      const input = hasInput ? qs(".ed-dlg-input", dlg) : null;
      if (input) {
        input.placeholder = placeholder || "";
        input.value = value || "";
      }

      const bCancel = qs("button.cancel", dlg);
      const bOk = qs("button.ok", dlg);

      bCancel.textContent = cancelText || "Cancel";
      bOk.textContent = okText || "OK";
      if (dangerOk) bOk.classList.add("danger");

      function cleanup(result) {
        document.removeEventListener("keydown", handleKey);
        backdrop.remove();
        resolve(result);
      }

      function handleKey(e) {
        if (e.key === "Escape") cleanup(null);
        if (e.key === "Enter") cleanup(input ? input.value : true);
      }

      backdrop.addEventListener("mousedown", (e) => {
        if (e.target === backdrop) cleanup(null);
      });

      bCancel.addEventListener("click", () => cleanup(null));
      bOk.addEventListener("click", () => cleanup(input ? input.value : true));

      backdrop.appendChild(dlg);
      document.body.appendChild(backdrop);

      document.addEventListener("keydown", handleKey);
      if (input) {
        setTimeout(() => {
          input.focus();
          input.select();
        }, 0);
      } else {
        setTimeout(() => bOk.focus(), 0);
      }
    });
  }

  function dlgAsk(options) {
    return dlgBase({ ...options, hasInput: true });
  }

  function dlgAlert(title, message) {
    return dlgBase({
      title,
      message,
      hasInput: false,
      okText: "OK",
      cancelText: "Close",
    });
  }

  function dlgConfirm(message, title) {
    return dlgBase({
      title: title || "Confirm",
      message,
      hasInput: false,
      okText: "OK",
      cancelText: "Cancel",
    });
  }

  function dlgFilePicker(options) {
    var title = (options && options.title) || "Select File";
    var startDir = (options && options.dir) || "";
    var filter = (options && options.filter) || null;

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

      var qs = window.Goop.core.qs;
      var escapeHtml = window.Goop.core.escapeHtml;
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
        window.Goop.api.fs.browse(dir).then(function(data) {
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
  }

  window.Goop = window.Goop || {};
  window.Goop.dialogs = { dlgAsk, dlgAlert, confirm: dlgConfirm, filePicker: dlgFilePicker };
})();
