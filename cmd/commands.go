package cmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mafia-creater/mypm/internal/config"
	"github.com/mafia-creater/mypm/internal/fetcher"
	"github.com/mafia-creater/mypm/internal/linker"
	"github.com/mafia-creater/mypm/internal/logger"
	"github.com/mafia-creater/mypm/internal/resolver"
	"github.com/mafia-creater/mypm/internal/store"
)

// ─── ADD ──────────────────────────────────────────────────────────────────────

func runAdd(args []string) {
	fs := flag.NewFlagSet("add", flag.ExitOnError)
	dev := fs.Bool("D", false, "add as devDependency")
	fs.Parse(args)

	packages := fs.Args()
	if len(packages) == 0 {
		logger.Error("usage: mypm add <package>[@version] [-D]")
		os.Exit(1)
	}

	logger.Banner("mypm", Version)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config: %v", err)
	}

	projectRoot, err := findProjectRoot()
	if err != nil {
		logger.Fatal("%v", err)
	}

	// Read package.json
	pkgPath := projectRoot + "/package.json"
	pkgJSON, err := readPackageJSON(pkgPath)
	if err != nil {
		logger.Fatal("cannot read package.json: %v", err)
	}
	if pkgJSON.Dependencies == nil {
		pkgJSON.Dependencies = make(map[string]string)
	}
	if pkgJSON.DevDependencies == nil {
		pkgJSON.DevDependencies = make(map[string]string)
	}

	s, err := store.New(cfg)
	if err != nil {
		logger.Fatal("store: %v", err)
	}

	res := resolver.New(cfg.Registry)
	f := fetcher.New(cfg, s)
	lnkr := linker.New(cfg)

	nodeModules := projectRoot + "/node_modules"
	os.MkdirAll(nodeModules, 0755)

	for _, pkg := range packages {
		// Split "react@18" or "react@^18.0.0" or "react@latest" or just "react"
		name, rangeStr := splitPackageArg(pkg, cfg.Registry)

		logger.Info("resolving %s@%s ...", name, rangeStr)

		// Resolve just this one package (and its transitive deps)
		deps := map[string]string{name: rangeStr}
		result, err := res.Resolve(deps, nil)
		if err != nil {
			logger.Error("resolution failed for %s: %v", name, err)
			continue
		}

		for _, w := range result.Warnings {
			logger.Warn("%s", w)
		}

		// Fetch all resolved packages
		var jobs []fetcher.FetchJob
		for _, p := range result.Lockfile.AllPackages() {
			jobs = append(jobs, fetcher.FetchJob{Name: p.Name, Version: p.Version})
		}
		fetchResults := f.FetchAll(jobs)
		for _, fr := range fetchResults {
			if fr.Err != nil {
				logger.Error("fetch %s: %v", fr.Name, fr.Err)
			}
		}

		// Link into node_modules
		for _, p := range result.Lockfile.AllPackages() {
			pkgStoreDir := cfg.PackageDir(p.Name, p.Version)
			lr := lnkr.LinkPackage(nodeModules, p.Name, pkgStoreDir)
			if lr.Err != nil {
				logger.Error("link %s: %v", p.Name, lr.Err)
			}
		}

		// Get the exact resolved version for the top-level package
		resolvedVersion := result.Lockfile.Dependencies[name]
		rangeToSave := "^" + resolvedVersion

		// Update package.json
		if *dev {
			pkgJSON.DevDependencies[name] = rangeToSave
			logger.Success("added %s@%s to devDependencies", name, rangeToSave)
		} else {
			pkgJSON.Dependencies[name] = rangeToSave
			logger.Success("added %s@%s to dependencies", name, rangeToSave)
		}
	}

	// Write updated package.json
	if err := writePackageJSON(pkgPath, pkgJSON); err != nil {
		logger.Fatal("writing package.json: %v", err)
	}

	// Rewrite lockfile with all current deps
	allResult, err := res.Resolve(pkgJSON.Dependencies, pkgJSON.DevDependencies)
	if err != nil {
		logger.Warn("could not update lockfile: %v", err)
		return
	}
	if err := allResult.Lockfile.Write(projectRoot); err != nil {
		logger.Warn("writing lockfile: %v", err)
	}

	fmt.Println()
}

// ─── REMOVE ───────────────────────────────────────────────────────────────────

func runRemove(args []string) {
	fs := flag.NewFlagSet("remove", flag.ExitOnError)
	fs.Parse(args)

	packages := fs.Args()
	if len(packages) == 0 {
		logger.Error("usage: mypm remove <package>")
		os.Exit(1)
	}

	logger.Banner("mypm", Version)

	projectRoot, err := findProjectRoot()
	if err != nil {
		logger.Fatal("%v", err)
	}

	pkgPath := projectRoot + "/package.json"
	pkgJSON, err := readPackageJSON(pkgPath)
	if err != nil {
		logger.Fatal("cannot read package.json: %v", err)
	}

	nodeModules := projectRoot + "/node_modules"

	for _, name := range packages {
		removed := false
		if _, ok := pkgJSON.Dependencies[name]; ok {
			delete(pkgJSON.Dependencies, name)
			removed = true
		}
		if _, ok := pkgJSON.DevDependencies[name]; ok {
			delete(pkgJSON.DevDependencies, name)
			removed = true
		}
		if !removed {
			logger.Warn("%s not found in package.json", name)
			continue
		}

		// Remove from node_modules
		pkgDir := nodeModules + "/" + name
		if err := os.RemoveAll(pkgDir); err != nil {
			logger.Warn("could not remove %s from node_modules: %v", name, err)
		} else {
			logger.Success("removed %s", name)
		}
	}

	// Write updated package.json
	if err := writePackageJSON(pkgPath, pkgJSON); err != nil {
		logger.Fatal("writing package.json: %v", err)
	}

	// Rewrite lockfile without removed packages
	cfg, _ := config.Load()
	res := resolver.New(cfg.Registry)
	allResult, err := res.Resolve(pkgJSON.Dependencies, pkgJSON.DevDependencies)
	if err != nil {
		logger.Warn("could not update lockfile: %v", err)
		return
	}
	if err := allResult.Lockfile.Write(projectRoot); err != nil {
		logger.Warn("writing lockfile: %v", err)
	}
	logger.Info("updated package.json and mypm.lock")
	fmt.Println()
}

