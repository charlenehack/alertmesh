// Package version defines the AlertMesh release version.
// The pattern mirrors https://github.com/harness/harness/blob/drone/version/version.go
// so that release tooling and -ldflags overrides follow the same convention.
package version

import "fmt"

var (
	// VersionMajor is incremented for API-incompatible changes.
	VersionMajor int64

	// VersionMinor is incremented for backwards-compatible feature additions.
	VersionMinor int64 = 1

	// VersionPatch is incremented for backwards-compatible bug fixes.
	VersionPatch int64

	// VersionPre indicates a pre-release label (e.g. "alpha", "beta.1").
	// Empty string means a stable release.
	VersionPre = "alpha"

	// VersionDev is set by -ldflags at build time to the git commit short-SHA
	// or branch name.  Empty in official releases.
	VersionDev = "dev"
)

// String returns the canonical semver string, e.g. "0.1.0-alpha+dev".
func String() string {
	v := fmt.Sprintf("%d.%d.%d", VersionMajor, VersionMinor, VersionPatch)
	if VersionPre != "" {
		v += "-" + VersionPre
	}
	if VersionDev != "" {
		v += "+" + VersionDev
	}
	return v
}
