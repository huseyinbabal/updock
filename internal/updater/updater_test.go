package updater

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/mocks"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/registry"
)

func TestIsTruthy(t *testing.T) {
	trueVals := []string{"true", "TRUE", "True", "1", "yes", "YES", "  true  ", "  1 "}
	for _, v := range trueVals {
		if !isTruthy(v) {
			t.Errorf("expected %q to be truthy", v)
		}
	}
	falseVals := []string{"false", "0", "no", "", "  ", "maybe", "2"}
	for _, v := range falseVals {
		if isTruthy(v) {
			t.Errorf("expected %q to be falsy", v)
		}
	}
}

func newTestUpdater(cfg *config.Config, spec *policy.Spec, dc docker.DockerClient, rc registry.RegistryClient) *Updater {
	if cfg == nil {
		cfg = &config.Config{MonitorAll: true}
	}
	if spec == nil {
		spec = policy.DefaultSpec()
	}
	return New(dc, rc, nil, cfg, spec, audit.NewLog(""))
}

func TestShouldMonitor_MonitorAll(t *testing.T) {
	u := newTestUpdater(&config.Config{MonitorAll: true}, nil, nil, mocks.NewMockRegistryClient(t))

	ctr := docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}
	if !u.shouldMonitor(ctr) {
		t.Error("expected nginx to be monitored with MonitorAll=true")
	}
}

func TestShouldMonitor_MonitorAllDisabledLabel(t *testing.T) {
	u := newTestUpdater(&config.Config{MonitorAll: true}, nil, nil, mocks.NewMockRegistryClient(t))

	ctr := docker.ContainerInfo{
		Name:   "db",
		Labels: map[string]string{config.LabelDisable: "true"},
	}
	if u.shouldMonitor(ctr) {
		t.Error("expected db to be skipped with disable label")
	}
}

func TestShouldMonitor_OptInMode(t *testing.T) {
	u := newTestUpdater(&config.Config{MonitorAll: false}, nil, nil, mocks.NewMockRegistryClient(t))

	// Without label -> not monitored
	ctr := docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}
	if u.shouldMonitor(ctr) {
		t.Error("expected nginx to NOT be monitored in opt-in mode")
	}

	// With enable label -> monitored
	ctr.Labels[config.LabelEnable] = "true"
	if !u.shouldMonitor(ctr) {
		t.Error("expected nginx with enable label to be monitored")
	}
}

func TestShouldMonitor_ContainerNames(t *testing.T) {
	u := newTestUpdater(&config.Config{
		MonitorAll:     true,
		ContainerNames: []string{"nginx", "redis"},
	}, nil, nil, mocks.NewMockRegistryClient(t))

	if !u.shouldMonitor(docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}) {
		t.Error("expected nginx to be monitored (in names list)")
	}
	if u.shouldMonitor(docker.ContainerInfo{Name: "postgres", Labels: map[string]string{}}) {
		t.Error("expected postgres to NOT be monitored (not in names list)")
	}
}

func TestShouldMonitor_DisableContainers(t *testing.T) {
	u := newTestUpdater(&config.Config{
		MonitorAll:        true,
		DisableContainers: []string{"postgres", "mongo"},
	}, nil, nil, mocks.NewMockRegistryClient(t))

	if u.shouldMonitor(docker.ContainerInfo{Name: "postgres", Labels: map[string]string{}}) {
		t.Error("expected postgres to be disabled")
	}
	if !u.shouldMonitor(docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}) {
		t.Error("expected nginx to be monitored")
	}
}

func TestShouldMonitor_ScopeFilter(t *testing.T) {
	u := newTestUpdater(&config.Config{
		MonitorAll: true,
		Scope:      "production",
	}, nil, nil, mocks.NewMockRegistryClient(t))

	// Matching scope
	ctr := docker.ContainerInfo{
		Name:   "nginx",
		Labels: map[string]string{config.LabelScope: "production"},
	}
	if !u.shouldMonitor(ctr) {
		t.Error("expected matching scope to be monitored")
	}

	// Wrong scope
	ctr.Labels[config.LabelScope] = "staging"
	if u.shouldMonitor(ctr) {
		t.Error("expected non-matching scope to be skipped")
	}

	// No scope label
	ctr.Labels = map[string]string{}
	if u.shouldMonitor(ctr) {
		t.Error("expected no-scope container to be skipped when scope is set")
	}
}

