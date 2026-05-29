package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ripudaman/mypm/internal/config"
	"github.com/ripudaman/mypm/internal/linker"
	"github.com/ripudaman/mypm/internal/logger"
	"github.com/ripudaman/mypm/internal/store"
)

// PackageJSON represents the subset of package.json we care about
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func runInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	frozenLockfile := fs.Bool("frozen-lockfile", false, "fail if lockfile is out of sync")
	linkType := fs.String("link-type", "", "override link strategy: hardlink|symlink|copy")
	storeDir := fs.String("store-dir", "", "override store directory")
	fs.Parse(args)

	start := time.Now()
	logger.Banner("mypm", Version)

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config error: %v", err)
	}
	if *linkType != "" {
		cfg.LinkType = config.LinkType(*linkType)
	}
	if *storeDir != "" {
		cfg.StoreDir = *storeDir
	}

	// Find project root (directory containing package.json)
	projectRoot, err := findProjectRoot()
	if err != nil {
		logger.Fatal("%v", err)
	}
	logger.Info("project root: %s", projectRoot)

	// Read package.json
	pkgJSON, err := readPackageJSON(filepath.Join(projectRoot, "package.json"))
	if err != nil {
		logger.Fatal("cannot read package.json: %v", err)
	}

	if *frozenLockfile {
		logger.Info("--frozen-lockfile: lockfile validation is planned for Phase 3")
	}

	// Initialise store
	s, err := store.New(cfg)
	if err != nil {
		logger.Fatal("store init failed: %v", err)
	}

	// Merge all deps
	allDeps := mergeDeps(pkgJSON.Dependencies, pkgJSON.DevDependencies)
	if len(allDeps) == 0 {
		logger.Warn("no dependencies found in package.json")
		return
	}

	logger.Info("found %d top-level dependencies", len(allDeps))
	logger.Info("store: %s", cfg.StoreDir)
	fmt.Println()

	// Phase 1 MVP: link packages that already exist in node_modules into the store
	// Full resolver + fetcher comes in Phase 3
	nodeModules := filepath.Join(projectRoot, "node_modules")
	lnkr := linker.New(cfg)

	linked, fromStore, skipped := 0, 0, 0

	for pkgName := range allDeps {
		srcDir := filepath.Join(nodeModules, pkgName)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			logger.Warn("%-30s not in node_modules (run npm install first, then mypm link)", pkgName)
			skipped++
			continue
		}

		// Check if already in store
		version := allDeps[pkgName]
		if s.HasPackage(pkgName, version) {
			fromStore++
			logger.Success("%-30s %s (cached)", pkgName, version)
			continue
		}

		// Walk package files and populate store
		if err := populateStore(s, cfg, pkgName, version, srcDir); err != nil {
			logger.Error("%-30s store write failed: %v", pkgName, err)
			continue
		}

		// Link from store back (demonstrates the link path)
		result := lnkr.LinkPackage(nodeModules, pkgName, cfg.PackageDir(pkgName, version))
		if result.Err != nil {
			logger.Error("%-30s link failed: %v", pkgName, result.Err)
			continue
		}

		linked++
		logger.Success("%-30s %s [%s]", pkgName, version, result.Strategy)
	}

	// Store status
	status, _ := s.Status()
	storeMB := float64(status.TotalBytes) / 1024 / 1024

	logger.Summary(linked, fromStore, skipped, storeMB, time.Since(start))
}

// populateStore walks a package directory and writes each file to the CAS store
func populateStore(s *store.Store, cfg *config.Config, name, version, srcDir string) error {
	pkgDir := cfg.PackageDir(name, version)
	if err := os.MkdirAll(pkgDir, 0755); err != nil {
		return err
	}

	var files []string
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(srcDir, path)
		destPath := filepath.Join(pkgDir, rel)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		// Write to file store (CAS) and also copy to package dir
		if _, err := s.WriteFile(path); err != nil {
			return fmt.Errorf("storing %s: %w", rel, err)
		}
		// Copy to package store dir so linker can hard-link from there
		if err := copyToPackageStore(path, destPath); err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return err
	}

	return s.WriteMeta(&store.PackageMeta{
		Name:    name,
		Version: version,
		Files:   files,
	})
}

func copyToPackageStore(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = copyIO(in, out)
	return err
}

// copyIO is a minimal io.Copy wrapper
func copyIO(r interface{ Read([]byte) (int, error) }, w interface{ Write([]byte) (int, error) }) (int64, error) {
	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, err := r.Read(buf)
		if n > 0 {
			written, werr := w.Write(buf[:n])
			total += int64(written)
			if werr != nil {
				return total, werr
			}
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return total, err
		}
	}
	return total, nil
}

func mergeDeps(deps ...map[string]string) map[string]string {
	merged := make(map[string]string)
	for _, d := range deps {
		for k, v := range d {
			merged[k] = v
		}
	}
	return merged
}

func readPackageJSON(path string) (*PackageJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no package.json found in current directory or any parent")
}
