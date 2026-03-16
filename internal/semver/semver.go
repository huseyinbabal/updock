// Package semver provides semantic versioning parsing and comparison for Docker image tags.
//
// It handles common Docker tag formats including:
//   - Standard semver: 1.2.3, v1.2.3
//   - Partial semver: 1.2, 8.0
//   - Pre-release: 1.2.3-alpine, 8.0.45-bookworm
//   - Build metadata: 1.2.3+build
//
// Tags that are not valid semver (e.g. "latest", "alpine", "bookworm") are
// rejected by [Parse] and excluded from version comparisons.
package semver

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease string // e.g. "alpine", "rc1"
	Original   string // the original tag string
}

// semverRegex matches tags like: 1.2.3, v1.2.3, 1.2, 8.0.45, 1.2.3-alpine
var semverRegex = regexp.MustCompile(
	`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?(?:[-]([a-zA-Z0-9._-]+))?(?:\+[a-zA-Z0-9._-]+)?$`,
)

// Parse attempts to parse a Docker image tag as a semantic version.
// Returns an error if the tag is not a valid semver-like string.
//
// Examples:
//
//	Parse("1.2.3")       -> Version{1, 2, 3, "", "1.2.3"}
//	Parse("v2.0.0")      -> Version{2, 0, 0, "", "v2.0.0"}
//	Parse("8.0.45")      -> Version{8, 0, 45, "", "8.0.45"}
//	Parse("1.25")        -> Version{1, 25, 0, "", "1.25"}
//	Parse("3-alpine")    -> Version{3, 0, 0, "alpine", "3-alpine"}
//	Parse("latest")      -> error
func Parse(tag string) (Version, error) {
	matches := semverRegex.FindStringSubmatch(tag)
	if matches == nil {
		return Version{}, fmt.Errorf("not a semver tag: %q", tag)
	}

	v := Version{Original: tag}

	v.Major, _ = strconv.Atoi(matches[1])

	if matches[2] != "" {
		v.Minor, _ = strconv.Atoi(matches[2])
	}
	if matches[3] != "" {
		v.Patch, _ = strconv.Atoi(matches[3])
	}
	if matches[4] != "" {
		v.PreRelease = matches[4]
	}

	return v, nil
}

// String returns the original tag string.
func (v Version) String() string {
	return v.Original
}

// Compare returns -1, 0, or 1 comparing v to other.
// Pre-release versions are considered lower than release versions.
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return intCmp(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return intCmp(v.Minor, other.Minor)
	}
	if v.Patch != other.Patch {
		return intCmp(v.Patch, other.Patch)
	}
	// Pre-release ordering: no pre-release > any pre-release
	if v.PreRelease == "" && other.PreRelease != "" {
		return 1
	}
	if v.PreRelease != "" && other.PreRelease == "" {
		return -1
	}
	return strings.Compare(v.PreRelease, other.PreRelease)
}

// IsNewerThan returns true if v is a higher version than other.
func (v Version) IsNewerThan(other Version) bool {
	return v.Compare(other) > 0
}

// IsPatchUpdate returns true if v is a patch update relative to base
// (same major.minor, higher patch).
func (v Version) IsPatchUpdate(base Version) bool {
	return v.Major == base.Major && v.Minor == base.Minor && v.Patch > base.Patch
}

// IsMinorUpdate returns true if v is a minor update relative to base
// (same major, higher minor, or same minor with higher patch).
func (v Version) IsMinorUpdate(base Version) bool {
	if v.Major != base.Major {
		return false
	}
	if v.Minor > base.Minor {
		return true
	}
	return v.Minor == base.Minor && v.Patch > base.Patch
}

// IsMajorUpdate returns true if v is any version higher than base.
func (v Version) IsMajorUpdate(base Version) bool {
	return v.IsNewerThan(base)
}

// HasSamePreReleaseSuffix checks if two versions have the same pre-release
// suffix type. For example, "8.0.45-bookworm" and "8.0.46-bookworm" have
// the same suffix, but "8.0.45-alpine" and "8.0.45-bookworm" do not.
func (v Version) HasSamePreReleaseSuffix(other Version) bool {
	return v.PreRelease == other.PreRelease
}

// FilterByStrategy returns the best candidate tag from a list of tags,
// given the current version and an update strategy.
//
// Strategy rules:
//   - "patch": only allow v.Major == current.Major && v.Minor == current.Minor && v.Patch > current.Patch
//   - "minor": only allow v.Major == current.Major && (v.Minor > current.Minor || same minor with higher patch)
//   - "major": allow any higher version
//   - "all": allow any higher version (same as major)
//   - "digest": not applicable (returns nil)
//   - "pin": not applicable (returns nil)
//
// Only tags with the same pre-release suffix as the current tag are considered.
// For example, if the current tag is "8.0.45-bookworm", only "-bookworm" candidates match.
func FilterByStrategy(current Version, candidates []Version, strategy string) *Version {
	if strategy == "digest" || strategy == "pin" || strategy == "" {
		return nil
	}

	var best *Version
	for i := range candidates {
		c := candidates[i]

		// Must have the same pre-release suffix
		if !c.HasSamePreReleaseSuffix(current) {
			continue
		}

		// Must be newer
		if !c.IsNewerThan(current) {
			continue
		}

		// Apply strategy constraint
		switch strategy {
		case "patch":
			if !c.IsPatchUpdate(current) {
				continue
			}
		case "minor":
			if !c.IsMinorUpdate(current) {
				continue
			}
		case "major", "all":
			if !c.IsMajorUpdate(current) {
				continue
			}
		}

		if best == nil || c.IsNewerThan(*best) {
			best = &c
		}
	}

	return best
}

func intCmp(a, b int) int {
	if a < b {
		return -1
	}
	return 1
}
