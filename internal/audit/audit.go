// Package audit provides an append-only audit log for all update operations.
//
// Every update attempt, rollback, approval, and configuration change is
// recorded with a timestamp, actor, and outcome. The audit log is stored
// in a local SQLite-compatible JSON file and exposed via the REST API.
//
// Unlike simple notification systems, the audit log provides a complete,
// queryable history of all Updock actions — essential for compliance,
// debugging, and understanding what changed and when.
//
// # Entry Types
//
//	update.started   - An update check was initiated
//	update.pulled    - A new image was pulled
//	update.applied   - A container was recreated with the new image
//	update.skipped   - Update was skipped (policy, window, or approval)
//	update.failed    - Update attempt failed
//	rollback.auto    - Automatic rollback triggered by health check failure
//	rollback.manual  - Manual rollback triggered via API or UI
//	approval.pending - Update queued for manual approval
//	approval.granted - Manual approval granted
//	approval.denied  - Manual approval denied
package audit

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// EventType categorizes audit log entries.
type EventType string

const (
	EventUpdateStarted   EventType = "update.started"
	EventUpdatePulled    EventType = "update.pulled"
	EventUpdateApplied   EventType = "update.applied"
	EventUpdateSkipped   EventType = "update.skipped"
	EventUpdateFailed    EventType = "update.failed"
	EventRollbackAuto    EventType = "rollback.auto"
	EventRollbackManual  EventType = "rollback.manual"
	EventApprovalPending EventType = "approval.pending"
	EventApprovalGranted EventType = "approval.granted"
	EventApprovalDenied  EventType = "approval.denied"
)

// Entry is a single audit log record.
type Entry struct {
	ID            int       `json:"id"`
	Timestamp     time.Time `json:"timestamp"`
	Type          EventType `json:"type"`
	ContainerName string    `json:"container_name,omitempty"`
	ContainerID   string    `json:"container_id,omitempty"`
	Image         string    `json:"image,omitempty"`
	OldImageID    string    `json:"old_image_id,omitempty"`
	NewImageID    string    `json:"new_image_id,omitempty"`
	Policy        string    `json:"policy,omitempty"`
	Actor         string    `json:"actor"` // "system", "api", "ui", "schedule"
	Message       string    `json:"message"`
	Error         string    `json:"error,omitempty"`
}

// Log is an append-only audit log. It is safe for concurrent use.
type Log struct {
	mu      sync.RWMutex
	entries []Entry
	nextID  int
	path    string // file path for persistence
}

// NewLog creates a new audit log. If path is non-empty, entries are
// persisted to that file. If the file exists, previous entries are loaded.
func NewLog(path string) *Log {
	l := &Log{
		entries: make([]Entry, 0),
		nextID:  1,
		path:    path,
	}

	if path != "" {
		l.load()
	}

	return l
}

// Record adds a new entry to the audit log.
func (l *Log) Record(e Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e.ID = l.nextID
	e.Timestamp = time.Now()
	l.nextID++

	l.entries = append(l.entries, e)

	// Keep at most 10000 entries in memory
	if len(l.entries) > 10000 {
		l.entries = l.entries[len(l.entries)-10000:]
	}

	if l.path != "" {
		l.persist()
	}
}

// Query returns entries matching the given filter. If containerName is empty,
// all entries are returned. Results are ordered newest-first.
func (l *Log) Query(containerName string, eventType EventType, limit int) []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if limit <= 0 {
		limit = 100
	}

	var results []Entry
	// Iterate in reverse for newest-first
	for i := len(l.entries) - 1; i >= 0 && len(results) < limit; i-- {
		e := l.entries[i]
		if containerName != "" && e.ContainerName != containerName {
			continue
		}
		if eventType != "" && e.Type != eventType {
			continue
		}
		results = append(results, e)
	}

	return results
}

// All returns all entries, newest-first, up to the given limit.
func (l *Log) All(limit int) []Entry {
	return l.Query("", "", limit)
}

// load reads entries from the persisted file.
func (l *Log) load() {
	data, err := os.ReadFile(l.path)
	if err != nil {
		return
	}

	var entries []Entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return
	}

	l.entries = entries
	if len(entries) > 0 {
		l.nextID = entries[len(entries)-1].ID + 1
	}
}

// persist writes all entries to the file.
func (l *Log) persist() {
	data, err := json.MarshalIndent(l.entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(l.path, data, 0644)
}
