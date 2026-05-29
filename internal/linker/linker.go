package linker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ripudaman/mypm/internal/config"
	"github.com/ripudaman/mypm/internal/logger"
)

// Result reports what happened when linking a package
type Result struct {
	Package  string
	Strategy string // "hardlink", "symlink", or "copy"
	Err      error
}

// Linker creates project node_modules entries pointing to the store
type Linker struct {
	cfg *config.Config
}

func New(cfg *config.Config) *Linker {
	return &Linker{cfg: cfg}
}

// LinkPackage links all files of a store package into the project's node_modules.
// Strategy priority: hardlink → symlink → copy (with OS-aware fallback)
func (l *Linker) LinkPackage(nodeModulesDir, pkgName, pkgStoreDir string) *Result {
	result := &Result{Package: pkgName}

	targetDir := filepath.Join(nodeModulesDir, pkgName)

	err := filepath.Walk(pkgStoreDir, func(srcPath string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(pkgStoreDir, srcPath)
		if err != nil {
			return err
		}

		if rel == "meta.json" {
			return nil
		}

		destPath := filepath.Join(targetDir, rel)

		if info.IsDir() {
			return os.MkdirAll(destPath, 0755)
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}

		strategy, linkErr := l.linkFile(srcPath, destPath)
		if result.Strategy == "" {
			result.Strategy = strategy
		}
		return linkErr
	})

	result.Err = err
	return result
}

// linkFile tries the configured strategy with graceful fallback.
// Cross-device detection is handled per-OS via isCrossDevice (see linker_unix.go / linker_windows.go)
func (l *Linker) linkFile(src, dest string) (string, error) {
	os.Remove(dest)

	switch l.cfg.LinkType {
	case config.LinkTypeCopy:
		return "copy", copyFile(src, dest)

	case config.LinkTypeSymlink:
		if err := os.Symlink(src, dest); err == nil {
			return "symlink", nil
		}
		logger.Warn("symlink failed for %s, falling back to copy", filepath.Base(src))
		return "copy", copyFile(src, dest)

	default: // hardlink (preferred)
		if err := os.Link(src, dest); err == nil {
			return "hardlink", nil
		} else if isCrossDevice(err) {
			logger.Warn("cross-filesystem detected — falling back to symlink")
			l.cfg.LinkType = config.LinkTypeSymlink
			if err2 := os.Symlink(src, dest); err2 == nil {
				return "symlink", nil
			}
			return "copy", copyFile(src, dest)
		} else {
			logger.Warn("hardlink failed (%v), falling back to symlink", err)
			if err2 := os.Symlink(src, dest); err2 == nil {
				return "symlink", nil
			}
			return "copy", copyFile(src, dest)
		}
	}
}

func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("copy open src: %w", err)
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("copy create dest: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy data: %w", err)
	}
	return out.Sync()
}

// CleanNodeModules removes node_modules in the given project root
func CleanNodeModules(projectRoot string) error {
	nm := filepath.Join(projectRoot, "node_modules")
	if _, err := os.Stat(nm); os.IsNotExist(err) {
		return nil
	}
	return os.RemoveAll(nm)
}
