// frontend/src/main.js
import "./style.css";
import splashUrl from "./assets/images/goop2-splash.png";
import {
  clear, div, btn, input, h1, h2, p,
  normalizeTheme, applyTheme, normalizeBase
} from "./utils.js";

/*
Goal:
- Launcher (Wails SPA) for peer selection.
- After peer selected + started: REPLACE the whole document with the viewer app (no iframe).
- Theme:
  - Launcher reads authoritative theme from Go (App.GetTheme()).
  - Viewer receives ?theme=... and ?bridge=... on entry.
  - Viewer posts changes back to bridge -> Go updates data/ui.json.
*/

// ----------------------
// Theme (launcher side)
// ----------------------

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
  host.classList.add("launcher-host");

  const launcher = div("launcher");

  // Left: splash image
  const splash = div("launcher-splash");
  const img = document.createElement("img");
  img.src = splashUrl;
  img.alt = "Goop²";
  splash.appendChild(img);
  launcher.appendChild(splash);

  // Right: panel
  const panel = div("launcher-panel");

  const top = div("top");
  top.appendChild(h1("Goop² - Launcher"));
  top.appendChild(p("Pick a peer, or create a new one."));
  panel.appendChild(top);

  // Create new peer card
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
  panel.appendChild(createCard);

  // Peers card
  const peersCard = div("card peersCard");
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

  panel.appendChild(peersCard);
  launcher.appendChild(panel);
  host.appendChild(launcher);

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
