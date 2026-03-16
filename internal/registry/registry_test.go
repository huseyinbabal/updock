package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeReference(t *testing.T) {
	tests := []struct {
		input    string
		wantRepo string
		wantTag  string
	}{
		{"nginx", "library/nginx", "latest"},
		{"nginx:1.25", "library/nginx", "1.25"},
		{"nginx:latest", "library/nginx", "latest"},
		{"myuser/myapp", "myuser/myapp", "latest"},
		{"myuser/myapp:v2", "myuser/myapp", "v2"},
		{"docker.io/library/nginx", "library/nginx", "latest"},
		{"docker.io/myuser/myapp:v1", "myuser/myapp", "v1"},
		{"index.docker.io/library/redis", "library/redis", "latest"},
		{"index.docker.io/myuser/app:beta", "myuser/app", "beta"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			repo, tag := normalizeReference(tt.input)
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

func TestExtractRegistryHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"nginx", "docker.io"},
		{"nginx:1.25", "docker.io"},
		{"myuser/myapp", "docker.io"},
		{"ghcr.io/owner/repo", "ghcr.io"},
		{"ghcr.io/owner/repo:latest", "ghcr.io"},
		{"registry.example.com/myapp:v1", "registry.example.com"},
		{"localhost:5000/myapp", "docker.io"}, // colon without dot treated as user/repo
		{"my-registry.io/path/image@sha256:abc", "my-registry.io"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := extractRegistryHost(tt.input)
			if got != tt.want {
				t.Errorf("extractRegistryHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsDockerHub(t *testing.T) {
	trueHosts := []string{"", "docker.io", "index.docker.io", "registry-1.docker.io"}
	for _, h := range trueHosts {
		if !isDockerHub(h) {
			t.Errorf("expected %q to be Docker Hub", h)
		}
	}

	falseHosts := []string{"ghcr.io", "registry.example.com", "localhost:5000"}
	for _, h := range falseHosts {
		if isDockerHub(h) {
			t.Errorf("expected %q to NOT be Docker Hub", h)
		}
	}
}

func TestEncodeAuth(t *testing.T) {
	auth := AuthConfig{Username: "user", Password: "pass"}
	encoded := encodeAuth(auth)
	if encoded == "" {
		t.Fatal("expected non-empty encoded auth")
	}
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}
	var result AuthConfig
	if err := json.Unmarshal(decoded, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if result.Username != "user" || result.Password != "pass" {
		t.Errorf("unexpected decoded auth: %+v", result)
	}
}

func TestNewClient_NoConfig(t *testing.T) {
	c := NewClient("")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if len(c.authConfigs) != 0 {
		t.Errorf("expected 0 auth configs, got %d", len(c.authConfigs))
	}
}

func TestNewClient_WithConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	authStr := base64.StdEncoding.EncodeToString([]byte("myuser:mypass"))
	cfg := DockerConfig{
		Auths: map[string]AuthConfig{
			"https://index.docker.io/v1/": {Auth: authStr},
			"ghcr.io":                     {Username: "ghuser", Password: "ghpass"},
		},
	}
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	c := NewClient(path)
	if len(c.authConfigs) != 2 {
		t.Fatalf("expected 2 auth configs, got %d", len(c.authConfigs))
	}

	// Check Docker Hub credentials were decoded
	dhAuth := c.authConfigs["https://index.docker.io/v1/"]
	if dhAuth.Username != "myuser" || dhAuth.Password != "mypass" {
		t.Errorf("unexpected Docker Hub auth: %+v", dhAuth)
	}

	// Check ghcr.io (already has username/password)
	ghAuth := c.authConfigs["ghcr.io"]
	if ghAuth.Username != "ghuser" {
		t.Errorf("unexpected ghcr auth: %+v", ghAuth)
	}
}

func TestNewClient_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)

	c := NewClient(path)
	if c == nil {
		t.Fatal("expected non-nil client even with bad config")
	}
	if len(c.authConfigs) != 0 {
		t.Errorf("expected 0 auth configs for bad config, got %d", len(c.authConfigs))
	}
}

func TestGetRegistryAuth_Public(t *testing.T) {
	c := NewClient("")
	auth := c.GetRegistryAuth("nginx:latest")
	if auth != "" {
		t.Errorf("expected empty auth for public image, got %q", auth)
	}
}

func TestGetRegistryAuth_PrivateMatch(t *testing.T) {
	c := &Client{
		authConfigs: map[string]AuthConfig{
			"ghcr.io": {Username: "u", Password: "p"},
		},
	}
	auth := c.GetRegistryAuth("ghcr.io/owner/repo:latest")
	if auth == "" {
		t.Error("expected non-empty auth for matched registry")
	}
}

func TestGetRegistryAuth_DockerHubAlias(t *testing.T) {
	c := &Client{
		authConfigs: map[string]AuthConfig{
			"https://index.docker.io/v1/": {Username: "u", Password: "p"},
		},
	}
	auth := c.GetRegistryAuth("nginx:latest")
	if auth == "" {
		t.Error("expected auth for Docker Hub alias match")
	}
}

// ---------------------------------------------------------------------------
// httptest-based tests for registry HTTP methods
// ---------------------------------------------------------------------------

// newTestClient creates a registry Client with auth/registry URLs pointing to test servers.
func newTestClient(authURL, registryURL string) *Client {
	return &Client{
		httpClient:  &http.Client{},
		authConfigs: make(map[string]AuthConfig),
		authURL:     authURL,
		registryURL: registryURL,
	}
}

func TestGetToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "scope=repository:library/nginx:pull")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok-123"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	token, err := c.getToken(context.Background(), "library/nginx")
	assert.NoError(t, err)
	assert.Equal(t, "tok-123", token)
}