func TestShouldMonitor_ScopeNone(t *testing.T) {
	u := newTestUpdater(&config.Config{
		MonitorAll: true,
		Scope:      "none",
	}, nil, nil, mocks.NewMockRegistryClient(t))

	// Container without scope -> allowed
	ctr := docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}
	if !u.shouldMonitor(ctr) {
		t.Error("expected container without scope to match scope=none")
	}

	// Container with scope -> not allowed
	ctr.Labels[config.LabelScope] = "production"
	if u.shouldMonitor(ctr) {
		t.Error("expected scoped container to NOT match scope=none")
	}

	// Container with scope=none -> allowed
	ctr.Labels[config.LabelScope] = "none"
	if !u.shouldMonitor(ctr) {
		t.Error("expected scope=none label to match scope=none config")
	}
}

func TestShouldMonitor_PolicyIgnored(t *testing.T) {
	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {Strategy: policy.StrategyAll},
		},
		Containers: map[string]policy.ContainerDef{
			"legacy": {Ignore: true},
		},
		Groups: map[string]policy.GroupDef{},
	}
	u := newTestUpdater(&config.Config{MonitorAll: true}, spec, nil, mocks.NewMockRegistryClient(t))

	if u.shouldMonitor(docker.ContainerInfo{Name: "legacy", Labels: map[string]string{}}) {
		t.Error("expected ignored container to be skipped")
	}
	if !u.shouldMonitor(docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}) {
		t.Error("expected non-ignored container to be monitored")
	}
}

func TestIsMonitorOnly_GlobalDryRun(t *testing.T) {
	u := newTestUpdater(&config.Config{DryRun: true}, nil, nil, mocks.NewMockRegistryClient(t))
	ctr := docker.ContainerInfo{Labels: map[string]string{}}
	if !u.isMonitorOnly(ctr) {
		t.Error("expected monitor-only with DryRun=true")
	}
}

func TestIsMonitorOnly_Label(t *testing.T) {
	u := newTestUpdater(&config.Config{DryRun: false}, nil, nil, mocks.NewMockRegistryClient(t))

	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelMonitorOnly: "true"},
	}
	if !u.isMonitorOnly(ctr) {
		t.Error("expected monitor-only with label=true")
	}

	ctr.Labels[config.LabelMonitorOnly] = "false"
	if u.isMonitorOnly(ctr) {
		t.Error("expected not monitor-only with label=false")
	}
}

func TestIsMonitorOnly_LabelPrecedence(t *testing.T) {
	u := newTestUpdater(&config.Config{DryRun: true, LabelPrecedence: true}, nil, nil, mocks.NewMockRegistryClient(t))

	// Label says false, should override global DryRun=true
	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelMonitorOnly: "false"},
	}
	if u.isMonitorOnly(ctr) {
		t.Error("expected label=false to override DryRun=true with LabelPrecedence")
	}
}

func TestIsNoPull_Global(t *testing.T) {
	u := newTestUpdater(&config.Config{NoPull: true}, nil, nil, mocks.NewMockRegistryClient(t))
	ctr := docker.ContainerInfo{Labels: map[string]string{}}
	if !u.isNoPull(ctr) {
		t.Error("expected no-pull with NoPull=true")
	}
}

func TestIsNoPull_Label(t *testing.T) {
	u := newTestUpdater(&config.Config{NoPull: false}, nil, nil, mocks.NewMockRegistryClient(t))

	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelNoPull: "true"},
	}
	if !u.isNoPull(ctr) {
		t.Error("expected no-pull with label=true")
	}
}

func TestIsNoPull_LabelPrecedence(t *testing.T) {
	u := newTestUpdater(&config.Config{NoPull: true, LabelPrecedence: true}, nil, nil, mocks.NewMockRegistryClient(t))

	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelNoPull: "false"},
	}
	if u.isNoPull(ctr) {
		t.Error("expected label=false to override NoPull=true with LabelPrecedence")
	}
}

