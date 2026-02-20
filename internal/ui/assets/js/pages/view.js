// View page: tabbed embedded peer site viewer.
(function() {
  var viewPage = document.querySelector('.view-page');
  if (!viewPage) return;

  // If external mode is enabled, redirect away from view page
  if (window._openSitesExternal) {
    window.location.href = '/peers';
    return;
  }

  var tabsBar = document.getElementById('view-tabs');
  var frameContainer = document.getElementById('view-frame-container');
  var emptyMsg = document.getElementById('view-empty');

  // Tab state stored in sessionStorage so tabs survive page navigation
  var STORAGE_KEY = 'goop2_view_tabs';
  var ACTIVE_KEY = 'goop2_view_active';

  function loadTabs() {
    try {
      var raw = sessionStorage.getItem(STORAGE_KEY);
      return raw ? JSON.parse(raw) : [];
    } catch(e) { return []; }
  }

  function saveTabs(tabs) {
    sessionStorage.setItem(STORAGE_KEY, JSON.stringify(tabs));
  }

  function getActive() {
    return sessionStorage.getItem(ACTIVE_KEY) || '';
  }

  function setActive(id) {
    sessionStorage.setItem(ACTIVE_KEY, id);
  }

  // Check URL params for a new tab to open
  var params = new URLSearchParams(window.location.search);
  var openId = params.get('open');
  var openName = params.get('name');

  if (openId) {
    var tabs = loadTabs();
    var exists = tabs.some(function(t) { return t.id === openId; });
    if (!exists) {
      tabs.push({ id: openId, name: openName || openId.substring(0, 8) + '...' });
      saveTabs(tabs);
    }
    setActive(openId);
    // Clean URL so refreshing doesn't re-add
    history.replaceState(null, '', '/view');
  }

  function removeTab(peerID) {
    var tabs = loadTabs();
    tabs = tabs.filter(function(t) { return t.id !== peerID; });
    saveTabs(tabs);

    if (tabs.length === 0) {
      setActive('');
      window.location.href = '/peers';
      return;
    }

    if (getActive() === peerID) {
      setActive(tabs[0].id);
    }
    renderTabs();
  }

  function renderTabs() {
    var tabs = loadTabs();
    var active = getActive();

    if (tabs.length === 0) {
      tabsBar.innerHTML = '';
      frameContainer.innerHTML = '';
      if (emptyMsg) {
        frameContainer.appendChild(emptyMsg);
        emptyMsg.style.display = '';
      }
      updateNavView(false);
      return;
    }

    // If active tab no longer exists, activate first
    if (!tabs.some(function(t) { return t.id === active; })) {
      active = tabs[0].id;
      setActive(active);
    }

    // Render tab bar
    tabsBar.innerHTML = tabs.map(function(tab) {
      var cls = tab.id === active ? 'view-tab active' : 'view-tab';
      return '<div class="' + cls + '" data-tab-id="' + escapeAttr(tab.id) + '">' +
        '<span class="view-tab-label">' + escapeHtml(tab.name) + '</span>' +
        '<button class="view-tab-close" data-close-id="' + escapeAttr(tab.id) + '" title="Close tab">&times;</button>' +
      '</div>';
    }).join('') +
    '<span class="view-tabs-hint muted small">right-click tab for options</span>';

    // Render iframe for active tab
    if (emptyMsg) emptyMsg.style.display = 'none';

    var existingFrame = frameContainer.querySelector('.view-frame');
    var currentSrc = existingFrame ? existingFrame.getAttribute('data-peer-id') : '';

    if (currentSrc !== active) {
      // Remove old iframe
      if (existingFrame) existingFrame.remove();

      var iframe = document.createElement('iframe');
      iframe.className = 'view-frame';
      iframe.setAttribute('data-peer-id', active);
      iframe.src = '/p/' + active + '/';
      iframe.setAttribute('sandbox', 'allow-scripts allow-same-origin allow-forms allow-popups');
      frameContainer.appendChild(iframe);
    }

    updateNavView(true);

    // Attach tab click handlers
    tabsBar.querySelectorAll('.view-tab').forEach(function(el) {
      el.addEventListener('click', function(e) {
        if (e.target.closest('.view-tab-close')) return;
        var id = el.getAttribute('data-tab-id');
        setActive(id);
        renderTabs();
      });
    });

    // Attach close handlers
    tabsBar.querySelectorAll('.view-tab-close').forEach(function(btn) {
      btn.addEventListener('click', function(e) {
        e.stopPropagation();
        removeTab(btn.getAttribute('data-close-id'));
      });
    });

    // Attach context menu on right-click
    tabsBar.querySelectorAll('.view-tab').forEach(function(el) {
      el.addEventListener('contextmenu', function(e) {
        e.preventDefault();
        var id = el.getAttribute('data-tab-id');
        showCtxMenu(e.clientX, e.clientY, id);
      });
    });
  }

  // ── Tab context menu ──
  var ctxMenu = document.createElement('div');
  ctxMenu.className = 'view-ctx hidden';
  ctxMenu.innerHTML =
    '<button data-action="refresh">Refresh</button>' +
    '<button data-action="open-external">Open in Browser</button>' +
    '<hr>' +
    '<button data-action="close-others">Close Other Tabs</button>' +
    '<button data-action="close">Close</button>';
  document.body.appendChild(ctxMenu);

  var ctxTarget = null;

  function showCtxMenu(x, y, tabId) {
    ctxTarget = tabId;

    // Show/hide "Close Other Tabs" depending on tab count
    var others = ctxMenu.querySelector('[data-action="close-others"]');
    if (others) others.style.display = loadTabs().length > 1 ? 'block' : 'none';

    ctxMenu.style.left = x + 'px';
    ctxMenu.style.top = y + 'px';
    ctxMenu.classList.remove('hidden');

    // Keep menu within viewport
    requestAnimationFrame(function() {
      var rect = ctxMenu.getBoundingClientRect();
      if (rect.right > window.innerWidth) ctxMenu.style.left = (x - rect.width) + 'px';
      if (rect.bottom > window.innerHeight) ctxMenu.style.top = (y - rect.height) + 'px';
    });
  }

  function hideCtxMenu() {
    ctxMenu.classList.add('hidden');
    ctxTarget = null;
  }

  document.addEventListener('click', hideCtxMenu);
  document.addEventListener('keydown', function(e) {
    if (e.key === 'Escape') hideCtxMenu();
  });

  ctxMenu.addEventListener('click', function(e) {
    var btn = e.target.closest('button[data-action]');
    if (!btn || !ctxTarget) return;

    e.preventDefault();
    var id = ctxTarget;
    hideCtxMenu();

    switch (btn.dataset.action) {
      case 'refresh':
        // Activate the tab if not active, then reload its iframe
        if (getActive() !== id) {
          setActive(id);
          renderTabs();
        }
        var frame = frameContainer.querySelector('.view-frame');
        if (frame) frame.src = frame.src;
        break;

      case 'open-external':
        var fullUrl = window.location.origin + '/p/' + id + '/';
        openExternal(fullUrl);
        break;

      case 'close-others':
        var tabs = loadTabs();
        var keep = tabs.filter(function(t) { return t.id === id; });
        saveTabs(keep);
        setActive(id);
        renderTabs();
        break;

      case 'close':
        removeTab(id);
        break;
    }
  });

  function updateNavView(show) {
    var navView = document.getElementById('nav-view');
    if (navView) {
      // In embedded mode, always keep the View nav visible
      navView.style.display = (show || !window._openSitesExternal) ? '' : 'none';
    }
  }

  function escapeHtml(s) {
    return window.Goop && window.Goop.core ? window.Goop.core.escapeHtml(s) : s.replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  function escapeAttr(s) {
    return s.replace(/&/g, '&amp;').replace(/"/g, '&quot;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }

  renderTabs();

  // Auto-close tabs when peers drop off the network
  var peersSSE = new EventSource('/api/peers/events');

  peersSSE.addEventListener('remove', function(e) {
    try {
      var data = JSON.parse(e.data);
      if (data.peer_id) {
        var tabs = loadTabs();
        if (tabs.some(function(t) { return t.id === data.peer_id; })) {
          removeTab(data.peer_id);
        }
      }
    } catch(err) {}
  });

  peersSSE.onerror = function() {
    console.error('View: peers SSE connection lost');
  };
})();
