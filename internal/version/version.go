// Package version provides build-time version information.
// Version and BuildTimestamp are injected via ldflags during build.
package version //nolint:revive // intentional package name

// Version is the semantic version string, injected at build time via ldflags.
// Defaults to "dev" for development builds.
var Version = "dev"

// BuildTimestamp is the ISO 8601 build timestamp, injected at build time via ldflags.
// Defaults to "unknown" for development builds.
var BuildTimestamp = "unknown"
