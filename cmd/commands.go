package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/mafia-creater/mypm/internal/config"
	"github.com/mafia-creater/mypm/internal/logger"
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

	logger.Banner("mypm add", Version)

	for _, pkg := range packages {
		if *dev {
			logger.Info("adding %s as devDependency", pkg)
		} else {
			logger.Info("adding %s as dependency", pkg)
		}
		// Phase 3: resolver + fetcher will handle this
		// For now, inform user
		logger.Warn("full fetcher not yet implemented — use `npm install %s` then `mypm install` to link", pkg)
	}

	fmt.Println()
	logger.Info("Phase 3 roadmap: mypm add will resolve + fetch + link without npm")
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

	logger.Banner("mypm remove", Version)

	projectRoot, err := findProjectRoot()
	if err != nil {
		logger.Fatal("%v", err)
	}

	pkgJSON, err := readPackageJSON(projectRoot + "/package.json")
	if err != nil {
		logger.Fatal("cannot read package.json: %v", err)
	}

	for _, pkg := range packages {
		if _, ok := pkgJSON.Dependencies[pkg]; ok {
			logger.Info("removing %s from dependencies", pkg)
		} else if _, ok := pkgJSON.DevDependencies[pkg]; ok {
			logger.Info("removing %s from devDependencies", pkg)
		} else {
			logger.Warn("%s not found in package.json", pkg)
			continue
		}
		// Phase 3: will edit package.json + re-link
		logger.Warn("package.json editing not yet implemented — manually remove %s, then run mypm install", pkg)
	}
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
		// Rough savings estimate: average Next.js project = 300MB
		// If we have N packages and store is X MB, savings per project = 300 - (X/N * deps)
		logger.Info("every new project that shares these packages costs ~0 extra disk space")
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
	return "not found (will be created on first install)", true // non-fatal
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
		return fmt.Sprintf("not supported on this filesystem (%v) — will use symlinks", err), true // warn not fail
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
	// Phase 3: do an actual HTTP HEAD to registry
	// For now just report the configured URL
	cfg, _ := config.Load()
	return fmt.Sprintf("configured: %s (network check in Phase 3)", cfg.Registry), true
}
