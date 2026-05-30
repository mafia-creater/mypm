package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMapCreatePackage(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"next@latest", "create-next-app@latest"},
		{"next", "create-next-app@latest"},
		{"react@18", "create-react@18"},
		{"create-vite@latest", "create-vite@latest"},
		{"@scope/foo@1.2.3", "@scope/create-foo@1.2.3"},
		{"@scope/create-bar@2.0.0", "@scope/create-bar@2.0.0"},
	}

	for _, c := range cases {
		if got := mapCreatePackage(c.in); got != c.want {
			t.Errorf("mapCreatePackage(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolvePackageBinString(t *testing.T) {
	pkgDir := t.TempDir()
	pkgJSON := pkgBinJSON{
		Name: "create-test",
		Bin:  mustRawJSON(t, "bin.js"),
	}
	writePkgJSON(t, pkgDir, pkgJSON)

	binName, binPath, err := resolvePackageBin(pkgDir, "create-test")
	if err != nil {
		t.Fatalf("resolvePackageBin error: %v", err)
	}
	if binName != "create-test" {
		t.Fatalf("binName = %q, want %q", binName, "create-test")
	}
	wantPath := filepath.Join(pkgDir, "bin.js")
	if binPath != wantPath {
		t.Fatalf("binPath = %q, want %q", binPath, wantPath)
	}
}

func TestResolvePackageBinMap(t *testing.T) {
	pkgDir := t.TempDir()
	binMap := map[string]string{
		"create-test": "bin/create-test.js",
		"other":       "bin/other.js",
	}
	pkgJSON := pkgBinJSON{
		Name: "create-test",
		Bin:  mustRawJSON(t, binMap),
	}
	writePkgJSON(t, pkgDir, pkgJSON)

	binName, binPath, err := resolvePackageBin(pkgDir, "create-test")
	if err != nil {
		t.Fatalf("resolvePackageBin error: %v", err)
	}
	if binName != "create-test" {
		t.Fatalf("binName = %q, want %q", binName, "create-test")
	}
	wantPath := filepath.Join(pkgDir, "bin", "create-test.js")
	if binPath != wantPath {
		t.Fatalf("binPath = %q, want %q", binPath, wantPath)
	}
}

func TestResolvePackageBinMapFallback(t *testing.T) {
	pkgDir := t.TempDir()
	binMap := map[string]string{"only": "bin/only.js"}
	pkgJSON := pkgBinJSON{
		Name: "create-test",
		Bin:  mustRawJSON(t, binMap),
	}
	writePkgJSON(t, pkgDir, pkgJSON)

	binName, binPath, err := resolvePackageBin(pkgDir, "create-test")
	if err != nil {
		t.Fatalf("resolvePackageBin error: %v", err)
	}
	if binName != "only" {
		t.Fatalf("binName = %q, want %q", binName, "only")
	}
	wantPath := filepath.Join(pkgDir, "bin", "only.js")
	if binPath != wantPath {
		t.Fatalf("binPath = %q, want %q", binPath, wantPath)
	}
}

func TestBinNameFromPackage(t *testing.T) {
	if got := binNameFromPackage("@scope/foo"); got != "foo" {
		t.Fatalf("binNameFromPackage scoped = %q, want %q", got, "foo")
	}
	if got := binNameFromPackage("bar"); got != "bar" {
		t.Fatalf("binNameFromPackage = %q, want %q", got, "bar")
	}
}

func writePkgJSON(t *testing.T, pkgDir string, pkg pkgBinJSON) {
	data, err := json.Marshal(pkg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	path := filepath.Join(pkgDir, "package.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
}

func mustRawJSON(t *testing.T, v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	return data
}
