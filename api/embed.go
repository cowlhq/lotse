// Package api holds the description of a hub's HTTP API, api/openapi.yaml,
// from which the hub's server code, the Go client and the web interface's
// types are made. The hub serves it at /api/openapi.yaml.
package api

import _ "embed"

//go:embed openapi.yaml
var OpenAPI []byte
