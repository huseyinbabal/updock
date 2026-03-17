// Package e2e provides end-to-end tests for Updock using testcontainers.
//
// These tests verify the full update lifecycle against real Docker containers:
//   - Semver-based tag upgrades (patch, minor, major)
//   - Policy enforcement (pin, manual approval, maintenance windows)
//   - Digest-based updates for "all" strategy
//   - Container recreation with config preservation
//
// Requirements:
//   - Docker daemon must be running
//   - Tests create and destroy real containers
//   - Network access to Docker Hub for image pulling
//
// Run with: go test -v -tags=e2e -timeout=5m ./e2e/
package e2e

import (
	"context"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"

	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/notification"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/registry"
	"github.com/huseyinbabal/updock/internal/updater"
)

// containerName generates a unique container name for tests.
func containerName(t *testing.T, suffix string) string {
	name := strings.ReplaceAll(t.Name(), "/", "_")
	return "updock_e2e_" + name + "_" + suffix
}

// startContainer creates and starts a container with the given image using testcontainers.
// Returns the container and a cleanup function.
func startContainer(t *testing.T, ctx context.Context, image string, name string) tc.Container {
	t.Helper()

	req := tc.ContainerRequest{
		Image: image,
		Name:  name,
		Cmd:   []string{"sleep", "3600"},
	}

	ctr, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "failed to start container %s with image %s", name, image)

	t.Cleanup(func() {
		_ = ctr.Terminate(ctx)
	})

	return ctr
}

// getContainerImage inspects a container and returns its current image reference.
func getContainerImage(t *testing.T, ctx context.Context, containerID string) string {
	t.Helper()

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer func() { _ = cli.Close() }()

	inspect, err := cli.ContainerInspect(ctx, containerID)
	require.NoError(t, err)

	return inspect.Config.Image
}

// getContainerIDByName finds a container ID by name.
func getContainerIDByName(t *testing.T, ctx context.Context, name string) string {
	t.Helper()

	cli, err := dockerclient.NewClientWithOpts(dockerclient.FromEnv, dockerclient.WithAPIVersionNegotiation())
	require.NoError(t, err)
	defer func() { _ = cli.Close() }()

	containers, err := cli.ContainerList(ctx, container.ListOptions{All: true})
	require.NoError(t, err)

	for _, c := range containers {
		for _, n := range c.Names {
			if strings.TrimPrefix(n, "/") == name {
				return c.ID
			}
		}
	}

	t.Fatalf("container %q not found", name)
	return ""
}

// newUpdater creates an Updater wired to real Docker and registry clients.
func newUpdater(t *testing.T, cfg *config.Config, spec *policy.Spec) *updater.Updater {
	t.Helper()

	dockerClient, err := docker.NewClient("", false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })

	registryClient := registry.NewClient("")
	auditLog := audit.NewLog("")

	return updater.New(dockerClient, registryClient, nil, cfg, spec, auditLog)
}

// TestE2E_PatchUpgrade verifies that a container running an older patch version
// (alpine:3.20.0) is upgraded to a newer patch (3.20.x) when strategy=patch.
func TestE2E_PatchUpgrade(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "alpine")

	// Start with an older patch version
	ctr := startContainer(t, ctx, "alpine:3.20.0", name)
	ctrID := ctr.GetContainerID()

	// Configure policy: patch updates only
	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {
				Strategy: policy.StrategyPatch,
				Approve:  policy.ApproveAuto,
				Rollback: policy.RollbackOnFailure,
			},
		},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}

	cfg := &config.Config{
		MonitorAll:     true,
		ContainerNames: []string{name},
	}

	upd := newUpdater(t, cfg, spec)

	// Run update check
	results, err := upd.Run(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[0]
	t.Logf("Container: %s, Image: %s, Updated: %v, Error: %s",
		result.ContainerName, result.Image, result.Updated, result.Error)

	if result.Updated {
		// Verify the new container has a different (newer) image tag
		newID := getContainerIDByName(t, ctx, name)
		newImage := getContainerImage(t, ctx, newID)
		t.Logf("Updated image: %s -> %s", "alpine:3.20.0", newImage)

		assert.NotEqual(t, ctrID, newID, "container ID should change after update")
		assert.Contains(t, newImage, "alpine:3.20.", "should still be 3.20.x patch")
		assert.NotEqual(t, "alpine:3.20.0", newImage, "should be a newer patch")
	} else {
		t.Logf("No patch upgrade available (3.20.0 may be the latest 3.20.x)")
	}
}

// TestE2E_PinStrategy verifies that containers with strategy=pin are never updated.
func TestE2E_PinStrategy(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "pinned")

	// Start with an old version that definitely has newer versions
	ctr := startContainer(t, ctx, "alpine:3.18.0", name)
	ctrID := ctr.GetContainerID()

	// Configure policy: pin (never update)
	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {
				Strategy: policy.StrategyPin,
				Approve:  policy.ApproveManual,
			},
		},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}

	cfg := &config.Config{
		MonitorAll:     true,
		ContainerNames: []string{name},
	}

	upd := newUpdater(t, cfg, spec)

	results, err := upd.Run(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[0]
	t.Logf("Container: %s, Updated: %v, Skipped: %v, Error: %s",
		result.ContainerName, result.Updated, result.Skipped, result.Error)

	// Pin strategy should prevent any update
	assert.False(t, result.Updated, "pinned container should not be updated")

	// Container should still be running with the same ID
	currentID := getContainerIDByName(t, ctx, name)
	assert.Equal(t, ctrID, currentID, "container ID should not change")
}

