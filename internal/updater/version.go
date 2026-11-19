package updater

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var safeTagPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// NormalizeVersion trims whitespace and strips any leading "v" or "V" prefix.
func NormalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	return v
}

// EnsureTag validates a release version/tag string and returns it with a leading "v".
func EnsureTag(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", fmt.Errorf("empty version tag")
	}
	if !strings.HasPrefix(v, "v") && !strings.HasPrefix(v, "V") {
		v = "v" + v
	} else if strings.HasPrefix(v, "V") {
		v = "v" + v[1:]
	}
	if !safeTagPattern.MatchString(v) || strings.Contains(v, "..") {
		return "", fmt.Errorf("invalid version tag: %q", v)
	}
	return v, nil
}

type parsedSemVer struct {
	major      int
	minor      int
	patch      int
	prerelease string
	valid      bool
}

func parseSemVer(raw string) parsedSemVer {
	s := NormalizeVersion(raw)
	if s == "" {
		return parsedSemVer{}
	}
	// Strip build metadata (+...)
	if idx := strings.IndexByte(s, '+'); idx >= 0 {
		s = s[:idx]
	}
	var pre string
	if idx := strings.IndexByte(s, '-'); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return parsedSemVer{}
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return parsedSemVer{}
		}
		nums[i] = n
	}
	return parsedSemVer{
		major:      nums[0],
		minor:      nums[1],
		patch:      nums[2],
		prerelease: pre,
		valid:      true,
	}
}

// CompareVersions compares two semantic version strings (with or without leading "v").
// Returns 1 if a > b, -1 if a < b, and 0 if equal.
func CompareVersions(a, b string) int {
	pa := parseSemVer(a)
	pb := parseSemVer(b)
	if !pa.valid || !pb.valid {
		na := NormalizeVersion(a)
		nb := NormalizeVersion(b)
		if na == nb {
			return 0
		}
		if na > nb {
			return 1
		}
		return -1
	}

	if pa.major != pb.major {
		if pa.major > pb.major {
			return 1
		}
		return -1
	}
	if pa.minor != pb.minor {
		if pa.minor > pb.minor {
			return 1
		}
		return -1
	}
	if pa.patch != pb.patch {
		if pa.patch > pb.patch {
			return 1
		}
		return -1
	}

	// Stable release > prerelease of same major.minor.patch
	if pa.prerelease == pb.prerelease {
		return 0
	}
	if pa.prerelease == "" {
		return 1
	}
	if pb.prerelease == "" {
		return -1
	}
	return comparePrerelease(pa.prerelease, pb.prerelease)
}

func comparePrerelease(a, b string) int {
	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")
	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}
	for i := 0; i < maxLen; i++ {
		if i >= len(aParts) {
			return -1
		}
		if i >= len(bParts) {
			return 1
		}
		ai, aErr := strconv.Atoi(aParts[i])
		bi, bErr := strconv.Atoi(bParts[i])
		switch {
		case aErr == nil && bErr == nil:
			if ai != bi {
				if ai > bi {
					return 1
				}
				return -1
			}
		case aErr == nil && bErr != nil:
			// SemVer 2.0: numeric identifiers always have lower precedence than non-numeric
			return -1
		case aErr != nil && bErr == nil:
			return 1
		default:
			if aParts[i] != bParts[i] {
				if aParts[i] > bParts[i] {
					return 1
				}
				return -1
			}
		}
	}
	return 0
}

// IsNewer reports whether latest is strictly newer than current.
func IsNewer(latest, current string) bool {
	if strings.TrimSpace(latest) == "" {
		return false
	}
	return CompareVersions(latest, current) > 0
}
