(() => {
  const { qs, qsa, closest, escapeCss } = window.Goop.core;

  const ed = qs("#ed");
  const tree = qs("#ed-tree");
  const menu = qs("#ed-menu");
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
    qsa(".ed-tree-item", tree).forEach((n) => n.classList.remove("selected"));
    if (type === "dir" && p === "") return;

    const selector = `.ed-tree-item[data-type="${type}"][data-path="${escapeCss(p)}"]`;
    const node = qs(selector, tree);
    if (node) node.classList.add("selected");
  }

  function setSelectedDir(dir) {
    dir = (dir && typeof dir === "string") ? dir : "";
    state.selectedDir = dir;
    ed.dataset.selectedDir = state.selectedDir;

    const label = qs("#ed-selected-dir-label");
    if (label) label.textContent = state.selectedDir || "(root)";

    setSelectedNode("dir", state.selectedDir || "");
    updateImgPanel();
  }

  const rootLink = qs("#ed-root-link");
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
    qsa(".ed-tree-item", tree).forEach((li) => {
      const p = li.dataset.path || "";
      const label = qs(".ed-tree-label", li);
      if (label) label.textContent = basename(p);
      li.title = p;

      const depth = depthFromClass(li);
      li.style.paddingLeft = (10 + depth * 14) + "px";
    });
  }

  // Dialogs (kept local to editor)
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

      qs(".ed-dlg-title", dlg).textContent = title || "Input";
      qs(".ed-dlg-msg", dlg).textContent = message || "";

      const input = qs(".ed-dlg-input", dlg);
      input.placeholder = placeholder || "";
      input.value = value || "";

      const bCancel = qs("button.cancel", dlg);
      const bOk = qs("button.ok", dlg);

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

      qs(".ed-dlg-title", dlg).textContent = title || "Notice";
      qs(".ed-dlg-msg", dlg).textContent = message || "";

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

      qs("button.ok", dlg).addEventListener("click", cleanup);

      backdrop.appendChild(dlg);
      document.body.appendChild(backdrop);

      document.addEventListener("keydown", proveKey);
      setTimeout(() => qs("button.ok", dlg).focus(), 0);
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
    const li = closest(e.target, ".ed-tree-item");
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

    qsa("[data-show]", menu).forEach((btn) => {
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

  tree.addEventListener("contextmenu", (e) => {
    const li = closest(e.target, ".ed-tree-item");
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

  // Menu actions
  menu.addEventListener("click", async (e) => {
    const btn = closest(e.target, "button[data-action]");
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

  // Header "New…" button
  const newBtn = qs("#ed-new-btn");
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

  // Image upload panel
  const imgInput = qs("#ed-img-input");
  const imgBtn = qs("#ed-img-btn");
  const imgHint = qs("#ed-img-hint");

  function isImagesDir(dir) {
    return dir === "images" || dir.startsWith("images/");
  }

  function updateImgPanel() {
    if (!imgBtn) return;
    const enabled = isImagesDir(state.selectedDir);
    imgBtn.disabled = !enabled;
    if (imgHint) {
      imgHint.textContent = enabled
        ? "Upload to " + state.selectedDir + "/"
        : "Select the images/ folder to enable.";
    }
  }

  if (imgBtn) {
    imgBtn.addEventListener("click", () => {
      if (imgInput && !imgBtn.disabled) imgInput.click();
    });
  }

  if (imgInput) {
    imgInput.addEventListener("change", async () => {
      const file = imgInput.files[0];
      if (!file) return;
      imgInput.value = "";

      const dest = state.selectedDir + "/" + file.name;
      const fd = new FormData();
      fd.append("path", dest);
      fd.append("file", file);

      const res = await fetch("/api/site/upload", {
        method: "POST",
        body: fd,
      });

      if (!res.ok) {
        const text = await res.text().catch(() => "");
        await dlgAlert("Upload failed", text.slice(0, 600));
        return;
      }

      window.location.reload();
    });
  }

  initLabelsAndIndent();
  setSelectedDir(state.selectedDir);
  if (state.openPath) setSelectedNode("file", state.openPath);
})();
