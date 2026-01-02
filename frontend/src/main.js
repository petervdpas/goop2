// frontend/src/main.js
import "./style.css";

/*
Goal:
- Launcher (Wails SPA) for peer selection.
- After peer selected + started: REPLACE the whole document with the viewer app (no iframe).
- Theme:
  - Launcher reads authoritative theme from Go (App.GetTheme()).
  - Viewer receives ?theme=... and ?bridge=... on entry.
  - Viewer posts changes back to bridge -> Go updates data/ui.json.
*/

function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

function el(tag, cls) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  return e;
}
function div(cls) {
  return el("div", cls);
}

function btn(label, kind) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = kind ? `btn ${kind}` : "btn";
  b.textContent = label;
  return b;
}

function input(placeholder) {
  const i = document.createElement("input");
  i.type = "text";
  i.className = "input";
  i.placeholder = placeholder;
  i.autocomplete = "off";
  i.spellcheck = false;
  return i;
}

function h1(text) {
  const h = div("h1");
  h.textContent = text;
  return h;
}

function h2(text) {
  const h = div("h2");
  h.textContent = text;
  return h;
}

function p(text) {
  const d = div("p");
  d.textContent = text;
  return d;
}

// ----------------------
// Theme (launcher side)
// ----------------------

function normalizeTheme(t) {
  return t === "light" || t === "dark" ? t : "dark";
}

function applyTheme(t) {
  try {
    t = normalizeTheme(t);
    document.documentElement.setAttribute("data-theme", t);

    // Optional localStorage for launcher-only styling continuity.
    // Not relied upon for cross-origin sync (ui.json is the authority).
    localStorage.setItem("goop.theme", t);
  } catch {}
}

async function loadThemeAuthoritative() {
  try {
    const t = await window.go.main.App.GetTheme();
    return normalizeTheme(t);
  } catch {
    return "dark";
  }
}

async function setThemeAuthoritative(t) {
  t = normalizeTheme(t);
  applyTheme(t);
  try {
    await window.go.main.App.SetTheme(t);
  } catch {
    // ignore
  }
}

async function wireThemeToggle() {
  const themeToggle = document.getElementById("themeToggle");
  if (!themeToggle) return;

  const t0 = await loadThemeAuthoritative();
  applyTheme(t0);
  themeToggle.checked = t0 === "light";

  themeToggle.addEventListener("change", async () => {
    const t = themeToggle.checked ? "light" : "dark";
    await setThemeAuthoritative(t);
  });
}

// ----------------------
// Navigation to viewer (REAL REPLACE)
// ----------------------

function normalizeBase(viewerURL) {
  return String(viewerURL || "").replace(/\/+$/, "");
}

async function goViewer(viewerURL, path) {
  const base = normalizeBase(viewerURL);
  if (!base) throw new Error("viewerURL is empty");

  const pth = path && String(path).startsWith("/") ? path : "/peers";

  // Theme source of truth: launcher DOM (already set from ui.json via Go)
  const theme = normalizeTheme(
    document.documentElement.getAttribute("data-theme")
  );

  // Bridge URL so internal viewer can sync theme back to Wails
  let bridge = "";
  try {
    bridge = await window.go.main.App.GetBridgeURL();
  } catch {
    bridge = "";
  }

  const u = new URL(base + pth);
  u.searchParams.set("theme", theme);
  if (bridge) u.searchParams.set("bridge", bridge);

  window.location.replace(u.toString());
}

// ----------------------
// Launcher UI (peer picker)
// ----------------------

