package resolver

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"18.2.0", "18.2.0"},
		{"v1.2.3", "1.2.3"},
		{"1.0.0-beta.1", "1.0.0-beta.1"},
		{"0.0.1", "0.0.1"},
	}
	for _, c := range cases {
		v, err := ParseVersion(c.in)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", c.in, err)
			continue
		}
		if v.String() != c.want {
			t.Errorf("ParseVersion(%q) = %q, want %q", c.in, v.String(), c.want)
		}
	}
}

func TestRangeSatisfies(t *testing.T) {
	cases := []struct {
		rangeStr string
		version  string
		want     bool
	}{
		// Caret
		{"^18.0.0", "18.2.0", true},
		{"^18.0.0", "18.0.0", true},
		{"^18.0.0", "17.9.9", false},
		{"^18.0.0", "19.0.0", false},
		{"^0.2.3", "0.2.5", true},
		{"^0.2.3", "0.3.0", false},
		{"^0.0.3", "0.0.3", true},
		{"^0.0.3", "0.0.4", false},
		// Tilde
		{"~1.2.3", "1.2.5", true},
		{"~1.2.3", "1.3.0", false},
		{"~1.2.3", "1.2.2", false},
		// Comparison
		{">=1.0.0", "1.0.0", true},
		{">=1.0.0", "0.9.9", false},
		{">=1.0.0 <2.0.0", "1.5.0", true},
		{">=1.0.0 <2.0.0", "2.0.0", false},
		{">= 2.1.2 < 3.0.0", "2.1.2", true},
		{">= 2.1.2 < 3.0.0", "3.0.0", false},
		// Wildcard
		{"*", "99.99.99", true},
		{"latest", "1.0.0", true},
		// Exact
		{"1.2.3", "1.2.3", true},
		{"1.2.3", "1.2.4", false},
		// X ranges
		{"1.x", "1.5.0", true},
		{"1.x", "2.0.0", false},
		{"1.2.x", "1.2.9", true},
		{"1.2.x", "1.3.0", false},
		{"1.x.x", "1.9.9", true},
		// Prerelease excluded unless explicitly targeted
		{"^1.0.0", "1.1.0-beta.1", false},
	}

	for _, c := range cases {
		r, err := ParseRange(c.rangeStr)
		if err != nil {
			t.Errorf("ParseRange(%q) error: %v", c.rangeStr, err)
			continue
		}
		v, err := ParseVersion(c.version)
		if err != nil {
			t.Errorf("ParseVersion(%q) error: %v", c.version, err)
			continue
		}
		got := r.Satisfies(v)
		if got != c.want {
			t.Errorf("range %q.Satisfies(%q) = %v, want %v", c.rangeStr, c.version, got, c.want)
		}
	}
}

func TestBestMatch(t *testing.T) {
	cases := []struct {
		rangeStr   string
		candidates []string
		want       string
	}{
		{
			rangeStr:   "^18.0.0",
			candidates: []string{"16.0.0", "17.0.0", "18.0.0", "18.1.0", "18.2.0", "19.0.0"},
			want:       "18.2.0",
		},
		{
			rangeStr:   "^17.0.0",
			candidates: []string{"16.0.0", "17.0.0", "18.0.0", "18.1.0", "18.2.0", "19.0.0"},
			want:       "17.0.0",
		},
		{
			rangeStr:   ">=16.0.0 <18.0.0",
			candidates: []string{"16.0.0", "17.0.0", "18.0.0", "18.1.0", "18.2.0", "19.0.0"},
			want:       "17.0.0",
		},
		{
			rangeStr:   "~18.1.0",
			candidates: []string{"16.0.0", "17.0.0", "18.0.0", "18.1.0", "18.2.0", "19.0.0"},
			want:       "18.1.0",
		},
		{
			rangeStr:   "*",
			candidates: []string{"16.0.0", "17.0.0", "18.0.0", "18.1.0", "18.2.0", "19.0.0"},
			want:       "19.0.0",
		},
		{
			rangeStr:   "^1.0.0 || ^2.0.0",
			candidates: []string{"1.0.0", "1.5.0", "2.0.0", "2.4.0", "3.0.0"},
			want:       "2.4.0",
		},
		{
			rangeStr:   "1.x",
			candidates: []string{"1.0.0", "1.5.0", "2.0.0", "2.4.0", "3.0.0"},
			want:       "1.5.0",
		},
		{
			rangeStr:   "1.2.x",
			candidates: []string{"1.2.0", "1.2.5", "1.3.0"},
			want:       "1.2.5",
		},
	}

	for _, c := range cases {
		r, err := ParseRange(c.rangeStr)
		if err != nil {
			t.Fatalf("ParseRange(%q): %v", c.rangeStr, err)
		}
		got, err := r.BestMatch(c.candidates)
		if err != nil {
			t.Errorf("BestMatch(%q) error: %v", c.rangeStr, err)
			continue
		}
		if got != c.want {
			t.Errorf("BestMatch(%q) = %q, want %q", c.rangeStr, got, c.want)
		}
	}
}
