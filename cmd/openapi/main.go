// Command openapi prints the OpenAPI 3.1 spec of the API as JSON.
//
//	go run ./cmd/openapi > openapi.json
//
// /openapi.json is admin-only on a running server; this command is how the
// web and the app get the contract to generate their TypeScript types
// (openapi-typescript).
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/victorradael/condoguard/api/pkg/openapi"
)

func main() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(openapi.NewSpec()); err != nil {
		fmt.Fprintln(os.Stderr, "openapi:", err)
		os.Exit(1)
	}
}
