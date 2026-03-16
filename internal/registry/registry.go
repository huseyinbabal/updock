// Package registry provides Docker image registry interaction for Updock.
//
// It supports checking for updated images by comparing local and remote
// digests using the Docker Registry HTTP API V2. This avoids pulling full
// images just to check for updates, significantly reducing bandwidth usage
// and registry rate limiting.
//
// # Authentication
//
// Public Docker Hub images are supported out of the box using anonymous
// token-based authentication. Private registries are supported by providing
// a Docker config.json file containing base64-encoded credentials.
//
// # How Digest Checking Works
//
//  1. Obtain an auth token for the repository (anonymous for public images,
//     credentials-based for private registries).
//  2. Send a HEAD request to the manifest endpoint with the image tag.
//  3. Compare the returned Docker-Content-Digest header with the local digest.
//  4. Only pull the image if the digests differ.
package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// Client checks remote registries for image updates.
// It maintains an HTTP client with appropriate timeouts and supports
// both public Docker Hub and private registry authentication.
type Client struct {
	httpClient  *http.Client
	authConfigs map[string]AuthConfig // registry hostname -> credentials

	// registryURL and authURL are overridable for testing.
	// In production these point to Docker Hub endpoints.
	registryURL string // default: https://registry-1.docker.io
	authURL     string // default: https://auth.docker.io
}

// AuthConfig holds credentials for a Docker registry.
// These are typically loaded from a Docker config.json file.
type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"` // base64(username:password)
}

// DockerConfig represents the structure of a Docker config.json file.
// This file is created by "docker login" and contains registry credentials.
type DockerConfig struct {
	Auths map[string]AuthConfig `json:"auths"`
}

// NewClient creates a new registry client. If configPath is non-empty and
// points to a valid Docker config.json file, private registry credentials
// are loaded from it.
func NewClient(configPath string) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		authConfigs: make(map[string]AuthConfig),
		registryURL: "https://registry-1.docker.io",
		authURL:     "https://auth.docker.io",
	}

	if configPath != "" {
		if err := c.loadDockerConfig(configPath); err != nil {
			log.Debugf("Could not load Docker config from %s: %v", configPath, err)
		}
	}

	return c
}

// loadDockerConfig reads credentials from a Docker config.json file.
// The file contains a map of registry hostnames to auth credentials.
//
// Example config.json:
//
//	{
//	  "auths": {
//	    "https://index.docker.io/v1/": {"auth": "base64(user:pass)"},
//	    "my-registry.example.com":     {"auth": "base64(user:pass)"}
//	  }
//	}
func (c *Client) loadDockerConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var cfg DockerConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("parsing docker config: %w", err)
	}

	for registry, auth := range cfg.Auths {
		// Decode base64 auth string into username:password
		if auth.Auth != "" && auth.Username == "" {
			decoded, err := base64.StdEncoding.DecodeString(auth.Auth)
			if err == nil {
				parts := strings.SplitN(string(decoded), ":", 2)
				if len(parts) == 2 {
					auth.Username = parts[0]
					auth.Password = parts[1]
				}
			}
		}
		c.authConfigs[registry] = auth
		log.Debugf("Loaded registry credentials for %s", registry)
	}

	return nil
}

// GetRegistryAuth returns a base64-encoded JSON auth config for use with
// the Docker pull API. Returns empty string for public registries.
func (c *Client) GetRegistryAuth(imageRef string) string {
	registryHost := extractRegistryHost(imageRef)

	// Check for exact match
	if auth, ok := c.authConfigs[registryHost]; ok {
		return encodeAuth(auth)
	}

	// Check Docker Hub variants
	dockerHubAliases := []string{
		"https://index.docker.io/v1/",
		"index.docker.io",
		"docker.io",
		"registry-1.docker.io",
	}

	if isDockerHub(registryHost) {
		for _, alias := range dockerHubAliases {
			if auth, ok := c.authConfigs[alias]; ok {
				return encodeAuth(auth)
			}
		}
	}

	return ""
}

// tokenResponse represents the OAuth2 token response from a Docker registry.
type tokenResponse struct {
	Token string `json:"token"`
}