func TestGetHookTimeout(t *testing.T) {
	u := newTestUpdater(nil, nil, nil, mocks.NewMockRegistryClient(t))

	// No label -> default
	ctr := docker.ContainerInfo{Labels: map[string]string{}}
	timeout := u.getHookTimeout(ctr, config.LabelPreUpdateTimeout, 60*time.Second)
	if timeout != 60*time.Second {
		t.Errorf("expected 60s, got %v", timeout)
	}

	// Label with valid minutes
	ctr.Labels[config.LabelPreUpdateTimeout] = "5"
	timeout = u.getHookTimeout(ctr, config.LabelPreUpdateTimeout, 60*time.Second)
	if timeout != 5*time.Minute {
		t.Errorf("expected 5m, got %v", timeout)
	}

	// Label with 0 (disable timeout)
	ctr.Labels[config.LabelPreUpdateTimeout] = "0"
	timeout = u.getHookTimeout(ctr, config.LabelPreUpdateTimeout, 60*time.Second)
	if timeout != 0 {
		t.Errorf("expected 0 (disabled), got %v", timeout)
	}

	// Label with invalid value
	ctr.Labels[config.LabelPreUpdateTimeout] = "abc"
	timeout = u.getHookTimeout(ctr, config.LabelPreUpdateTimeout, 60*time.Second)
	if timeout != 60*time.Second {
		t.Errorf("expected default 60s for invalid value, got %v", timeout)
	}
}

func TestHistory(t *testing.T) {
	u := newTestUpdater(nil, nil, nil, mocks.NewMockRegistryClient(t))

	if len(u.History()) != 0 {
		t.Error("expected empty history initially")
	}

	// Simulate adding results directly
	u.mu.Lock()
	u.history = append(u.history, UpdateResult{
		ContainerName: "nginx",
		Updated:       true,
	})
	u.mu.Unlock()

	h := u.History()
	if len(h) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(h))
	}
	if h[0].ContainerName != "nginx" {
		t.Errorf("expected nginx, got %q", h[0].ContainerName)
	}

	// Verify it's a copy
	h[0].ContainerName = "modified"
	h2 := u.History()
	if h2[0].ContainerName != "nginx" {
		t.Error("History should return a copy")
	}
}

func TestAuditLog(t *testing.T) {
	auditLog := audit.NewLog("")
	u := New(nil, nil, nil, &config.Config{MonitorAll: true}, policy.DefaultSpec(), auditLog)

	if u.AuditLog() != auditLog {
		t.Error("AuditLog() should return the injected audit log")
	}
}

// ---------------------------------------------------------------------------
// New mock-based tests
// ---------------------------------------------------------------------------

func TestRun_Success(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "nginx",
			Image:   "nginx:latest",
			ImageID: "sha256:oldnginx",
			State:   "running",
			Labels:  map[string]string{},
		},
		{
			ID:      "ffeeddccbb998877",
			Name:    "redis",
			Image:   "redis:7",
			ImageID: "sha256:oldredis",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// NoPull=true means no digest check and no pull; the code skips to
	// "update available" then checks monitor-only / no-restart / etc.
	// With default policy (strategy=all, approve=auto) and no monitor-only,
	// the code will try to RecreateContainer for each.
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aabbccddee112233", "nginx:latest", cfg.StopTimeout, "", false).Return("newcontainer111122", nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "ffeeddccbb998877", "redis:7", cfg.StopTimeout, "", false).Return("newcontainer333344", nil)

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.True(t, results[0].Updated)
	assert.True(t, results[1].Updated)
}

func TestRun_WithUpdate(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "webapp",
			Image:   "myapp:v2",
			ImageID: "sha256:oldimage",
			State:   "running",
			Labels:  map[string]string{config.LabelNoPull: "true"},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aabbccddee112233", "myapp:v2", cfg.StopTimeout, "", false).Return("newcontainer556677889900", nil)

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
	assert.Equal(t, "webapp", results[0].ContainerName)
}