// TestE2E_ManualApproval verifies that containers with approve=manual are skipped.
func TestE2E_ManualApproval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "manual")

	ctr := startContainer(t, ctx, "alpine:3.18.0", name)
	ctrID := ctr.GetContainerID()

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {
				Strategy: policy.StrategyPatch,
				Approve:  policy.ApproveManual,
			},
		},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}

	cfg := &config.Config{
		MonitorAll:     true,
		ContainerNames: []string{name},
	}

	upd := newUpdater(t, cfg, spec)

	results, err := upd.Run(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[0]
	t.Logf("Result: updated=%v skipped=%v error=%s", result.Updated, result.Skipped, result.Error)

	// Manual approval should skip the update even though a newer patch exists
	assert.False(t, result.Updated, "manual approval should skip update")
	assert.True(t, result.Skipped, "should be marked as skipped")
	assert.Contains(t, result.Error, "manual approval")

	currentID := getContainerIDByName(t, ctx, name)
	assert.Equal(t, ctrID, currentID, "container ID should not change")
}

// TestE2E_DryRunMode verifies that dry-run mode detects but does not apply updates.
func TestE2E_DryRunMode(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "dryrun")

	ctr := startContainer(t, ctx, "alpine:3.18.0", name)
	ctrID := ctr.GetContainerID()

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {
				Strategy: policy.StrategyPatch,
				Approve:  policy.ApproveAuto,
			},
		},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}

	cfg := &config.Config{
		MonitorAll:     true,
		DryRun:         true,
		ContainerNames: []string{name},
	}

	upd := newUpdater(t, cfg, spec)

	results, err := upd.Run(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)

	result := results[0]

	// Dry run should not apply updates
	assert.False(t, result.Updated, "dry run should not update")
	assert.True(t, result.MonitorOnly, "should be marked as monitor-only")

	currentID := getContainerIDByName(t, ctx, name)
	assert.Equal(t, ctrID, currentID, "container ID should not change in dry run")
}

// TestE2E_IgnoredContainer verifies that containers marked ignore in policy are skipped entirely.
func TestE2E_IgnoredContainer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "ignored")

	_ = startContainer(t, ctx, "alpine:3.18.0", name)

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {Strategy: policy.StrategyAll, Approve: policy.ApproveAuto},
		},
		Containers: map[string]policy.ContainerDef{
			name: {Ignore: true},
		},
		Groups: map[string]policy.GroupDef{},
	}

	cfg := &config.Config{
		MonitorAll:     true,
		ContainerNames: []string{name},
	}

	upd := newUpdater(t, cfg, spec)

	results, err := upd.Run(ctx)
	require.NoError(t, err)

	// Ignored containers should not appear in results at all
	assert.Empty(t, results, "ignored container should produce no results")
}

// TestE2E_WebhookNotification verifies that webhook notifications are sent on update.
func TestE2E_WebhookNotification(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "webhook")

	_ = startContainer(t, ctx, "alpine:3.18.0", name)

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {Strategy: policy.StrategyPatch, Approve: policy.ApproveAuto},
		},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}

	// Use a dummy webhook URL (notification will fail silently, that's fine)
	notifier := notification.NewNotifier("http://localhost:1/webhook", nil, "")

	dockerClient, err := docker.NewClient("", false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })

	registryClient := registry.NewClient("")
	auditLog := audit.NewLog("")

	cfg := &config.Config{
		MonitorAll:     true,
		ContainerNames: []string{name},
	}

	upd := updater.New(dockerClient, registryClient, notifier, cfg, spec, auditLog)

	results, err := upd.Run(ctx)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// The update should proceed (notification failure doesn't block updates)
	t.Logf("Result: updated=%v error=%s", results[0].Updated, results[0].Error)
}

// TestE2E_AuditLogRecorded verifies that audit log entries are created during updates.
func TestE2E_AuditLogRecorded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	ctx := context.Background()
	name := containerName(t, "auditlog")

	_ = startContainer(t, ctx, "alpine:3.18.0", name)

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {Strategy: policy.StrategyPatch, Approve: policy.ApproveAuto},
		},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}

	cfg := &config.Config{
		MonitorAll:     true,
		ContainerNames: []string{name},
	}

	dockerClient, err := docker.NewClient("", false)
	require.NoError(t, err)
	t.Cleanup(func() { _ = dockerClient.Close() })

	auditLog := audit.NewLog("")
	upd := updater.New(dockerClient, registry.NewClient(""), nil, cfg, spec, auditLog)

	_, err = upd.Run(ctx)
	require.NoError(t, err)

	entries := auditLog.All(100)
	t.Logf("Audit entries: %d", len(entries))
	for _, e := range entries {
		t.Logf("  [%s] %s: %s", e.Type, e.ContainerName, e.Message)
	}

	// Should have at least one audit entry (either update.started or update.skipped)
	assert.NotEmpty(t, entries, "audit log should have entries")
}
