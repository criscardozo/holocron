// Package version exposes the build's version so the app can tell whether a
// newer release exists.
package version

import "runtime/debug"

// Version is stamped at build time with the release tag:
//
//	go build -ldflags "-X github.com/cristian/holocron/internal/version.Version=v1.2.3"
//
// A build without the flag (a plain `go build`, or `go run`) reports "dev".
var Version = "dev"

// Current returns the running version, falling back to the VCS revision when
// the binary was built without a tag.
func Current() string {
	if Version != "dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && len(s.Value) >= 7 {
				return "dev-" + s.Value[:7]
			}
		}
	}
	return Version
}

// IsRelease reports whether this build came from a tagged release. Only those
// can be meaningfully compared against what GitHub publishes.
func IsRelease() bool { return Version != "dev" }
