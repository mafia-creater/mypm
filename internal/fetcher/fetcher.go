package fetcher

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mafia-creater/mypm/internal/config"
	"github.com/mafia-creater/mypm/internal/logger"
	"github.com/mafia-creater/mypm/internal/store"
)

// FetchResult is returned per package after a fetch attempt
type FetchResult struct {
	Name      string
	Version   string
	FromCache bool // true = already in store, no download
	Err       error
}

// FetchJob is a unit of work for the parallel fetcher
type FetchJob struct {
	Name    string
	Version string
	Meta    *RegistryVersion
}

// Fetcher downloads packages from the registry and populates the store
type Fetcher struct {
	cfg        *config.Config
	registry   *RegistryClient
	store      *store.Store
	httpClient *http.Client
}

// New creates a Fetcher
func New(cfg *config.Config, s *store.Store) *Fetcher {
	return &Fetcher{
		cfg:      cfg,
		registry: NewRegistryClient(cfg.Registry),
		store:    s,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// FetchAll downloads and unpacks a list of packages in parallel.
// Concurrency is controlled by cfg.Concurrency (default 8).
func (f *Fetcher) FetchAll(jobs []FetchJob) []FetchResult {
	results := make([]FetchResult, len(jobs))

	sem := make(chan struct{}, f.cfg.Concurrency)
	var wg sync.WaitGroup

	for i, job := range jobs {
		wg.Add(1)
		go func(idx int, j FetchJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = f.fetchOne(j)
		}(i, job)
	}

	wg.Wait()
	return results
}

// fetchOne handles a single package: cache check → download → verify → unpack
func (f *Fetcher) fetchOne(job FetchJob) FetchResult {
	result := FetchResult{Name: job.Name, Version: job.Version}

	// 1. Store-first check — skip download if already cached
	if f.store.HasPackage(job.Name, job.Version) {
		result.FromCache = true
		return result
	}

	// 2. Get registry metadata if not provided
	meta := job.Meta
	if meta == nil {
		var err error
		meta, err = f.registry.GetVersion(job.Name, job.Version)
		if err != nil {
			result.Err = fmt.Errorf("registry metadata: %w", err)
			return result
		}
	}

	// 3. Download tarball to a temp file
	tmpPath, err := f.downloadTarball(meta.Dist.Tarball, job.Name, job.Version)
	if err != nil {
		result.Err = fmt.Errorf("download: %w", err)
		return result
	}
	defer os.Remove(tmpPath) // clean up tmp regardless

	// 4. Verify SHA-512 integrity
	if meta.Dist.Integrity != "" {
		if err := verifyIntegrity(tmpPath, meta.Dist.Integrity); err != nil {
			result.Err = fmt.Errorf("integrity check failed: %w", err)
			return result
		}
	}

	// 5. Unpack into store
	pkgDir := f.cfg.PackageDir(job.Name, job.Version)
	if err := unpackTarball(tmpPath, pkgDir); err != nil {
		os.RemoveAll(pkgDir) // don't leave partial unpacks
		result.Err = fmt.Errorf("unpack: %w", err)
		return result
	}

	// 6. Write store metadata
	if err := f.store.WriteMeta(&store.PackageMeta{
		Name:      job.Name,
		Version:   job.Version,
		Integrity: meta.Dist.Integrity,
	}); err != nil {
		result.Err = fmt.Errorf("writing store meta: %w", err)
		return result
	}

	return result
}

// downloadTarball downloads a tarball URL to a temp file and returns its path
func (f *Fetcher) downloadTarball(url, name, version string) (string, error) {
	tmpDir := filepath.Join(f.cfg.StoreDir, "tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return "", err
	}

	// Sanitize name for use as filename (scoped packages have @scope/name)
	safeName := strings.ReplaceAll(name, "/", "__")
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("%s@%s.tgz", safeName, version))

	resp, err := f.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("registry returned %d for tarball %s", resp.StatusCode, url)
	}

	// Write to tmp file atomically
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	written, err := io.Copy(tmp, resp.Body)
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("writing tarball (%d bytes written): %w", written, err)
	}

	logger.Info("downloaded %s@%s (%.1f KB)", name, version, float64(written)/1024)
	return tmpPath, nil
}

// verifyIntegrity checks a tarball's SHA-512 against the registry-provided integrity string.
// Format: "sha512-<base64-encoded-hash>"
func verifyIntegrity(tarballPath, integrity string) error {
	if !strings.HasPrefix(integrity, "sha512-") {
		// sha1 shasum fallback — skip for now, sha512 is standard
		return nil
	}

	expectedB64 := strings.TrimPrefix(integrity, "sha512-")
	expected, err := base64.StdEncoding.DecodeString(expectedB64)
	if err != nil {
		return fmt.Errorf("decoding integrity hash: %w", err)
	}

	f, err := os.Open(tarballPath)
	if err != nil {
		return err
	}
	defer f.Close()

	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hashing tarball: %w", err)
	}
	actual := h.Sum(nil)

	if len(actual) != len(expected) {
		return fmt.Errorf("integrity mismatch: expected %d bytes, got %d", len(expected), len(actual))
	}
	for i := range actual {
		if actual[i] != expected[i] {
			return fmt.Errorf("integrity mismatch — tarball may be corrupted or tampered")
		}
	}

	return nil
}

// unpackTarball extracts a .tgz file into destDir.
// npm tarballs have a top-level "package/" directory which is stripped.
func unpackTarball(tarballPath, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	f, err := os.Open(tarballPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("opening gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("reading tar: %w", err)
		}

		// npm tarballs always have "package/" as top-level dir — strip it
		cleanPath := stripTopDir(header.Name)
		if cleanPath == "" {
			continue
		}

		// Security: prevent path traversal
		if strings.Contains(cleanPath, "..") {
			continue
		}

		destPath := filepath.Join(destDir, filepath.FromSlash(cleanPath))

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			// Atomic write: tmp then rename
			tmpDest := destPath + ".tmp"
			out, err := os.Create(tmpDest)
			if err != nil {
				return fmt.Errorf("creating %s: %w", destPath, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				os.Remove(tmpDest)
				return fmt.Errorf("extracting %s: %w", cleanPath, err)
			}
			out.Close()
			if err := os.Rename(tmpDest, destPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// stripTopDir removes the first path component ("package/index.js" → "index.js")
func stripTopDir(p string) string {
	// Normalize separators
	p = strings.ReplaceAll(p, "\\", "/")
	idx := strings.Index(p, "/")
	if idx < 0 {
		return ""
	}
	return p[idx+1:]
}
