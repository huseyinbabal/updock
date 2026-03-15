// Package policy implements the declarative update policy engine for Updock.
//
// Updock uses a policy-based approach to container updates, fundamentally
// different from simple label-based tools. Policies are defined in an
// updock.yml configuration file and support:
//
//   - Semver-aware update constraints (patch, minor, major, or pinned)
//   - Maintenance windows (only update during specified time ranges)
//   - Approval modes (auto-approve or require manual approval via UI/API)
//   - Per-container and per-group policy overrides
//   - Health-check aware rollback
//
// # Example updock.yml
//
//	policies:
//	  default:
//	    strategy: patch        # only allow patch version updates
//	    approve: auto          # apply immediately
//	    rollback: on-failure   # rollback if health check fails
//
//	  critical:
//	    strategy: pin          # never auto-update
//	    approve: manual        # require manual approval
//
//	containers:
//	  nginx:
//	    policy: default
//	    schedule: "02:00-04:00"  # maintenance window
//
//	  postgres:
//	    policy: critical
//
//	groups:
//	  web-stack:
//	    members: [nginx, app, redis]
//	    strategy: rolling
//	    order: [redis, app, nginx]
package policy

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Strategy defines how aggressively Updock should update a container.
type Strategy string

const (
	// StrategyAll allows any image change (tag or digest), the most permissive.
	StrategyAll Strategy = "all"

	// StrategyMajor allows major, minor, and patch semver updates.
	StrategyMajor Strategy = "major"

	// StrategyMinor allows only minor and patch semver updates.
	StrategyMinor Strategy = "minor"

	// StrategyPatch allows only patch semver updates (safest auto-update mode).
	StrategyPatch Strategy = "patch"

	// StrategyDigest updates only when the digest changes for the same tag.
	StrategyDigest Strategy = "digest"

	// StrategyPin never auto-updates. Updates require manual approval.
	StrategyPin Strategy = "pin"
)

// ApproveMode defines whether updates are applied automatically or require approval.
type ApproveMode string

const (
	// ApproveAuto applies updates immediately without human intervention.
	ApproveAuto ApproveMode = "auto"

	// ApproveManual queues updates for manual approval via the Web UI or API.
	ApproveManual ApproveMode = "manual"
)

// RollbackMode defines when Updock should automatically rollback a failed update.
type RollbackMode string

const (
	// RollbackOnFailure rolls back if the new container fails to start
	// or does not pass its health check within the configured timeout.
	RollbackOnFailure RollbackMode = "on-failure"

	// RollbackNever disables automatic rollback. Failed updates are left
	// in their failed state for manual intervention.
	RollbackNever RollbackMode = "never"
)

// Spec is the top-level structure of an updock.yml configuration file.
// It defines update policies, per-container overrides, and container groups.
type Spec struct {
	// Policies maps policy names to their definitions.
	// The "default" policy is applied to containers without an explicit assignment.
	Policies map[string]PolicyDef `yaml:"policies"`

	// Containers maps container names to their configuration overrides.
	Containers map[string]ContainerDef `yaml:"containers"`

	// Groups defines sets of containers that should be updated together
	// in a specific order.
	Groups map[string]GroupDef `yaml:"groups"`
}

// PolicyDef defines an update policy with strategy, approval mode,
// rollback behavior, and health check settings.
type PolicyDef struct {
	// Strategy controls which image changes trigger an update.
	Strategy Strategy `yaml:"strategy"`

	// Approve controls whether updates are applied automatically or queued.
	Approve ApproveMode `yaml:"approve"`

	// Rollback controls automatic rollback behavior on failure.
	Rollback RollbackMode `yaml:"rollback"`

	// HealthTimeout is how long to wait for a container to become healthy
	// after an update before triggering a rollback (default: 30s).
	HealthTimeout time.Duration `yaml:"health_timeout"`
}

// ContainerDef defines per-container configuration in updock.yml.
type ContainerDef struct {
	// Policy is the name of the policy to apply (references Spec.Policies).
	Policy string `yaml:"policy"`

	// Schedule is a maintenance window in "HH:MM-HH:MM" format (24h).
	// Updates are only applied within this window.
	Schedule string `yaml:"schedule"`

	// Ignore excludes this container from all update checks.
	Ignore bool `yaml:"ignore"`
}

// GroupDef defines a container group for coordinated updates.
type GroupDef struct {
	// Members lists the container names in this group.
	Members []string `yaml:"members"`

	// Strategy defines the update strategy: "parallel" or "rolling".
	Strategy string `yaml:"strategy"`

	// Order defines the restart order for rolling updates.
	// Containers are restarted in the specified order.
	Order []string `yaml:"order"`
}

