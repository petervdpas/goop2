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
  document.querySelectorAll('.log-tab').forEach(function(btn) {
    btn.addEventListener('click', function() {
      document.querySelectorAll('.log-tab').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      var tab = btn.dataset.tab;
      if (tab === 'all') {
        box.removeAttribute('data-tab');
      } else {
        box.setAttribute('data-tab', tab);
      }
      box.scrollTop = box.scrollHeight;
    });
  });

  // ── Color classification ───────────────────────────────────────────────────
  // Returns a CSS class; every line gets one (log-default for unclassified).
  function classifyLine(s) {
    if (/\bCALL\b/.test(s) ||
        /\[call-native\]|\[call-ui\]|\[webrtc\]/.test(s)) return 'log-call';
    if (/\brelay\b/.test(s)) return 'log-relay';
    if (/\bGROUP\b|\bREALTIME\b/.test(s)) return 'log-realtime';
    if (/\bprobe\b/.test(s)) return 'log-probe';
    if (/\[update\]|\[online\]/.test(s)) return 'log-peer';
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
