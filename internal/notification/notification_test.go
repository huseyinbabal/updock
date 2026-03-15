package notification

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewNotifier_NilWhenNoURLs(t *testing.T) {
	n := NewNotifier("", nil, "")
	if n != nil {
		t.Error("expected nil when no URLs")
	}
}

func TestNewNotifier_SingleURL(t *testing.T) {
	n := NewNotifier("https://example.com/hook", nil, "")
	if n == nil {
		t.Fatal("expected non-nil")
	}
	if len(n.webhookURLs) != 1 {
		t.Errorf("expected 1 URL, got %d", len(n.webhookURLs))
	}
}

func TestNewNotifier_MultipleURLs(t *testing.T) {
	n := NewNotifier("https://a.com", []string{"https://b.com", "https://c.com"}, "")
	if n == nil {
		t.Fatal("expected non-nil")
	}
	if len(n.webhookURLs) != 3 {
		t.Errorf("expected 3 URLs, got %d", len(n.webhookURLs))
	}
}

func TestNewNotifier_AdditionalOnly(t *testing.T) {
	n := NewNotifier("", []string{"https://b.com"}, "")
	if n == nil {
		t.Fatal("expected non-nil")
	}
	if len(n.webhookURLs) != 1 {
		t.Errorf("expected 1 URL, got %d", len(n.webhookURLs))
	}
}

func TestNewNotifier_InvalidTemplate(t *testing.T) {
	n := NewNotifier("https://example.com", nil, "{{.Invalid")
	if n == nil {
		t.Fatal("expected non-nil even with bad template")
	}
	// Should fall back to default template
}

func TestNotifyUpdate(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("expected application/json content type")
		}
		if r.Header.Get("User-Agent") != "Updock/1.0" {
			t.Error("expected Updock/1.0 user agent")
		}
		body, _ := io.ReadAll(r.Body)
		var data map[string]interface{}
		if err := json.Unmarshal(body, &data); err != nil {
			t.Errorf("invalid JSON body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNotifier(server.URL, nil, "")
	n.NotifyUpdate(map[string]string{
		"container_name": "nginx",
		"image":          "nginx:latest",
	})

	// Wait for async send
	time.Sleep(200 * time.Millisecond)
	if received.Load() != 1 {
		t.Errorf("expected 1 webhook call, got %d", received.Load())
	}
}

func TestNotifyUpdate_MultipleURLs(t *testing.T) {
	var received atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	s1 := httptest.NewServer(handler)
	s2 := httptest.NewServer(handler)
	defer s1.Close()
	defer s2.Close()

	n := NewNotifier(s1.URL, []string{s2.URL}, "")
	n.NotifyUpdate(map[string]string{"test": "data"})

	time.Sleep(200 * time.Millisecond)
	if received.Load() != 2 {
		t.Errorf("expected 2 webhook calls, got %d", received.Load())
	}
}

func TestNotifyUpdate_NilNotifier(t *testing.T) {
	// Should not panic
	var n *Notifier
	n.NotifyUpdate(map[string]string{"test": "data"})
}

func TestNotifyError(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		var data map[string]interface{}
		_ = json.Unmarshal(body, &data)
		if data["type"] != "error" {
			t.Errorf("expected type 'error', got %v", data["type"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNotifier(server.URL, nil, "")
	n.NotifyError("nginx", "something broke")

	time.Sleep(200 * time.Millisecond)
	if received.Load() != 1 {
		t.Errorf("expected 1 call, got %d", received.Load())
	}
}

func TestNotifyError_NilNotifier(t *testing.T) {
	var n *Notifier
	n.NotifyError("nginx", "error") // should not panic
}

func TestNotifyStartup(t *testing.T) {
	var received atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		body, _ := io.ReadAll(r.Body)
		var data map[string]interface{}
		_ = json.Unmarshal(body, &data)
		if data["type"] != "startup" {
			t.Errorf("expected type 'startup', got %v", data["type"])
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	n := NewNotifier(server.URL, nil, "")
	n.NotifyStartup("1.0.0")

	time.Sleep(200 * time.Millisecond)
	if received.Load() != 1 {
		t.Errorf("expected 1 call, got %d", received.Load())
	}
}

func TestNotifyStartup_NilNotifier(t *testing.T) {
	var n *Notifier
	n.NotifyStartup("1.0.0") // should not panic
}

func TestSend_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	n := NewNotifier(server.URL, nil, "")
	n.send(server.URL, []byte(`{"test":"data"}`))
	// Should not panic, just log warning
}

func TestFormatUpdateMessage(t *testing.T) {
	msg := FormatUpdateMessage("nginx", "nginx:1.25", "sha256:aaaaaaaabbbb", "sha256:ccccccccdddd")
	if msg == "" {
		t.Error("expected non-empty message")
	}
	// shortID truncates to 12 chars
	expected := "Container nginx updated: nginx:1.25 (sha256:aaaaa -> sha256:ccccc)"
	if msg != expected {
		t.Errorf("unexpected message: %q, want %q", msg, expected)
	}
}

func TestShortID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef1234567890", "abcdef123456"},
		{"short", "short"},
		{"exactly12chrs", "exactly12chr"},
		{"123456789012", "123456789012"},
		{"", ""},
	}
	for _, tt := range tests {
		got := shortID(tt.input)
		if got != tt.want {
			t.Errorf("shortID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
