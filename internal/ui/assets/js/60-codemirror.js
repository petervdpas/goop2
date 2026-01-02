// internal/ui/assets/js/60-codemirror.js
(() => {
  const { qs } = window.Goop.core;
  const Theme = window.Goop.theme;

  if (!window.CodeMirror) return;

  const ta = qs("textarea.ed-area[name='content']");
  if (!ta) return;

  const form = ta.closest("form");
  const pathInput = qs("input[name='path']");

  function modeFromPath(p) {
    p = (p || "").toLowerCase();
    if (p.endsWith(".html") || p.endsWith(".htm")) return "htmlmixed";
    if (p.endsWith(".css")) return "css";
    if (p.endsWith(".js") || p.endsWith(".mjs") || p.endsWith(".cjs")) return "javascript";
    if (p.endsWith(".json")) return { name: "javascript", json: true };
    if (p.endsWith(".md") || p.endsWith(".markdown")) return "markdown";
    return null;
  }

  function cmThemeFromAppTheme(appTheme) {
    return appTheme === "light" ? "xq-light" : "xq-dark";
  }

  const cm = window.CodeMirror.fromTextArea(ta, {
    lineNumbers: true,
    lineWrapping: true,
    indentUnit: 2,
    tabSize: 2,
    indentWithTabs: false,
    mode: modeFromPath(pathInput ? pathInput.value : "") || undefined,
    theme: cmThemeFromAppTheme(Theme ? Theme.get() : "dark"),
  });

  if (form) {
    form.addEventListener("submit", () => {
      ta.value = cm.getValue();
    });
  }

  cm.addKeyMap({
    "Ctrl-S": () => form && (form.requestSubmit ? form.requestSubmit() : form.submit()),
    "Cmd-S":  () => form && (form.requestSubmit ? form.requestSubmit() : form.submit()),
  });

  cm.setSize("100%", "70vh");

  if (Theme) {
    window.addEventListener(Theme.EVT, (e) => {
      const t = e?.detail?.theme === "light" ? "light" : "dark";
      cm.setOption("theme", cmThemeFromAppTheme(t));
      cm.refresh();
    });
  }
})();
