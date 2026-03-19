# Executor Protocol

The executor protocol defines the contract between Goop2's cluster dispatcher and external executor binaries. A worker peer starts the binary as a child process and communicates over stdin/stdout using newline-delimited JSON.

<div id="redoc-container"></div>

<script>
(function() {
  var el = document.getElementById('redoc-container');
  if (!el) return;
  var s = document.createElement('script');
  s.src = 'https://cdn.redoc.ly/redoc/v2.1.5/bundles/redoc.standalone.js';
  s.onload = function() {
    Redoc.init('/api/executor-api.yaml', {
      scrollYOffset: 0,
      hideDownloadButton: false,
      theme: {
        colors: { primary: { main: '#818cf8' } },
        typography: { fontFamily: 'inherit' },
        sidebar: { backgroundColor: 'transparent' },
        rightPanel: { backgroundColor: 'rgba(0,0,0,0.2)' }
      }
    }, el);
  };
  s.onerror = function() {
    el.innerHTML = '<p style="padding:16px;color:var(--text-secondary)">Redoc could not be loaded (no internet access?). The raw spec is available at <a href="/api/executor-api.yaml">/api/executor-api.yaml</a>.</p>';
  };
  document.head.appendChild(s);
})();
</script>

## Code examples

Example executor implementations are available in the source repository under `docs/examples/`:

- **Bash** -- Daemon mode executor in a single shell script
- **Python** -- Full-featured executor with job type routing
- **Go** -- Idiomatic Go executor with JSON marshaling
- **C** -- Minimal C executor using cJSON
- **C++** -- Modern C++ executor with nlohmann/json