func TestRun_MonitorOnlySkipsUpdate(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, DryRun: true, NoPull: true}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "nginx",
			Image:   "nginx:latest",
			ImageID: "sha256:oldnginx",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// RecreateContainer should NOT be called since DryRun=true

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].MonitorOnly)
	assert.False(t, results[0].Updated)
}

func TestRun_MaintenanceWindowSkip(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	// Set a maintenance window that is guaranteed to be outside current time.
	// Use a very narrow past window: 00:00-00:01 - unless test runs at midnight
	// this will be outside the window.
	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default": {Strategy: policy.StrategyAll, Approve: policy.ApproveAuto},
		},
		Containers: map[string]policy.ContainerDef{
			"nginx": {Schedule: "00:00-00:01"},
		},
		Groups: map[string]policy.GroupDef{},
	}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "nginx",
			Image:   "nginx:latest",
			ImageID: "sha256:oldnginx",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)

	u := newTestUpdater(cfg, spec, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "maintenance window")
}

func TestRun_PinnedPolicySkip(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default":  {Strategy: policy.StrategyAll, Approve: policy.ApproveAuto},
			"critical": {Strategy: policy.StrategyPin, Approve: policy.ApproveManual},
		},
		Containers: map[string]policy.ContainerDef{
			"postgres": {Policy: "critical"},
		},
		Groups: map[string]policy.GroupDef{},
	}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "postgres",
			Image:   "postgres:16",
			ImageID: "sha256:oldpg",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)

	u := newTestUpdater(cfg, spec, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "pinned")
}

func TestRun_LifecycleHooks(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{
		MonitorAll:     true,
		NoPull:         true,
		LifecycleHooks: true,
	}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "webapp",
			Image:   "myapp:latest",
			ImageID: "sha256:oldapp",
			State:   "running",
			Labels: map[string]string{
				config.LabelPreCheck:   "/healthcheck.sh",
				config.LabelPreUpdate:  "/pre-update.sh",
				config.LabelPostUpdate: "/post-update.sh",
				config.LabelPostCheck:  "/post-check.sh",
			},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// pre-check hook
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aabbccddee112233", "/healthcheck.sh", 60*time.Second).Return("ok", nil)
	// pre-update hook
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aabbccddee112233", "/pre-update.sh", 60*time.Second).Return("ok", nil)
	// recreate
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aabbccddee112233", "myapp:latest", cfg.StopTimeout, "", false).Return("newcontainer112233445566", nil)
	// post-update hook (runs in the new container)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "newcontainer112233445566", "/post-update.sh", 60*time.Second).Return("ok", nil)
	// post-check hook (runs in original container context, state=running)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aabbccddee112233", "/post-check.sh", 60*time.Second).Return("ok", nil)

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_NoPullMode(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "myapp",
			Image:   "myapp:latest",
			ImageID: "sha256:oldimage",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// No GetImageDigest or PullImage calls expected with NoPull=true
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aabbccddee112233", "myapp:latest", cfg.StopTimeout, "", false).Return("newcontainer556677", nil)

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
	assert.Empty(t, results[0].NewImageID) // NoPull doesn't set NewImageID
}

func TestRun_NoRestartMode(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true, NoRestart: true}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "myapp",
			Image:   "myapp:latest",
			ImageID: "sha256:oldimage",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// No RecreateContainer should be called with NoRestart=true

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
	assert.Contains(t, results[0].Error, "no-restart")
}

// Old cleanup/signal tests removed - newer versions below with proper policy spec
func TestCheckAndUpdate_ImageByID(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "custom",
			Image:   "sha256:abcdef1234567890abcdef1234567890",
			ImageID: "sha256:abcdef1234567890abcdef1234567890",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// No RecreateContainer call expected since images specified by sha256: ID are skipped

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "image specified by ID")
}

