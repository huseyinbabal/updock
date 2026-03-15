package policy

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultSpec(t *testing.T) {
	spec := DefaultSpec()
	if spec == nil {
		t.Fatal("DefaultSpec returned nil")
	}
	if len(spec.Policies) != 1 {
		t.Fatalf("expected 1 default policy, got %d", len(spec.Policies))
	}
	p, ok := spec.Policies["default"]
	if !ok {
		t.Fatal("missing 'default' policy")
	}
	if p.Strategy != StrategyAll {
		t.Errorf("expected strategy %q, got %q", StrategyAll, p.Strategy)
	}
	if p.Approve != ApproveAuto {
		t.Errorf("expected approve %q, got %q", ApproveAuto, p.Approve)
	}
	if p.Rollback != RollbackOnFailure {
		t.Errorf("expected rollback %q, got %q", RollbackOnFailure, p.Rollback)
	}
	if p.HealthTimeout != 30*time.Second {
		t.Errorf("expected health_timeout 30s, got %v", p.HealthTimeout)
	}
}

func TestLoadSpec_FileNotExists(t *testing.T) {
	spec, err := LoadSpec("/nonexistent/updock.yml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got %v", err)
	}
	if spec == nil {
		t.Fatal("expected default spec, got nil")
	}
	if len(spec.Policies) != 1 {
		t.Fatalf("expected default spec with 1 policy, got %d", len(spec.Policies))
	}
}

