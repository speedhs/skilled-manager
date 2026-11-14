// Package version holds the build identity injected with -ldflags.
package version

import "fmt"

// Version is the release version, or "dev" for a local build.
var Version = "dev"

// Commit is the git commit used for the build.
var Commit = "none"

// Date is the UTC build time.
var Date = "unknown"

// String returns a single-line version summary.
func String() string {
	return fmt.Sprintf("skills %s (commit %s, built %s)", Version, Commit, Date)
}
