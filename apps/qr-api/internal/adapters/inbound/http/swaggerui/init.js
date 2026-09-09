// Boots Swagger UI against the embedded contract. Kept out of index.html so the
// /docs Content-Security-Policy can stay script-src 'self'.
window.ui = SwaggerUIBundle({
  url: '/openapi.yaml',
  dom_id: '#swagger-ui',
  deepLinking: true,
  presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
  plugins: [SwaggerUIBundle.plugins.DownloadUrl],
  layout: 'StandaloneLayout',
  tryItOutEnabled: true,
  persistAuthorization: false,
  defaultModelsExpandDepth: 1,
});
