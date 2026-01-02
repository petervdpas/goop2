// internal/ui/assets/app.js
(() => {
  function onReady(fn) {
    if (document.readyState === "loading") {
      document.addEventListener("DOMContentLoaded", fn, { once: true });
    } else {
      fn();
    }
  }

  function safeLocalStorageGet(key) {
    try { return localStorage.getItem(key); } catch { return null; }
  }
  function safeLocalStorageSet(key, value) {
    try { localStorage.setItem(key, value); } catch {}
  }

  // 1) Optional: peers autorefresh
  (() => {
    const url = new URL(window.location.href);
    if (url.pathname === "/peers" && url.searchParams.get("autorefresh") === "1") {
      setInterval(() => {
        if (document.hasFocus()) window.location.reload();
      }, 5000);
    }
  })();

  // 2) Theme (app + event bus)
  const Theme = (() => {
    const KEY = "goop.theme";
    const EVT = "goop:theme";

    function normalize(t) {
      return t === "light" || t === "dark" ? t : "dark";
    }

    function get() {
      const dom = document.documentElement.getAttribute("data-theme");
      if (dom === "light" || dom === "dark") return dom;
      return normalize(safeLocalStorageGet(KEY));
    }

    function set(t) {
      t = normalize(t);
      document.documentElement.setAttribute("data-theme", t);
      safeLocalStorageSet(KEY, t);
      window.dispatchEvent(new CustomEvent(EVT, { detail: { theme: t } }));
    }

    function initToggle() {
      const toggle = document.getElementById("themeToggle");
      if (!toggle) return;

      toggle.checked = (get() === "light");

      toggle.addEventListener("change", () => {
        set(toggle.checked ? "light" : "dark");
      });

      window.addEventListener(EVT, (e) => {
        const t = e?.detail?.theme === "light" ? "light" : "dark";
        toggle.checked = (t === "light");
      });
    }

    return { get, set, initToggle, EVT };
  })();

  onReady(() => Theme.initToggle());

  // 2.5) Rendezvous toggle wiring (Me page)
  onReady(() => {
    const host = document.getElementById("rv_host");
    const port = document.getElementById("rv_port");
    const open = document.getElementById("rv-open");
    const link = document.getElementById("rv-open-link");

    if (!host || !port || !open) return;

    function normalizePort(v) {
      v = String(v || "").trim();
      if (!v) return "8787";
      if (!/^\d+$/.test(v)) return "8787";
      return v;
    }

    function sync() {
      const on = !!host.checked;

      port.disabled = !on;
      port.classList.toggle("rv-disabled", !on);

      // hidden when OFF, visible when ON.
      open.classList.toggle("hidden", !on);

      if (link) {
        const p = normalizePort(port.value);
        link.href = `http://127.0.0.1:${p}/`;
      }
    }

    host.addEventListener("change", sync);
    port.addEventListener("input", sync);
    sync();
  });

  // 3) Editor tree + context menu + dialogs
  (() => {
    const ed = document.getElementById("ed");
    const tree = document.getElementById("ed-tree");
    const menu = document.getElementById("ed-menu");
    if (!ed || !tree || !menu) return;

    const state = {
      csrf: ed.dataset.csrf || "",
      openPath: ed.dataset.openPath || "",
      selectedDir: ed.dataset.selectedDir || "",
      menuTarget: null, // { type: "dir"|"file", path: "..." }
    };

    function basename(p) {
      if (!p) return "";
      const parts = p.split("/").filter(Boolean);
      return parts.length ? parts[parts.length - 1] : "";
    }

    function dirname(p) {
      if (!p) return "";
      const parts = p.split("/").filter(Boolean);
      parts.pop();
      return parts.join("/");
    }

    function isDirItem(li) {
      return li.dataset.type === "dir";
    }

    function setSelectedNode(type, p) {
      p = (p && typeof p === "string") ? p : "";

      tree.querySelectorAll(".ed-tree-item").forEach((n) => n.classList.remove("selected"));

      // If root is selected (p=""), there is no root node to highlight.
      if (type === "dir" && p === "") return;

      const selector = `.ed-tree-item[data-type="${type}"][data-path="${CSS.escape(p)}"]`;
      const node = tree.querySelector(selector);
      if (node) node.classList.add("selected");
    }

    function setSelectedDir(dir) {
      dir = (dir && typeof dir === "string") ? dir : "";
      state.selectedDir = dir;
      ed.dataset.selectedDir = state.selectedDir;

      const label = document.getElementById("ed-selected-dir-label");
      if (label) label.textContent = state.selectedDir || "(root)";

      setSelectedNode("dir", state.selectedDir || "");
    }

    // Click "site/" to target root.
    const rootLink = document.getElementById("ed-root-link");
    if (rootLink) {
      rootLink.addEventListener("click", (e) => {
        e.preventDefault();
        setSelectedDir("");
      });
    }

    function destDirForMenuAction() {
      const t = state.menuTarget;

      if (t && t.type === "dir") return t.path || "";
      if (t && t.type === "file") return dirname(t.path || "");
      return state.selectedDir || "";
    }

    function depthFromClass(li) {
      const m = li.className.match(/\bdepth-(\d+)\b/);
      return m ? parseInt(m[1], 10) : 0;
    }

    function initLabelsAndIndent() {
      tree.querySelectorAll(".ed-tree-item").forEach((li) => {
        const p = li.dataset.path || "";
        const label = li.querySelector(".ed-tree-label");
        if (label) label.textContent = basename(p);
        li.title = p;

        const depth = depthFromClass(li);
        li.style.paddingLeft = (10 + depth * 14) + "px";
      });
    }

    async function post(action, params) {
      const body = new URLSearchParams({ csrf: state.csrf, ...params }).toString();

      const res = await fetch(action, {
        method: "POST",
        headers: {
          "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
          "X-CSRF-Token": state.csrf,
        },
        body,
        credentials: "same-origin",
      });

      if (res.redirected) {
        window.location.href = res.url;
        return;
      }

      if (!res.ok) {
        const text = await res.text().catch(() => "");
        await dlgAlert(`Action failed (${res.status})`, text.slice(0, 600));
        return;
      }

      window.location.reload();
    }

    // Tree click
    tree.addEventListener("click", (e) => {
      const li = e.target.closest(".ed-tree-item");
      if (!li) return;

      const p = li.dataset.path || "";
      if (isDirItem(li)) {
        setSelectedDir(p);
      } else {
        window.location.href = "/edit?path=" + encodeURIComponent(p);
      }
    });

    // Context menu UI
    function hideMenu() {
      menu.classList.add("hidden");
      state.menuTarget = null;
    }

    function showMenu(x, y, target) {
      state.menuTarget = target;

      menu.querySelectorAll("[data-show]").forEach((btn) => {
        const show = btn.dataset.show;
        btn.style.display = (show === "any" || show === target.type) ? "block" : "none";
      });

      menu.style.left = `${x}px`;
      menu.style.top = `${y}px`;
      menu.classList.remove("hidden");
    }

    document.addEventListener("click", () => hideMenu());
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") hideMenu();
    });

    // Right click on tree item
    tree.addEventListener("contextmenu", (e) => {
      const li = e.target.closest(".ed-tree-item");
      if (!li) return;

      e.preventDefault();

      const type = li.dataset.type || "file";
      const p = li.dataset.path || "";

      if (type === "dir") {
        setSelectedDir(p);
      } else {
        setSelectedNode("file", p);
      }

      showMenu(e.clientX, e.clientY, { type, path: p });
    });

    // Dialog helpers
    function q(html) {
      const t = document.createElement("template");
      t.innerHTML = html.trim();
      return t.content.firstElementChild;
    }

    function dlgAsk({ title, message, placeholder, value, okText, cancelText, dangerOk }) {
      return new Promise((resolve) => {
        const backdrop = q(`<div class="ed-dlg-backdrop"></div>`);
        const dlg = q(`
          <div class="ed-dlg" role="dialog" aria-modal="true">
            <div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>
            <div class="ed-dlg-body">
              <div class="ed-dlg-msg"></div>
              <input class="ed-dlg-input" />
            </div>
            <div class="ed-dlg-foot">
              <button type="button" class="ed-dlg-btn cancel"></button>
              <button type="button" class="ed-dlg-btn ok"></button>
            </div>
          </div>
        `);

        dlg.querySelector(".ed-dlg-title").textContent = title || "Input";
        dlg.querySelector(".ed-dlg-msg").textContent = message || "";

        const input = dlg.querySelector(".ed-dlg-input");
        input.placeholder = placeholder || "";
        input.value = value || "";

        const bCancel = dlg.querySelector("button.cancel");
        const bOk = dlg.querySelector("button.ok");

        bCancel.textContent = cancelText || "Cancel";
        bOk.textContent = okText || "OK";
        if (dangerOk) bOk.classList.add("danger");

        function cleanup(result) {
          document.removeEventListener("keydown", proveKey);
          backdrop.remove();
          resolve(result);
        }

        function proveKey(e) {
          if (e.key === "Escape") cleanup(null);
          if (e.key === "Enter") cleanup(input.value);
        }

        backdrop.addEventListener("mousedown", (e) => {
          if (e.target === backdrop) cleanup(null);
        });

        bCancel.addEventListener("click", () => cleanup(null));
        bOk.addEventListener("click", () => cleanup(input.value));

        backdrop.appendChild(dlg);
        document.body.appendChild(backdrop);

        document.addEventListener("keydown", proveKey);
        setTimeout(() => {
          input.focus();
          input.select();
        }, 0);
      });
    }

    function dlgAlert(title, message) {
      return new Promise((resolve) => {
        const backdrop = q(`<div class="ed-dlg-backdrop"></div>`);
        const dlg = q(`
          <div class="ed-dlg" role="dialog" aria-modal="true">
            <div class="ed-dlg-head"><div class="ed-dlg-title"></div></div>
            <div class="ed-dlg-body"><div class="ed-dlg-msg"></div></div>
            <div class="ed-dlg-foot">
              <button type="button" class="ed-dlg-btn ok">OK</button>
            </div>
          </div>
        `);

        dlg.querySelector(".ed-dlg-title").textContent = title || "Notice";
        dlg.querySelector(".ed-dlg-msg").textContent = message || "";

        function cleanup() {
          document.removeEventListener("keydown", proveKey);
          backdrop.remove();
          resolve();
        }

        function proveKey(e) {
          if (e.key === "Escape" || e.key === "Enter") cleanup();
        }

        backdrop.addEventListener("mousedown", (e) => {
          if (e.target === backdrop) cleanup();
        });

        dlg.querySelector("button.ok").addEventListener("click", cleanup);

        backdrop.appendChild(dlg);
        document.body.appendChild(backdrop);

        document.addEventListener("keydown", proveKey);
        setTimeout(() => dlg.querySelector("button.ok").focus(), 0);
      });
    }

    async function dlgConfirmDelete(p, isDir) {
      const msg = isDir
        ? `Delete folder "${p}"?\n\nType DELETE to confirm.`
        : `Delete file "${p}"?\n\nType DELETE to confirm.`;

      const v = await dlgAsk({
        title: "Confirm delete",
        message: msg,
        placeholder: "Type DELETE",
        value: "",
        okText: "Delete",
        cancelText: "Cancel",
        dangerOk: true,
      });

      return v === "DELETE";
    }

    // Menu actions
    menu.addEventListener("click", async (e) => {
      const btn = e.target.closest("button[data-action]");
      if (!btn || !state.menuTarget) return;

      e.preventDefault();
      const t = state.menuTarget;
      hideMenu();

      switch (btn.dataset.action) {
        case "new-folder": {
          const dir = destDirForMenuAction();
          const name = await dlgAsk({
            title: "New folder",
            message: `Create folder under "${dir || "(root)"}"`,
            placeholder: "e.g. sub",
            value: "sub",
            okText: "Create",
            cancelText: "Cancel",
          });
          if (!name) return;
          await post("/edit/mkdir", { dir, name });
          break;
        }

        case "new-file": {
          const dir = destDirForMenuAction();
          const name = await dlgAsk({
            title: "New file",
            message: `Create file under "${dir || "(root)"}"`,
            placeholder: "e.g. about.html",
            value: "about.html",
            okText: "Create",
            cancelText: "Cancel",
          });
          if (!name) return;
          await post("/edit/new", { dir, name });
          break;
        }

        case "rename": {
          const from = t.path;
          const to = await dlgAsk({
            title: "Rename / Move",
            message: `From: ${from}`,
            placeholder: "e.g. sub_new or pages/home.html",
            value: basename(from),
            okText: "Apply",
            cancelText: "Cancel",
          });
          if (!to) return;
          await post("/edit/rename", { from, to });
          break;
        }

        case "delete": {
          const p = t.path;
          const isDir = t.type === "dir";
          const ok = await dlgConfirmDelete(p, isDir);
          if (!ok) return;

          const recursive = isDir ? "1" : "";
          await post("/edit/delete", { path: p, recursive });
          break;
        }

        case "open": {
          if (t.type === "file") {
            window.location.href = "/edit?path=" + encodeURIComponent(t.path);
          }
          break;
        }
      }
    });

    // Header "New…" button: create in currently selected folder (root by default).
    const newBtn = document.getElementById("ed-new-btn");
    if (newBtn) {
      newBtn.addEventListener("click", async () => {
        const dir = state.selectedDir || "";

        const kind = await dlgAsk({
          title: "Create",
          message: `Create in "${dir || "(root)"}"\n\nType "folder" or "file"`,
          placeholder: "folder",
          value: "folder",
          okText: "Next",
          cancelText: "Cancel",
        });
        if (!kind) return;

        const k = kind.trim().toLowerCase();
        if (k !== "folder" && k !== "file") return;

        const name = await dlgAsk({
          title: k === "folder" ? "New folder" : "New file",
          message: `Create ${k} under "${dir || "(root)"}"`,
          placeholder: k === "folder" ? "e.g. sub" : "e.g. about.html",
          value: k === "folder" ? "sub" : "about.html",
          okText: "Create",
          cancelText: "Cancel",
        });
        if (!name) return;

        if (k === "folder") await post("/edit/mkdir", { dir, name });
        else await post("/edit/new", { dir, name });
      });
    }

    initLabelsAndIndent();
    setSelectedDir(state.selectedDir);

    // Highlight the open file (separately from folder target)
    if (state.openPath) setSelectedNode("file", state.openPath);
  })();

  // 4) CodeMirror 5 hookup + theming
  (() => {
    if (!window.CodeMirror) return;

    const ta = document.querySelector("textarea.ed-area[name='content']");
    if (!ta) return;

    const form = ta.closest("form");
    const pathInput = document.querySelector("input[name='path']");

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
      theme: cmThemeFromAppTheme(Theme.get()),
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

    window.addEventListener(Theme.EVT, (e) => {
      const t = e?.detail?.theme === "light" ? "light" : "dark";
      cm.setOption("theme", cmThemeFromAppTheme(t));
      cm.refresh();
    });
  })();
})();
