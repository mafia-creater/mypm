package resolver

import (
	"fmt"
	"strconv"
	"strings"
)

// Version holds a parsed semantic version
type Version struct {
	Major int
	Minor int
	Patch int
	Pre   string // prerelease tag e.g. "alpha.1", "beta.2"
}

// ParseVersion parses "18.2.0", "18.2.0-beta.1", "v1.2.3"
func ParseVersion(s string) (Version, error) {
	s = strings.TrimPrefix(s, "v")
	// Strip prerelease/build suffix after first split
	pre := ""
	if idx := strings.IndexAny(s, "-+"); idx >= 0 {
		pre = s[idx+1:]
		s = s[:idx]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	pat, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return Version{}, fmt.Errorf("invalid version %q", s)
	}
	return Version{Major: maj, Minor: min, Patch: pat, Pre: pre}, nil
}

func (v Version) String() string {
	s := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Pre != "" {
		s += "-" + v.Pre
	}
	return s
}

// Compare returns -1, 0, or 1
func (v Version) Compare(other Version) int {
	if v.Major != other.Major {
		return cmp(v.Major, other.Major)
	}
	if v.Minor != other.Minor {
		return cmp(v.Minor, other.Minor)
	}
	if v.Patch != other.Patch {
		return cmp(v.Patch, other.Patch)
	}
	// No prerelease > prerelease (1.0.0 > 1.0.0-alpha)
	if v.Pre == "" && other.Pre != "" {
		return 1
	}
	if v.Pre != "" && other.Pre == "" {
		return -1
	}
	return strings.Compare(v.Pre, other.Pre)
}

