(async function () {
  var h = Goop.dom;
  var db = Goop.data;
  var todo = db.api("todo");
  var listEl = document.getElementById("todo-list");
  var statsEl = document.getElementById("stats");
  var subtitle = document.getElementById("subtitle");

  var toast = Goop.ui.toast(document.getElementById("toasts"), {
    toastClass: "gc-toast",
    titleClass: "gc-toast-title",
    messageClass: "gc-toast-message",
    enterClass: "gc-toast-enter",
    exitClass: "gc-toast-exit",
  });

  Goop.ui.dialog(document.getElementById("confirm-dialog"), {
    title: ".gc-dialog-title",
    message: ".gc-dialog-message",
    ok: ".gc-dialog-ok",
    cancel: ".gc-dialog-cancel",
    hiddenClass: "hidden",
  });

  var ctx = await Goop.peer();
  var isOwner = ctx.isOwner;
  var r = isOwner ? { role: "owner", permissions: { read: true, insert: true, update: true, delete: true } } : await db.role("todos");
  var canRead = isOwner || (r.permissions && r.permissions.read);
  var canWrite = isOwner || (r.permissions && r.permissions.insert);
  var todos = [];
  var sortable = null;

  if (canRead) {
    subtitle.textContent = canWrite ? "Shared task list" : "View only";
    document.getElementById("todo-main").classList.remove("hidden");
    load();
  } else {
    subtitle.textContent = "Members only";
    document.getElementById("todo-gate").classList.remove("hidden");
  }

  async function load() {
    try {
      var result = await todo("list");
      todos = result.todos || [];
      render();
    } catch (e) {
      Goop.render(listEl, Goop.ui.empty("Failed to load: " + (e.message || "unknown"), { class: "loading" }));
    }
  }

  async function render() {
    var total = todos.length;
    var done = 0;
    for (var i = 0; i < todos.length; i++) { if (todos[i].done) done++; }
    statsEl.textContent = total === 0 ? "" : done + " of " + total + " done";

    if (total === 0) {
      Goop.render(listEl, h("p", { class: "empty-msg" },
        canWrite ? "No tasks yet. Add one above!" : "No tasks yet."
      ));
      destroyDrag();
      return;
    }

    var container = h("div", { class: "todo-items" });
    for (var i = 0; i < todos.length; i++) {
      var item = await Goop.partial("todo-item", todos[i]);
      var t = todos[i];

      if (canWrite) {
        (function(item, t) {
          item.querySelector(".todo-check").addEventListener("click", function (e) {
            e.stopPropagation();
            toggleTodo(t._id);
          });
        })(item, t);
      }

      if (isOwner) {
        (function(item, t) {
          var del = h("button", { class: "todo-delete", title: "Delete", onclick: function (e) {
            e.stopPropagation();
            deleteTodo(t._id);
          } }, "\u00D7");
          item.appendChild(del);
        })(item, t);
      }

      container.appendChild(item);
    }

    Goop.render(listEl, container);
    initDrag();
  }

  async function toggleTodo(id) {
    try {
      await todo("toggle", { id: id });
      load();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to toggle" });
    }
  }

  async function deleteTodo(id) {
    if (!(await Goop.ui.confirm("Delete this task?"))) return;
    try {
      await todo("delete", { id: id });
      load();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to delete" });
    }
  }

  function initDrag() {
    destroyDrag();
    if (!isOwner || !Goop.drag) return;
    var container = listEl.querySelector(".todo-items");
    if (!container) return;
    sortable = Goop.drag.sortable(container, {
      items: ".todo-item",
      direction: "vertical",
      onEnd: function () {
        var items = container.querySelectorAll(".todo-item");
        var ids = [];
        items.forEach(function (el) { ids.push(parseInt(el.getAttribute("data-id"))); });
        todo("reorder", { ids: ids });
      }
    });
  }

  function destroyDrag() {
    if (sortable) { sortable.destroy(); sortable = null; }
  }

  var input = document.getElementById("new-todo");
  if (!canWrite) {
    document.getElementById("input-row").classList.add("hidden");
  }
  document.getElementById("btn-add").addEventListener("click", addTodo);
  input.addEventListener("keydown", function (e) { if (e.key === "Enter") addTodo(); });

  async function addTodo() {
    var text = input.value.trim();
    if (!text) { input.focus(); return; }
    try {
      await todo("add", { text: text, peer_name: ctx.label || ctx.myId });
      input.value = "";
      input.focus();
      load();
    } catch (e) {
      toast({ title: "Error", message: e.message || "Failed to add" });
    }
  }
})();