// ─── STORE ────────────────────────────────────────────────────────────────────

func runStore(args []string) {
	if len(args) == 0 {
		fmt.Println("  usage: mypm store <path|status>")
		return
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config: %v", err)
	}

	switch args[0] {
	case "path":
		fmt.Println(cfg.StoreDir)
	case "status":
		runStoreStatus(cfg)
	default:
		logger.Error("unknown store subcommand: %s", args[0])
		os.Exit(1)
	}
}

func runStoreStatus(cfg *config.Config) {
	logger.Banner("mypm store", Version)

	s, err := store.New(cfg)
	if err != nil {
		logger.Fatal("store: %v", err)
	}

	status, err := s.Status()
	if err != nil {
		logger.Fatal("reading store status: %v", err)
	}

	storeMB := float64(status.TotalBytes) / 1024 / 1024

	fmt.Printf("  Store path   : %s\n", status.StorePath)
	fmt.Printf("  Packages     : %d\n", status.PackageCount)
	fmt.Printf("  Unique files : %.1f MB on disk\n", storeMB)
	fmt.Println()
	if storeMB > 0 {
		logger.Info("every new project sharing these packages costs ~0 extra disk space")
	} else {
		logger.Info("store is empty — run `mypm install` in a project to populate it")
	}
	fmt.Println()
}

// ─── DOCTOR ───────────────────────────────────────────────────────────────────

func runDoctor(args []string) {
	logger.Banner("mypm doctor", Version)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config: %v", err)
	}

	checks := []struct {
		label string
		fn    func(*config.Config) (string, bool)
	}{
		{"Store directory exists", checkStoreExists},
		{"Store directory writable", checkStoreWritable},
		{"Hard link support", checkHardLinkSupport},
		{"Config file present", checkConfigFile},
		{"Registry reachable", checkRegistry},
	}

	fmt.Println()
	allGood := true
	for _, c := range checks {
		msg, ok := c.fn(cfg)
		if ok {
			logger.Success("%-30s %s", c.label, msg)
		} else {
			logger.Error("%-30s %s", c.label, msg)
			allGood = false
		}
	}
	fmt.Println()

	if allGood {
		logger.Success("all checks passed — mypm is healthy")
	} else {
		logger.Warn("some checks failed — see above for details")
	}
	fmt.Println()
}

func checkStoreExists(cfg *config.Config) (string, bool) {
	if _, err := os.Stat(cfg.StoreDir); err == nil {
		return cfg.StoreDir, true
	}
	return "not found (will be created on first install)", true
}

func checkStoreWritable(cfg *config.Config) (string, bool) {
	if err := os.MkdirAll(cfg.StoreDir, 0755); err != nil {
		return fmt.Sprintf("cannot create: %v", err), false
	}
	testFile := cfg.StoreDir + "/.write-test"
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		return fmt.Sprintf("not writable: %v", err), false
	}
	os.Remove(testFile)
	return "writable", true
}

func checkHardLinkSupport(cfg *config.Config) (string, bool) {
	if err := os.MkdirAll(cfg.StoreDir, 0755); err != nil {
		return "cannot test (store dir not accessible)", false
	}
	srcPath := cfg.StoreDir + "/.link-test-src"
	dstPath := cfg.StoreDir + "/.link-test-dst"
	os.WriteFile(srcPath, []byte("test"), 0644)
	defer os.Remove(srcPath)
	defer os.Remove(dstPath)
	if err := os.Link(srcPath, dstPath); err != nil {
		return fmt.Sprintf("not supported (%v) — will use symlinks", err), true
	}
	return "supported (optimal)", true
}

func checkConfigFile(cfg *config.Config) (string, bool) {
	home, _ := os.UserHomeDir()
	rcPath := home + "/.mypmrc"
	if _, err := os.Stat(rcPath); err == nil {
		return "found at " + rcPath, true
	}
	return "not present (using defaults — create ~/.mypmrc to customise)", true
}

func checkRegistry(_ *config.Config) (string, bool) {
	cfg, _ := config.Load()
	return fmt.Sprintf("configured: %s (network check in Phase 3)", cfg.Registry), true
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// splitPackageArg splits "react@18" into ("react", "18")
// If no @ given, returns ("react", "latest")
func splitPackageArg(pkg, registry string) (name, rangeStr string) {
	// Handle scoped packages like @types/node@18
	if strings.HasPrefix(pkg, "@") {
		// "@scope/name@version" → find the second @
		rest := pkg[1:] // drop leading @
		if idx := strings.LastIndex(rest, "@"); idx >= 0 {
			return "@" + rest[:idx], rest[idx+1:]
		}
		return pkg, "latest"
	}

	if idx := strings.LastIndex(pkg, "@"); idx > 0 {
		return pkg[:idx], pkg[idx+1:]
	}
	return pkg, "latest"
}

// writePackageJSON writes an updated PackageJSON back to disk
func writePackageJSON(path string, pkg *PackageJSON) error {
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
