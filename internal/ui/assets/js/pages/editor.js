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

  var dlg = window.Goop.dialog;

  function dlgAsk(opts) {
    return dlg.prompt(opts);
  }

  function dlgAlert(title, message) {
    return dlg.alert(title, message);
  }

  async function dlgConfirmDelete(p, isDir) {
    var label = isDir ? 'folder' : 'file';
    var result = await dlg.confirmDanger({
      title: "Confirm delete",
      message: 'Delete ' + label + ' "' + p + '"?',
      match: "DELETE",
      okText: "Delete",
    });
    return result === "DELETE";
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

  function loadFile(p) {
    if (!p || isImageExt(p)) {
      window.location.href = "/edit?path=" + encodeURIComponent(p);
      return;
    }
    var api = window.Goop.api;
    var cm = window.Goop.cm;
    if (!api || !api.site || !cm) {
      window.location.href = "/edit?path=" + encodeURIComponent(p);
      return;
    }
    api.site.content(p).then(function (data) {
      cm.setContent(data.content, p);
      state.openPath = p;
      ed.dataset.openPath = p;
      var pathInput = qs("input[name='path']");
      if (pathInput) pathInput.value = p;
      var etagInput = qs("input[name='if_match']");
      if (etagInput) etagInput.value = data.etag;
      var nameEl = qs(".ed-savebar-title code");
      if (nameEl) nameEl.textContent = p;
      var etagEl = qs(".ed-savebar-etag code");
      if (etagEl) etagEl.textContent = data.etag;
      setSelectedDir(dirname(p));
      setSelectedNode("file", p);
      var form = qs("form[data-saveable]");
      if (form && form._saveableReset) form._saveableReset();
      history.replaceState(null, "", "/edit?path=" + encodeURIComponent(p));
    }).catch(function () {
      window.location.href = "/edit?path=" + encodeURIComponent(p);
    });
  }

  function isImageExt(p) {
    return /\.(png|jpe?g|gif|webp|svg|bmp|ico)$/i.test(p);
  }

  // Tree click
  tree.addEventListener("click", (e) => {
    const li = closest(e.target, ".ed-tree-item");
    if (!li) return;

    const p = li.dataset.path || "";
    if (isDirItem(li)) {
      setSelectedDir(p);
    } else {
      loadFile(p);
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
          loadFile(t.path);
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
  const imgPanel = qs("#ed-img-panel");
  const imgUploadBtn = qs("#ed-img-upload");
  const imgHint = qs("#ed-img-hint");

  const imgPicker = window.Goop.filepicker && imgPanel
    ? window.Goop.filepicker.init(qs(".filepicker", imgPanel), {
        title: "Select Image",
        extensions: ["png", "jpg", "jpeg", "gif", "svg", "webp", "ico"],
        onChange: function(path) {
          if (imgUploadBtn) imgUploadBtn.disabled = !path || !isImagesDir(state.selectedDir);
        },
      })
    : null;

  function isImagesDir(dir) {
    return dir === "images" || dir.startsWith("images/");
  }

  function updateImgPanel() {
    const enabled = isImagesDir(state.selectedDir);
    if (imgPicker) imgPicker.setEnabled(enabled);
    if (imgUploadBtn) imgUploadBtn.disabled = !enabled || !(imgPicker && imgPicker.value());
    if (imgHint) {
      imgHint.textContent = enabled
        ? "Upload to " + state.selectedDir + "/"
        : "Select the images/ folder to enable.";
    }
  }

  if (imgUploadBtn) {
    imgUploadBtn.addEventListener("click", async () => {
      if (!imgPicker) return;
      const srcPath = imgPicker.value();
      if (!srcPath || !isImagesDir(state.selectedDir)) return;

      const filename = srcPath.split("/").pop();
      const destPath = state.selectedDir + "/" + filename;

      imgUploadBtn.disabled = true;
      try {
        const res = await window.Goop.core.api("/api/site/upload-local", { src_path: srcPath, dest_path: destPath });
        imgPicker.clear();
        window.location.reload();
      } catch (err) {
        await dlgAlert("Upload failed", err.message);
        imgUploadBtn.disabled = false;
      }
    });
  }

  initLabelsAndIndent();
  setSelectedDir(state.selectedDir);
  if (state.openPath) setSelectedNode("file", state.openPath);
})();