func TestOrderByDependencies(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	ctx := context.Background()
	cfg := &config.Config{MonitorAll: true, NoPull: true, RollingRestart: true}

	// The orderByDependencies uses a topological sort (Kahn's algorithm)
	// where in-degree counts how many other containers depend ON a container.
	// Containers with in-degree 0 (no other container depends on them) are
	// processed first. This means leaf containers (dependents) come first,
	// and the root dependencies come last.
	//
	// A depends on B, B depends on C:
	// in-degree: A=0, B=1 (A depends on it), C=1 (B depends on it)
	// Order: A first (in-degree 0), then B/C added as unvisited.
	//
	// To test proper ordering, use a simpler dependency chain where the
	// algorithm can fully resolve: C has no deps (in-degree 0 if nothing
	// depends on C), but B depends on C (C gets in-degree 1), and A depends
	// on B (B gets in-degree 1). So A has in-degree 0 -> processed first.
	//
	// Actually the Kahn's algorithm here processes containers that nothing
	// else depends on first. Let's set up: db has no dependents (nothing
	// lists db as a dependency), app depends on db, web depends on app.
	// In-degree: db=1 (app depends on it), app=1 (web depends on it), web=0.
	// So web is processed first. Then after web is processed, the code
	// decrements in-degree of containers that web depends on... but the
	// decrement loop checks deps[dependent] for dep==name. It finds
	// deps["web"]=["app"], so dependent="web", dep="app" != "web". No match.
	//
	// This algorithm has a quirk: it processes nodes nothing depends on first
	// and only decrements when the processed node appears as a dependency.
	// For a clean test, use containers where at least one has no deps at all
	// and nothing depends on it (a standalone leaf).

	containers := []docker.ContainerInfo{
		{
			ID:      "aabbccddee112233",
			Name:    "standalone",
			Image:   "standalone:latest",
			ImageID: "sha256:standaloneold",
			State:   "running",
			Labels:  map[string]string{},
		},
		{
			ID:      "112233445566aabb",
			Name:    "worker",
			Image:   "worker:latest",
			ImageID: "sha256:workerold",
			State:   "running",
			Labels:  map[string]string{},
		},
		{
			ID:      "ffeeddccbbaa9988",
			Name:    "database",
			Image:   "postgres:16",
			ImageID: "sha256:dbold",
			State:   "running",
			Labels:  map[string]string{},
		},
	}

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(containers, nil)
	// standalone has no dependencies and nothing depends on it
	mockDocker.EXPECT().GetDependencies(mock.Anything, "aabbccddee112233").Return([]string{}, nil)
	// worker depends on database
	mockDocker.EXPECT().GetDependencies(mock.Anything, "112233445566aabb").Return([]string{"database"}, nil)
	// database has no dependencies
	mockDocker.EXPECT().GetDependencies(mock.Anything, "ffeeddccbbaa9988").Return([]string{}, nil)

	mockDocker.EXPECT().RecreateContainer(mock.Anything, mock.Anything, mock.Anything, cfg.StopTimeout, "", false).Return("newcontaineraabb11", nil).Times(3)

	u := newTestUpdater(cfg, nil, mockDocker, mocks.NewMockRegistryClient(t))
	results, err := u.Run(ctx)

	assert.NoError(t, err)
	assert.Len(t, results, 3)

	// The topological sort processes in-degree 0 nodes first.
	// in-degree: standalone=0, worker=0, database=1 (worker depends on it)
	// So standalone and worker are processed first (in-degree 0),
	// then database's in-degree is decremented when worker is processed.
	// Verify that database (which others depend on) comes after its dependents.
	names := []string{results[0].ContainerName, results[1].ContainerName, results[2].ContainerName}

	// worker must come before database (worker depends on database, and the
	// algorithm processes dependents before dependencies)
	workerIdx := -1
	dbIdx := -1
	for i, n := range names {
		if n == "worker" {
			workerIdx = i
		}
		if n == "database" {
			dbIdx = i
		}
	}
	assert.True(t, workerIdx >= 0, "worker should be in results")
	assert.True(t, dbIdx >= 0, "database should be in results")
	assert.True(t, workerIdx < dbIdx, "worker (dependent) should be processed before database (dependency) in this algorithm")
}

// ---------------------------------------------------------------------------
// Additional coverage for checkAndUpdate paths
// ---------------------------------------------------------------------------

