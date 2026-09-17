// Command openapi writes the OpenAPI document generated from the homey API
// code to stdout, to be redirected to api/openapi.yaml:
//
//	go run ./cmd/openapi > api/openapi.yaml
//
// The artifact is committed and CI regenerates it, failing the build on any
// drift; the spec therefore cannot get out of sync with the code. The
// generator never invokes handlers, so a spec-only API is enough.
package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/nicolasalberti00/homey/internal/server"
)

func main() {
	api := server.NewAPI(http.NewServeMux(), server.Deps{})
	data, err := api.OpenAPI().YAML()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generating OpenAPI document: %v\n", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		fmt.Fprintf(os.Stderr, "writing OpenAPI document: %v\n", err)
		os.Exit(1)
	}
}
