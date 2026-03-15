package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewLog_InMemory(t *testing.T) {
	l := NewLog("")
	if l == nil {
		t.Fatal("NewLog returned nil")
	}
	if len(l.entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(l.entries))
	}
	if l.nextID != 1 {
		t.Errorf("expected nextID 1, got %d", l.nextID)
	}
}

func TestRecord(t *testing.T) {
	l := NewLog("")

	l.Record(Entry{
		Type:          EventUpdateApplied,
		ContainerName: "nginx",
		Actor:         "system",
		Message:       "updated",
	})

	if len(l.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(l.entries))
	}
	e := l.entries[0]
	if e.ID != 1 {
		t.Errorf("expected ID 1, got %d", e.ID)
	}
	if e.Type != EventUpdateApplied {
		t.Errorf("expected type %q, got %q", EventUpdateApplied, e.Type)
	}
	if e.ContainerName != "nginx" {
		t.Errorf("expected container 'nginx', got %q", e.ContainerName)
	}
	if e.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}

	l.Record(Entry{
		Type:    EventUpdateFailed,
		Actor:   "api",
		Message: "failed",
	})
	if len(l.entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(l.entries))
	}
	if l.entries[1].ID != 2 {
		t.Errorf("expected ID 2, got %d", l.entries[1].ID)
	}
}

func TestRecord_Retention(t *testing.T) {
	l := NewLog("")

	for i := 0; i < 10010; i++ {
		l.Record(Entry{Type: EventUpdateStarted, Message: "test"})
	}

	if len(l.entries) > 10000 {
		t.Errorf("expected max 10000 entries, got %d", len(l.entries))
	}
}

func TestQuery(t *testing.T) {
	l := NewLog("")

	l.Record(Entry{Type: EventUpdateApplied, ContainerName: "nginx", Message: "1"})
	l.Record(Entry{Type: EventUpdateFailed, ContainerName: "redis", Message: "2"})
	l.Record(Entry{Type: EventUpdateApplied, ContainerName: "nginx", Message: "3"})
	l.Record(Entry{Type: EventRollbackAuto, ContainerName: "nginx", Message: "4"})
	l.Record(Entry{Type: EventUpdateSkipped, ContainerName: "postgres", Message: "5"})

	// All entries
	all := l.Query("", "", 100)
	if len(all) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(all))
	}
	// Newest first
	if all[0].Message != "5" {
		t.Errorf("expected newest first, got %q", all[0].Message)
	}

	// Filter by container
	nginxEntries := l.Query("nginx", "", 100)
	if len(nginxEntries) != 3 {
		t.Fatalf("expected 3 nginx entries, got %d", len(nginxEntries))
	}

	// Filter by event type
	applied := l.Query("", EventUpdateApplied, 100)
	if len(applied) != 2 {
		t.Fatalf("expected 2 applied entries, got %d", len(applied))
	}

	// Filter by both
	nginxApplied := l.Query("nginx", EventUpdateApplied, 100)
	if len(nginxApplied) != 2 {
		t.Fatalf("expected 2 nginx applied entries, got %d", len(nginxApplied))
	}

	// Limit
	limited := l.Query("", "", 2)
	if len(limited) != 2 {
		t.Fatalf("expected 2 limited entries, got %d", len(limited))
	}

	// Default limit (0 -> 100)
	defaultLimit := l.Query("", "", 0)
	if len(defaultLimit) != 5 {
		t.Fatalf("expected 5 with default limit, got %d", len(defaultLimit))
	}
}

func TestAll(t *testing.T) {
	l := NewLog("")
	l.Record(Entry{Type: EventUpdateApplied, Message: "a"})
	l.Record(Entry{Type: EventUpdateFailed, Message: "b"})

	all := l.All(10)
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
}

func TestPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.json")

	// Write entries
	l1 := NewLog(path)
	l1.Record(Entry{Type: EventUpdateApplied, ContainerName: "nginx", Message: "first"})
	l1.Record(Entry{Type: EventUpdateFailed, ContainerName: "redis", Message: "second"})

	// Verify file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("expected audit file to exist")
	}

	// Load in new instance
	l2 := NewLog(path)
	if len(l2.entries) != 2 {
		t.Fatalf("expected 2 loaded entries, got %d", len(l2.entries))
	}
	if l2.entries[0].ContainerName != "nginx" {
		t.Errorf("expected nginx, got %q", l2.entries[0].ContainerName)
	}
	if l2.nextID != 3 {
		t.Errorf("expected nextID 3, got %d", l2.nextID)
	}

	// Add more entries and verify persistence
	l2.Record(Entry{Type: EventRollbackAuto, ContainerName: "app", Message: "third"})

	l3 := NewLog(path)
	if len(l3.entries) != 3 {
		t.Fatalf("expected 3 entries after re-load, got %d", len(l3.entries))
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	_ = os.WriteFile(path, []byte("not json"), 0644)

	l := NewLog(path)
	if len(l.entries) != 0 {
		t.Errorf("expected 0 entries for bad JSON, got %d", len(l.entries))
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	_ = os.WriteFile(path, []byte(""), 0644)

	l := NewLog(path)
	if len(l.entries) != 0 {
		t.Errorf("expected 0 entries for empty file, got %d", len(l.entries))
	}
}
