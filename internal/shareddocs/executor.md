# Executor Protocol

The executor protocol defines the contract between Goop2's cluster dispatcher and external executor binaries. A worker peer starts the binary as a child process and communicates over stdin/stdout using newline-delimited JSON.

<div id="redoc-container"></div>

<script>
(function() {
  var el = document.getElementById('redoc-container');
  if (!el) return;
  var s = document.createElement('script');
  s.src = '/assets/vendor/redoc/redoc.standalone.js';
  s.onload = function() {
    var isDark = document.documentElement.dataset.theme === 'dark';
    Redoc.init('/api/executor-api.yaml', {
      scrollYOffset: 0,
      hideDownloadButton: false,
      theme: {
        colors: {
          primary: { main: isDark ? '#818cf8' : '#0969da' },
          text: { primary: isDark ? '#e6edf3' : '#1f2328' },
          http: { post: isDark ? '#3fb950' : '#1a7f37' }
        },
        typography: {
          fontFamily: 'inherit',
          headings: { fontFamily: 'inherit' },
          code: { fontFamily: "'Courier New', Consolas, monospace" }
        },
        sidebar: { backgroundColor: 'transparent', textColor: isDark ? '#b0b8c1' : '#424a53' },
        rightPanel: { backgroundColor: isDark ? 'rgba(0,0,0,0.2)' : '#1a1a2e', textColor: '#e6edf3' },
        schema: { typeNameColor: isDark ? '#818cf8' : '#0969da' }
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
