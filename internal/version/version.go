// Package version provides the build-time version string for vision-mcp.
// Version is injected via ldflags at build time (e.g. -X internal/version.Version=b1234).
// When not set, Version defaults to "dev" and the updater skips all checks.
package version

// Version is the current build version. Set via ldflags during CI builds:
//
//	go build -ldflags="-s -w -X github.com/cristian-guerrero/go-vision-mcp/internal/version.Version=b1024"
//
// Defaults to "dev" for local development builds.
var Version = "dev"