func TestLoadSpec_ValidFile(t *testing.T) {
	content := `
policies:
  default:
    strategy: patch
    approve: manual
    rollback: never
  strict:
    strategy: pin
    approve: manual
containers:
  nginx:
    policy: default
    schedule: "02:00-04:00"
  postgres:
    policy: strict
    ignore: false
  legacy:
    ignore: true
groups:
  web:
    members: [nginx, app]
    strategy: rolling
    order: [app, nginx]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "updock.yml")
	_ = os.WriteFile(path, []byte(content), 0644)

	spec, err := LoadSpec(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(spec.Policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(spec.Policies))
	}
	if spec.Policies["default"].Strategy != StrategyPatch {
		t.Errorf("expected patch strategy, got %q", spec.Policies["default"].Strategy)
	}
	if spec.Policies["strict"].Approve != ApproveManual {
		t.Errorf("expected manual approve, got %q", spec.Policies["strict"].Approve)
	}
	if len(spec.Containers) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(spec.Containers))
	}
	if spec.Containers["nginx"].Schedule != "02:00-04:00" {
		t.Error("missing nginx schedule")
	}
	if !spec.Containers["legacy"].Ignore {
		t.Error("legacy should be ignored")
	}
	if len(spec.Groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(spec.Groups))
	}
	g := spec.Groups["web"]
	if len(g.Members) != 2 {
		t.Errorf("expected 2 members, got %d", len(g.Members))
	}
	if g.Strategy != "rolling" {
		t.Errorf("expected rolling, got %q", g.Strategy)
	}
}

func TestLoadSpec_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")
	_ = os.WriteFile(path, []byte(":::invalid:::yaml"), 0644)

	_, err := LoadSpec(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadSpec_InvalidStrategy(t *testing.T) {
	content := `
policies:
  default:
    strategy: yolo
`
	dir := t.TempDir()
	path := filepath.Join(dir, "updock.yml")
	_ = os.WriteFile(path, []byte(content), 0644)

	_, err := LoadSpec(path)
	if err == nil {
		t.Fatal("expected error for invalid strategy")
	}
}

func TestGetContainerPolicy(t *testing.T) {
	spec := &Spec{
		Policies: map[string]PolicyDef{
			"default":  {Strategy: StrategyAll, Approve: ApproveAuto},
			"lockdown": {Strategy: StrategyPin, Approve: ApproveManual},
		},
		Containers: map[string]ContainerDef{
			"db": {Policy: "lockdown"},
		},
	}

	// Container with explicit policy
	p := spec.GetContainerPolicy("db")
	if p.Strategy != StrategyPin {
		t.Errorf("expected pin, got %q", p.Strategy)
	}

	// Container without explicit policy -> default
	p = spec.GetContainerPolicy("nginx")
	if p.Strategy != StrategyAll {
		t.Errorf("expected all, got %q", p.Strategy)
	}

	// Container referencing nonexistent policy -> default
	spec.Containers["bad"] = ContainerDef{Policy: "nonexistent"}
	p = spec.GetContainerPolicy("bad")
	if p.Strategy != StrategyAll {
		t.Errorf("expected all for nonexistent policy ref, got %q", p.Strategy)
	}

	// No default policy at all
	spec2 := &Spec{
		Policies:   map[string]PolicyDef{},
		Containers: map[string]ContainerDef{},
	}
	p = spec2.GetContainerPolicy("anything")
	if p.Strategy != StrategyAll {
		t.Errorf("expected all fallback, got %q", p.Strategy)
	}
}

func TestIsInMaintenanceWindow(t *testing.T) {
	spec := &Spec{
		Policies:   map[string]PolicyDef{},
		Containers: map[string]ContainerDef{},
	}

	// No container def -> always allowed
	if !spec.IsInMaintenanceWindow("nginx") {
		t.Error("expected true for container without schedule")
	}

	// Container with empty schedule -> always allowed
	spec.Containers["nginx"] = ContainerDef{Schedule: ""}
	if !spec.IsInMaintenanceWindow("nginx") {
		t.Error("expected true for empty schedule")
	}

	// Container with schedule (tested via isInWindow below)
	spec.Containers["nginx"] = ContainerDef{Schedule: "02:00-04:00"}
	// We can't control time.Now() here, so just verify it returns a bool
	_ = spec.IsInMaintenanceWindow("nginx")
}

func TestIsIgnored(t *testing.T) {
	spec := &Spec{
		Policies: map[string]PolicyDef{},
		Containers: map[string]ContainerDef{
			"old": {Ignore: true},
			"new": {Ignore: false},
		},
	}

	if !spec.IsIgnored("old") {
		t.Error("expected old to be ignored")
	}
	if spec.IsIgnored("new") {
		t.Error("expected new to not be ignored")
	}
	if spec.IsIgnored("unknown") {
		t.Error("expected unknown to not be ignored")
	}
}

func TestGetGroup(t *testing.T) {
	spec := &Spec{
		Policies:   map[string]PolicyDef{},
		Containers: map[string]ContainerDef{},
		Groups: map[string]GroupDef{
			"web": {
				Members:  []string{"nginx", "app", "redis"},
				Strategy: "rolling",
			},
		},
	}

	name, g, ok := spec.GetGroup("nginx")
	if !ok {
		t.Fatal("expected nginx to be in a group")
	}
	if name != "web" {
		t.Errorf("expected group 'web', got %q", name)
	}
	if g.Strategy != "rolling" {
		t.Errorf("expected rolling, got %q", g.Strategy)
	}

	_, _, ok = spec.GetGroup("postgres")
	if ok {
		t.Error("expected postgres to not be in any group")
	}
}

func TestIsInWindow(t *testing.T) {
	tests := []struct {
		name     string
		window   string
		hour     int
		minute   int
		expected bool
	}{
		{"inside window", "02:00-04:00", 3, 0, true},
		{"at window start", "02:00-04:00", 2, 0, true},
		{"before window", "02:00-04:00", 1, 59, false},
		{"at window end", "02:00-04:00", 4, 0, false},
		{"after window", "02:00-04:00", 5, 0, false},
		{"midnight crossing inside", "22:00-06:00", 23, 0, true},
		{"midnight crossing inside early", "22:00-06:00", 2, 0, true},
		{"midnight crossing outside", "22:00-06:00", 12, 0, false},
		{"midnight crossing at start", "22:00-06:00", 22, 0, true},
		{"invalid format", "invalid", 3, 0, true},
		{"missing dash", "0200", 3, 0, true},
		{"missing colon start", "0200-04:00", 3, 0, true},
		{"missing colon end", "02:00-0400", 3, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Date(2026, 3, 16, tt.hour, tt.minute, 0, 0, time.UTC)
			result := isInWindow(tt.window, now)
			if result != tt.expected {
				t.Errorf("isInWindow(%q, %02d:%02d) = %v, want %v",
					tt.window, tt.hour, tt.minute, result, tt.expected)
			}
		})
	}
}

func TestParseHM(t *testing.T) {
	h, m := parseHM("02", "30")
	if h != 2 || m != 30 {
		t.Errorf("expected 2:30, got %d:%d", h, m)
	}
	h, m = parseHM("23", "59")
	if h != 23 || m != 59 {
		t.Errorf("expected 23:59, got %d:%d", h, m)
	}
	h, m = parseHM("xx", "yy")
	if h != 0 || m != 0 {
		t.Errorf("expected 0:0 for invalid input, got %d:%d", h, m)
	}
}

func TestValidateStrategy(t *testing.T) {
	valid := []Strategy{StrategyAll, StrategyMajor, StrategyMinor, StrategyPatch, StrategyDigest, StrategyPin, ""}
	for _, s := range valid {
		if err := validateStrategy(s); err != nil {
			t.Errorf("expected no error for %q, got %v", s, err)
		}
	}
	if err := validateStrategy("yolo"); err == nil {
		t.Error("expected error for invalid strategy 'yolo'")
	}
}
