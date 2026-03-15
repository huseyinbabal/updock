package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/updater"
)

func newTestServer(token string) *Server {
	cfg := &config.Config{
		HTTPAddr:       ":0",
		HTTPEnabled:    true,
		HTTPAPIToken:   token,
		MetricsEnabled: true,
		MonitorAll:     true,
	}
	upd := updater.New(nil, nil, nil, cfg, policy.DefaultSpec(), audit.NewLog(""))
	// Note: docker client is nil, so handlers that call Docker will fail.
	// We test auth/routing/info/history which don't need Docker.
	return NewServer(nil, upd, cfg)
}

func TestWithAuth_NoToken(t *testing.T) {
	s := newTestServer("")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 without token config, got %d", w.Code)
	}
}

func TestWithAuth_ValidBearerToken(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d", w.Code)
	}
}

func TestWithAuth_InvalidBearerToken(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", w.Code)
	}
}

func TestWithAuth_QueryParam(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test?token=secret123", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid query token, got %d", w.Code)
	}
}

func TestWithAuth_MissingToken(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without token, got %d", w.Code)
	}
}

func TestWithAuth_InvalidAuthHeaderFormat(t *testing.T) {
	s := newTestServer("secret123")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for Basic auth, got %d", w.Code)
	}
}

func TestCorsMiddleware(t *testing.T) {
	s := newTestServer("")
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Normal request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
	if w.Header().Get("Access-Control-Allow-Headers") != "Content-Type, Authorization" {
		t.Error("missing or wrong CORS allow-headers")
	}

	// OPTIONS preflight
	req = httptest.NewRequest("OPTIONS", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for OPTIONS, got %d", w.Code)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "value"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected application/json")
	}
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result["key"] != "value" {
		t.Errorf("unexpected body: %s", w.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "something wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var result map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if result["error"] != "something wrong" {
		t.Errorf("unexpected error message: %s", w.Body.String())
	}
}

func TestHandleInfo(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/api/info", nil)
	w := httptest.NewRecorder()
	s.handleInfo(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var result map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &result)
	if _, ok := result["version"]; !ok {
		t.Error("expected version in info response")
	}
	if _, ok := result["monitorAll"]; !ok {
		t.Error("expected monitorAll in info response")
	}
}

func TestHandleHistory(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()
	s.handleHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleAuditLog(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/api/audit", nil)
	w := httptest.NewRecorder()
	s.handleAuditLog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleAuditLog_WithParams(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/api/audit?container=nginx&limit=5", nil)
	w := httptest.NewRecorder()
	s.handleAuditLog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleAuditLog_InvalidLimit(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/api/audit?limit=abc", nil)
	w := httptest.NewRecorder()
	s.handleAuditLog(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even with invalid limit, got %d", w.Code)
	}
}

func TestHandleUI_Root(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.handleUI(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Error("expected text/html content type")
	}
}

func TestHandleUI_NotRoot(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/other", nil)
	w := httptest.NewRecorder()
	s.handleUI(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for non-root path, got %d", w.Code)
	}
}

func TestHandleContainerDetail_MissingID(t *testing.T) {
	s := newTestServer("")

	req := httptest.NewRequest("GET", "/api/containers/", nil)
	// PathValue returns "" for missing path param
	w := httptest.NewRecorder()
	s.handleContainerDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing ID, got %d", w.Code)
	}
}

func TestStop_NilServer(t *testing.T) {
	s := newTestServer("")
	// server.server is nil before Start()
	err := s.Stop(context.Background())
	if err != nil {
		t.Errorf("expected nil error for nil server, got %v", err)
	}
}