async function renderLauncher(host) {
  clear(host);

  const shell = div("shell");
  const top = div("top");
  top.appendChild(h1("Goop² - Launcher"));
  top.appendChild(p("Pick a peer, or create a new one."));
  shell.appendChild(top);

  const grid = div("grid");

  const peersCard = div("card");
  const peersHead = div("cardHead");
  peersHead.appendChild(p("Peers"));
  peersCard.appendChild(peersHead);

  const peersBody = div("cardBody");
  const list = div("tileList");
  peersBody.appendChild(list);
  peersCard.appendChild(peersBody);

  const foot = div("cardFoot");
  const start = btn("Start", "primary");
  start.disabled = true;

  const del = btn("Delete", "danger");
  del.disabled = true;

  const status = div("status");
  const err = div("error");

  foot.appendChild(start);
  foot.appendChild(del);
  foot.appendChild(status);
  foot.appendChild(err);
  peersCard.appendChild(foot);

  const createCard = div("card");
  const createHead = div("cardHead");
  createHead.appendChild(p("Create new peer"));
  createCard.appendChild(createHead);

  const createBody = div("cardBody");
  const row = div("row");
  const name = input("peerC");
  const create = btn("Create", "secondary");
  row.appendChild(name);
  row.appendChild(create);
  createBody.appendChild(row);
  createCard.appendChild(createBody);

  grid.appendChild(peersCard);
  grid.appendChild(createCard);
  shell.appendChild(grid);

  host.appendChild(shell);

  let peers = await window.go.main.App.ListPeers();
  let selected = "";

  function setSelected(v) {
    selected = v;
    start.disabled = !selected;
    del.disabled = !selected;
    err.textContent = "";
    status.textContent = selected ? `Selected: ${selected}` : "";
  }

  function renderList() {
    clear(list);

    if (!peers || peers.length === 0) {
      const empty = div("empty");
      empty.textContent = "No peers found.";
      list.appendChild(empty);
      setSelected("");
      return;
    }

    for (const peer of peers) {
      const tile = div("tile");
      const left = div("tileLeft");

      const radio = document.createElement("input");
      radio.type = "radio";
      radio.name = "peer";
      radio.checked = peer === selected;
      radio.addEventListener("change", () => setSelected(peer));

      const meta = div("tileMeta");
      const nm = div("tileName");
      nm.textContent = peer;
      const path = div("tilePath");
      path.textContent = `./peers/${peer}/goop.json`;

      meta.appendChild(nm);
      meta.appendChild(path);

      left.appendChild(radio);
      left.appendChild(meta);

      tile.addEventListener("click", (e) => {
        if (e.target === radio) return;
        radio.checked = true;
        setSelected(peer);
      });

      tile.appendChild(left);
      list.appendChild(tile);
    }
  }

  async function refreshPeers(selectName) {
    peers = await window.go.main.App.ListPeers();
    if (selectName && peers.includes(selectName)) setSelected(selectName);
    else if (!peers.includes(selected)) setSelected("");
    renderList();
  }

  start.addEventListener("click", async () => {
    if (!selected) return;

    start.disabled = true;
    del.disabled = true;
    err.textContent = "";
    status.textContent = "Starting…";

    try {
      await window.go.main.App.StartPeer(selected);
      const st = await window.go.main.App.GetStatus();

      if (!st || !st.viewerURL)
        throw new Error("Started but viewerURL missing from status.");
      await goViewer(st.viewerURL, "/peers");
    } catch (e) {
      err.textContent = String(e);
      status.textContent = "";
      start.disabled = false;
      del.disabled = false;
    }
  });

  del.addEventListener("click", async () => {
    if (!selected) return;

    const ok = window.confirm(
      `Delete peer "${selected}"?\n\nThis will remove ./peers/${selected}/`
    );
    if (!ok) return;

    err.textContent = "";
    status.textContent = "Deleting…";
    start.disabled = true;
    del.disabled = true;

    try {
      await window.go.main.App.DeletePeer(selected);
      await refreshPeers("");
      status.textContent = "Deleted.";
    } catch (e) {
      err.textContent = String(e);
      status.textContent = "";
    } finally {
      start.disabled = !selected;
      del.disabled = !selected;
    }
  });

  create.addEventListener("click", async () => {
    err.textContent = "";
    status.textContent = "";

    const v = name.value.trim();
    if (!v) {
      err.textContent = "Enter a peer name.";
      return;
    }

    create.disabled = true;
    create.textContent = "Creating…";

    try {
      const created = await window.go.main.App.CreatePeer(v);
      name.value = "";
      await refreshPeers(created);
      status.textContent = `Created: ${created}`;
    } catch (e) {
      err.textContent = String(e);
    } finally {
      create.disabled = false;
      create.textContent = "Create";
    }
  });

  renderList();
  setSelected("");

  if (peers && peers.length > 0) {
    setSelected(peers[0]);
    renderList();
  }
}

// ----------------------
// Boot
// ----------------------

async function boot() {
  await wireThemeToggle();

  // If a peer is already started, immediately replace with viewer.
  const st = await window.go.main.App.GetStatus();
  if (st && st.started === "true" && st.viewerURL) {
    await goViewer(st.viewerURL, "/peers");
    return;
  }

  const content =
    document.getElementById("content") || document.getElementById("app");
  if (!content) return;

  await renderLauncher(content);
}

boot();
