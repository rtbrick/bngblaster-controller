// Starts Swagger UI for the API definition next to this page. Kept out of
// index.html so the page needs no inline script: the controller serves it
// with a Content-Security-Policy that allows only its own scripts.
window.onload = function () {
  window.ui = SwaggerUIBundle({
    url: 'swagger.yaml',
    dom_id: '#swagger-ui',
    deepLinking: true,
    // Never load a different definition named in the page URL
    // (?url=/?configUrl=): it would show attacker-supplied operations
    // under this origin.
    queryConfigEnabled: false,
    // No calls to the public validator service.
    validatorUrl: null,
    presets: [SwaggerUIBundle.presets.apis],
    plugins: [SwaggerUIBundle.plugins.DownloadUrl],
  });
};
