// Package version holds the version of this build.
package version

// Version is set at build time with
// -ldflags "-X lotse.sh/internal/version.Version=…".
var Version = "dev"
