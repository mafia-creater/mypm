package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mafia-creater/mypm/internal/config"
	"github.com/mafia-creater/mypm/internal/fetcher"
	"github.com/mafia-creater/mypm/internal/logger"
	"github.com/mafia-creater/mypm/internal/resolver"
	"github.com/mafia-creater/mypm/internal/store"
)

type pkgBinJSON struct {
	Name string          `json:"name"`
	Bin  json.RawMessage `json:"bin"`
}

// runDlx downloads a package into the store (if needed) and executes its bin.
func runDlx(args []string) {
	fs := flag.NewFlagSet("dlx", flag.ExitOnError)
	fs.Parse(args)

	rest := fs.Args()
	if len(rest) == 0 {
		logger.Error("usage: mypm dlx <package>[@version] [args...]")
		os.Exit(1)
	}

	pkgArg := rest[0]
	cmdArgs := rest[1:]

	logger.Banner("mypm", Version)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("config: %v", err)
	}

	s, err := store.New(cfg)
	if err != nil {
		logger.Fatal("store: %v", err)
	}

	name, rangeStr := splitPackageArg(pkgArg, cfg.Registry)
	logger.Info("resolving %s@%s ...", name, rangeStr)

	res := resolver.New(cfg.Registry)
	result, err := res.Resolve(map[string]string{name: rangeStr}, nil)
	if err != nil {
		logger.Fatal("resolution failed: %v", err)
	}

	for _, w := range result.Warnings {
		logger.Warn("%s", w)
	}

	// Fetch all resolved packages
	f := fetcher.New(cfg, s)
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

	resolvedVersion := result.Lockfile.Dependencies[name]
	if resolvedVersion == "" {
		logger.Fatal("could not determine resolved version for %s", name)
	}

	pkgDir := cfg.PackageDir(name, resolvedVersion)
	binName, binPath, err := resolvePackageBin(pkgDir, name)
	if err != nil {
		logger.Fatal("bin resolution failed: %v", err)
	}

	nodePath, err := exec.LookPath("node")
	if err != nil {
		logger.Fatal("node not found in PATH")
	}

	logger.Info("running %s ...", binName)
	if err := runNodeScript(nodePath, binPath, cmdArgs); err != nil {
		code := exitCode(err)
		if code > 0 {
			os.Exit(code)
		}
		logger.Fatal("run failed: %v", err)
	}
}

// runCreate maps `create <name>` to a create-* package and runs it.
func runCreate(args []string) {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	fs.Parse(args)
	rest := fs.Args()
	if len(rest) == 0 {
		logger.Error("usage: mypm create <name>[@version] [args...]")
		os.Exit(1)
	}
	pkgArg := rest[0]
	mapped := mapCreatePackage(pkgArg)
	newArgs := append([]string{mapped}, rest[1:]...)
	runDlx(newArgs)
}

func mapCreatePackage(arg string) string {
	name, rangeStr := splitPackageArg(arg, "")
	mappedName := name

	if strings.HasPrefix(name, "@") {
		// @scope/name -> @scope/create-name
		parts := strings.SplitN(name[1:], "/", 2)
		if len(parts) == 2 {
			scope, pkg := parts[0], parts[1]
			if !strings.HasPrefix(pkg, "create-") {
				pkg = "create-" + pkg
			}
			mappedName = "@" + scope + "/" + pkg
		}
	} else {
		if name == "next" {
			mappedName = "create-next-app"
		} else if !strings.HasPrefix(name, "create-") {
			mappedName = "create-" + name
		}
	}

	if rangeStr == "" {
		rangeStr = "latest"
	}
	return mappedName + "@" + rangeStr
}

func resolvePackageBin(pkgDir, pkgName string) (string, string, error) {
	pkgPath := filepath.Join(pkgDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return "", "", err
	}

	var pj pkgBinJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return "", "", err
	}

	if len(pj.Bin) == 0 || string(pj.Bin) == "null" {
		return "", "", fmt.Errorf("package has no bin field")
	}

	// bin can be a string or object
	var binStr string
	if err := json.Unmarshal(pj.Bin, &binStr); err == nil {
		binName := binNameFromPackage(pkgName)
		return binName, filepath.Join(pkgDir, filepath.FromSlash(binStr)), nil
	}

	var binMap map[string]string
	if err := json.Unmarshal(pj.Bin, &binMap); err != nil {
		return "", "", err
	}

	if len(binMap) == 0 {
		return "", "", fmt.Errorf("package has empty bin map")
	}

	baseName := binNameFromPackage(pkgName)
	if rel, ok := binMap[baseName]; ok {
		return baseName, filepath.Join(pkgDir, filepath.FromSlash(rel)), nil
	}
	if rel, ok := binMap[pkgName]; ok {
		return pkgName, filepath.Join(pkgDir, filepath.FromSlash(rel)), nil
	}

	// Fallback: use first entry for single-bin packages
	if len(binMap) == 1 {
		for k, v := range binMap {
			return k, filepath.Join(pkgDir, filepath.FromSlash(v)), nil
		}
	}

	return "", "", fmt.Errorf("could not determine which bin to run")
}

func binNameFromPackage(name string) string {
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(name[1:], "/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return name
}

func runNodeScript(nodePath, scriptPath string, args []string) error {
	cmd := exec.Command(nodePath, append([]string{scriptPath}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ProcessState != nil {
			return exitErr.ProcessState.ExitCode()
		}
	}
	return 1
}
