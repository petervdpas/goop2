(() => {
  var swaggerPanel = document.getElementById('swagger-panel');
  var redocPanel = document.getElementById('redoc-panel');
  if (!swaggerPanel) return;

  var swaggerReady = false;
  var redocReady = false;

  function initSwaggerUI() {
    if (swaggerReady) return;
    swaggerReady = true;
    var cssLink = document.createElement('link');
    cssLink.rel = 'stylesheet';
    cssLink.href = 'https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css';
    document.head.appendChild(cssLink);

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
      swaggerPanel.innerHTML = '<p style="padding:16px;color:#c00">Swagger UI could not be loaded (no internet access?). The raw spec is at <a href="/api/openapi.json">/api/openapi.json</a></p>';
    };
    document.body.appendChild(script);
  }

  function initRedoc() {
    if (redocReady) return;
    redocReady = true;
    var script = document.createElement('script');
    script.src = 'https://cdn.redoc.ly/redoc/v2.1.5/bundles/redoc.standalone.js';
    script.onload = function() {
      Redoc.init('/api/executor-api.yaml', {
        scrollYOffset: 0,
        hideDownloadButton: false,
        expandResponses: '200',
        theme: {
          colors: { primary: { main: '#7c8aff' } },
          typography: { fontSize: '14px', fontFamily: '-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif' },
          rightPanel: { backgroundColor: '#1a1a2e' },
          sidebar: { backgroundColor: '#fafafa' }
        }
      }, redocPanel);
    };
    script.onerror = function() {
      redocPanel.innerHTML = '<p style="padding:16px;color:#c00">Redoc could not be loaded (no internet access?). The raw spec is at <a href="/api/executor-api.yaml">/api/executor-api.yaml</a></p>';
    };
    document.body.appendChild(script);
  }

  document.querySelectorAll('[data-tab]').forEach(function(btn) {
    btn.addEventListener('click', function() {
      document.querySelectorAll('[data-tab]').forEach(function(b) { b.classList.remove('active'); });
      btn.classList.add('active');
      var tab = btn.dataset.tab;
      swaggerPanel.style.display = tab === 'api' ? '' : 'none';
      redocPanel.style.display = tab === 'executor' ? '' : 'none';
      if (tab === 'api') initSwaggerUI();
      if (tab === 'executor') initRedoc();
    });
  });

  initSwaggerUI();
})();
