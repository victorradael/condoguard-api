package openapi

import (
	"crypto/sha256"
	"encoding/base64"
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
		w.Header().Set("Cache-Control", "no-store") // admin-only: never in shared caches
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(specJSON)
	})
}

// Swagger UI is loaded from the CDN at an exact version, and the browser
// checks each file against its Subresource Integrity hash: a changed or
// compromised package fails to load instead of running on the API origin,
// where the admin's session cookie is valid. Bump all five together
// (SRI = base64 SHA-384 of the file, e.g. via https://www.srihash.org).
const (
	swaggerUIBase      = "https://unpkg.com/swagger-ui-dist@5.34.1/"
	swaggerUICSSSRI    = "sha384-4c3vsE5f+trhFyHrFrLM1fFGUn531NIaC7dcqEWIJdoRQJ5CkJbSuSCwXLpaS1KF"
	swaggerUIBundleSRI = "sha384-XFYN47VzPiDY6qPtxYKg0nqUeW/upo20xf1j2SUE7gT/i35FJB4cuS/71Iegzk8Z"
	swaggerUIPresetSRI = "sha384-My2aDM4r2Mbm3ybHcubKm9O9U8FEjvF/O5nGvE9YK5dzqOTbWEKa79RPJ1krdMaF"
	// The bundle's logo SVG carries its own <style>; allowed by its CSP hash
	// (the browser console names the hash of a blocked block).
	swaggerUILogoStyleHash = "'sha256-RL3ie0nH+Lzz2YNqQN83mnU0J1ot4QL7b99vMdIX99w='"
)

// swaggerUIInitJS starts Swagger UI. Authorization is not persisted
// (localStorage would keep a pasted bearer token) and the external
// validator badge is off, so the page only talks to the CDN and the API.
const swaggerUIInitJS = `
    window.onload = function () {
      SwaggerUIBundle({
        url: "openapi.json",
        dom_id: "#swagger-ui",
        presets: [
          SwaggerUIBundle.presets.apis,
          SwaggerUIStandalonePreset
        ],
        layout: "StandaloneLayout",
        deepLinking: true,
        persistAuthorization: false,
        validatorUrl: null,
        tryItOutEnabled: true,
        displayRequestDuration: true,
        filter: true
      });
    };
  `

const swaggerUIStyle = `
    body { margin: 0; }
    #swagger-ui .topbar { background-color: #1a1a2e; }
    #swagger-ui .topbar .link { display: none; }
  `

// swaggerUIHTML is the Swagger UI page served at GET /docs.
var swaggerUIHTML = `<!DOCTYPE html>
<html lang="pt-BR">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CondoGuard API — Documentação</title>
  <link rel="stylesheet" href="` + swaggerUIBase + `swagger-ui.css" integrity="` + swaggerUICSSSRI + `" crossorigin="anonymous">
  <style>` + swaggerUIStyle + `</style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="` + swaggerUIBase + `swagger-ui-bundle.js" integrity="` + swaggerUIBundleSRI + `" crossorigin="anonymous"></script>
  <script src="` + swaggerUIBase + `swagger-ui-standalone-preset.js" integrity="` + swaggerUIPresetSRI + `" crossorigin="anonymous"></script>
  <script>` + swaggerUIInitJS + `</script>
</body>
</html>`

// SwaggerUICSP is the Content-Security-Policy of /docs: scripts and styles
// only from the pinned CDN path or the exact inline blocks above (by hash),
// requests only to the API itself, and no framing.
var SwaggerUICSP = "default-src 'none'" +
	"; script-src " + swaggerUIBase + " " + inlineHash(swaggerUIInitJS) +
	"; style-src " + swaggerUIBase + " " + inlineHash(swaggerUIStyle) + " " + swaggerUILogoStyleHash +
	"; img-src 'self' data:" +
	"; connect-src 'self'" +
	"; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

func inlineHash(block string) string {
	sum := sha256.Sum256([]byte(block))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

// UIHandler returns an http.Handler that serves the Swagger UI page under a
// strict CSP. It reads the spec from openapi.json next to the page, so it
// also works when a proxy mounts the API under a prefix (/api/docs).
func UIHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", SwaggerUICSP)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(swaggerUIHTML))
	})
}
