// frontend/src/main.js
import "./style.css";
import splashUrl from "./assets/images/goop2-splash.png";
import iconUrl from "./assets/images/goop2.png";
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
// Helpers
// ----------------------

// Find a peer info object by name in the peers array.
function findPeer(peers, name) {
  return peers.find(p => p.name === name);
}

// Check if a peer name exists in the peers array.
function peerExists(peers, name) {
  return peers.some(p => p.name === name);
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

  // peers is now [{name, rendezvous_only}, ...]
  let peers = await window.go.main.App.ListPeers();
  let selected = ""; // selected peer name

  function setSelected(v) {
    selected = v;
    start.disabled = !selected;
    del.disabled = !selected;
    err.textContent = "";

    if (selected) {
      const info = findPeer(peers, selected);
      if (info && info.rendezvous_only) {
        start.textContent = "Configure";
        status.textContent = `Selected: ${selected} (rendezvous)`;
      } else {
        start.textContent = "Start";
        status.textContent = `Selected: ${selected}`;
      }
    } else {
      start.textContent = "Start";
      status.textContent = "";
    }
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
      radio.checked = peer.name === selected;
      radio.addEventListener("change", () => setSelected(peer.name));

      const meta = div("tileMeta");
      const nmRow = div("tileNameRow");
      const nm = document.createElement("span");
      nm.className = "tileName";
      nm.textContent = peer.name;
      nmRow.appendChild(nm);

      if (peer.rendezvous_only) {
        const badge = document.createElement("span");
        badge.className = "rv-badge";
        badge.textContent = "Rendezvous";
        nmRow.appendChild(badge);
      }

      const path = div("tilePath");
      path.textContent = `./peers/${peer.name}/goop.json`;

      meta.appendChild(nmRow);
      meta.appendChild(path);

      left.appendChild(radio);
      left.appendChild(meta);

      tile.addEventListener("click", (e) => {
        if (e.target === radio) return;
        radio.checked = true;
        setSelected(peer.name);
      });

      tile.appendChild(left);
      list.appendChild(tile);
    }
  }

  async function refreshPeers(selectName) {
    peers = await window.go.main.App.ListPeers();
    if (selectName && peerExists(peers, selectName)) setSelected(selectName);
    else if (!peerExists(peers, selected)) setSelected("");
    renderList();
  }

  start.addEventListener("click", async () => {
    if (!selected) return;

    const info = findPeer(peers, selected);
    const isRV = info && info.rendezvous_only;

    start.disabled = true;
    del.disabled = true;
    err.textContent = "";
    status.textContent = isRV ? "Configuring…" : "Starting…";

    try {
      await window.go.main.App.StartPeer(selected);
      const st = await window.go.main.App.GetStatus();

      if (!st || !st.viewerURL)
        throw new Error("Started but viewerURL missing from status.");

      // Rendezvous-only peers open settings; regular peers open peer list
      await goViewer(st.viewerURL, isRV ? "/self" : "/peers");
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
    setSelected(peers[0].name);
    renderList();
  }
}

// ----------------------
// Boot
// ----------------------

async function boot() {
  const brandIcon = document.querySelector(".brand-icon");
  if (brandIcon) brandIcon.src = iconUrl;

  await wireThemeToggle();

  // If a peer is already started, immediately replace with viewer.
  const st = await window.go.main.App.GetStatus();
  if (st && st.started === "true" && st.viewerURL) {
    const path = st.rendezvousOnly === "true" ? "/self" : "/peers";
    await goViewer(st.viewerURL, path);
    return;
  }

  const content =
    document.getElementById("content") || document.getElementById("app");
  if (!content) return;

  await renderLauncher(content);
}

boot();
