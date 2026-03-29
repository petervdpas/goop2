# Draggable Split Pane (vertical resizer)

## Goal

Add a draggable vertical divider between left and right panes across all pages that use a sidebar + content layout. Like VS Code's sidebar resize handle.

## Pages that need it

From the screenshots:

1. **Settings** (Me) — settings nav (left) | settings main (right)
2. **Editor** (Create > Editor) — file tree (left) | editor (right)
3. **Lua Scripts** (Create > Lua Scripts) — script list (left) | editor (right)
4. **Database > Tables** — table list (left) | table content (right)
5. **Database > Schemas** — schema list (left) | schema editor (right)
6. **Database > Transformations** — tx list (left) | tx editor (right)
7. **Groups > Hosted** — create form (left) | hosted groups list (right)
8. **Groups > Files** — file browser (left) | preview (right)
9. **Groups > Cluster** — cluster config (left) | cluster detail (right) — **needs a wrapper pane on the right side**

## Implementation approach

### JS: `split-pane.js` (shared component)

A single reusable JS module: `Goop.splitPane = { init }`

```js
Goop.splitPane.init(container)
```

Finds elements with class `split-layout` inside `container` (or document), inserts a drag handle between the two child panes, and wires mousedown/mousemove/mouseup for resizing.

**Markup pattern:**
```html
<div class="split-layout" data-split-key="db-tables" data-split-min="180" data-split-max="50%">
  <div class="split-left">...</div>
  <!-- JS inserts: <div class="split-handle"></div> -->
  <div class="split-right">...</div>
</div>
```

**Attributes:**
- `data-split-key` — localStorage key for persisting width (per-page)
- `data-split-min` — minimum left pane width in px (default: 150)
- `data-split-max` — maximum left pane width in px or % (default: 50%)

**Behavior:**
- Drag handle between panes, 4-6px wide, `cursor: col-resize`
- On mousedown: start tracking, add `user-select: none` to body
- On mousemove: update left pane width via CSS custom property or inline style
- On mouseup: stop tracking, save width to localStorage
- On page load: restore saved width from localStorage
- Double-click handle: reset to default width

### CSS: in `components.css`

```css
.split-layout {
  display: flex;
  flex: 1 1 0;
  min-height: 0;
  gap: 0;
}
.split-left {
  flex: 0 0 auto;
  min-width: 0;
  overflow: hidden;
}
.split-right {
  flex: 1 1 0;
  min-width: 0;
  overflow: hidden;
}
.split-handle {
  flex: 0 0 5px;
  cursor: col-resize;
  background: transparent;
  position: relative;
}
.split-handle:hover,
.split-handle.dragging {
  background: color-mix(in srgb, var(--accent) 30%, transparent);
}
```

### Migration per page

Each page currently uses its own layout classes (`.db-page`, `.ed-layout`, `.settings-page`, etc.) with hardcoded grid or flex widths. Migration:

1. Add `split-layout` class to the existing container
2. Add `split-left` / `split-right` to the two child panes
3. Remove hardcoded widths from CSS (let split-pane JS control them)
4. Add `data-split-key` for persistence

### Cluster page wrapper

The cluster page's right side currently has multiple direct children (stats, submit job, job list) without a wrapping pane. Needs a `<div class="split-right scroll-pane">` wrapper around them.

## Files to create/modify

| File | Change |
|------|--------|
| `js/split-pane.js` | New: drag handle logic, persistence |
| `css/components.css` | Add `.split-layout`, `.split-handle` styles |
| `app.js` | Add to shared JS load order |
| `templates/self.html` | Add split-layout to settings |
| `templates/editor.html` | Add split-layout to editor |
| `templates/lua.html` | Add split-layout to lua |
| `templates/database.html` | Add split-layout to all 3 tabs |
| `templates/groups_hosted.html` | Add split-layout |
| `templates/groups_files.html` | Add split-layout |
| `templates/groups_cluster.html` | Add split-layout + right wrapper |
| Page CSS files | Remove hardcoded sidebar widths |