// DefaultSpec returns a Spec with sensible defaults when no updock.yml exists.
func DefaultSpec() *Spec {
	return &Spec{
		Policies: map[string]PolicyDef{
			"default": {
				Strategy:      StrategyAll,
				Approve:       ApproveAuto,
				Rollback:      RollbackOnFailure,
				HealthTimeout: 30 * time.Second,
			},
		},
		Containers: make(map[string]ContainerDef),
		Groups:     make(map[string]GroupDef),
	}
}

// LoadSpec reads and parses an updock.yml file. If the file does not exist,
// it returns the default spec. If the file exists but is invalid, an error
// is returned.
func LoadSpec(path string) (*Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSpec(), nil
		}
		return nil, fmt.Errorf("reading policy file %s: %w", path, err)
	}

	spec := DefaultSpec()
	if err := yaml.Unmarshal(data, spec); err != nil {
		return nil, fmt.Errorf("parsing policy file %s: %w", path, err)
	}

	// Validate
	for name, p := range spec.Policies {
		if err := validateStrategy(p.Strategy); err != nil {
			return nil, fmt.Errorf("policy %q: %w", name, err)
		}
	}

	return spec, nil
}

// GetContainerPolicy returns the effective policy for a container.
// It checks container-specific assignment first, then falls back to "default".
func (s *Spec) GetContainerPolicy(containerName string) PolicyDef {
	if cDef, ok := s.Containers[containerName]; ok && cDef.Policy != "" {
		if pDef, ok := s.Policies[cDef.Policy]; ok {
			return pDef
		}
	}

	if pDef, ok := s.Policies["default"]; ok {
		return pDef
	}

	return PolicyDef{
		Strategy: StrategyAll,
		Approve:  ApproveAuto,
		Rollback: RollbackOnFailure,
	}
}

// IsInMaintenanceWindow checks whether the current time falls within
// the container's maintenance window. Returns true if no window is set
// (always allowed) or if the current time is within the window.
func (s *Spec) IsInMaintenanceWindow(containerName string) bool {
	cDef, ok := s.Containers[containerName]
	if !ok || cDef.Schedule == "" {
		return true // no window = always allowed
	}

	return isInWindow(cDef.Schedule, time.Now())
}

// IsIgnored checks whether a container is marked as ignored in the spec.
func (s *Spec) IsIgnored(containerName string) bool {
	cDef, ok := s.Containers[containerName]
	return ok && cDef.Ignore
}

// GetGroup returns the group a container belongs to, if any.
func (s *Spec) GetGroup(containerName string) (string, *GroupDef, bool) {
	for name, g := range s.Groups {
		for _, m := range g.Members {
			if m == containerName {
				return name, &g, true
			}
		}
	}
	return "", nil, false
}

// isInWindow checks if the current time is within an "HH:MM-HH:MM" window.
func isInWindow(window string, now time.Time) bool {
	parts := strings.SplitN(window, "-", 2)
	if len(parts) != 2 {
		return true
	}

	startParts := strings.SplitN(strings.TrimSpace(parts[0]), ":", 2)
	endParts := strings.SplitN(strings.TrimSpace(parts[1]), ":", 2)
	if len(startParts) != 2 || len(endParts) != 2 {
		return true
	}

	startH, startM := parseHM(startParts[0], startParts[1])
	endH, endM := parseHM(endParts[0], endParts[1])

	nowMinutes := now.Hour()*60 + now.Minute()
	startMinutes := startH*60 + startM
	endMinutes := endH*60 + endM

	if startMinutes <= endMinutes {
		return nowMinutes >= startMinutes && nowMinutes < endMinutes
	}
	// Window crosses midnight
	return nowMinutes >= startMinutes || nowMinutes < endMinutes
}

func parseHM(h, m string) (int, int) {
	hh, mh := 0, 0
	fmt.Sscanf(h, "%d", &hh)
	fmt.Sscanf(m, "%d", &mh)
	return hh, mh
}

func validateStrategy(s Strategy) error {
	switch s {
	case StrategyAll, StrategyMajor, StrategyMinor, StrategyPatch, StrategyDigest, StrategyPin:
		return nil
	case "":
		return nil // defaults to "all"
	default:
		return fmt.Errorf("unknown strategy %q (valid: all, major, minor, patch, digest, pin)", s)
	}
}