func cmp(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// ─── Range ───────────────────────────────────────────────────────────────────

// Range represents a parsed semver range that can test versions
type Range struct {
	raw    string
	groups [][]condition
}

type condition struct {
	op      string // ">=", ">", "<=", "<", "=", "~", "^", "*"
	version Version
}

// ParseRange parses a semver range string like "^18.0.0", "~1.2.3", ">=1.0.0 <2.0.0", "*", "latest"
func ParseRange(s string) (*Range, error) {
	s = strings.TrimSpace(s)
	r := &Range{raw: s}

	parts := strings.Split(s, "||")
	for _, part := range parts {
		group, err := parseRangeSegment(part)
		if err != nil {
			return nil, err
		}
		if len(group) > 0 {
			r.groups = append(r.groups, group)
		}
	}
	if len(r.groups) == 0 {
		r.groups = [][]condition{{{op: "*"}}}
	}
	return r, nil
}

func parseRangeSegment(s string) ([]condition, error) {
	s = strings.TrimSpace(s)

	// Special cases
	if s == "" || s == "*" || s == "latest" || strings.EqualFold(s, "x") {
		return []condition{{op: "*"}}, nil
	}

	// Handle space-separated AND ranges e.g. ">=1.0.0 <2.0.0"
	parts := strings.Fields(s)
	conds := make([]condition, 0, len(parts))
	for _, p := range parts {
		c, err := parseCondition(p)
		if err != nil {
			return nil, err
		}
		conds = append(conds, c)
	}
	return conds, nil
}

func parseCondition(s string) (condition, error) {
	s = strings.TrimSpace(s)

	if c, ok, err := parseXRange(s); ok {
		return c, err
	} else if err != nil {
		return condition{}, err
	}

	// Operators: >=, <=, >, <, =, ^, ~
	for _, op := range []string{">=", "<=", ">", "<", "="} {
		if strings.HasPrefix(s, op) {
			v, err := ParseVersion(strings.TrimPrefix(s, op))
			if err != nil {
				return condition{}, err
			}
			return condition{op: op, version: v}, nil
		}
	}

	if strings.HasPrefix(s, "^") {
		v, err := ParseVersion(strings.TrimPrefix(s, "^"))
		if err != nil {
			return condition{}, err
		}
		return condition{op: "^", version: v}, nil
	}

	if strings.HasPrefix(s, "~") {
		v, err := ParseVersion(strings.TrimPrefix(s, "~"))
		if err != nil {
			return condition{}, err
		}
		return condition{op: "~", version: v}, nil
	}

	// Bare major "1" or "12" → treat as "^1.0.0" / "^12.0.0"
	if isBareInt(s) {
		v, err := ParseVersion(s + ".0.0")
		if err != nil {
			return condition{}, err
		}
		return condition{op: "^", version: v}, nil
	}

	// Bare major.minor "1.2" → treat as "^1.2.0"
	if isBareMinor(s) {
		v, err := ParseVersion(s + ".0")
		if err != nil {
			return condition{}, err
		}
		return condition{op: "^", version: v}, nil
	}

	// Plain version "1.2.3" → exact match
	v, err := ParseVersion(s)
	if err != nil {
		return condition{}, fmt.Errorf("cannot parse range condition %q: %w", s, err)
	}
	return condition{op: "=", version: v}, nil
}

func parseXRange(s string) (condition, bool, error) {
	parts := strings.Split(s, ".")
	wildcardIndex := -1
	for i, p := range parts {
		if isXRangePart(p) {
			wildcardIndex = i
			break
		}
	}
	if wildcardIndex == -1 {
		return condition{}, false, nil
	}
	if wildcardIndex == 0 {
		return condition{op: "*"}, true, nil
	}
	if !isBareInt(parts[0]) {
		return condition{}, false, fmt.Errorf("invalid x-range %q", s)
	}
	major, _ := strconv.Atoi(parts[0])
	if wildcardIndex == 1 {
		return condition{op: "^", version: Version{Major: major}}, true, nil
	}
	if len(parts) < 2 || !isBareInt(parts[1]) {
		return condition{}, false, fmt.Errorf("invalid x-range %q", s)
	}
	minor, _ := strconv.Atoi(parts[1])
	return condition{op: "~", version: Version{Major: major, Minor: minor}}, true, nil
}

func isXRangePart(s string) bool {
	return s == "x" || s == "X" || s == "*"
}

func isBareInt(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isBareMinor(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 2 {
		return false
	}
	return isBareInt(parts[0]) && isBareInt(parts[1])
}

// Satisfies returns true if version v satisfies this range
func (r *Range) Satisfies(v Version) bool {
	// Prerelease versions are excluded unless the range explicitly targets them
	if v.Pre != "" {
		for _, group := range r.groups {
			if !groupHasPreTarget(group) {
				continue
			}
			if groupSatisfies(group, v) {
				return true
			}
		}
		return false
	}

	for _, group := range r.groups {
		if groupSatisfies(group, v) {
			return true
		}
	}
	return false
}

func groupHasPreTarget(conds []condition) bool {
	for _, c := range conds {
		if c.version.Pre != "" {
			return true
		}
	}
	return false
}

func groupSatisfies(conds []condition, v Version) bool {
	for _, c := range conds {
		if !c.satisfies(v) {
			return false
		}
	}
	return true
}

func (c condition) satisfies(v Version) bool {
	switch c.op {
	case "*":
		return true
	case "=":
		return v.Compare(c.version) == 0
	case ">":
		return v.Compare(c.version) > 0
	case ">=":
		return v.Compare(c.version) >= 0
	case "<":
		return v.Compare(c.version) < 0
	case "<=":
		return v.Compare(c.version) <= 0
	case "^":
		// Caret: compatible with version
		// ^1.2.3 := >=1.2.3 <2.0.0
		// ^0.2.3 := >=0.2.3 <0.3.0  (major=0, minor is locked)
		// ^0.0.3 := >=0.0.3 <0.0.4  (major=0, minor=0, patch is locked)
		if v.Compare(c.version) < 0 {
			return false
		}
		if c.version.Major > 0 {
			return v.Major == c.version.Major
		}
		if c.version.Minor > 0 {
			return v.Major == 0 && v.Minor == c.version.Minor
		}
		return v.Major == 0 && v.Minor == 0 && v.Patch == c.version.Patch
	case "~":
		// Tilde: approximately equivalent
		// ~1.2.3 := >=1.2.3 <1.3.0
		// ~1.2   := >=1.2.0 <1.3.0
		if v.Compare(c.version) < 0 {
			return false
		}
		return v.Major == c.version.Major && v.Minor == c.version.Minor
	}
	return false
}

func (r *Range) String() string { return r.raw }

// BestMatch returns the highest version from candidates that satisfies the range
func (r *Range) BestMatch(candidates []string) (string, error) {
	var best *Version
	var bestStr string

	for _, s := range candidates {
		v, err := ParseVersion(s)
		if err != nil {
			continue // skip malformed versions
		}
		if !r.Satisfies(v) {
			continue
		}
		if best == nil || v.Compare(*best) > 0 {
			cp := v
			best = &cp
			bestStr = s
		}
	}

	if best == nil {
		return "", fmt.Errorf("no version satisfies range %q", r.raw)
	}
	return bestStr, nil
}
