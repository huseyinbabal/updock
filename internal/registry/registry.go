// Package registry provides Docker image registry interaction for Updock.
//
// It supports checking for updated images by comparing local and remote
// digests using the Docker Registry HTTP API V2. This avoids pulling full
// images just to check for updates, significantly reducing bandwidth usage
// and registry rate limiting.
//
// # Supported Registries
//
// The client uses the standard OCI distribution spec challenge-based auth flow,
// making it compatible with any V2 registry:
//
//   - Docker Hub (docker.io)
//   - GitHub Container Registry (ghcr.io)
//   - Amazon ECR
//   - Google GCR / Artifact Registry
//   - Azure ACR
//   - Self-hosted registries
//
// # How Digest Checking Works
//
//  1. Send an anonymous request to the registry's manifest endpoint.
//  2. If the registry returns 401, parse the WWW-Authenticate header to
//     discover the token endpoint (realm), service, and scope.
//  3. Obtain a bearer token from the discovered endpoint.
//  4. Retry the manifest request with the token.
//  5. Compare the Docker-Content-Digest header with the local digest.
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

	"github.com/huseyinbabal/updock/internal/logger"
)

// Client checks remote registries for image updates.
type Client struct {
	httpClient  *http.Client
	authConfigs map[string]AuthConfig // registry hostname -> credentials

	// scheme is the URL scheme for registry requests ("https" in production).
	// Overridable for testing with httptest servers.
	scheme string
}

// AuthConfig holds credentials for a Docker registry.
type AuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"` // base64(username:password)
}

// DockerConfig represents the structure of a Docker config.json file.
type DockerConfig struct {
	Auths map[string]AuthConfig `json:"auths"`
}

// NewClient creates a new registry client.
func NewClient(configPath string) *Client {
	c := &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		authConfigs: make(map[string]AuthConfig),
		scheme:      "https",
	}

	if configPath != "" {
		if err := c.loadDockerConfig(configPath); err != nil {
			logger.Debug().Msgf("Could not load Docker config from %s: %v", configPath, err)
		}
	}

	return c
}

// loadDockerConfig reads credentials from a Docker config.json file.
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
		logger.Debug().Msgf("Loaded registry credentials for %s", registry)
	}

	return nil
}

// GetRegistryAuth returns a base64-encoded JSON auth config for use with
// the Docker pull API. Returns empty string for public registries.
func (c *Client) GetRegistryAuth(imageRef string) string {
	registryHost := extractRegistryHost(imageRef)

	if auth, ok := c.authConfigs[registryHost]; ok {
		return encodeAuth(auth)
	}

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
	Token       string `json:"token"`
	AccessToken string `json:"access_token"`
}

// challenge holds parsed WWW-Authenticate header fields.
type challenge struct {
	Realm   string
	Service string
	Scope   string
}

// parseChallenge extracts realm, service, and scope from a WWW-Authenticate header.
//
//	Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:user/repo:pull"
func parseChallenge(header string) *challenge {
	header = strings.TrimPrefix(header, "Bearer ")
	header = strings.TrimPrefix(header, "bearer ")

	ch := &challenge{}
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		switch key {
		case "realm":
			ch.Realm = val
		case "service":
			ch.Service = val
		case "scope":
			ch.Scope = val
		}
	}
	return ch
}

// getToken performs OCI distribution spec challenge-based auth:
//  1. Hit the registry's /v2/ or manifest endpoint anonymously.
//  2. Parse the 401 WWW-Authenticate header for the token endpoint.
//  3. Request a bearer token from that endpoint.
//
// This works for Docker Hub, GHCR, ECR, GCR, and any OCI-compliant registry.
func (c *Client) getToken(ctx context.Context, registryHost, repo string) (string, error) {
	// Step 1: Discover auth challenge by hitting the manifest endpoint
	challengeURL := fmt.Sprintf("%s://%s/v2/", c.scheme, registryHost)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, challengeURL, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("discovering auth for %s: %w", registryHost, err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	// If 200, no auth needed
	if resp.StatusCode == http.StatusOK {
		return "", nil
	}

	if resp.StatusCode != http.StatusUnauthorized {
		return "", fmt.Errorf("unexpected status %d from %s", resp.StatusCode, challengeURL)
	}

	// Step 2: Parse WWW-Authenticate header
	wwwAuth := resp.Header.Get("WWW-Authenticate")
	if wwwAuth == "" {
		return "", fmt.Errorf("no WWW-Authenticate header from %s", registryHost)
	}

	ch := parseChallenge(wwwAuth)
	if ch.Realm == "" {
		return "", fmt.Errorf("no realm in WWW-Authenticate header from %s", registryHost)
	}

	// Step 3: Request token
	tokenURL := ch.Realm
	sep := "?"
	if strings.Contains(tokenURL, "?") {
		sep = "&"
	}
	if ch.Service != "" {
		tokenURL += sep + "service=" + ch.Service
		sep = "&"
	}
	scope := ch.Scope
	if scope == "" {
		scope = fmt.Sprintf("repository:%s:pull", repo)
	}
	tokenURL += sep + "scope=" + scope

	tokenReq, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		return "", err
	}

	// Add basic auth if we have credentials for this registry
	if auth := c.getCredentials(registryHost); auth != nil {
		tokenReq.SetBasicAuth(auth.Username, auth.Password)
	}

	tokenResp, err := c.httpClient.Do(tokenReq)
	if err != nil {
		return "", fmt.Errorf("requesting token from %s: %w", ch.Realm, err)
	}
	defer func() { _ = tokenResp.Body.Close() }()

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request returned %d from %s", tokenResp.StatusCode, ch.Realm)
	}

	var tok tokenResponse
	if err := json.NewDecoder(tokenResp.Body).Decode(&tok); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}

	if tok.Token != "" {
		return tok.Token, nil
	}
	return tok.AccessToken, nil
}