// getToken obtains a bearer token from the Docker Hub authentication service.
// For public repositories, an anonymous token is returned.
// For private repositories, credentials from the config are used.
func (c *Client) getToken(ctx context.Context, repo string) (string, error) {
	url := fmt.Sprintf(
		"%s/token?service=registry.docker.io&scope=repository:%s:pull",
		c.authURL, repo,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}

	// Add basic auth for private Docker Hub repos
	for _, alias := range []string{
		"https://index.docker.io/v1/",
		"index.docker.io",
		"docker.io",
	} {
		if auth, ok := c.authConfigs[alias]; ok && auth.Username != "" {
			req.SetBasicAuth(auth.Username, auth.Password)
			break
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting auth token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth token request returned status %d", resp.StatusCode)
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	return tokenResp.Token, nil
}

// normalizeReference parses a Docker image reference into its repository
// and tag components.
//
// Examples:
//   - "nginx"                    -> ("library/nginx", "latest")
//   - "nginx:1.25"               -> ("library/nginx", "1.25")
//   - "myuser/myapp:v2"          -> ("myuser/myapp", "v2")
//   - "docker.io/library/nginx"  -> ("library/nginx", "latest")
func normalizeReference(ref string) (repo, tag string) {
	// Strip Docker Hub prefixes for normalization
	ref = strings.TrimPrefix(ref, "docker.io/")
	ref = strings.TrimPrefix(ref, "index.docker.io/")

	// Split tag from reference
	parts := strings.SplitN(ref, ":", 2)
	repo = parts[0]
	tag = "latest"
	if len(parts) == 2 {
		tag = parts[1]
	}

	// Official images use the "library/" prefix
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}

	return repo, tag
}

// GetRemoteDigest fetches the remote image digest using a HEAD request to the
// registry's manifest endpoint. This is a lightweight operation that does not
// download the image layers.
func (c *Client) GetRemoteDigest(ctx context.Context, imageRef string) (string, error) {
	repo, tag := normalizeReference(imageRef)

	log.Debugf("Checking remote digest for %s:%s", repo, tag)

	token, err := c.getToken(ctx, repo)
	if err != nil {
		return "", fmt.Errorf("getting auth token for %s: %w", repo, err)
	}

	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.registryURL, repo, tag)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	// Accept both Docker manifest v2 and OCI image index formats
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking remote manifest: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest request returned status %d for %s:%s", resp.StatusCode, repo, tag)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest returned for %s:%s", repo, tag)
	}

	return digest, nil
}

// HasNewImage compares the remote registry digest with the local digest to
// determine if a newer image is available. Returns true if an update exists,
// along with the remote digest.
func (c *Client) HasNewImage(ctx context.Context, imageRef string, localDigest string) (bool, string, error) {
	remoteDigest, err := c.GetRemoteDigest(ctx, imageRef)
	if err != nil {
		return false, "", err
	}

	// Compare digests - local digest may contain the full reference
	if remoteDigest != localDigest && !strings.Contains(localDigest, remoteDigest) {
		log.Infof("New image available for %s: remote=%s local=%s", imageRef, remoteDigest, localDigest)
		return true, remoteDigest, nil
	}

	return false, remoteDigest, nil
}

// extractRegistryHost extracts the registry hostname from an image reference.
// Returns "docker.io" for official Docker Hub images.
func extractRegistryHost(imageRef string) string {
	// Remove tag/digest
	ref := strings.Split(imageRef, ":")[0]
	ref = strings.Split(ref, "@")[0]

	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		return "docker.io"
	}

	// Check if first part looks like a hostname (contains dot or colon)
	if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
		return parts[0]
	}

	return "docker.io"
}

// isDockerHub checks if a registry hostname is Docker Hub.
func isDockerHub(host string) bool {
	return host == "" || host == "docker.io" || host == "index.docker.io" ||
		host == "registry-1.docker.io" || strings.Contains(host, "docker.io")
}

// encodeAuth encodes an AuthConfig as a base64 JSON string for the Docker API.
func encodeAuth(auth AuthConfig) string {
	jsonAuth, err := json.Marshal(auth)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(jsonAuth)
}
