package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LinkType controls how packages are linked into node_modules
type LinkType string

const (
	LinkTypeHardlink LinkType = "hardlink"
	LinkTypeSymlink  LinkType = "symlink"
	LinkTypeCopy     LinkType = "copy"
)

// Config holds all mypm configuration
type Config struct {
	StoreDir  string
	Registry  string
	LinkType  LinkType
	Concurrency int
}

// Default returns sane defaults
func Default() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		StoreDir:    filepath.Join(home, ".mypm", "store"),
		Registry:    "https://registry.npmjs.org",
		LinkType:    LinkTypeHardlink,
		Concurrency: 8,
	}
}

// Load reads ~/.mypmrc and merges with defaults
// File format (ini-style):
//   store-dir = /home/user/.mypm/store
//   registry  = https://registry.npmjs.org
//   link-type = hardlink
//   concurrency = 8
func Load() (*Config, error) {
	cfg := Default()

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil // return defaults, not fatal
	}

	rcPath := filepath.Join(home, ".mypmrc")
	f, err := os.Open(rcPath)
	if os.IsNotExist(err) {
		return cfg, nil // no config file is fine
	}
	if err != nil {
		return nil, fmt.Errorf("reading ~/.mypmrc: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "store-dir":
			cfg.StoreDir = val
		case "registry":
			cfg.Registry = val
		case "link-type":
			cfg.LinkType = LinkType(val)
		case "concurrency":
			var n int
			fmt.Sscanf(val, "%d", &n)
			if n > 0 {
				cfg.Concurrency = n
			}
		}
	}

	return cfg, scanner.Err()
}

// EnsureStoreDir creates the store directory if it doesn't exist
func (c *Config) EnsureStoreDir() error {
	dirs := []string{
		c.StoreDir,
		filepath.Join(c.StoreDir, "v1", "packages"),
		filepath.Join(c.StoreDir, "v1", "files"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating store dir %s: %w", d, err)
		}
	}
	return nil
}

// PackageDir returns the store path for a given package@version
func (c *Config) PackageDir(name, version string) string {
	return filepath.Join(c.StoreDir, "v1", "packages", name+"@"+version)
}

// FileDir returns the store path for a content-addressed file
func (c *Config) FileDir(sha256 string) string {
	return filepath.Join(c.StoreDir, "v1", "files", sha256)
}
