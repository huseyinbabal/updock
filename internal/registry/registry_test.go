package registry

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testHost extracts host from httptest server URL.
func testHost(srv *httptest.Server) string {
	return strings.TrimPrefix(srv.URL, "http://")
}

func TestParseReference(t *testing.T) {
	tests := []struct {
		input    string
		wantHost string
		wantRepo string
		wantTag  string
	}{
		{"nginx", "registry-1.docker.io", "library/nginx", "latest"},
		{"nginx:1.25", "registry-1.docker.io", "library/nginx", "1.25"},
		{"myuser/myapp", "registry-1.docker.io", "myuser/myapp", "latest"},
		{"myuser/myapp:v2", "registry-1.docker.io", "myuser/myapp", "v2"},
		{"ghcr.io/user/repo:latest", "ghcr.io", "user/repo", "latest"},
		{"ghcr.io/user/repo", "ghcr.io", "user/repo", "latest"},
		{"docker.io/library/nginx", "registry-1.docker.io", "library/nginx", "latest"},
		{"docker.io/myuser/app:v1", "registry-1.docker.io", "myuser/app", "v1"},
		{"index.docker.io/library/redis", "registry-1.docker.io", "library/redis", "latest"},
		{"registry.example.com/myapp:v1", "registry.example.com", "myapp", "v1"},
		{"localhost:5000/myapp:dev", "localhost:5000", "myapp", "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			host, repo, tag := parseReference(tt.input)
			assert.Equal(t, tt.wantHost, host)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantTag, tag)
		})
	}
}

func TestExtractRegistryHost(t *testing.T) {
	assert.Equal(t, "docker.io", extractRegistryHost("nginx"))
	assert.Equal(t, "ghcr.io", extractRegistryHost("ghcr.io/owner/repo"))
	assert.Equal(t, "registry.example.com", extractRegistryHost("registry.example.com/myapp:v1"))
}

func TestIsDockerHub(t *testing.T) {
	assert.True(t, isDockerHub("docker.io"))
	assert.True(t, isDockerHub("registry-1.docker.io"))
	assert.True(t, isDockerHub(""))
	assert.False(t, isDockerHub("ghcr.io"))
}

func TestEncodeAuth(t *testing.T) {
	encoded := encodeAuth(AuthConfig{Username: "u", Password: "p"})
	assert.NotEmpty(t, encoded)
	decoded, err := base64.URLEncoding.DecodeString(encoded)
	require.NoError(t, err)
	var a AuthConfig
	require.NoError(t, json.Unmarshal(decoded, &a))
	assert.Equal(t, "u", a.Username)
}

func TestNewClient_NoConfig(t *testing.T) {
	c := NewClient("")
	assert.NotNil(t, c)
	assert.Equal(t, "https", c.scheme)
}

func TestNewClient_WithConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	authStr := base64.StdEncoding.EncodeToString([]byte("user:pass"))
	data, _ := json.Marshal(DockerConfig{Auths: map[string]AuthConfig{
		"ghcr.io": {Auth: authStr},
	}})
	_ = os.WriteFile(path, data, 0644)
	c := NewClient(path)
	assert.Len(t, c.authConfigs, 1)
	assert.Equal(t, "user", c.authConfigs["ghcr.io"].Username)
}

func TestNewClient_BadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("nope"), 0644)
	c := NewClient(path)
	assert.Empty(t, c.authConfigs)
}

func TestGetRegistryAuth(t *testing.T) {
	c := NewClient("")
	assert.Empty(t, c.GetRegistryAuth("nginx"))

	c.authConfigs["ghcr.io"] = AuthConfig{Username: "u", Password: "p"}
	assert.NotEmpty(t, c.GetRegistryAuth("ghcr.io/owner/repo"))

	c2 := &Client{authConfigs: map[string]AuthConfig{"https://index.docker.io/v1/": {Username: "u", Password: "p"}}}
	assert.NotEmpty(t, c2.GetRegistryAuth("nginx"))
}

func TestParseChallenge(t *testing.T) {
	ch := parseChallenge(`Bearer realm="https://ghcr.io/token",service="ghcr.io",scope="repository:u/r:pull"`)
	assert.Equal(t, "https://ghcr.io/token", ch.Realm)
	assert.Equal(t, "ghcr.io", ch.Service)
	assert.Equal(t, "repository:u/r:pull", ch.Scope)

	ch2 := parseChallenge(`Bearer realm="https://auth.example.com/token"`)
	assert.Equal(t, "https://auth.example.com/token", ch2.Realm)
	assert.Empty(t, ch2.Service)

	ch3 := parseChallenge("")
	assert.Empty(t, ch3.Realm)
}

