package openapi

import (
	"encoding/json"
	"net/http"
	"sync"
)

var (
	once     sync.Once
	specJSON []byte
)

// Handler returns an http.Handler that serves the OpenAPI spec as JSON on
// any request. The spec is built and marshalled once on the first call.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() {
			b, err := json.MarshalIndent(NewSpec(), "", "  ")
			if err != nil {
				// This can only happen if the spec has a cycle — panic is appropriate.
				panic("openapi: failed to marshal spec: " + err.Error())
			}
			specJSON = b
		})

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(specJSON)
	})
}

// UIHandler returns an http.Handler that serves a self-contained Swagger UI
// page. It references the spec from GET /openapi.json via CDN assets —
// no static files required.
func UIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(swaggerUIHTML))
	})
}

// swaggerUIHTML is the Swagger UI page served at GET /docs.
// It loads Swagger UI from the official CDN and points it at /openapi.json.
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CondoGuard API — Documentação</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; }
    #swagger-ui .topbar { background-color: #1a1a2e; }
    #swagger-ui .topbar .link { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-standalone-preset.js"></script>
  <script>
    window.onload = function () {
      SwaggerUIBundle({
        url: "/openapi.json",
        dom_id: "#swagger-ui",
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout",
        deepLinking: true,
        persistAuthorization: true,
        tryItOutEnabled: true,
        displayRequestDuration: true,
        filter: true
      });
    };
  </script>
</body>
</html>`
