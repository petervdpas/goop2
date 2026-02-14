// Logs page: load snapshot, SSE stream, copy button.
(() => {
  const box = document.getElementById("logbox");
  if (!box) return;

  var core = window.Goop && window.Goop.core || {};
  var api = core.api;

  const copyBtn = document.getElementById("copyLogsBtn");
  const maxChars = 200000;

  function appendLine(s) {
    if (!s) return;
    box.textContent += s + "\n";
    if (box.textContent.length > maxChars) {
      box.textContent = box.textContent.slice(box.textContent.length - maxChars);
    }
    box.scrollTop = box.scrollHeight;
  }

  // Copy button handler
  if (copyBtn) {
    copyBtn.addEventListener("click", () => {
      const text = box.textContent;
      navigator.clipboard.writeText(text).then(() => {
        const orig = copyBtn.textContent;
        copyBtn.textContent = "Copied!";
        setTimeout(() => { copyBtn.textContent = orig; }, 2000);
      }).catch(err => {
        const textarea = document.createElement("textarea");
        textarea.value = text;
        textarea.style.position = "fixed";
        textarea.style.opacity = "0";
        document.body.appendChild(textarea);
        textarea.select();
        try {
          document.execCommand("copy");
          const orig = copyBtn.textContent;
          copyBtn.textContent = "Copied!";
          setTimeout(() => { copyBtn.textContent = orig; }, 2000);
        } catch(e) {
          alert("Failed to copy: " + e.message);
        }
        document.body.removeChild(textarea);
      });
    });
  }

  // Load snapshot then stream.
  api("/api/logs")
    .then(items => {
      if (items.length > 0) {
        box.textContent = items.map(it => it.msg).filter(Boolean).join("\n") + "\n";
        if (box.textContent.length > maxChars) {
          box.textContent = box.textContent.slice(box.textContent.length - maxChars);
        }
        box.scrollTop = box.scrollHeight;
      }
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
