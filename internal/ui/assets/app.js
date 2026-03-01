(() => {
  // Viewer UI utilities — no SDK dependency.
  const sharedFiles = [
    "/assets/js/core.js",
    "/assets/js/api.js",
    "/assets/js/mq/base.js",
    "/assets/js/mq/topics.js",
    "/assets/js/mq/peers.js",
    "/assets/js/select.js",
    "/assets/js/layout.js",
    "/assets/js/notify.js",
    "/assets/js/theme.js",
    "/assets/js/banners.js",
    "/assets/js/codemirror.js",
    "/assets/js/emoji.js",
    "/assets/js/log.js",
    "/assets/js/listen.js",
    "/assets/js/settings-popup.js",
    "/assets/js/dialogs.js",
    "/assets/js/toast.js",
    "/assets/js/groups.js",
  ];

  // Viewer-only call layer — single unified file handles both browser and native modes.
  const callFiles = [
    "/assets/js/call.js",
    "/assets/js/call-ui.js",
  ];

  const pageFiles = [
    "/assets/js/pages/peers.js",
    "/assets/js/pages/rendezvous.js",
    "/assets/js/pages/editor.js",
    "/assets/js/pages/database.js",
    "/assets/js/pages/logs.js",
    "/assets/js/pages/topology.js",
    "/assets/js/pages/lua.js",
    "/assets/js/pages/view.js",
    "/assets/js/pages/peer.js",
    "/assets/js/pages/groups.js",
    "/assets/js/pages/create_groups.js",
    "/assets/js/pages/documents.js",
    "/assets/js/pages/templates.js",
    "/assets/js/pages/self.js",
  ];

  const files = [...sharedFiles, ...callFiles, ...pageFiles];

  function loadSequentially(list, i = 0) {
    if (i >= list.length) return;

    const s = document.createElement("script");
    s.src = list[i];
    s.defer = true;
    s.onload = () => loadSequentially(list, i + 1);
    s.onerror = () => {
      console.error("Failed to load", s.src);
      // Continue loading the rest so the app isn't dead if one file is missing.
      loadSequentially(list, i + 1);
    };
    document.head.appendChild(s);
  }

  loadSequentially(files);
})();
