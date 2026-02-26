// Logs page: color-coded tail from Go process + browser log bridge.
(() => {
  const box = document.getElementById("logbox");
  if (!box) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  const copyBtn = document.getElementById("copyLogsBtn");
  const MAX_LINES = 2000;
  var lineCount = 0;

  // ── Tab filtering ──────────────────────────────────────────────────────────
  var swaggerPanel = document.getElementById('swagger-panel');
  var swaggerReady = false;

  function initSwaggerUI() {
    if (swaggerReady) return;
    swaggerReady = true;
    // Load Swagger UI from CDN — pinned to a stable release
    var cssLink = document.createElement('link');
    cssLink.rel = 'stylesheet';
    cssLink.href = 'https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css';
    document.head.appendChild(cssLink);

    var script = document.createElement('script');
    script.src = 'https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui-bundle.js';
    script.onload = function() {
      SwaggerUIBundle({
        url:           '/api/openapi.json',
        dom_id:        '#swagger-panel',
        presets:       [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
        layout:        'BaseLayout',
        deepLinking:   true,
        tryItOutEnabled: true,
      });
    };
    script.onerror = function() {
      swaggerPanel.innerHTML = '<p style="padding:16px;color:#c00">Swagger UI could not be loaded (no internet access?). The raw spec is at <a href="/api/openapi.json">/api/openapi.json</a></p>';
    };
    document.body.appendChild(script);
  }

  document.querySelectorAll('.log-tab').forEach(function(btn) {
    btn.addEventListener('click', function() {
      document.querySelectorAll('.log-tab').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      var tab = btn.dataset.tab;
      if (tab === 'api') {
        box.style.display = 'none';
        if (copyBtn) copyBtn.style.display = 'none';
        swaggerPanel.style.display = '';
        initSwaggerUI();
      } else {
        box.style.display = '';
        if (copyBtn) copyBtn.style.display = '';
        swaggerPanel.style.display = 'none';
        if (tab === 'all') {
          box.removeAttribute('data-tab');
        } else {
          box.setAttribute('data-tab', tab);
        }
        box.scrollTop = box.scrollHeight;
      }
    });
  });

  // ── Color classification ───────────────────────────────────────────────────
  // Returns a CSS class; every line gets one (log-default for unclassified).
  function classifyLine(s) {
    if (/\bCALL\b/.test(s) ||
        /\[call-native\]|\[call-ui\]|\[webrtc\]/.test(s)) return 'log-call';
    if (/\brelay\b/.test(s)) return 'log-relay';
    if (/\bGROUP\b|\bLISTEN\b/.test(s)) return 'log-group';
    if (/\bprobe\b/.test(s)) return 'log-probe';
    if (/\[update\]|\[online\]/.test(s)) return 'log-peer';
    if (/\[data\]/.test(s)) return 'log-data';
    if (/UNREACHABLE|error|ERROR/.test(s)) return 'log-error';
    if (/\bREACHABLE\b/.test(s)) return 'log-ok';
    return 'log-default';
  }

  // ── Make one line span ─────────────────────────────────────────────────────
  function makeLine(s) {
    const span = document.createElement('span');
    span.className = classifyLine(s);
    span.textContent = s + '\n';
    return span;
  }

  // ── Append one line ────────────────────────────────────────────────────────
  function appendLine(s) {
    if (!s) return;
    box.appendChild(makeLine(s));
    lineCount++;
    if (lineCount > MAX_LINES) {
      const trim = lineCount - MAX_LINES;
      for (let i = 0; i < trim; i++) {
        if (box.firstChild) box.removeChild(box.firstChild);
      }
      lineCount = MAX_LINES;
    }
    box.scrollTop = box.scrollHeight;
  }

  // ── Bulk-load snapshot ─────────────────────────────────────────────────────
  function loadSnapshot(items) {
    box.innerHTML = '';
    lineCount = 0;
    const lines = items.map(it => it.msg).filter(Boolean).slice(-MAX_LINES);
    const frag = document.createDocumentFragment();
    lines.forEach(s => { frag.appendChild(makeLine(s)); lineCount++; });
    box.appendChild(frag);
    box.scrollTop = box.scrollHeight;
  }

  // ── Copy button — copies only visible lines ────────────────────────────────
  if (copyBtn) {
    copyBtn.addEventListener("click", () => {
      var lines = [];
      box.querySelectorAll('span').forEach(function(sp) {
        if (getComputedStyle(sp).display !== 'none') lines.push(sp.textContent);
      });
      var text = lines.join('');
      navigator.clipboard.writeText(text).then(() => {
        const orig = copyBtn.textContent;
        copyBtn.textContent = "Copied!";
        setTimeout(() => { copyBtn.textContent = orig; }, 2000);
      }).catch(() => {
        const ta = document.createElement("textarea");
        ta.value = text;
        ta.style.cssText = "position:fixed;opacity:0";
        document.body.appendChild(ta);
        ta.select();
        try {
          document.execCommand("copy");
          const orig = copyBtn.textContent;
          copyBtn.textContent = "Copied!";
          setTimeout(() => { copyBtn.textContent = orig; }, 2000);
        } catch(e) {
          alert("Failed to copy: " + e.message);
        }
        document.body.removeChild(ta);
      });
    });
  }

  // ── MQ structured event log ────────────────────────────────────────────────
  // Receives log:mq events from PublishLocal and renders them as log lines.
  // Group-topic lines get class log-group so the Group tab filter works.
  function appendMQLogLine(payload) {
    if (!payload) return;
    var dir   = payload.dir === 'send' ? '→' : (payload.dir === 'recv' ? '←' : '✗');
    var rawPeer = payload.peer || '';
    // Use cached peer name when available, fall back to 8-char ID prefix.
    var peerName = window.Goop && Goop.mq && Goop.mq.getPeerName(rawPeer);
    var peer = peerName ? peerName.slice(0, 14) : rawPeer.slice(0, 8);
    var topic = payload.topic || '';
    var ts    = payload.ts ? new Date(payload.ts).toLocaleTimeString() : '';
    var via   = payload.via && payload.via !== 'direct' ? ' ~' + payload.via : '';
    var s     = ts + ' ' + dir + ' ' + peer + '  ' + topic + via;
    if (payload.error) s += '  [' + payload.error + ']';

    var span = document.createElement('span');
    if (topic.startsWith('group:') || topic === 'group.invite') {
      span.className = 'log-group';
    } else if (topic.startsWith('peer:')) {
      span.className = 'log-peer';
    } else {
      span.className = 'log-default';
    }
    span.textContent = s + '\n';
    box.appendChild(span);
    lineCount++;
    if (lineCount > MAX_LINES) {
      var trim = lineCount - MAX_LINES;
      for (var i = 0; i < trim; i++) {
        if (box.firstChild) box.removeChild(box.firstChild);
      }
      lineCount = MAX_LINES;
    }
    box.scrollTop = box.scrollHeight;
  }

  function initMQLog() {
    if (!window.Goop || !window.Goop.mq) { setTimeout(initMQLog, 50); return; }
    Goop.mq.onLogMQ( function(from, topic, payload, ack) {
      appendMQLogLine(payload);
      ack();
    });
  }
  initMQLog();

  // ── Call hardware log ───────────────────────────────────────────────────────
  // Receives log:call events from Go's call layer (hardware capture errors etc.)
  // and renders them as Video-tab entries (log-call class).
  function appendCallLogLine(payload) {
    if (!payload) return;
    var level  = payload.level || 'info';
    var source = payload.source || 'call';
    var msg    = payload.msg || '';
    var ts     = payload.ts ? new Date(payload.ts).toLocaleTimeString() : '';
    var s      = ts + ' [' + level + '] [' + source + '] ' + msg;
    var span   = document.createElement('span');
    span.className = 'log-call';
    span.textContent = s + '\n';
    box.appendChild(span);
    lineCount++;
    if (lineCount > MAX_LINES) {
      if (box.firstChild) box.removeChild(box.firstChild);
      lineCount = MAX_LINES;
    }
    box.scrollTop = box.scrollHeight;
  }

  function initCallLog() {
    if (!window.Goop || !window.Goop.mq) { setTimeout(initCallLog, 50); return; }
    Goop.mq.onLogCall(function(from, topic, payload, ack) {
      appendCallLogLine(payload);
      ack();
    });
  }
  initCallLog();

  // ── Load snapshot then stream ──────────────────────────────────────────────
  api("/api/logs")
    .then(items => {
      if (items.length > 0) loadSnapshot(items);
      const es = new EventSource("/api/logs/stream");
      es.addEventListener("message", (ev) => {
        try {
          const it = JSON.parse(ev.data);
          appendLine(it.msg);
        } catch {
          appendLine(ev.data);
        }
      });
      es.onerror = () => {};
    })
    .catch(err => appendLine("logs error: " + err));
})();