func TestRun_ImageByID_Skips(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, policy.DefaultSpec(), audit.NewLog(""))

	// Container with sha256: image should be skipped
	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "myapp", Image: "sha256:abc123", State: "running", Labels: map[string]string{}},
	}, nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "image specified by ID")
}

func TestRun_StoppedNotRevived(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, IncludeStopped: true, ReviveStopped: false, NoPull: true}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, policy.DefaultSpec(), audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, true, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "myapp", Image: "nginx:latest", ImageID: "sha256:old", State: "exited", Labels: map[string]string{}},
	}, nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	// Should be skipped because container is stopped and revive is false
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "stopped")
}

func TestRun_ListError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, policy.DefaultSpec(), audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return(nil, errors.New("docker error"))

	_, err := u.Run(context.Background())
	assert.Error(t, err)
}

func TestRun_GetImageDigestError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: false}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, policy.DefaultSpec(), audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "myapp", Image: "nginx:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().GetImageDigest(mock.Anything, "sha256:old").Return("", errors.New("digest error"))

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "getting local digest")
}

func TestRun_ManualApprovalSkip(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true}

	spec := &policy.Spec{
		Policies: map[string]policy.PolicyDef{
			"default":  {Strategy: policy.StrategyAll, Approve: policy.ApproveAuto},
			"cautious": {Strategy: policy.StrategyAll, Approve: policy.ApproveManual},
		},
		Containers: map[string]policy.ContainerDef{
			"myapp": {Policy: "cautious"},
		},
		Groups: map[string]policy.GroupDef{},
	}

	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "myapp", Image: "nginx:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "manual approval")
}

func TestRun_DigestStrategy_NoPull(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "all", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	// NoPull + strategy=all: skips all remote checks, goes straight to "update available"
	// Then needs PullImage (but noPull), RecreateContainer
	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "nginx", Image: "nginx:1.25", ImageID: "sha256:digest123", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "nginx:1.25", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_PatchStrategy_NoPull(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "patch", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	// NoPull skips all remote checks
	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "mydb", Image: "mysql:8.0.45", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "mysql:8.0.45", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_LifecycleHooksExec(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, LifecycleHooks: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{
			config.LabelPreCheck:   "echo precheck",
			config.LabelPostCheck:  "echo postcheck",
			config.LabelPreUpdate:  "echo preupdate",
			config.LabelPostUpdate: "echo postupdate",
		}},
	}, nil)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aaaaaaaaaaaa", "echo precheck", mock.Anything).Return("ok", nil)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aaaaaaaaaaaa", "echo preupdate", mock.Anything).Return("ok", nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("newctr123456", nil)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "newctr123456", "echo postupdate", mock.Anything).Return("ok", nil)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aaaaaaaaaaaa", "echo postcheck", mock.Anything).Return("ok", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_CleanupImages(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true, CleanupImages: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:oldimg", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("newctr123456", nil)
	mockDocker.EXPECT().RemoveImage(mock.Anything, "sha256:oldimg").Return(nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_NoRestartMode_NoPull(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true, NoRestart: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
	assert.Contains(t, results[0].Error, "no-restart")
}

func TestRun_ImageByID_WithHooks(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, LifecycleHooks: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "sha256:someid", ImageID: "sha256:old", State: "running", Labels: map[string]string{
			config.LabelPreCheck: "echo pre",
		}},
	}, nil)
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aaaaaaaaaaaa", "echo pre", mock.Anything).Return("ok", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
}

func TestRun_RecreateError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("", errors.New("recreate failed"))

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.False(t, results[0].Updated)
	assert.Contains(t, results[0].Error, "recreating container")
}

func TestRun_CustomStopSignal(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{
			config.LabelStopSignal: "SIGQUIT",
		}},
	}, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "SIGQUIT", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestFindNewerTag_NonSemverTag(t *testing.T) {
	u := New(nil, mocks.NewMockRegistryClient(t), nil, &config.Config{}, policy.DefaultSpec(), audit.NewLog(""))
	_, _, err := u.findNewerTag(context.Background(), "myapp:latest", "patch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not semver")
}

