package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// tagsResponse represents the Docker Registry V2 tags/list response.
type tagsResponse struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// ListTags returns all tags for a given image reference from the remote registry.
// It uses the same OCI challenge-based auth flow as digest checking.
//
// Example:
//
//	tags, err := client.ListTags(ctx, "mysql:8.0.45")
//	// returns: ["8.0.44", "8.0.45", "8.0.46", "8.1.0", "9.0.0", ...]
func (c *Client) ListTags(ctx context.Context, imageRef string) ([]string, error) {
	host, repo, _ := parseReference(imageRef)
	return c.listTagsDirect(ctx, host, repo)
}

// listTagsDirect lists tags for a specific host/repo. Extracted for testability.
func (c *Client) listTagsDirect(ctx context.Context, host, repo string) ([]string, error) {
	log.Debugf("Listing tags for %s/%s", host, repo)

	token, err := c.getToken(ctx, host, repo)
	if err != nil {
		return nil, fmt.Errorf("getting auth token for %s/%s: %w", host, repo, err)
	}

	url := fmt.Sprintf("%s://%s/v2/%s/tags/list", c.scheme, host, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("listing tags: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tags/list returned status %d for %s/%s", resp.StatusCode, host, repo)
	}

	var tagsResp tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResp); err != nil {
		return nil, fmt.Errorf("decoding tags response: %w", err)
	}

	return tagsResp.Tags, nil
}
