package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mafia-creater/mypm/internal/config"
	"github.com/mafia-creater/mypm/internal/fetcher"
	"github.com/mafia-creater/mypm/internal/linker"
	"github.com/mafia-creater/mypm/internal/logger"
	"github.com/mafia-creater/mypm/internal/resolver"
	"github.com/mafia-creater/mypm/internal/store"
)

type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Scripts         map[string]string `json:"scripts"`
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

	projectRoot, err := findProjectRoot()
	if err != nil {
		logger.Fatal("%v", err)
	}
	logger.Info("project root: %s", projectRoot)

	pkgJSON, err := readPackageJSON(filepath.Join(projectRoot, "package.json"))
	if err != nil {
		logger.Fatal("cannot read package.json: %v", err)
	}

	s, err := store.New(cfg)
	if err != nil {
		logger.Fatal("store init failed: %v", err)
	}

	// ── Step 1: check for existing lockfile ─────────────────────────────────
	lf, err := resolver.ReadLockfile(projectRoot)
	if err != nil {
		logger.Fatal("reading lockfile: %v", err)
	}

	if lf != nil && *frozenLockfile {
		logger.Info("using existing lockfile (--frozen-lockfile)")
	} else if lf == nil || lockfileOutOfSync(lf, pkgJSON) {
		// ── Step 2: resolve ───────────────────────────────────────────────
		fmt.Println()
		logger.Step(1, 3, "resolving dependency tree...")
		fmt.Println()

		res := resolver.New(cfg.Registry)
		result, err := res.Resolve(pkgJSON.Dependencies, pkgJSON.DevDependencies)
		if err != nil {
			logger.Fatal("resolution failed: %v", err)
		}

		// Print warnings (peer deps etc.)
		for _, w := range result.Warnings {
			logger.Warn("%s", w)
		}

		lf = result.Lockfile
		if err := lf.Write(projectRoot); err != nil {
			logger.Fatal("writing lockfile: %v", err)
		}
		logger.Info("wrote %s (%d packages)", resolver.LockfileName, len(lf.Packages))
	} else {
		logger.Info("lockfile up to date — skipping resolution")
	}

	// ── Step 3: fetch ────────────────────────────────────────────────────────
	fmt.Println()
	logger.Step(2, 3, "fetching packages (concurrency: %d)...", cfg.Concurrency)
	fmt.Println()

	f := fetcher.New(cfg, s)
	var jobs []fetcher.FetchJob
	for _, pkg := range lf.AllPackages() {
		jobs = append(jobs, fetcher.FetchJob{
			Name:    pkg.Name,
			Version: pkg.Version,
		})
	}

	fetchResults := f.FetchAll(jobs)
	downloaded, fromCache, failed := 0, 0, 0
	for _, r := range fetchResults {
		if r.Err != nil {
			logger.Error("%-30s %v", r.Name, r.Err)
			failed++
			continue
		}
		if r.FromCache {
			fromCache++
		} else {
			downloaded++
		}
	}
	logger.Info("%d downloaded, %d from cache, %d failed", downloaded, fromCache, failed)

	// ── Step 4: link ─────────────────────────────────────────────────────────
	fmt.Println()
	logger.Step(3, 3, "linking into node_modules...")
	fmt.Println()

	nodeModules := filepath.Join(projectRoot, "node_modules")
	if err := os.MkdirAll(nodeModules, 0755); err != nil {
		logger.Fatal("cannot create node_modules: %v", err)
	}

	lnkr := linker.New(cfg)
	linked := 0
	for _, pkg := range lf.AllPackages() {
		pkgStoreDir := cfg.PackageDir(pkg.Name, pkg.Version)
		res := lnkr.LinkPackage(nodeModules, pkg.Name, pkgStoreDir)
		if res.Err != nil {
			logger.Error("link %-28s %v", pkg.Name, res.Err)
			continue
		}
		linked++
	}

	// ── Summary ──────────────────────────────────────────────────────────────
	status, _ := s.Status()
	storeMB := float64(status.TotalBytes) / 1024 / 1024
	logger.Summary(downloaded+fromCache, fromCache, failed, storeMB, time.Since(start))

	if failed > 0 {
		os.Exit(1)
	}
}

// lockfileOutOfSync returns true if package.json has deps not present in the lockfile
func lockfileOutOfSync(lf *resolver.Lockfile, pkg *PackageJSON) bool {
	for name := range pkg.Dependencies {
		if _, ok := lf.Dependencies[name]; !ok {
			return true
		}
	}
	for name := range pkg.DevDependencies {
		if _, ok := lf.Dependencies[name]; !ok {
			return true
		}
	}
	return false
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
