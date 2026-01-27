// frontend/src/utils.js
// Shared utility functions for frontend

/**
 * DOM helper functions
 */
export function clear(node) {
  while (node.firstChild) node.removeChild(node.firstChild);
}

export function el(tag, cls) {
  const e = document.createElement(tag);
  if (cls) e.className = cls;
  return e;
}

export function div(cls) {
  return el("div", cls);
}

export function btn(label, kind) {
  const b = document.createElement("button");
  b.type = "button";
  b.className = kind ? `btn ${kind}` : "btn";
  b.textContent = label;
  return b;
}

export function input(placeholder) {
  const i = document.createElement("input");
  i.type = "text";
  i.className = "input";
  i.placeholder = placeholder;
  i.autocomplete = "off";
  i.spellcheck = false;
  return i;
}

export function h1(text) {
  const h = div("h1");
  h.textContent = text;
  return h;
}

export function h2(text) {
  const h = div("h2");
  h.textContent = text;
  return h;
}

export function p(text) {
  const d = div("p");
  d.textContent = text;
  return d;
}

/**
 * Theme utilities
 */
export function normalizeTheme(t) {
  return t === "light" || t === "dark" ? t : "dark";
}

export function applyTheme(t) {
  try {
    t = normalizeTheme(t);
    document.documentElement.setAttribute("data-theme", t);
    localStorage.setItem("goop.theme", t);
  } catch {}
}

/**
 * URL utilities
 */
export function normalizeBase(viewerURL) {
  return String(viewerURL || "").replace(/\/+$/, "");
}
