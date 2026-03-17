(() => {
  var core = window.Goop && window.Goop.core;
  if (!core) return;

  var qs = core.qs;
  var on = core.on;
  var setHidden = core.setHidden;

  function initPicker(container, options, dialogFn) {
    if (!container) return null;

    var title = (options && options.title) || "Select";
    var input = qs(".filepicker-input, .pathpicker-input", container);
    var browseBtn = qs(".filepicker-browse, .pathpicker-browse", container);
    var clearBtn = qs(".filepicker-clear, .pathpicker-clear", container);
    var onChange = (options && options.onChange) || null;

    function openPicker() {
      if (!window.Goop.dialogs) return;
      var startDir = input.value.trim();
      if (startDir) {
        var lastSlash = startDir.lastIndexOf("/");
        if (lastSlash > 0) startDir = startDir.substring(0, lastSlash);
      }
      dialogFn({ title: title, dir: startDir }).then(function(path) {
        if (!path) return;
        input.value = path;
        setHidden(clearBtn, false);
        if (onChange) onChange(path);
      });
    }

    on(browseBtn, "click", openPicker);
    on(input, "click", openPicker);

    on(clearBtn, "click", function() {
      input.value = "";
      setHidden(clearBtn, true);
      if (onChange) onChange("");
    });

    return {
      value: function() { return input.value.trim(); },
      setValue: function(v) {
        input.value = v || "";
        setHidden(clearBtn, !v);
      },
      clear: function() {
        input.value = "";
        setHidden(clearBtn, true);
      }
    };
  }

  function initFile(container, options) {
    return initPicker(container, options, function(opts) {
      return window.Goop.dialogs.filePicker(opts);
    });
  }

  function initPath(container, options) {
    return initPicker(container, options, function(opts) {
      return window.Goop.dialogs.pathPicker(opts);
    });
  }

  window.Goop = window.Goop || {};
  window.Goop.filepicker = { init: initFile };
  window.Goop.pathpicker = { init: initPath };
})();