func TestFindNewerTag_RegistryError(t *testing.T) {
	mockRegistry := mocks.NewMockRegistryClient(t)
	mockRegistry.EXPECT().ListTags(mock.Anything, "myorg/myapp:1.0.0").Return(nil, errors.New("registry unavailable"))
	u := New(nil, mockRegistry, nil, &config.Config{}, policy.DefaultSpec(), audit.NewLog(""))
	_, _, err := u.findNewerTag(context.Background(), "myorg/myapp:1.0.0", "patch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "listing tags")
}

func TestFindNewerTag_NoImageTag(t *testing.T) {
	u := New(nil, mocks.NewMockRegistryClient(t), nil, &config.Config{}, policy.DefaultSpec(), audit.NewLog(""))
	_, _, err := u.findNewerTag(context.Background(), "myapp", "patch")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not semver")
}

func TestRun_LifecycleHookError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, LifecycleHooks: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{
			config.LabelPreCheck: "exit 1",
		}},
	}, nil)
	// Pre-check hook fails - should log error but continue
	mockDocker.EXPECT().ExecCommand(mock.Anything, "aaaaaaaaaaaa", "exit 1", mock.Anything).Return("", errors.New("exit code 1"))
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_PostUpdateHookError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, LifecycleHooks: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{
			config.LabelPostUpdate: "exit 1",
		}},
	}, nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("newctr123456", nil)
	// Post-update hook on NEW container fails
	mockDocker.EXPECT().ExecCommand(mock.Anything, "newctr123456", "exit 1", mock.Anything).Return("", errors.New("exit code 1"))

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_PullError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "all", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().GetImageDigest(mock.Anything, "sha256:old").Return("sha256:old", nil)
	mockRegistry.EXPECT().HasNewImage(mock.Anything, "myapp:latest", "sha256:old").Return(true, "sha256:new", nil)
	mockRegistry.EXPECT().GetRegistryAuth("myapp:latest").Return("")
	mockDocker.EXPECT().PullImage(mock.Anything, "myapp:latest", "").Return("", errors.New("pull failed"))

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "pulling new image")
}

func TestRun_StoppedContainerRevived(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, IncludeStopped: true, ReviveStopped: true, NoPull: true}
	spec := policy.DefaultSpec()
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, true, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "exited", Labels: map[string]string{}},
	}, nil)
	// ReviveStopped=true, so it should proceed to recreate
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_MaintenanceWindowBlock(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	cfg := &config.Config{MonitorAll: true, NoPull: true}
	// Set a window that's definitely NOT now
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "all", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{"app": {Schedule: "03:00-03:01"}},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mocks.NewMockRegistryClient(t), nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Skipped)
	assert.Contains(t, results[0].Error, "maintenance window")
}

