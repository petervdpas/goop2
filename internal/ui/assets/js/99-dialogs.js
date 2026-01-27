// internal/ui/assets/js/99-dialogs.js
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

  window.Goop = window.Goop || {};
  window.Goop.dialogs = { dlgAsk, dlgAlert };
})();