func TestGetCredentials(t *testing.T) {
	c := &Client{authConfigs: map[string]AuthConfig{
		"ghcr.io":                     {Username: "gh", Password: "p"},
		"https://index.docker.io/v1/": {Username: "dh", Password: "p"},
	}}
	assert.NotNil(t, c.getCredentials("ghcr.io"))
	assert.NotNil(t, c.getCredentials("registry-1.docker.io"))
	assert.Nil(t, c.getCredentials("unknown.io"))
}

// --- Full OCI flow tests via httptest ---

func TestGetToken_OCIChallenge(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.RawQuery, "scope=repository:lib/img:pull")
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok-123"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`",service="test",scope="repository:lib/img:pull"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	token, err := c.getToken(context.Background(), testHost(registrySrv), "lib/img")
	require.NoError(t, err)
	assert.Equal(t, "tok-123", token)
}

func TestGetToken_NoAuthNeeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	token, err := c.getToken(context.Background(), testHost(srv), "lib/img")
	require.NoError(t, err)
	assert.Empty(t, token)
}

func TestGetToken_AccessTokenField(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{AccessToken: "access-tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	token, err := c.getToken(context.Background(), testHost(registrySrv), "lib/img")
	require.NoError(t, err)
	assert.Equal(t, "access-tok", token)
}

func TestGetToken_WithCredentials(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		assert.True(t, ok)
		assert.Equal(t, "myuser", user)
		assert.Equal(t, "mypass", pass)
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "authed-tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`",service="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer registrySrv.Close()

	host := testHost(registrySrv)
	c := &Client{
		httpClient:  &http.Client{},
		authConfigs: map[string]AuthConfig{host: {Username: "myuser", Password: "mypass"}},
		scheme:      "http",
	}
	token, err := c.getToken(context.Background(), host, "lib/img")
	require.NoError(t, err)
	assert.Equal(t, "authed-tok", token)
}

func TestGetToken_ChallengeError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	_, err := c.getToken(context.Background(), testHost(srv), "lib/img")
	assert.Error(t, err)
}

func TestGetToken_NoWWWAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized) // no WWW-Authenticate header
	}))
	defer srv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	_, err := c.getToken(context.Background(), testHost(srv), "lib/img")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no WWW-Authenticate")
}

func TestGetToken_NoRealm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer service="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	_, err := c.getToken(context.Background(), testHost(srv), "lib/img")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no realm")
}

func TestGetToken_TokenEndpointFails(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`"`)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	_, err := c.getToken(context.Background(), testHost(registrySrv), "lib/img")
	assert.Error(t, err)
}

func TestGetRemoteDigest_Success(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`",service="test"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:abc123")
		w.WriteHeader(http.StatusOK)
	}))
	defer registrySrv.Close()

	host := testHost(registrySrv)
	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}

	// Call getToken + manifest directly since parseReference won't return our test host
	digest, err := c.getRemoteDigestDirect(context.Background(), host, "lib/img", "latest")
	require.NoError(t, err)
	assert.Equal(t, "sha256:abc123", digest)
}

func TestGetRemoteDigest_ManifestNotFound(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	_, err := c.getRemoteDigestDirect(context.Background(), testHost(registrySrv), "lib/img", "latest")
	assert.Error(t, err)
}

func TestGetRemoteDigest_NoDigest(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK) // no digest header
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	_, err := c.getRemoteDigestDirect(context.Background(), testHost(registrySrv), "lib/img", "latest")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no digest")
}

func TestHasNewImage_New(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:newdigest")
		w.WriteHeader(http.StatusOK)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	hasNew, digest, err := c.hasNewImageDirect(context.Background(), testHost(registrySrv), "lib/img", "latest", "sha256:olddigest")
	require.NoError(t, err)
	assert.True(t, hasNew)
	assert.Equal(t, "sha256:newdigest", digest)
}

func TestHasNewImage_UpToDate(t *testing.T) {
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{Token: "tok"})
	}))
	defer tokenSrv.Close()

	registrySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+tokenSrv.URL+`"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Docker-Content-Digest", "sha256:samedigest")
		w.WriteHeader(http.StatusOK)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	hasNew, _, err := c.hasNewImageDirect(context.Background(), testHost(registrySrv), "lib/img", "latest", "sha256:samedigest")
	require.NoError(t, err)
	assert.False(t, hasNew)
}