func TestGetToken_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	_, err := c.getToken(context.Background(), "library/nginx")
	assert.Error(t, err)
}

func TestGetToken_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	_, err := c.getToken(context.Background(), "library/nginx")
	assert.Error(t, err)
}

func TestGetRemoteDigest_Success(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer authSrv.Close()

	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer tok", r.Header.Get("Authorization"))
		w.Header().Set("Docker-Content-Digest", "sha256:abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer regSrv.Close()

	c := newTestClient(authSrv.URL, regSrv.URL)
	digest, err := c.GetRemoteDigest(context.Background(), "nginx:latest")
	assert.NoError(t, err)
	assert.Equal(t, "sha256:abc123", digest)
}

func TestGetRemoteDigest_ManifestError(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer authSrv.Close()

	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer regSrv.Close()

	c := newTestClient(authSrv.URL, regSrv.URL)
	_, err := c.GetRemoteDigest(context.Background(), "nginx:latest")
	assert.Error(t, err)
}

func TestGetRemoteDigest_NoDigestHeader(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer authSrv.Close()

	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // no Docker-Content-Digest header
	}))
	defer regSrv.Close()

	c := newTestClient(authSrv.URL, regSrv.URL)
	_, err := c.GetRemoteDigest(context.Background(), "nginx:latest")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no digest")
}

func TestGetRemoteDigest_AuthFailure(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer authSrv.Close()

	c := newTestClient(authSrv.URL, "http://unused")
	_, err := c.GetRemoteDigest(context.Background(), "nginx:latest")
	assert.Error(t, err)
}

func TestHasNewImage_NewAvailable(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer authSrv.Close()

	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Docker-Content-Digest", "sha256:newdigest")
		w.WriteHeader(http.StatusOK)
	}))
	defer regSrv.Close()

	c := newTestClient(authSrv.URL, regSrv.URL)
	hasNew, digest, err := c.HasNewImage(context.Background(), "nginx:latest", "sha256:olddigest")
	assert.NoError(t, err)
	assert.True(t, hasNew)
	assert.Equal(t, "sha256:newdigest", digest)
}

func TestHasNewImage_UpToDate(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer authSrv.Close()

	regSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Docker-Content-Digest", "sha256:samedigest")
		w.WriteHeader(http.StatusOK)
	}))
	defer regSrv.Close()

	c := newTestClient(authSrv.URL, regSrv.URL)
	hasNew, _, err := c.HasNewImage(context.Background(), "nginx:latest", "sha256:samedigest")
	assert.NoError(t, err)
	assert.False(t, hasNew)
}

func TestHasNewImage_Error(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer authSrv.Close()

	c := newTestClient(authSrv.URL, "http://unused")
	_, _, err := c.HasNewImage(context.Background(), "nginx:latest", "sha256:old")
	assert.Error(t, err)
}

func TestGetToken_WithCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "myuser", user)
		assert.Equal(t, "mypass", pass)
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "authed-tok"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "")
	c.authConfigs["https://index.docker.io/v1/"] = AuthConfig{Username: "myuser", Password: "mypass"}

	token, err := c.getToken(context.Background(), "library/nginx")
	assert.NoError(t, err)
	assert.Equal(t, "authed-tok", token)
}

func TestNewClient_MissingConfigFile(t *testing.T) {
	c := NewClient("/nonexistent/path/to/config.json")
	assert.NotNil(t, c)
	assert.Empty(t, c.authConfigs)
}

func TestLoadDockerConfig_InvalidBase64Auth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	cfg := DockerConfig{
		Auths: map[string]AuthConfig{
			"ghcr.io": {Auth: "not-valid-base64!!!"},
		},
	}
	data, _ := json.Marshal(cfg)
	_ = os.WriteFile(path, data, 0644)

	c := NewClient(path)
	assert.NotNil(t, c)
	// The auth entry is still stored but username/password won't be decoded
	auth := c.authConfigs["ghcr.io"]
	assert.Empty(t, auth.Username)
	assert.Empty(t, auth.Password)
}

func TestGetRegistryAuth_MultipleDockerHubAliases(t *testing.T) {
	c := &Client{
		authConfigs: map[string]AuthConfig{
			"docker.io": {Username: "user1", Password: "pass1"},
		},
	}
	// nginx is a Docker Hub image -> should match docker.io alias
	auth := c.GetRegistryAuth("nginx:latest")
	assert.NotEmpty(t, auth, "expected auth match via docker.io alias for Docker Hub image")
}

func TestNormalizeReference_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantRepo string
		wantTag  string
	}{
		{
			name:     "single word image defaults to library",
			input:    "alpine",
			wantRepo: "library/alpine",
			wantTag:  "latest",
		},
		{
			name:     "image with port-like tag",
			input:    "myuser/myapp:3.14",
			wantRepo: "myuser/myapp",
			wantTag:  "3.14",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, tag := normalizeReference(tt.input)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantTag, tag)
		})
	}
}
