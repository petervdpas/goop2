(() => {
  window.Goop = window.Goop || {};
  window.Goop.ui = window.Goop.ui || {};

  Goop.ui.dialog = function(el, opts) {
    opts = opts || {};
    var titleEl = opts.title ? el.querySelector(opts.title) : null;
    var messageEl = opts.message ? el.querySelector(opts.message) : null;
    var inputWrap = opts.inputWrap ? el.querySelector(opts.inputWrap) : null;
    var inputEl = opts.input ? el.querySelector(opts.input) : null;
    var okBtn = opts.ok ? el.querySelector(opts.ok) : null;
    var cancelBtn = opts.cancel ? el.querySelector(opts.cancel) : null;
    var hiddenClass = opts.hiddenClass || "hidden";
    var dangerClass = opts.dangerClass || "";
    var disabledClass = opts.disabledClass || "";
    var pending = null;

    function show(config) {
      config = config || {};
      return new Promise(function(resolve) {
        if (titleEl) titleEl.textContent = config.title || "";
        if (messageEl) messageEl.textContent = config.message || "";

        if (inputWrap) {
          if (config.showInput) inputWrap.classList.remove(hiddenClass);
          else inputWrap.classList.add(hiddenClass);
        }
        if (inputEl) {
          inputEl.value = config.inputValue || "";
          if (config.placeholder) inputEl.placeholder = config.placeholder;
          if (config.inputType) inputEl.type = config.inputType;
        }

        if (okBtn) {
          okBtn.textContent = config.okLabel || "OK";
          if (dangerClass) {
            if (config.danger) okBtn.classList.add(dangerClass);
            else okBtn.classList.remove(dangerClass);
          }
          okBtn.disabled = false;
          if (config.match && inputEl) {
            okBtn.disabled = true;
            if (disabledClass) okBtn.classList.add(disabledClass);
          }
        }

        if (cancelBtn) {
          if (config.hideCancel) cancelBtn.classList.add(hiddenClass);
          else cancelBtn.classList.remove(hiddenClass);
          cancelBtn.textContent = config.cancelLabel || "Cancel";
        }

        if (config.match && inputEl) {
          inputEl.addEventListener("input", matchHandler);
        }

        el.classList.remove(hiddenClass);
        pending = { resolve: resolve, config: config };

        if (config.showInput && inputEl) {
          setTimeout(function() { inputEl.focus(); inputEl.select(); }, 0);
        } else if (okBtn) {
          setTimeout(function() { okBtn.focus(); }, 0);
        }
      });
    }

    function matchHandler() {
      if (!pending || !pending.config.match || !inputEl || !okBtn) return;
      var ok = inputEl.value === pending.config.match;
      okBtn.disabled = !ok;
      if (disabledClass) {
        if (!ok) okBtn.classList.add(disabledClass);
        else okBtn.classList.remove(disabledClass);
      }
    }

    function hide(result) {
      el.classList.add(hiddenClass);
      if (inputEl) inputEl.removeEventListener("input", matchHandler);
      if (pending) {
        var p = pending;
        pending = null;
        p.resolve(result);
      }
    }

    if (okBtn) okBtn.addEventListener("click", function() {
      if (okBtn.disabled) return;
      if (pending && pending.config.match && inputEl && inputEl.value !== pending.config.match) return;
      hide(inputEl && pending && pending.config.showInput ? inputEl.value : true);
    });

    if (cancelBtn) cancelBtn.addEventListener("click", function() { hide(null); });

    el.addEventListener("mousedown", function(e) { if (e.target === el) hide(null); });

    function onKey(e) {
      if (!pending) return;
      if (e.key === "Escape") { hide(null); return; }
      if (e.key === "Enter") {
        if (pending.config.match && inputEl && inputEl.value !== pending.config.match) return;
        hide(inputEl && pending.config.showInput ? inputEl.value : true);
      }
    }
    document.addEventListener("keydown", onKey);

    var api = {
      show: show,
      close: function() { hide(null); },
      confirm: function(message, title) {
        return show({ title: title || "Confirm", message: message });
      },
      alert: function(title, message) {
        return show({ title: title, message: message, hideCancel: true });
      },
      prompt: function(config) {
        if (typeof config === "string") config = { message: config };
        config = config || {};
        return show({
          title: config.title || "Input",
          message: config.message || "",
          showInput: true,
          inputValue: config.value || "",
          placeholder: config.placeholder || "",
          inputType: config.type || "text",
          okLabel: config.okText || "OK",
          cancelLabel: config.cancelText || "Cancel",
          danger: config.danger || false,
        });
      },
      confirmDanger: function(config) {
        config = config || {};
        return show({
          title: config.title || "Confirm",
          message: config.message || "",
          showInput: true,
          placeholder: config.placeholder || "Type " + (config.match || "DELETE"),
          match: config.match || "DELETE",
          okLabel: config.okText || "Delete",
          cancelLabel: config.cancelText || "Cancel",
          danger: true,
        });
      },
      destroy: function() { document.removeEventListener("keydown", onKey); },
      el: el,
    };

    Goop.ui.confirm = api.confirm;
    Goop.ui.alert = api.alert;
    Goop.ui.prompt = api.prompt;
    Goop.ui.confirmDanger = api.confirmDanger;

    return api;
  };
})();
