// Package client reads from a lotse hub over its read API, with an API
// token (decision 0020):
//
//	hub, err := client.New("https://lotse.example.com", os.Getenv("LOTSE_TOKEN"))
//	status, err := hub.Status(ctx)
//	hosts, err := hub.Hosts(ctx)
//
// A token sees only what is in its scope: everything, or one customer,
// project or environment.
//
// Unlike the rest of lotse, which is AGPL-3.0, this package is licensed under
// Apache-2.0 (see LICENSE in this directory), so any program can use it.
package client

//go:generate go tool oapi-codegen -config oapi.yaml ../api/openapi.yaml
