package fetcher

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// RegistryPackage is the full metadata returned by registry.npmjs.org/<name>
type RegistryPackage struct {
	Name     string                     `json:"name"`
	Versions map[string]RegistryVersion `json:"versions"`
	DistTags map[string]string          `json:"dist-tags"` // "latest" → "18.2.0"
}

// RegistryVersion is the metadata for one specific version
type RegistryVersion struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Dependencies map[string]string `json:"dependencies"`
	PeerDeps     map[string]string `json:"peerDependencies"`
	Engines      json.RawMessage   `json:"engines"`
	Dist         struct {
		Tarball   string `json:"tarball"`   // download URL for the .tgz
		Integrity string `json:"integrity"` // sha512-<base64>
		Shasum    string `json:"shasum"`    // legacy SHA-1
	} `json:"dist"`
}

// RegistryClient talks to the npm registry
type RegistryClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewRegistryClient creates a client pointing at the given registry URL
func NewRegistryClient(registryURL string) *RegistryClient {
	return &RegistryClient{
		baseURL: registryURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetPackage fetches full package metadata (all versions) from the registry.
// GET https://registry.npmjs.org/<name>
func (c *RegistryClient) GetPackage(name string) (*RegistryPackage, error) {
	url := fmt.Sprintf("%s/%s", c.baseURL, name)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("registry request failed for %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("package %q not found in registry", name)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d for %s", resp.StatusCode, name)
	}

	var pkg RegistryPackage
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return nil, fmt.Errorf("parsing registry response for %s: %w", name, err)
	}

	return &pkg, nil
}

// GetVersion fetches metadata for a specific version only.
// GET https://registry.npmjs.org/<name>/<version>
// Much lighter than fetching all versions.
func (c *RegistryClient) GetVersion(name, version string) (*RegistryVersion, error) {
	url := fmt.Sprintf("%s/%s/%s", c.baseURL, name, version)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("registry request failed for %s@%s: %w", name, version, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("package %s@%s not found in registry", name, version)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("registry returned %d for %s@%s", resp.StatusCode, name, version)
	}

	var v RegistryVersion
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, fmt.Errorf("parsing registry response for %s@%s: %w", name, version, err)
	}

	return &v, nil
}

// LatestVersion returns the version string tagged as "latest" for a package
func (c *RegistryClient) LatestVersion(name string) (string, error) {
	pkg, err := c.GetPackage(name)
	if err != nil {
		return "", err
	}
	latest, ok := pkg.DistTags["latest"]
	if !ok {
		return "", fmt.Errorf("no latest tag for %s", name)
	}
	return latest, nil
}
