(() => {
  var swaggerPanel = document.getElementById('swagger-panel');
  var redocPanel = document.getElementById('redoc-panel');
  if (!swaggerPanel) return;

  var swaggerReady = false;
  var redocReady = false;

  function isDark() {
    return document.documentElement.dataset.theme !== 'light';
  }

  function initSwaggerUI() {
    if (swaggerReady) return;
    swaggerReady = true;
    var cssLink = document.createElement('link');
    cssLink.rel = 'stylesheet';
    cssLink.href = 'https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css';
    document.head.appendChild(cssLink);

    var dark = isDark();
    if (dark) {
      var style = document.createElement('style');
      style.textContent = [
        '#swagger-panel .swagger-ui { color: var(--text); }',
        '#swagger-panel .swagger-ui .info .title, #swagger-panel .swagger-ui .opblock-tag { color: var(--text); }',
        '#swagger-panel .swagger-ui .info p, #swagger-panel .swagger-ui .info li { color: var(--muted); }',
        '#swagger-panel .swagger-ui .scheme-container { background: var(--panel); border-color: var(--line); }',
        '#swagger-panel .swagger-ui .opblock-tag { border-color: var(--line); }',
        '#swagger-panel .swagger-ui .opblock { border-color: var(--line); background: var(--panel); }',
        '#swagger-panel .swagger-ui .opblock .opblock-summary { border-color: var(--line); }',
        '#swagger-panel .swagger-ui .opblock .opblock-summary-description { color: var(--muted); }',
        '#swagger-panel .swagger-ui .opblock .opblock-section-header { background: var(--chip); }',
        '#swagger-panel .swagger-ui .opblock .opblock-section-header h4 { color: var(--text); }',
        '#swagger-panel .swagger-ui table thead tr td, #swagger-panel .swagger-ui table thead tr th { color: var(--muted); border-color: var(--line); }',
        '#swagger-panel .swagger-ui .parameter__name, #swagger-panel .swagger-ui .parameter__type { color: var(--text); }',
        '#swagger-panel .swagger-ui .response-col_status { color: var(--text); }',
        '#swagger-panel .swagger-ui .response-col_description { color: var(--muted); }',
        '#swagger-panel .swagger-ui .model-title { color: var(--text); }',
        '#swagger-panel .swagger-ui .model { color: var(--text); }',
        '#swagger-panel .swagger-ui input[type=text] { background: var(--field-bg); border-color: var(--line); color: var(--text); }',
        '#swagger-panel .swagger-ui select { background: var(--field-bg); border-color: var(--line); color: var(--text); }',
        '#swagger-panel .swagger-ui .btn { border-color: var(--line); color: var(--text); }',
        '#swagger-panel .swagger-ui .filter input[type=text] { background: var(--field-bg); border-color: var(--line); color: var(--text); }',
        '#swagger-panel .swagger-ui .loading-container .loading::after { color: var(--muted); }',
      ].join('\n');
      document.head.appendChild(style);
    }

    var script = document.createElement('script');
    script.src = 'https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui-bundle.js';
    script.onload = function() {
      SwaggerUIBundle({
        url: '/api/openapi.json',
        dom_id: '#swagger-panel',
        presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
        layout: 'BaseLayout',
        deepLinking: true,
        defaultModelsExpandDepth: 0,
        docExpansion: 'list',
        filter: true,
        tryItOutEnabled: true
      });
    };
    script.onerror = function() {
      swaggerPanel.innerHTML = '<p style="padding:16px;color:var(--muted)">Swagger UI could not be loaded (no internet access?). The raw spec is at <a href="/api/openapi.json">/api/openapi.json</a></p>';
    };
    document.body.appendChild(script);
  }

  function initRedoc() {
    if (redocReady) return;
    redocReady = true;
    var dark = isDark();
    var script = document.createElement('script');
    script.src = 'https://cdn.redoc.ly/redoc/v2.1.5/bundles/redoc.standalone.js';
    script.onload = function() {
      Redoc.init('/api/executor-api.yaml', {
        scrollYOffset: 0,
        hideDownloadButton: false,
        expandResponses: '200',
        theme: {
          colors: {
            primary: { main: dark ? '#7aa2ff' : '#5a3dff' },
            text: { primary: dark ? '#e6e9ef' : '#101325' },
            http: { post: dark ? '#49e97c' : '#1a7f37' }
          },
          typography: { fontSize: '14px', fontFamily: 'inherit' },
          rightPanel: { backgroundColor: dark ? '#0f1115' : '#1a1a2e', textColor: '#e6edf3' },
          sidebar: {
            backgroundColor: 'transparent',
            textColor: dark ? '#9aa3b2' : '#4a4f6b',
            activeTextColor: dark ? '#7aa2ff' : '#5a3dff'
          },
          schema: { typeNameColor: dark ? '#7aa2ff' : '#5a3dff' }
        }
      }, redocPanel);
    };
    script.onerror = function() {
      redocPanel.innerHTML = '<p style="padding:16px;color:var(--muted)">Redoc could not be loaded (no internet access?). The raw spec is at <a href="/api/executor-api.yaml">/api/executor-api.yaml</a></p>';
    };
    document.body.appendChild(script);
  }

  var panels = {
    sdk: document.getElementById('sdk-panel'),
    lua: document.getElementById('lua-panel'),
    api: swaggerPanel,
    executor: redocPanel
  };
  var inits = { api: initSwaggerUI, executor: initRedoc };

  document.querySelectorAll('[data-tab]').forEach(function(btn) {
    btn.addEventListener('click', function() {
      document.querySelectorAll('[data-tab]').forEach(function(b) { b.classList.remove('active'); });
      btn.classList.add('active');
      var tab = btn.dataset.tab;
      Object.keys(panels).forEach(function(k) {
        if (panels[k]) panels[k].style.display = k === tab ? '' : 'none';
      });
      if (inits[tab]) inits[tab]();
    });
  });
})();
