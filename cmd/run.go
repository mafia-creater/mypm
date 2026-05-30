package cmd

import (
	"errors"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mafia-creater/mypm/internal/logger"
)

// ─── RUN ───────────────────────────────────────────────────────────────────────

func runRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	fs.Parse(args)
	rest := fs.Args()
	if len(rest) == 0 {
		logger.Error("usage: mypm run <script> [args...]")
		os.Exit(1)
	}
	runScript(rest[0], rest[1:])
}

// ─── START ─────────────────────────────────────────────────────────────────────

func runStart(args []string) {
	runScript("start", args)
}

// ─── TEST ──────────────────────────────────────────────────────────────────────

func runTest(args []string) {
	runScript("test", args)
}

// ─── BUILD ─────────────────────────────────────────────────────────────────────

func runBuild(args []string) {
	runScript("build", args)
}

func runScript(name string, args []string) {
	logger.Banner("mypm", Version)

	projectRoot, err := findProjectRoot()
	if err != nil {
		logger.Fatal("%v", err)
	}

	pkgJSON, err := readPackageJSON(filepath.Join(projectRoot, "package.json"))
	if err != nil {
		logger.Fatal("cannot read package.json: %v", err)
	}

	if pkgJSON.Scripts == nil {
		logger.Error("no scripts found in package.json")
		os.Exit(1)
	}

	cmdStr, ok := pkgJSON.Scripts[name]
	if !ok || strings.TrimSpace(cmdStr) == "" {
		logger.Error("script %q not found in package.json", name)
		os.Exit(1)
	}

	if len(args) > 0 {
		cmdStr = cmdStr + " " + strings.Join(args, " ")
	}

	cmd := buildShellCommand(cmdStr)
	cmd.Dir = projectRoot
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = envWithLocalBin(os.Environ(), filepath.Join(projectRoot, "node_modules", ".bin"))

	if err := cmd.Run(); err != nil {
		code := scriptExitCode(err)
		if code > 0 {
			os.Exit(code)
		}
		logger.Fatal("run failed: %v", err)
	}
}

func buildShellCommand(cmdStr string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd", "/C", cmdStr)
	}
	return exec.Command("sh", "-c", cmdStr)
}

func scriptExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitErr.ProcessState != nil {
			return exitErr.ProcessState.ExitCode()
		}
	}
	return 1
}

func envWithLocalBin(env []string, binDir string) []string {
	updated := make([]string, 0, len(env)+1)
	pathValue := ""
	pathIndex := -1

	for i, entry := range env {
		if strings.HasPrefix(entry, "PATH=") {
			pathIndex = i
			pathValue = strings.TrimPrefix(entry, "PATH=")
			continue
		}
		updated = append(updated, entry)
	}

	if pathValue == "" {
		pathValue = os.Getenv("PATH")
	}
	if pathValue != "" {
		pathValue = binDir + string(os.PathListSeparator) + pathValue
	} else {
		pathValue = binDir
	}

	updated = append(updated, "PATH="+pathValue)
	if pathIndex >= 0 {
		return updated
	}
	return updated
}
