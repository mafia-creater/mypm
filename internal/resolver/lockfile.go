package resolver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const LockfileVersion = 1
const LockfileName = "mypm.lock"

// Lockfile is the root structure written to mypm.lock
type Lockfile struct {
	Version      int                       `json:"lockfileVersion"`
	Dependencies map[string]string         `json:"dependencies"` // name → resolved exact version
	Packages     map[string]*LockedPackage `json:"packages"`     // "name@version" → details
}

// LockedPackage records everything needed to fetch and link one package
type LockedPackage struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Resolved     string            `json:"resolved"`   // tarball URL
	Integrity    string            `json:"integrity"`  // sha512-<base64>
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Dev          bool              `json:"dev,omitempty"`
}

// NewLockfile returns an empty lockfile
func NewLockfile() *Lockfile {
	return &Lockfile{
		Version:      LockfileVersion,
		Dependencies: make(map[string]string),
		Packages:     make(map[string]*LockedPackage),
	}
}

// Key returns the canonical map key for a package
func LockKey(name, version string) string {
	return name + "@" + version
}

// Add inserts or updates a package in the lockfile
func (lf *Lockfile) Add(pkg *LockedPackage) {
	lf.Dependencies[pkg.Name] = pkg.Version
	lf.Packages[LockKey(pkg.Name, pkg.Version)] = pkg
}

// Get returns a locked package by name+version, or nil
func (lf *Lockfile) Get(name, version string) *LockedPackage {
	return lf.Packages[LockKey(name, version)]
}

// Has returns true if name@version is already in the lockfile
func (lf *Lockfile) Has(name, version string) bool {
	return lf.Packages[LockKey(name, version)] != nil
}

// Write serialises the lockfile to projectRoot/mypm.lock
func (lf *Lockfile) Write(projectRoot string) error {
	data, err := json.MarshalIndent(lf, "", "  ")
	if err != nil {
		return fmt.Errorf("serialising lockfile: %w", err)
	}

	path := filepath.Join(projectRoot, LockfileName)
	tmp := path + ".tmp"

	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("writing lockfile: %w", err)
	}
	return os.Rename(tmp, path)
}

// ReadLockfile loads mypm.lock from projectRoot, or returns nil if it doesn't exist
func ReadLockfile(projectRoot string) (*Lockfile, error) {
	path := filepath.Join(projectRoot, LockfileName)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading lockfile: %w", err)
	}

	var lf Lockfile
	if err := json.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("parsing lockfile: %w", err)
	}
	return &lf, nil
}

// AllPackages returns all locked packages as a flat slice (for the fetcher)
func (lf *Lockfile) AllPackages() []*LockedPackage {
	out := make([]*LockedPackage, 0, len(lf.Packages))
	for _, p := range lf.Packages {
		out = append(out, p)
	}
	return out
}
