//
// Reusable drag-and-drop for site pages.
// Works standalone — does not depend on the viewer's CSS.
//
// Usage:
//
//   <script src="/sdk/goop-drag.js"></script>
//
//   var sortable = Goop.drag.sortable(container, {
//     items: ".card",           // selector for draggable children
//     handle: null,             // optional handle selector
//     group: "cards",           // items move between containers sharing a group
//     direction: "vertical",    // "vertical" or "horizontal"
//     placeholder: true,        // show insertion placeholder
//     onStart: function(evt){}, // { item, container, index }
//     onMove: function(evt){},  // { item, from, to, oldIndex, newIndex }
//     onEnd: function(evt){},   // { item, from, to, oldIndex, newIndex }
//     onCancel: function(evt){} // drag aborted (Escape)
//   });
//
//   sortable.destroy();
//
(function () {
  "use strict";

  window.Goop = window.Goop || {};

  // ── inject minimal CSS (only once) ──
  var STYLE_ID = "goop-drag-style";
  if (!document.getElementById(STYLE_ID)) {
    var s = document.createElement("style");
    s.id = STYLE_ID;
    s.textContent =
      ".goop-drag-ghost{position:fixed;z-index:99999;pointer-events:none;opacity:0.9;box-shadow:0 8px 24px rgba(0,0,0,0.18);transform:rotate(1.5deg) scale(1.03);border-radius:8px;}" +
      ".goop-drag-source{opacity:0.35;}" +
      ".goop-drag-placeholder{border:2px dashed var(--accent,#6366f1);border-radius:6px;min-height:2rem;background:rgba(99,102,241,0.06);transition:height .15s;}" +
      ".goop-drag-active{user-select:none;}" +
      ".goop-drag-over{outline:2px solid var(--accent,#6366f1);outline-offset:-2px;border-radius:8px;}";
    document.head.appendChild(s);
  }

  var THRESHOLD = 5;
  var instances = [];

  function indexInParent(el, selector) {
    var siblings = el.parentNode.querySelectorAll(":scope > " + selector);
    for (var i = 0; i < siblings.length; i++) {
      if (siblings[i] === el) return i;
    }
    return -1;
  }

  function closestItem(el, selector) {
    while (el) {
      if (el.matches && el.matches(selector)) return el;
      el = el.parentElement;
    }
    return null;
  }

  function sortable(container, opts) {
    opts = opts || {};
    var itemSelector = opts.items || "> *";
    var handleSelector = opts.handle || null;
    var group = opts.group || null;
    var direction = opts.direction || "vertical";
    var showPlaceholder = opts.placeholder !== false;
    var onStart = opts.onStart || null;
    var onMove = opts.onMove || null;
    var onEnd = opts.onEnd || null;
    var onCancel = opts.onCancel || null;

    var dragging = false;
    var dragItem = null;
    var ghost = null;
    var placeholder = null;
    var startX = 0, startY = 0;
    var offsetX = 0, offsetY = 0;
    var originContainer = null;
    var originIndex = -1;
    var originNext = null;
    var currentContainer = null;
    var destroyed = false;

    function getContainers() {
      if (!group) return [container];
      var all = [];
      for (var i = 0; i < instances.length; i++) {
        if (instances[i].group === group) all.push(instances[i].container);
      }
      return all;
    }

    function getItems(cont) {
      return cont.querySelectorAll(":scope > " + itemSelector);
    }

    function onPointerDown(e) {
      if (destroyed) return;
      if (e.button !== 0) return;

      var target = e.target;
      // If handle specified, only start from handle
      if (handleSelector) {
        if (!target.closest(handleSelector)) return;
      }

      var item = closestItem(target, itemSelector);
      if (!item || item.parentNode !== container) return;

      // Don't drag if clicking interactive elements (skip check when handle is set)
      if (!handleSelector && target.closest("button, a, input, textarea, select, [contenteditable]")) return;

      startX = e.clientX;
      startY = e.clientY;
      dragItem = item;

      var rect = item.getBoundingClientRect();
      offsetX = e.clientX - rect.left;
      offsetY = e.clientY - rect.top;

      document.addEventListener("pointermove", onPointerMove);
      document.addEventListener("pointerup", onPointerUp);
      document.addEventListener("keydown", onKeyDown);
    }

    function startDrag(e) {
      dragging = true;
      originContainer = container;
      originIndex = indexInParent(dragItem, itemSelector);
      originNext = dragItem.nextSibling;
      currentContainer = container;

      // Create ghost
      ghost = dragItem.cloneNode(true);
      ghost.classList.add("goop-drag-ghost");
      var rect = dragItem.getBoundingClientRect();
      ghost.style.width = rect.width + "px";
      ghost.style.height = rect.height + "px";
      ghost.style.left = (e.clientX - offsetX) + "px";
      ghost.style.top = (e.clientY - offsetY) + "px";
      document.body.appendChild(ghost);

      // Create placeholder
      if (showPlaceholder) {
        placeholder = document.createElement("div");
        placeholder.classList.add("goop-drag-placeholder");
        if (direction === "horizontal") {
          placeholder.style.width = rect.width + "px";
          placeholder.style.height = rect.height + "px";
        } else {
          placeholder.style.height = rect.height + "px";
        }
        dragItem.parentNode.insertBefore(placeholder, dragItem);
      }

      // Dim original in place
      dragItem.classList.add("goop-drag-source");
      currentContainer.classList.add("goop-drag-over");

      // Prevent text selection / touch scroll
      document.body.classList.add("goop-drag-active");
      container.style.touchAction = "none";

      if (onStart) {
        onStart({ item: dragItem, container: container, index: originIndex });
      }
    }

    function onPointerMove(e) {
      if (!dragItem) return;

      if (!dragging) {
        var dx = e.clientX - startX;
        var dy = e.clientY - startY;
        if (Math.sqrt(dx * dx + dy * dy) < THRESHOLD) return;
        startDrag(e);
      }

      // Move ghost
      ghost.style.left = (e.clientX - offsetX) + "px";
      ghost.style.top = (e.clientY - offsetY) + "px";

      // Find target container and insertion point
      var containers = getContainers();
      var targetContainer = null;
      var insertBefore = null;
      var newIndex = -1;

      for (var c = 0; c < containers.length; c++) {
        var cont = containers[c];
        var cRect = cont.getBoundingClientRect();
        if (e.clientX >= cRect.left && e.clientX <= cRect.right &&
            e.clientY >= cRect.top && e.clientY <= cRect.bottom) {
          targetContainer = cont;
          break;
        }
      }

      if (!targetContainer) return;

      var items = getItems(targetContainer);
      var found = false;

      for (var i = 0; i < items.length; i++) {
        var it = items[i];
        if (it === dragItem || it === placeholder) continue;
        var iRect = it.getBoundingClientRect();
        var mid = direction === "horizontal"
          ? iRect.left + iRect.width / 2
          : iRect.top + iRect.height / 2;
        var pos = direction === "horizontal" ? e.clientX : e.clientY;

        if (pos < mid) {
          insertBefore = it;
          newIndex = i;
          found = true;
          break;
        }
      }

      if (!found) {
        insertBefore = null;
        newIndex = items.length;
      }

      // Adjust index: don't count placeholder or hidden dragItem
      var countBefore = 0;
      for (var j = 0; j < newIndex; j++) {
        if (items[j] === placeholder || items[j] === dragItem) countBefore++;
      }
      newIndex -= countBefore;

      // Move placeholder
      if (showPlaceholder && placeholder) {
        if (insertBefore) {
          targetContainer.insertBefore(placeholder, insertBefore);
        } else {
          targetContainer.appendChild(placeholder);
        }
      }

      // Update container highlight + touch-action
      if (currentContainer !== targetContainer) {
        currentContainer.classList.remove("goop-drag-over");
        currentContainer.style.touchAction = "";
        targetContainer.classList.add("goop-drag-over");
        targetContainer.style.touchAction = "none";
      }
      currentContainer = targetContainer;

      if (onMove) {
        onMove({
          item: dragItem,
          from: originContainer,
          to: targetContainer,
          oldIndex: originIndex,
          newIndex: newIndex
        });
      }
    }

    function finishDrag(cancelled) {
      document.removeEventListener("pointermove", onPointerMove);
      document.removeEventListener("pointerup", onPointerUp);
      document.removeEventListener("keydown", onKeyDown);

      if (!dragging) {
        dragItem = null;
        return;
      }

      var targetContainer = currentContainer;
      var newIndex = -1;

      if (cancelled) {
        // Return to original position
        if (originNext) {
          originContainer.insertBefore(dragItem, originNext);
        } else {
          originContainer.appendChild(dragItem);
        }
        dragItem.classList.remove("goop-drag-source");
      } else {
        // Insert at placeholder position
        if (placeholder && placeholder.parentNode) {
          placeholder.parentNode.insertBefore(dragItem, placeholder);
        }
        dragItem.classList.remove("goop-drag-source");

        // Calculate final index
        var finalItems = getItems(dragItem.parentNode);
        for (var i = 0; i < finalItems.length; i++) {
          if (finalItems[i] === dragItem) { newIndex = i; break; }
        }
      }

      // Cleanup ghost
      if (ghost && ghost.parentNode) ghost.parentNode.removeChild(ghost);
      ghost = null;

      // Cleanup placeholder
      if (placeholder && placeholder.parentNode) placeholder.parentNode.removeChild(placeholder);
      placeholder = null;

      // Restore body state
      document.body.classList.remove("goop-drag-active");
      var containers = getContainers();
      for (var c = 0; c < containers.length; c++) {
        containers[c].style.touchAction = "";
        containers[c].classList.remove("goop-drag-over");
      }

      if (cancelled) {
        if (onCancel) onCancel({ item: dragItem, container: originContainer, index: originIndex });
      } else {
        if (onEnd) {
          onEnd({
            item: dragItem,
            from: originContainer,
            to: targetContainer,
            oldIndex: originIndex,
            newIndex: newIndex
          });
        }
      }

      dragging = false;
      dragItem = null;
      originContainer = null;
      currentContainer = null;
    }

    function onPointerUp() {
      finishDrag(false);
    }

    function onKeyDown(e) {
      if (e.key === "Escape") {
        finishDrag(true);
      }
    }

    container.addEventListener("pointerdown", onPointerDown);

    var instance = {
      container: container,
      group: group,
      destroy: function () {
        destroyed = true;
        container.removeEventListener("pointerdown", onPointerDown);
        document.removeEventListener("pointermove", onPointerMove);
        document.removeEventListener("pointerup", onPointerUp);
        document.removeEventListener("keydown", onKeyDown);
        if (ghost && ghost.parentNode) ghost.parentNode.removeChild(ghost);
        if (placeholder && placeholder.parentNode) placeholder.parentNode.removeChild(placeholder);
        for (var i = instances.length - 1; i >= 0; i--) {
          if (instances[i] === instance) { instances.splice(i, 1); break; }
        }
      }
    };

    instances.push(instance);
    return instance;
  }

  Goop.drag = { sortable: sortable };
})();
