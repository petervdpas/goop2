// Logs page: color-coded tail from Go process + browser log bridge.
(() => {
  const box = document.getElementById("logbox");
  if (!box) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  const copyBtn = document.getElementById("copyLogsBtn");
  const MAX_LINES = 2000;
  var lineCount = 0;

  // ── Color classification ───────────────────────────────────────────────────
  // Each line gets one CSS class based on the subsystem it belongs to.
  // Priority: most-specific patterns first.
  function classifyLine(s) {
    // Call / video subsystem (Go + browser)
    if (/\bCALL\b/.test(s) ||
        /\[call-native\]|\[call-ui\]|\[webrtc\]/.test(s)) return 'log-call';
    // Relay
    if (/\brelay\b/.test(s)) return 'log-relay';
    // Group / Realtime managers
    if (/\bGROUP\b|\bREALTIME\b/.test(s)) return 'log-realtime';
    // Probe lines — dimmed (very frequent, low signal)
    if (/\bprobe\b/.test(s)) return 'log-probe';
    // Peer presence updates
    if (/\[update\]|\[online\]/.test(s)) return 'log-peer';
    // Errors / unreachable
    if (/UNREACHABLE|error|ERROR/.test(s)) return 'log-error';
    // Reachable / OK
    if (/\bREACHABLE\b/.test(s)) return 'log-ok';
    return '';
  }

  // ── Append one line ────────────────────────────────────────────────────────
  function appendLine(s) {
    if (!s) return;
    const cls = classifyLine(s);
    if (cls) {
      const span = document.createElement('span');
      span.className = cls;
      span.textContent = s + '\n';
      box.appendChild(span);
    } else {
      box.appendChild(document.createTextNode(s + '\n'));
    }

    lineCount++;
    // Trim oldest lines to stay within MAX_LINES
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
    const lines = items.map(it => it.msg).filter(Boolean);
    // Only keep the last MAX_LINES
    const tail = lines.slice(-MAX_LINES);
    const frag = document.createDocumentFragment();
    tail.forEach(s => {
      const cls = classifyLine(s);
      if (cls) {
        const span = document.createElement('span');
        span.className = cls;
        span.textContent = s + '\n';
        frag.appendChild(span);
      } else {
        frag.appendChild(document.createTextNode(s + '\n'));
      }
      lineCount++;
    });
    box.appendChild(frag);
    box.scrollTop = box.scrollHeight;
  }

  // ── Copy button ────────────────────────────────────────────────────────────
  if (copyBtn) {
    copyBtn.addEventListener("click", () => {
      const text = box.textContent;
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
