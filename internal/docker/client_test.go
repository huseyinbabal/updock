package docker

import (
	"syscall"
	"testing"
)

func TestShortID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdef1234567890", "abcdef123456"},
		{"short", "short"},
		{"123456789012", "123456789012"},
		{"", ""},
		{"a", "a"},
		{"exactly12345", "exactly12345"[:12]},
	}
	for _, tt := range tests {
		got := shortID(tt.input)
		if got != tt.want {
			t.Errorf("shortID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseSignal(t *testing.T) {
	tests := []struct {
		input string
		want  syscall.Signal
	}{
		{"SIGHUP", syscall.SIGHUP},
		{"SIGINT", syscall.SIGINT},
		{"SIGQUIT", syscall.SIGQUIT},
		{"SIGTERM", syscall.SIGTERM},
		{"SIGUSR1", syscall.SIGUSR1},
		{"SIGUSR2", syscall.SIGUSR2},
		{"sighup", syscall.SIGHUP},
		{"  SIGTERM  ", syscall.SIGTERM},
		{"SIGFOO", 0},
		{"", 0},
		{"invalid", 0},
	}
	for _, tt := range tests {
		got := parseSignal(tt.input)
		if got != tt.want {
			t.Errorf("parseSignal(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestNewClient_Default(t *testing.T) {
	c, err := NewClient("", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	_ = c.Close()
}

func TestNewClient_CustomHost(t *testing.T) {
	c, err := NewClient("tcp://localhost:2375", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	_ = c.Close()
}
