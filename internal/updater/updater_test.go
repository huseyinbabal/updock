package updater

import (
	"testing"
	"time"

	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/policy"
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

func newTestUpdater(cfg *config.Config, spec *policy.Spec) *Updater {
	if cfg == nil {
		cfg = &config.Config{MonitorAll: true}
	}
	if spec == nil {
		spec = policy.DefaultSpec()
	}
	return New(nil, nil, nil, cfg, spec, audit.NewLog(""))
}

func TestShouldMonitor_MonitorAll(t *testing.T) {
	u := newTestUpdater(&config.Config{MonitorAll: true}, nil)

	ctr := docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}
	if !u.shouldMonitor(ctr) {
		t.Error("expected nginx to be monitored with MonitorAll=true")
	}
}

func TestShouldMonitor_MonitorAllDisabledLabel(t *testing.T) {
	u := newTestUpdater(&config.Config{MonitorAll: true}, nil)

	ctr := docker.ContainerInfo{
		Name:   "db",
		Labels: map[string]string{config.LabelDisable: "true"},
	}
	if u.shouldMonitor(ctr) {
		t.Error("expected db to be skipped with disable label")
	}
}

func TestShouldMonitor_OptInMode(t *testing.T) {
	u := newTestUpdater(&config.Config{MonitorAll: false}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	}, nil)

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
	u := newTestUpdater(&config.Config{MonitorAll: true}, spec)

	if u.shouldMonitor(docker.ContainerInfo{Name: "legacy", Labels: map[string]string{}}) {
		t.Error("expected ignored container to be skipped")
	}
	if !u.shouldMonitor(docker.ContainerInfo{Name: "nginx", Labels: map[string]string{}}) {
		t.Error("expected non-ignored container to be monitored")
	}
}

func TestIsMonitorOnly_GlobalDryRun(t *testing.T) {
	u := newTestUpdater(&config.Config{DryRun: true}, nil)
	ctr := docker.ContainerInfo{Labels: map[string]string{}}
	if !u.isMonitorOnly(ctr) {
		t.Error("expected monitor-only with DryRun=true")
	}
}

func TestIsMonitorOnly_Label(t *testing.T) {
	u := newTestUpdater(&config.Config{DryRun: false}, nil)

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
	u := newTestUpdater(&config.Config{DryRun: true, LabelPrecedence: true}, nil)

	// Label says false, should override global DryRun=true
	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelMonitorOnly: "false"},
	}
	if u.isMonitorOnly(ctr) {
		t.Error("expected label=false to override DryRun=true with LabelPrecedence")
	}
}

func TestIsNoPull_Global(t *testing.T) {
	u := newTestUpdater(&config.Config{NoPull: true}, nil)
	ctr := docker.ContainerInfo{Labels: map[string]string{}}
	if !u.isNoPull(ctr) {
		t.Error("expected no-pull with NoPull=true")
	}
}

func TestIsNoPull_Label(t *testing.T) {
	u := newTestUpdater(&config.Config{NoPull: false}, nil)

	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelNoPull: "true"},
	}
	if !u.isNoPull(ctr) {
		t.Error("expected no-pull with label=true")
	}
}

func TestIsNoPull_LabelPrecedence(t *testing.T) {
	u := newTestUpdater(&config.Config{NoPull: true, LabelPrecedence: true}, nil)

	ctr := docker.ContainerInfo{
		Labels: map[string]string{config.LabelNoPull: "false"},
	}
	if u.isNoPull(ctr) {
		t.Error("expected label=false to override NoPull=true with LabelPrecedence")
	}
}

func TestGetHookTimeout(t *testing.T) {
	u := newTestUpdater(nil, nil)

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
	u := newTestUpdater(nil, nil)

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