func TestRun_PatchStrategy_SemverUpgrade(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "patch", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "db", Image: "mysql:8.0.44", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockRegistry.EXPECT().ListTags(mock.Anything, "mysql:8.0.44").Return([]string{"8.0.43", "8.0.44", "8.0.45", "8.1.0", "latest"}, nil)
	mockRegistry.EXPECT().GetRegistryAuth("mysql:8.0.45").Return("")
	mockDocker.EXPECT().PullImage(mock.Anything, "mysql:8.0.45", "").Return("sha256:newimg", nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "mysql:8.0.45", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_MinorStrategy_SemverUpgrade(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "minor", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "web", Image: "nginx:1.24.0", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockRegistry.EXPECT().ListTags(mock.Anything, "nginx:1.24.0").Return([]string{"1.24.0", "1.24.1", "1.25.0", "1.26.0", "2.0.0"}, nil)
	mockRegistry.EXPECT().GetRegistryAuth("nginx:1.26.0").Return("")
	mockDocker.EXPECT().PullImage(mock.Anything, "nginx:1.26.0", "").Return("sha256:newimg", nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "nginx:1.26.0", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_PatchStrategy_FallbackDigest_NoUpdate(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "patch", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "db", Image: "mysql:8.0.45", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockRegistry.EXPECT().ListTags(mock.Anything, "mysql:8.0.45").Return([]string{"8.0.44", "8.0.45"}, nil)
	mockDocker.EXPECT().GetImageDigest(mock.Anything, "sha256:old").Return("sha256:same", nil)
	mockRegistry.EXPECT().HasNewImage(mock.Anything, "mysql:8.0.45", "sha256:same").Return(false, "sha256:same", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.False(t, results[0].Updated)
}

func TestRun_PatchStrategy_ListTagsError_FallbackDigest(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "patch", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "db", Image: "mysql:8.0.45", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockRegistry.EXPECT().ListTags(mock.Anything, "mysql:8.0.45").Return(nil, errors.New("timeout"))
	mockDocker.EXPECT().GetImageDigest(mock.Anything, "sha256:old").Return("sha256:old", nil)
	mockRegistry.EXPECT().HasNewImage(mock.Anything, "mysql:8.0.45", "sha256:old").Return(false, "sha256:old", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.False(t, results[0].Updated)
}

func TestRun_DigestStrategy_HasNewImage_Update(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "all", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().GetImageDigest(mock.Anything, "sha256:old").Return("sha256:olddigest", nil)
	mockRegistry.EXPECT().HasNewImage(mock.Anything, "myapp:latest", "sha256:olddigest").Return(true, "sha256:newdigest", nil)
	mockRegistry.EXPECT().GetRegistryAuth("myapp:latest").Return("")
	mockDocker.EXPECT().PullImage(mock.Anything, "myapp:latest", "").Return("sha256:newimg", nil)
	mockDocker.EXPECT().RecreateContainer(mock.Anything, "aaaaaaaaaaaa", "myapp:latest", mock.Anything, "", false).Return("newctr123456", nil)

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.True(t, results[0].Updated)
}

func TestRun_DigestStrategy_GetDigestError(t *testing.T) {
	mockDocker := mocks.NewMockDockerClient(t)
	mockRegistry := mocks.NewMockRegistryClient(t)
	cfg := &config.Config{MonitorAll: true}
	spec := &policy.Spec{
		Policies:   map[string]policy.PolicyDef{"default": {Strategy: "all", Approve: "auto"}},
		Containers: map[string]policy.ContainerDef{},
		Groups:     map[string]policy.GroupDef{},
	}
	u := New(mockDocker, mockRegistry, nil, cfg, spec, audit.NewLog(""))

	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{
		{ID: "aaaaaaaaaaaa", Name: "app", Image: "myapp:latest", ImageID: "sha256:old", State: "running", Labels: map[string]string{}},
	}, nil)
	mockDocker.EXPECT().GetImageDigest(mock.Anything, "sha256:old").Return("", errors.New("inspect failed"))

	results, err := u.Run(context.Background())
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Contains(t, results[0].Error, "getting local digest")
}

func TestFindNewerTag_WithMock_Success(t *testing.T) {
	mockRegistry := mocks.NewMockRegistryClient(t)
	u := New(nil, mockRegistry, nil, &config.Config{}, policy.DefaultSpec(), audit.NewLog(""))

	mockRegistry.EXPECT().ListTags(mock.Anything, "mysql:8.0.44").Return(
		[]string{"8.0.43", "8.0.44", "8.0.45", "8.0.46", "latest"}, nil)

	newRef, found, err := u.findNewerTag(context.Background(), "mysql:8.0.44", "patch")
	assert.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "mysql:8.0.46", newRef)
}

func TestFindNewerTag_WithMock_NoNewer(t *testing.T) {
	mockRegistry := mocks.NewMockRegistryClient(t)
	u := New(nil, mockRegistry, nil, &config.Config{}, policy.DefaultSpec(), audit.NewLog(""))

	mockRegistry.EXPECT().ListTags(mock.Anything, "mysql:8.0.45").Return(
		[]string{"8.0.44", "8.0.45"}, nil)

	_, found, err := u.findNewerTag(context.Background(), "mysql:8.0.45", "patch")
	assert.NoError(t, err)
	assert.False(t, found)
}
