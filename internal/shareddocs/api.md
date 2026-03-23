# API Reference

Interactive documentation for the Goop2 peer API. Every endpoint exposed by the local viewer is listed below with request/response schemas.

<div id="swagger-ui"></div>

<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css" />
<script>
(function() {
  var el = document.getElementById('swagger-ui');
  if (!el) return;
  var s = document.createElement('script');
  s.src = 'https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui-bundle.js';
  s.onload = function() {
    SwaggerUIBundle({
      url: '/api/openapi.json',
      dom_id: '#swagger-ui',
      presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
      layout: 'BaseLayout',
      deepLinking: true,
      defaultModelsExpandDepth: 0,
      docExpansion: 'list',
      filter: true
    });
  };
  s.onerror = function() {
    el.innerHTML = '<p style="padding:16px;color:var(--text-secondary)">Swagger UI could not be loaded (no internet access?). The raw spec is available at <a href="/api/openapi.json">/api/openapi.json</a>.</p>';
  };
  document.head.appendChild(s);
})();
</script>
