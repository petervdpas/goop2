// frontend/src/main.js
import "./style.css";

/*
Goal (what you actually want):
- Launcher (Wails SPA) for peer selection.
- After peer selected + started: REPLACE the whole document with the viewer app
  (no iframe, so scroll + scaling behave correctly).
- Theme toggle uses localStorage "goop.theme" (same as viewer).
*/

function clear(node) {
	while (node.firstChild) node.removeChild(node.firstChild);
}

function el(tag, cls) {
	const e = document.createElement(tag);
	if (cls) e.className = cls;
	return e;
}
function div(cls) { return el("div", cls); }

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

function p(text) {
	const d = div("p");
	d.textContent = text;
	return d;
}

// ----------------------
// Theme (same key/behavior as viewer layout.html)
// ----------------------

function loadTheme() {
	try {
		let t = localStorage.getItem("goop.theme");
		if (t !== "light" && t !== "dark") t = "dark";
		return t;
	} catch {
		return "dark";
	}
}

function applyTheme(t) {
	try {
		if (t !== "light" && t !== "dark") t = "dark";
		document.documentElement.setAttribute("data-theme", t);
		localStorage.setItem("goop.theme", t);
	} catch {}
}

function wireThemeToggle() {
	const themeToggle = document.getElementById("themeToggle");
	if (!themeToggle) return;

	applyTheme(loadTheme());
	themeToggle.checked = (document.documentElement.getAttribute("data-theme") || "dark") === "light";

	themeToggle.addEventListener("change", () => {
		const cur = document.documentElement.getAttribute("data-theme") || "dark";
		applyTheme(cur === "dark" ? "light" : "dark");
		themeToggle.checked = (document.documentElement.getAttribute("data-theme") || "dark") === "light";
	});
}

// ----------------------
// Navigation to viewer (REAL REPLACE)
// ----------------------

function normalizeBase(viewerURL) {
	return String(viewerURL || "").replace(/\/+$/, "");
}

function goViewer(viewerURL, path) {
	const base = normalizeBase(viewerURL);
	if (!base) throw new Error("viewerURL is empty");
	const p = path && String(path).startsWith("/") ? path : "/peers";
	// Real replacement: no iframe, no nested scrolling issues.
	window.location.replace(base + p);
}

// ----------------------
// Launcher UI (peer picker)
// ----------------------

async function renderLauncher(host) {
	clear(host);

	const shell = div("shell");
	const top = div("top");
	top.appendChild(h1("Goop"));
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

			if (!st || !st.viewerURL) throw new Error("Started but viewerURL missing from status.");
			goViewer(st.viewerURL, "/peers");
		} catch (e) {
			err.textContent = String(e);
			status.textContent = "";
			start.disabled = false;
			del.disabled = false;
		}
	});

	del.addEventListener("click", async () => {
		if (!selected) return;

		const ok = window.confirm(`Delete peer "${selected}"?\n\nThis will remove ./peers/${selected}/`);
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
}

// ----------------------
// Boot
// ----------------------

async function boot() {
	wireThemeToggle();

	// IMPORTANT: once a peer is already started, we *immediately* replace with viewer.
	const st = await window.go.main.App.GetStatus();
	if (st && st.started === "true" && st.viewerURL) {
		goViewer(st.viewerURL, "/peers");
		return;
	}

	const content = document.getElementById("content") || document.getElementById("app");
	if (!content) return;

	await renderLauncher(content);
}

boot();
