package registry

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
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
