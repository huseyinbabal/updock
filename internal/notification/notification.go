// Package notification provides webhook-based notifications for Updock.
//
// Notifications are sent as JSON HTTP POST requests to one or more webhook
// URLs. This supports integration with Slack (via incoming webhooks),
// Discord, Microsoft Teams, Gotify, and any custom HTTP endpoint.
//
// # Templates
//
// Notification messages can be customized using Go text/template syntax.
// The template receives an UpdateEvent struct with fields:
//
//	.Type           - "update", "error", or "startup"
//	.ContainerName  - name of the container
//	.Image          - image reference (e.g. "nginx:latest")
//	.OldImageID     - previous image ID (short form)
//	.NewImageID     - new image ID (short form)
//	.UpdatedAt      - timestamp of the update
//
// Example template:
//
//	"Container {{.ContainerName}} updated: {{.Image}} ({{.OldImageID}} -> {{.NewImageID}})"
//
// # Multiple Endpoints
//
// Multiple notification URLs can be configured via the UPDOCK_NOTIFICATION_URL
// environment variable (space-separated) or the --webhook-url flag (single URL).
// Each URL receives the same payload independently.
package notification

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"text/template"
	"time"

	"github.com/huseyinbabal/updock/internal/logger"
)

// UpdateEvent is the JSON payload sent to webhook endpoints.
// It contains all relevant information about an update event.
type UpdateEvent struct {
	Type          string `json:"type"` // "update", "error", or "startup"
	ContainerName string `json:"container_name"`
	Image         string `json:"image"`
	OldImageID    string `json:"old_image_id"`
	NewImageID    string `json:"new_image_id"`
	UpdatedAt     string `json:"updated_at"`
	Message       string `json:"message"`
}

// Notifier sends webhook notifications about container update events.
// It supports multiple webhook URLs and custom message templates.
type Notifier struct {
	webhookURLs []string
	httpClient  *http.Client
	tmpl        *template.Template
}

// NewNotifier creates a new Notifier.
//
// webhookURL is the primary webhook endpoint from --webhook-url flag.
// additionalURLs are extra endpoints from UPDOCK_NOTIFICATION_URL env var.
// tmplStr is a Go text/template string for formatting the "message" field;
// if empty, a default template is used.
//
// Returns nil if no webhook URLs are configured.
func NewNotifier(webhookURL string, additionalURLs []string, tmplStr string) *Notifier {
	var urls []string
	if webhookURL != "" {
		urls = append(urls, webhookURL)
	}
	urls = append(urls, additionalURLs...)

	if len(urls) == 0 {
		return nil
	}

	defaultTmpl := `Container {{.ContainerName}} updated: {{.Image}} ({{.OldImageID}} -> {{.NewImageID}})`
	if tmplStr == "" {
		tmplStr = defaultTmpl
	}

	tmpl, err := template.New("notification").Parse(tmplStr)
	if err != nil {
		logger.Warn().Msgf("Invalid notification template, using default: %v", err)
		tmpl, _ = template.New("notification").Parse(defaultTmpl)
	}

	return &Notifier{
		webhookURLs: urls,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		tmpl: tmpl,
	}
}

// NotifyUpdate sends a notification about a container update.
// The result parameter should be JSON-marshalable and typically is an
// updater.UpdateResult struct.
func (n *Notifier) NotifyUpdate(result interface{}) {
	if n == nil || len(n.webhookURLs) == 0 {
		return
	}

	data, err := json.Marshal(result)
	if err != nil {
		logger.Warn().Msgf("Failed to marshal update result for notification: %v", err)
		return
	}

	for _, url := range n.webhookURLs {
		go n.send(url, data)
	}
}

// NotifyError sends a notification about an error encountered during an update.
func (n *Notifier) NotifyError(containerName string, errMsg string) {
	if n == nil || len(n.webhookURLs) == 0 {
		return
	}

	event := UpdateEvent{
		Type:          "error",
		ContainerName: containerName,
		Message:       errMsg,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	for _, url := range n.webhookURLs {
		go n.send(url, data)
	}
}

// NotifyStartup sends a startup notification indicating that Updock has started.
func (n *Notifier) NotifyStartup(version string) {
	if n == nil || len(n.webhookURLs) == 0 {
		return
	}

	event := UpdateEvent{
		Type:      "startup",
		Message:   fmt.Sprintf("Updock %s started", version),
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	data, err := json.Marshal(event)
	if err != nil {
		return
	}

	for _, url := range n.webhookURLs {
		go n.send(url, data)
	}
}

// send performs the actual HTTP POST to a webhook URL.
func (n *Notifier) send(url string, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		logger.Warn().Msgf("Failed to create webhook request to %s: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Updock/1.0")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		logger.Warn().Msgf("Failed to send webhook notification to %s: %v", url, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		logger.Warn().Msgf("Webhook %s returned error status: %d", url, resp.StatusCode)
		return
	}

	logger.Debug().Msgf("Webhook notification sent to %s", url)
}

// FormatUpdateMessage creates a human-readable update message.
func FormatUpdateMessage(containerName, image, oldID, newID string) string {
	return fmt.Sprintf("Container %s updated: %s (%s -> %s)",
		containerName, image, shortID(oldID), shortID(newID))
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
