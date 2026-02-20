(() => {
  const sharedFiles = [
    "/assets/js/core.js",
    "/assets/js/select.js",
    "/assets/js/layout.js",
    "/assets/js/notify.js",
    "/assets/js/theme.js",
    "/assets/js/banners.js",
    "/assets/js/codemirror.js",
    "/assets/js/emoji.js",
    "/assets/js/log.js",
    "/assets/js/call-ui.js",
    "/assets/js/listen.js",
    "/assets/js/settings-popup.js",
    "/assets/js/dialogs.js",
    "/assets/js/toast.js",
    "/assets/js/groups.js",
  ];

  const sdkFiles = [
    "/sdk/goop-data.js",
    "/sdk/goop-peers.js",
    "/sdk/goop-chat.js",
    "/sdk/goop-identity.js",
    "/sdk/goop-ui.js",
    "/sdk/goop-forms.js",
    "/sdk/goop-site.js",
    "/sdk/goop-group.js",
    "/sdk/goop-realtime.js",
    "/sdk/goop-call.js",
  ];

  const pageFiles = [
    "/assets/js/pages/peers.js",
    "/assets/js/pages/rendezvous.js",
    "/assets/js/pages/editor.js",
    "/assets/js/pages/database.js",
    "/assets/js/pages/logs.js",
    "/assets/js/pages/lua.js",
    "/assets/js/pages/view.js",
    "/assets/js/pages/peer.js",
    "/assets/js/pages/groups.js",
    "/assets/js/pages/create_groups.js",
    "/assets/js/pages/documents.js",
    "/assets/js/pages/templates.js",
    "/assets/js/pages/self.js",
  ];

  const files = [...sharedFiles, ...sdkFiles, ...pageFiles];

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
