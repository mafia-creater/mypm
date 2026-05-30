package resolver

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/mafia-creater/mypm/internal/fetcher"
	"github.com/mafia-creater/mypm/internal/logger"
)

// Resolver builds the full dependency graph from package.json
type Resolver struct {
	registry *fetcher.RegistryClient

	// metadata cache: avoids hitting the registry twice for the same package
	mu    sync.Mutex
	cache map[string]*fetcher.RegistryPackage // name → full metadata
}

// New creates a Resolver using the given registry URL
func New(registryURL string) *Resolver {
	return &Resolver{
		registry: fetcher.NewRegistryClient(registryURL),
		cache:    make(map[string]*fetcher.RegistryPackage),
	}
}

// ResolveResult holds the output of a full resolution pass
type ResolveResult struct {
	Lockfile *Lockfile
	Warnings []string // peer dep issues, deprecated packages, etc.
}

// Resolve takes the top-level deps from package.json and produces a complete lockfile.
// It recursively resolves all transitive dependencies using a hoisting strategy.
func (r *Resolver) Resolve(deps map[string]string, devDeps map[string]string) (*ResolveResult, error) {
	lf := NewLockfile()
	result := &ResolveResult{Lockfile: lf}

	// visited guards against infinite recursion on circular deps
	visited := make(map[string]bool) // "name@version" → true

	// Resolve direct dependencies first
	for name, rangeStr := range deps {
		if err := r.resolvePackage(name, rangeStr, false, lf, visited, result, 0); err != nil {
			return nil, fmt.Errorf("resolving %s: %w", name, err)
		}
	}

	for name, rangeStr := range devDeps {
		if err := r.resolvePackage(name, rangeStr, true, lf, visited, result, 0); err != nil {
			return nil, fmt.Errorf("resolving dev dep %s: %w", name, err)
		}
	}

	return result, nil
}

// resolvePackage resolves one package (and its transitive deps) into the lockfile.
// depth is used for logging indentation and cycle detection safety.
func (r *Resolver) resolvePackage(
	name, rangeStr string,
	isDev bool,
	lf *Lockfile,
	visited map[string]bool,
	result *ResolveResult,
	depth int,
) error {
	if depth > 50 {
		// Safety net — legitimate dep trees rarely exceed 20 levels
		return fmt.Errorf("dependency depth limit reached for %s (possible circular dep)", name)
	}

	if shouldSkipRange(name, rangeStr) {
		return nil
	}

	// Fetch full package metadata (cached)
	meta, err := r.getPackageMeta(name)
	if err != nil {
		return err
	}

	resolvedVersion := ""
	if version, ok := resolveTag(rangeStr, meta); ok {
		resolvedVersion = version
	} else {
		// Parse the semver range
		semRange, err := ParseRange(rangeStr)
		if err != nil {
			logger.Warn("cannot parse range %q for %s, treating as latest", rangeStr, name)
			semRange, _ = ParseRange("*")
		}

		// Get sorted version list (descending — best match first)
		versions := sortedVersions(meta.Versions)

		// Pick best matching version
		resolvedVersion, err = semRange.BestMatch(versions)
		if err != nil {
			return fmt.Errorf("no version of %s satisfies %q (available: %s)",
				name, rangeStr, summarizeVersions(versions))
		}
	}

	key := LockKey(name, resolvedVersion)

	// Already resolved — hoisting in action:
	// if this exact version is already in the lockfile, we're done.
	// The linker will place it at the top level for all packages to share.
	if lf.Has(name, resolvedVersion) {
		return nil
	}

	// Circular dep guard
	if visited[key] {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("circular dependency detected: %s@%s", name, resolvedVersion))
		return nil
	}
	visited[key] = true

	vMeta, ok := meta.Versions[resolvedVersion]
	if !ok {
		return fmt.Errorf("version %s not found in metadata for %s", resolvedVersion, name)
	}

	// Peer dep warnings
	for peerName, peerRange := range vMeta.PeerDeps {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("peer dep: %s requires %s@%s", name, peerName, peerRange))
	}

	// Add to lockfile
	lf.Add(&LockedPackage{
		Name:         name,
		Version:      resolvedVersion,
		Resolved:     vMeta.Dist.Tarball,
		Integrity:    vMeta.Dist.Integrity,
		Dependencies: vMeta.Dependencies,
		Dev:          isDev,
	})

	logger.Success("%-30s %s", name, resolvedVersion)

	// Recurse into transitive deps
	for depName, depRange := range vMeta.Dependencies {
		if err := r.resolvePackage(depName, depRange, isDev, lf, visited, result, depth+1); err != nil {
			// Non-fatal: warn and continue rather than aborting the whole install
			logger.Warn("skipping transitive dep %s of %s: %v", depName, name, err)
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("could not resolve %s (dep of %s): %v", depName, name, err))
		}
	}

	return nil
}

func shouldSkipRange(name, rangeStr string) bool {
	switch {
	case strings.HasPrefix(rangeStr, "file:"):
		logger.Warn("%s: local file references not supported", name)
		return true
	case strings.HasPrefix(rangeStr, "git+"),
		strings.HasPrefix(rangeStr, "github:"),
		(strings.Count(rangeStr, "/") == 1 && !strings.HasPrefix(rangeStr, "@")):
		logger.Warn("%s: git references not supported", name)
		return true
	case strings.HasPrefix(rangeStr, "https://"), strings.HasPrefix(rangeStr, "http://"):
		logger.Warn("%s: URL references not supported", name)
		return true
	}
	return false
}

func resolveTag(tag string, meta *fetcher.RegistryPackage) (string, bool) {
	if meta == nil || meta.DistTags == nil {
		return "", false
	}
	version, ok := meta.DistTags[strings.TrimSpace(tag)]
	return version, ok
}

// getPackageMeta returns cached metadata or fetches from registry
func (r *Resolver) getPackageMeta(name string) (*fetcher.RegistryPackage, error) {
	r.mu.Lock()
	if cached, ok := r.cache[name]; ok {
		r.mu.Unlock()
		return cached, nil
	}
	r.mu.Unlock()

	meta, err := r.registry.GetPackage(name)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.cache[name] = meta
	r.mu.Unlock()

	return meta, nil
}

// sortedVersions returns version strings sorted descending (newest first)
func sortedVersions(versions map[string]fetcher.RegistryVersion) []string {
	keys := make([]string, 0, len(versions))
	for k := range versions {
		keys = append(keys, k)
	}

	sort.Slice(keys, func(i, j int) bool {
		vi, erri := ParseVersion(keys[i])
		vj, errj := ParseVersion(keys[j])
		if erri != nil || errj != nil {
			return keys[i] > keys[j]
		}
		return vi.Compare(vj) > 0 // descending
	})
	return keys
}

// summarizeVersions returns the last 5 versions as a short string for error messages
func summarizeVersions(versions []string) string {
	if len(versions) > 5 {
		versions = versions[:5]
	}
	return fmt.Sprintf("%v…", versions)
}
