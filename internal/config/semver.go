package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Semver struct {
	Major int
	Minor int
	Patch int
}

func (s Semver) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

// ParseSemver parses a version string like "1.2.3" or "v1.2.3".
func ParseSemver(s string) (Semver, error) {
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return Semver{}, fmt.Errorf("invalid semver: %q", s)
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid semver major: %q", parts[0])
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid semver minor: %q", parts[1])
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil {
		return Semver{}, fmt.Errorf("invalid semver patch: %q", parts[2])
	}

	return Semver{Major: major, Minor: minor, Patch: patch}, nil
}

// CompareSemver returns -1 if a < b, 0 if a == b, 1 if a > b.
func CompareSemver(a, b Semver) int {
	if a.Major != b.Major {
		return cmpInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmpInt(a.Minor, b.Minor)
	}
	return cmpInt(a.Patch, b.Patch)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// SemverAtLeast returns true if version >= minimum.
// Returns true for unparseable versions (e.g. "dev") so dev builds are never blocked.
func SemverAtLeast(version, minimum string) bool {
	v, err := ParseSemver(version)
	if err != nil {
		return true // dev builds pass
	}
	m, err := ParseSemver(minimum)
	if err != nil {
		return true // unparseable minimum passes
	}
	return CompareSemver(v, m) >= 0
}
