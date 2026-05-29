package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/ripudaman/mypm/internal/config"
)

// PackageMeta is stored as meta.json inside each package's store dir
type PackageMeta struct {
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Integrity string    `json:"integrity"` // SHA-512 of the tarball
	AddedAt   time.Time `json:"addedAt"`
	Files     []string  `json:"files"` // relative file paths inside the package
}

// Store provides content-addressable package storage
type Store struct {
	cfg *config.Config
}

// New creates a Store instance and ensures the store dirs exist
func New(cfg *config.Config) (*Store, error) {
	if err := cfg.EnsureStoreDir(); err != nil {
		return nil, err
	}
	return &Store{cfg: cfg}, nil
}

// HasPackage returns true if the package is already in the store
func (s *Store) HasPackage(name, version string) bool {
	metaPath := filepath.Join(s.cfg.PackageDir(name, version), "meta.json")
	_, err := os.Stat(metaPath)
	return err == nil
}

// PackageDir returns the store directory for a package
func (s *Store) PackageDir(name, version string) string {
	return s.cfg.PackageDir(name, version)
}

// WriteFile writes a file to the content-addressed file store.
// Returns the sha256 hex string (the file's key in the store).
func (s *Store) WriteFile(src string) (string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("opening file for store: %w", err)
	}
	defer f.Close()

	// Hash first pass
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hashing file: %w", err)
	}
	hash := hex.EncodeToString(h.Sum(nil))

	destPath := s.cfg.FileDir(hash)
	if _, err := os.Stat(destPath); err == nil {
		return hash, nil // already in store
	}

	// Seek back to start for copy
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	// Atomic write: write to tmp then rename
	tmpPath := destPath + ".tmp"
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", err
	}
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("creating tmp file: %w", err)
	}
	if _, err := io.Copy(tmp, f); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing to store: %w", err)
	}
	tmp.Close()

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("committing file to store: %w", err)
	}

	// Make store files read-only to prevent accidental modification
	os.Chmod(destPath, 0444)

	return hash, nil
}

// WriteMeta saves the package metadata to the store
func (s *Store) WriteMeta(meta *PackageMeta) error {
	dir := s.cfg.PackageDir(meta.Name, meta.Version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	metaPath := filepath.Join(dir, "meta.json")
	tmpPath := metaPath + ".tmp"

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing meta: %w", err)
	}
	return os.Rename(tmpPath, metaPath)
}

// ReadMeta reads the package metadata from the store
func (s *Store) ReadMeta(name, version string) (*PackageMeta, error) {
	metaPath := filepath.Join(s.cfg.PackageDir(name, version), "meta.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return nil, fmt.Errorf("reading meta for %s@%s: %w", name, version, err)
	}
	var meta PackageMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("parsing meta for %s@%s: %w", name, version, err)
	}
	return &meta, nil
}

// Status returns a snapshot of the store's current state
type Status struct {
	PackageCount int
	TotalBytes   int64
	StorePath    string
}

func (s *Store) Status() (*Status, error) {
	pkgDir := filepath.Join(s.cfg.StoreDir, "v1", "packages")
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &Status{StorePath: s.cfg.StoreDir}, nil
		}
		return nil, err
	}

	var totalBytes int64
	err = filepath.Walk(filepath.Join(s.cfg.StoreDir, "v1", "files"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		totalBytes += info.Size()
		return nil
	})

	return &Status{
		PackageCount: len(entries),
		TotalBytes:   totalBytes,
		StorePath:    s.cfg.StoreDir,
	}, err
}

// FileHashForPath computes SHA-256 of a file and returns the hex string
func FileHashForPath(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