// getCredentials finds credentials for a registry host.
func (c *Client) getCredentials(host string) *AuthConfig {
	// Direct match
	if auth, ok := c.authConfigs[host]; ok && auth.Username != "" {
		return &auth
	}

	// Try with https:// prefix
	if auth, ok := c.authConfigs["https://"+host]; ok && auth.Username != "" {
		return &auth
	}

	// Docker Hub aliases
	if isDockerHub(host) {
		for _, alias := range []string{
			"https://index.docker.io/v1/",
			"index.docker.io",
			"docker.io",
		} {
			if auth, ok := c.authConfigs[alias]; ok && auth.Username != "" {
				return &auth
			}
		}
	}

	return nil
}

// parseReference parses a full image reference into registry host, repository, and tag.
//
// Examples:
//
//	"nginx"                         -> ("registry-1.docker.io", "library/nginx", "latest")
//	"nginx:1.25"                    -> ("registry-1.docker.io", "library/nginx", "1.25")
//	"ghcr.io/user/repo:latest"     -> ("ghcr.io", "user/repo", "latest")
//	"my-registry.com:5000/app:v2"  -> ("my-registry.com:5000", "app", "v2")
func parseReference(ref string) (host, repo, tag string) {
	// Strip tag/digest
	tagSep := strings.LastIndex(ref, ":")
	digestSep := strings.LastIndex(ref, "@")

	tag = "latest"
	base := ref

	if digestSep > 0 {
		base = ref[:digestSep]
	} else if tagSep > 0 {
		// Make sure the colon is for a tag, not a port in the host
		afterColon := ref[tagSep+1:]
		if !strings.Contains(afterColon, "/") {
			tag = afterColon
			base = ref[:tagSep]
		}
	}

	// Split host from repo
	parts := strings.SplitN(base, "/", 2)
	if len(parts) == 1 {
		// No slash: official Docker Hub image (e.g. "nginx")
		return "registry-1.docker.io", "library/" + parts[0], tag
	}

	// Check if first part is a hostname (contains dot or colon or is "localhost")
	firstPart := parts[0]
	if strings.Contains(firstPart, ".") || strings.Contains(firstPart, ":") || firstPart == "localhost" {
		host = firstPart
		repo = parts[1]
	} else {
		// Docker Hub user/repo (e.g. "myuser/myapp")
		host = "registry-1.docker.io"
		repo = base
	}

	// Docker Hub alias normalization
	if host == "docker.io" || host == "index.docker.io" {
		host = "registry-1.docker.io"
	}

	return host, repo, tag
}

// GetRemoteDigest fetches the remote image digest using the OCI distribution
// spec challenge-based auth flow. Works with any V2 registry.
func (c *Client) GetRemoteDigest(ctx context.Context, imageRef string) (string, error) {
	host, repo, tag := parseReference(imageRef)
	return c.getRemoteDigestDirect(ctx, host, repo, tag)
}

// getRemoteDigestDirect fetches the digest for a specific host/repo/tag combination.
// Extracted for testability so httptest servers can pass the host directly.
func (c *Client) getRemoteDigestDirect(ctx context.Context, host, repo, tag string) (string, error) {
	logger.Debug().Msgf("Checking remote digest for %s/%s:%s", host, repo, tag)

	token, err := c.getToken(ctx, host, repo)
	if err != nil {
		return "", fmt.Errorf("getting auth token for %s/%s: %w", host, repo, err)
	}

	url := fmt.Sprintf("%s://%s/v2/%s/manifests/%s", c.scheme, host, repo, tag)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Add("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	req.Header.Add("Accept", "application/vnd.oci.image.index.v1+json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("checking remote manifest: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("manifest request returned status %d for %s/%s:%s", resp.StatusCode, host, repo, tag)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", fmt.Errorf("no digest returned for %s/%s:%s", host, repo, tag)
	}

	return digest, nil
}

// HasNewImage compares the remote registry digest with the local digest to
// determine if a newer image is available.
func (c *Client) HasNewImage(ctx context.Context, imageRef string, localDigest string) (bool, string, error) {
	host, repo, tag := parseReference(imageRef)
	return c.hasNewImageDirect(ctx, host, repo, tag, localDigest)
}

// hasNewImageDirect compares digests for a specific host/repo/tag.
func (c *Client) hasNewImageDirect(ctx context.Context, host, repo, tag, localDigest string) (bool, string, error) {
	remoteDigest, err := c.getRemoteDigestDirect(ctx, host, repo, tag)
	if err != nil {
		return false, "", err
	}

	if remoteDigest != localDigest && !strings.Contains(localDigest, remoteDigest) {
		logger.Info().Msgf("New image available for %s/%s:%s: remote=%s local=%s", host, repo, tag, remoteDigest, localDigest)
		return true, remoteDigest, nil
	}

	return false, remoteDigest, nil
}

// extractRegistryHost extracts the registry hostname from an image reference.
func extractRegistryHost(imageRef string) string {
	host, _, _ := parseReference(imageRef)
	if host == "registry-1.docker.io" {
		return "docker.io"
	}
	return host
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
