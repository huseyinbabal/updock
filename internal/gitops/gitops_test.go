package gitops

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, "main", cfg.Branch)
	assert.Equal(t, "docker-compose.yml", cfg.File)
	assert.Contains(t, cfg.CommitMessage, "{{.Image}}")
}

func TestNewClient_Defaults(t *testing.T) {
	c := NewClient(Config{})
	assert.Equal(t, "main", c.cfg.Branch)
	assert.Equal(t, "Updock Bot", c.cfg.AuthorName)
	assert.Equal(t, "updock@localhost", c.cfg.AuthorEmail)
}

func TestPushChange_Disabled(t *testing.T) {
	c := NewClient(Config{Enabled: false})
	err := c.PushChange(Change{OldRef: "a:1", NewRef: "a:2"})
	assert.NoError(t, err) // no-op
}

func TestPushChange_NoRepo(t *testing.T) {
	c := NewClient(Config{Enabled: true, Repository: ""})
	err := c.PushChange(Change{OldRef: "a:1", NewRef: "a:2"})
	assert.NoError(t, err) // no-op
}

func TestPushChange_FileNotFound(t *testing.T) {
	c := NewClient(Config{Enabled: true, Repository: "/nonexistent", File: "compose.yml"})
	err := c.PushChange(Change{OldRef: "a:1", NewRef: "a:2"})
	assert.Error(t, err)
}

func TestPushChange_ReferenceNotInFile(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "compose.yml"), []byte("image: other:1"), 0644)

	c := NewClient(Config{Enabled: true, Repository: dir, File: "compose.yml"})
	err := c.PushChange(Change{OldRef: "mysql:8.0.45", NewRef: "mysql:8.0.46"})
	assert.NoError(t, err) // skips silently
}

func TestPushChange_ReplacesReference(t *testing.T) {
	dir := t.TempDir()
	original := "services:\n  db:\n    image: mysql:8.0.45\n"
	_ = os.WriteFile(filepath.Join(dir, "compose.yml"), []byte(original), 0644)

	// Initialize a git repo so the git commands don't fail on missing repo
	// (they'll fail on push, but we can check the file was updated)
	c := NewClient(Config{Enabled: true, Repository: dir, File: "compose.yml", Branch: "main"})
	_ = c.PushChange(Change{OldRef: "mysql:8.0.45", NewRef: "mysql:8.0.46"})

	// Verify file was updated regardless of git push result
	data, err := os.ReadFile(filepath.Join(dir, "compose.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "mysql:8.0.46")
	assert.NotContains(t, string(data), "mysql:8.0.45")
}

func TestRenderCommitMessage(t *testing.T) {
	c := NewClient(Config{CommitMessage: "update {{.Image}} from {{.OldTag}} to {{.NewTag}}"})
	msg, err := c.renderCommitMessage(Change{Image: "mysql", OldTag: "8.0.45", NewTag: "8.0.46"})
	assert.NoError(t, err)
	assert.Equal(t, "update mysql from 8.0.45 to 8.0.46", msg)
}

func TestRenderCommitMessage_InvalidTemplate(t *testing.T) {
	c := NewClient(Config{CommitMessage: "{{.Invalid"})
	_, err := c.renderCommitMessage(Change{})
	assert.Error(t, err)
}
