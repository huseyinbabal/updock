// Package gitops provides Git-based change tracking for Updock.
//
// When Updock updates a container to a new image tag, it can optionally
// commit the change to a Git repository — similar to Flux CD's image
// automation. This creates an auditable, version-controlled record of
// every image change.
//
// # How It Works
//
//  1. Updock detects a new image version for a container.
//  2. After successfully updating the container, Updock scans the configured
//     file (e.g. docker-compose.yml) for the old image reference.
//  3. Replaces the old reference with the new one.
//  4. Commits and pushes the change to the configured Git repository.
//
// # Configuration
//
// GitOps is configured in updock.yml:
//
//	gitops:
//	  enabled: true
//	  repository: /path/to/repo        # local path or clone URL
//	  branch: main
//	  file: docker-compose.yml         # file to update
//	  commit_message: "updock: update {{.Image}} to {{.NewTag}}"
//	  author_name: Updock Bot
//	  author_email: updock@localhost
package gitops

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	log "github.com/sirupsen/logrus"
)

// Config holds GitOps configuration from updock.yml.
type Config struct {
	Enabled       bool   `yaml:"enabled"`
	Repository    string `yaml:"repository"`
	Branch        string `yaml:"branch"`
	File          string `yaml:"file"`
	CommitMessage string `yaml:"commit_message"`
	AuthorName    string `yaml:"author_name"`
	AuthorEmail   string `yaml:"author_email"`
}

// DefaultConfig returns sensible defaults for GitOps.
func DefaultConfig() Config {
	return Config{
		Enabled:       false,
		Branch:        "main",
		File:          "docker-compose.yml",
		CommitMessage: "updock: update {{.Image}} from {{.OldTag}} to {{.NewTag}}",
		AuthorName:    "Updock Bot",
		AuthorEmail:   "updock@localhost",
	}
}

// Change represents an image tag change to be committed.
type Change struct {
	Image  string // e.g. "mysql"
	OldTag string // e.g. "8.0.45"
	NewTag string // e.g. "8.0.46"
	OldRef string // e.g. "mysql:8.0.45"
	NewRef string // e.g. "mysql:8.0.46"
}

// Client handles Git operations for pushing image updates.
type Client struct {
	cfg Config
}

// NewClient creates a new GitOps client.
func NewClient(cfg Config) *Client {
	if cfg.Branch == "" {
		cfg.Branch = "main"
	}
	if cfg.CommitMessage == "" {
		cfg.CommitMessage = DefaultConfig().CommitMessage
	}
	if cfg.AuthorName == "" {
		cfg.AuthorName = DefaultConfig().AuthorName
	}
	if cfg.AuthorEmail == "" {
		cfg.AuthorEmail = DefaultConfig().AuthorEmail
	}
	return &Client{cfg: cfg}
}

// PushChange updates the configured file with the new image reference,
// commits, and pushes to the remote repository.
func (c *Client) PushChange(change Change) error {
	if !c.cfg.Enabled || c.cfg.Repository == "" {
		return nil
	}

	repoPath := c.cfg.Repository
	filePath := c.cfg.File

	log.Infof("GitOps: updating %s in %s/%s", change.NewRef, repoPath, filePath)

	// Step 1: Read the file
	fullPath := repoPath + "/" + filePath
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fullPath, err)
	}

	// Step 2: Replace old reference with new
	content := string(data)
	newContent := strings.ReplaceAll(content, change.OldRef, change.NewRef)
	if content == newContent {
		log.Warnf("GitOps: old reference %q not found in %s, skipping", change.OldRef, filePath)
		return nil
	}

	// Step 3: Write updated file
	if err := os.WriteFile(fullPath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", fullPath, err)
	}

	// Step 4: Git add + commit + push
	commitMsg, err := c.renderCommitMessage(change)
	if err != nil {
		commitMsg = fmt.Sprintf("updock: update %s to %s", change.Image, change.NewTag)
	}

	commands := [][]string{
		{"git", "-C", repoPath, "add", filePath},
		{"git", "-C", repoPath,
			"-c", fmt.Sprintf("user.name=%s", c.cfg.AuthorName),
			"-c", fmt.Sprintf("user.email=%s", c.cfg.AuthorEmail),
			"commit", "-m", commitMsg},
		{"git", "-C", repoPath, "push", "origin", c.cfg.Branch},
	}

	for _, args := range commands {
		cmd := exec.Command(args[0], args[1:]...) //nolint:gosec // args are controlled
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("git command %v failed: %s: %w", args[:3], strings.TrimSpace(string(out)), err)
		}
	}

	log.Infof("GitOps: pushed %s -> %s to %s:%s", change.OldRef, change.NewRef, repoPath, c.cfg.Branch)
	return nil
}

// renderCommitMessage renders the commit message template with change data.
func (c *Client) renderCommitMessage(change Change) (string, error) {
	tmpl, err := template.New("commit").Parse(c.cfg.CommitMessage)
	if err != nil {
		return "", err
	}

	data := struct {
		Image     string
		OldTag    string
		NewTag    string
		OldRef    string
		NewRef    string
		Timestamp string
	}{
		Image:     change.Image,
		OldTag:    change.OldTag,
		NewTag:    change.NewTag,
		OldRef:    change.OldRef,
		NewRef:    change.NewRef,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
