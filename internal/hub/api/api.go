// Package api holds the hub's HTTP API as generated from api/openapi.yaml:
// its types and the interface the hub implements. Change the spec, then run
// go generate.
package api

//go:generate go tool oapi-codegen -config oapi.yaml ../../../api/openapi.yaml
