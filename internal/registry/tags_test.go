package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListTagsDirect_Success(t *testing.T) {
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
		if strings.Contains(r.URL.Path, "/tags/list") {
			_ = json.NewEncoder(w).Encode(tagsResponse{Name: "library/mysql", Tags: []string{"8.0.44", "8.0.45", "8.0.46", "8.1.0", "latest"}})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	host := strings.TrimPrefix(registrySrv.URL, "http://")

	tags, err := c.listTagsDirect(context.Background(), host, "library/mysql")
	require.NoError(t, err)
	assert.Len(t, tags, 5)
	assert.Contains(t, tags, "8.0.46")
	assert.Contains(t, tags, "latest")
}

func TestListTagsDirect_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	host := strings.TrimPrefix(srv.URL, "http://")

	_, err := c.listTagsDirect(context.Background(), host, "library/mysql")
	assert.Error(t, err)
}

func TestListTagsDirect_BadJSON(t *testing.T) {
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
		_, _ = w.Write([]byte("not json"))
	}))
	defer registrySrv.Close()

	c := &Client{httpClient: &http.Client{}, authConfigs: make(map[string]AuthConfig), scheme: "http"}
	host := strings.TrimPrefix(registrySrv.URL, "http://")

	_, err := c.listTagsDirect(context.Background(), host, "library/mysql")
	assert.Error(t, err)
}
