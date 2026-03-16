// Package updater orchestrates the Docker container update process.
//
// It ties together the Docker client, registry client, and notification system
// to perform the full update lifecycle:
//
//  1. List containers matching the configured filters.
//  2. For each container, check if a newer image is available.
//  3. Execute lifecycle hooks (pre-check, pre-update).
//  4. Pull the new image and recreate the container.
//  5. Execute lifecycle hooks (post-update, post-check).
//  6. Send notifications about the update result.
//
// # Container Selection
//
// The updater applies multiple filter layers to determine which containers
// to monitor:
//
//   - MonitorAll: when true, all containers are monitored by default.
//   - Container names: when specified, only named containers are checked.
//   - DisableContainers: containers excluded by name.
//   - Labels: com.updock.enable, com.updock.disable, com.updock.scope.
//   - Scope: when set, only containers with matching scope labels.
//
// # Rolling Restart
//
// When RollingRestart is enabled, containers are updated one at a time with
// dependency ordering. Containers with com.updock.depends-on labels are
// restarted after their dependencies.
package updater

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/gitops"
	"github.com/huseyinbabal/updock/internal/metrics"
	"github.com/huseyinbabal/updock/internal/notification"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/registry"
	"github.com/huseyinbabal/updock/internal/semver"
	log "github.com/sirupsen/logrus"
)

// UpdateResult represents the outcome of an update check for a single container.
// It is stored in the history and used for API responses and notifications.
type UpdateResult struct {
	ContainerID   string    `json:"container_id"`
	ContainerName string    `json:"container_name"`
	Image         string    `json:"image"`
	OldImageID    string    `json:"old_image_id"`
	NewImageID    string    `json:"new_image_id"`
	Updated       bool      `json:"updated"`
	Skipped       bool      `json:"skipped"`
	MonitorOnly   bool      `json:"monitor_only"`
	Error         string    `json:"error,omitempty"`
	CheckedAt     time.Time `json:"checked_at"`
}

// Updater is the core update engine. It coordinates container discovery,
// image comparison, policy evaluation, and the update lifecycle.
// It is safe for concurrent use.
type Updater struct {
	docker   docker.DockerClient
	registry *registry.Client
	notifier *notification.Notifier
	cfg      *config.Config
	spec     *policy.Spec
	audit    *audit.Log

	mu      sync.RWMutex
	history []UpdateResult
}

// New creates a new Updater with the given dependencies.
// The policy spec and audit log enable Updock's declarative policy engine
// and compliance-grade audit trail.
func New(dockerClient docker.DockerClient, registryClient *registry.Client, notifier *notification.Notifier, cfg *config.Config, spec *policy.Spec, auditLog *audit.Log) *Updater {
	return &Updater{
		docker:   dockerClient,
		registry: registryClient,
		notifier: notifier,
		cfg:      cfg,
		spec:     spec,
		audit:    auditLog,
		history:  make([]UpdateResult, 0),
	}
}

// AuditLog returns the audit log for API access.
func (u *Updater) AuditLog() *audit.Log {
	return u.audit
}

// Run performs a single update cycle: discover containers, check for updates,
// and apply them if configured to do so. Returns a slice of results for each
// container that was checked.
func (u *Updater) Run(ctx context.Context) ([]UpdateResult, error) {
	log.Info("Starting update check cycle")
	start := time.Now()
	defer func() {
		metrics.CheckDuration.Observe(time.Since(start).Seconds())
	}()

	containers, err := u.docker.ListContainers(ctx, u.cfg.IncludeStopped, u.cfg.IncludeRestarting)
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	// Build dependency graph for ordering if rolling restart is enabled
	var toUpdate []docker.ContainerInfo
	for _, ctr := range containers {
		if !u.shouldMonitor(ctr) {
			log.Debugf("Skipping container %s (not monitored)", ctr.Name)
			continue
		}
		toUpdate = append(toUpdate, ctr)
	}

	metrics.MonitoredContainers.Set(float64(len(toUpdate)))

	// Order by dependencies if rolling restart is enabled
	if u.cfg.RollingRestart {
		toUpdate = u.orderByDependencies(ctx, toUpdate)
	}

	var results []UpdateResult

	for _, ctr := range toUpdate {
		metrics.ContainersChecked.Inc()

		// Execute pre-check lifecycle hook
		if u.cfg.LifecycleHooks && ctr.State == "running" {
			u.execLifecycleHook(ctx, ctr, config.LabelPreCheck, 60*time.Second)
		}

		result := u.checkAndUpdate(ctx, ctr)
		results = append(results, result)

		if result.Updated {
			metrics.ContainersUpdated.Inc()
		}
		if result.Error != "" {
			metrics.UpdateErrors.Inc()
		}

		// Execute post-check lifecycle hook
		if u.cfg.LifecycleHooks && ctr.State == "running" {
			u.execLifecycleHook(ctx, ctr, config.LabelPostCheck, 60*time.Second)
		}

		// Store in history (keep last 1000 entries)
		u.mu.Lock()
		u.history = append(u.history, result)
		if len(u.history) > 1000 {
			u.history = u.history[len(u.history)-1000:]
		}
		u.mu.Unlock()
	}

	metrics.ContainersScanned.Set(float64(len(results)))

	log.Infof("Update check cycle complete: %d containers checked", len(results))
	return results, nil
}

// shouldMonitor applies all configured filter layers to determine whether
// a container should be included in the update check.
func (u *Updater) shouldMonitor(ctr docker.ContainerInfo) bool {
	// Filter by container name list (positional args)
	if len(u.cfg.ContainerNames) > 0 {
		found := false
		for _, name := range u.cfg.ContainerNames {
			if ctr.Name == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Filter by disabled container names (--disable-containers)
	for _, name := range u.cfg.DisableContainers {
		if ctr.Name == name {
			return false
		}
	}

	// Check disable label
	if val, ok := ctr.Labels[config.LabelDisable]; ok && isTruthy(val) {
		return false
	}

	// Check if ignored in policy spec
	if u.spec != nil && u.spec.IsIgnored(ctr.Name) {
		return false
	}

	// Filter by scope
	if u.cfg.Scope != "" {
		containerScope := ctr.Labels[config.LabelScope]
		if u.cfg.Scope == "none" {
			// "none" scope matches containers without a scope label or with empty/none scope
			if containerScope != "" && containerScope != "none" {
				return false
			}
		} else if containerScope != u.cfg.Scope {
			return false
		}
	}

	// If monitor_all is true, monitor everything unless explicitly disabled
	if u.cfg.MonitorAll {
		return true
	}

	// Otherwise, only monitor if explicitly enabled via label
	if val, ok := ctr.Labels[config.LabelEnable]; ok && isTruthy(val) {
		return true
	}

	return false
}

// isMonitorOnly checks whether a container should only be monitored (not updated).
// Takes into account both global DryRun setting and per-container label,
// with label precedence if configured.
func (u *Updater) isMonitorOnly(ctr docker.ContainerInfo) bool {
	labelVal, hasLabel := ctr.Labels[config.LabelMonitorOnly]

	if u.cfg.LabelPrecedence && hasLabel {
		return isTruthy(labelVal)
	}

	if u.cfg.DryRun {
		return true
	}

	if hasLabel && isTruthy(labelVal) {
		return true
	}

	return false
}

// isNoPull checks whether image pulling is disabled for a container.
// Takes into account both global NoPull setting and per-container label.
func (u *Updater) isNoPull(ctr docker.ContainerInfo) bool {
	labelVal, hasLabel := ctr.Labels[config.LabelNoPull]

	if u.cfg.LabelPrecedence && hasLabel {
		return isTruthy(labelVal)
	}

	if u.cfg.NoPull {
		return true
	}

	if hasLabel && isTruthy(labelVal) {
		return true
	}

	return false
}

func isTruthy(val string) bool {
	v := strings.ToLower(strings.TrimSpace(val))
	return v == "true" || v == "1" || v == "yes"
}

// checkAndUpdate checks a single container for updates and applies them if needed.
// It handles the full lifecycle: digest check, pull, hooks, recreate, cleanup.
func (u *Updater) checkAndUpdate(ctx context.Context, ctr docker.ContainerInfo) UpdateResult {
	result := UpdateResult{
		ContainerID:   ctr.ID[:12],
		ContainerName: ctr.Name,
		Image:         ctr.Image,
		OldImageID:    ctr.ImageID,
		CheckedAt:     time.Now(),
	}

	// Skip containers using image IDs instead of tags
	if strings.HasPrefix(ctr.Image, "sha256:") {
		result.Skipped = true
		result.Error = "image specified by ID, skipping"
		return result
	}

	noPull := u.isNoPull(ctr)

	// Determine update strategy from policy
	pol := policy.PolicyDef{Strategy: policy.StrategyAll}
	if u.spec != nil {
		pol = u.spec.GetContainerPolicy(ctr.Name)
	}
	strategyStr := string(pol.Strategy)
	if strategyStr == "" {
		strategyStr = "all"
	}

	// newImageRef will hold the image reference to update to.
	// It may be the same tag (digest change) or a different tag (semver upgrade).
	newImageRef := ctr.Image

	if !noPull {
		// Try semver-based tag discovery for patch/minor/major strategies
		if strategyStr == "patch" || strategyStr == "minor" || strategyStr == "major" {
			newTag, found, err := u.findNewerTag(ctx, ctr.Image, strategyStr)
			if err != nil {
				log.Debugf("Semver tag discovery failed for %s, falling back to digest check: %v", ctr.Name, err)
			} else if found {
				newImageRef = newTag
				log.Infof("Semver upgrade found for %s: %s -> %s", ctr.Name, ctr.Image, newTag)
			} else {
				// No newer semver tag found; also check digest for same tag
				localDigest, err := u.docker.GetImageDigest(ctx, ctr.ImageID)
				if err != nil {
					result.Error = fmt.Sprintf("getting local digest: %v", err)
					log.Warnf("Error getting local digest for %s: %v", ctr.Name, err)
					return result
				}
				hasNew, _, err := u.registry.HasNewImage(ctx, ctr.Image, localDigest)
				if err != nil {
					result.Error = fmt.Sprintf("checking remote: %v", err)
					log.Warnf("Error checking remote for %s: %v", ctr.Name, err)
					return result
				}
				if !hasNew {
					log.Debugf("Container %s is up to date", ctr.Name)
					return result
				}
			}
		} else {
			// "all" or "digest" strategy: only check digest change for the same tag
			localDigest, err := u.docker.GetImageDigest(ctx, ctr.ImageID)
			if err != nil {
				result.Error = fmt.Sprintf("getting local digest: %v", err)
				log.Warnf("Error getting local digest for %s: %v", ctr.Name, err)
				return result
			}
			hasNew, _, err := u.registry.HasNewImage(ctx, ctr.Image, localDigest)
			if err != nil {
				result.Error = fmt.Sprintf("checking remote: %v", err)
				log.Warnf("Error checking remote for %s: %v", ctr.Name, err)
				return result
			}
			if !hasNew {
				log.Debugf("Container %s is up to date", ctr.Name)
				return result
			}
		}
	}

	log.Infof("Update available for container %s (%s -> %s)", ctr.Name, ctr.Image, newImageRef)

	// Record in audit log
	if u.audit != nil {
		u.audit.Record(audit.Entry{
			Type:          audit.EventUpdateStarted,
			ContainerName: ctr.Name,
			ContainerID:   ctr.ID[:12],
			Image:         ctr.Image,
			Actor:         "system",
			Message:       "Update available",
		})
	}

	// Check maintenance window (policy-based)
	if u.spec != nil && !u.spec.IsInMaintenanceWindow(ctr.Name) {
		result.Skipped = true
		result.Error = "outside maintenance window"
		log.Infof("[WINDOW] Skipping %s: outside maintenance window", ctr.Name)
		if u.audit != nil {
			u.audit.Record(audit.Entry{
				Type:          audit.EventUpdateSkipped,
				ContainerName: ctr.Name,
				Image:         ctr.Image,
				Actor:         "system",
				Message:       "Outside maintenance window",
			})
		}
		return result
	}

	// Check policy strategy (pin = never auto-update)
	if u.spec != nil {
		pol := u.spec.GetContainerPolicy(ctr.Name)
		if pol.Strategy == policy.StrategyPin {
			result.Skipped = true
			result.Error = "policy: pinned, requires manual approval"
			log.Infof("[POLICY] Skipping %s: pinned strategy", ctr.Name)
			if u.audit != nil {
				u.audit.Record(audit.Entry{
					Type:          audit.EventApprovalPending,
					ContainerName: ctr.Name,
					Image:         ctr.Image,
					Policy:        "pin",
					Actor:         "system",
					Message:       "Update requires manual approval",
				})
			}
			return result
		}

		if pol.Approve == policy.ApproveManual {
			result.Skipped = true
			result.Error = "policy: manual approval required"
			log.Infof("[POLICY] Skipping %s: manual approval required", ctr.Name)
			if u.audit != nil {
				u.audit.Record(audit.Entry{
					Type:          audit.EventApprovalPending,
					ContainerName: ctr.Name,
					Image:         ctr.Image,
					Actor:         "system",
					Message:       "Update requires manual approval",
				})
			}
			return result
		}
	}

	// Check monitor-only mode
	if u.isMonitorOnly(ctr) {
		result.MonitorOnly = true
		result.Error = "monitor-only mode, skipping update"
		log.Infof("[MONITOR ONLY] Would update container %s", ctr.Name)
		if u.notifier != nil {
			u.notifier.NotifyUpdate(result)
		}
		return result
	}

	// Handle stopped containers with revive logic
	if ctr.State != "running" {
		if !u.cfg.ReviveStopped {
			result.Skipped = true
			result.Error = "container is stopped, revive-stopped not enabled"
			return result
		}
	}

	// Pull the new image (unless no-pull mode)
	if !noPull {
		registryAuth := u.registry.GetRegistryAuth(newImageRef)
		newImageID, err := u.docker.PullImage(ctx, newImageRef, registryAuth)
		if err != nil {
			result.Error = fmt.Sprintf("pulling new image: %v", err)
			log.Errorf("Error pulling image for %s: %v", ctr.Name, err)
			return result
		}
		result.NewImageID = newImageID
	}

	// No-restart mode: pull but don't recreate
	if u.cfg.NoRestart {
		result.Updated = true
		result.Error = "no-restart mode, image pulled but container not restarted"
		log.Infof("[NO RESTART] Image pulled for %s but container not restarted", ctr.Name)
		return result
	}

	// Execute pre-update lifecycle hook
	if u.cfg.LifecycleHooks && ctr.State == "running" {
		timeout := u.getHookTimeout(ctr, config.LabelPreUpdateTimeout, 60*time.Second)
		u.execLifecycleHook(ctx, ctr, config.LabelPreUpdate, timeout)
	}

	// Get custom stop signal from label
	customSignal := ctr.Labels[config.LabelStopSignal]

	// Recreate the container with the new image reference (may be a different tag)
	newContainerID, err := u.docker.RecreateContainer(
		ctx, ctr.ID, newImageRef, u.cfg.StopTimeout, customSignal, u.cfg.RemoveVolumes,
	)
	if err != nil {
		result.Error = fmt.Sprintf("recreating container: %v", err)
		log.Errorf("Error recreating container %s: %v", ctr.Name, err)
		return result
	}

	result.Updated = true
	log.Infof("Successfully updated container %s -> %s (%s)", ctr.Name, newContainerID[:12], newImageRef)

	// Execute post-update lifecycle hook
	if u.cfg.LifecycleHooks {
		timeout := u.getHookTimeout(ctr, config.LabelPostUpdateTimeout, 60*time.Second)
		u.execLifecycleHookByID(ctx, newContainerID, ctr, config.LabelPostUpdate, timeout)
	}

	// Clean up old image if configured
	if u.cfg.CleanupImages {
		if err := u.docker.RemoveImage(ctx, ctr.ImageID); err != nil {
			log.Warnf("Failed to remove old image %s: %v", ctr.ImageID, err)
		}
	}

	// GitOps: commit image change to Git
	if u.spec != nil && u.spec.GitOps.Enabled && newImageRef != ctr.Image {
		gc := gitops.NewClient(u.spec.GitOps)
		if err := gc.PushChange(gitops.Change{
			Image:  strings.Split(ctr.Image, ":")[0],
			OldTag: strings.SplitN(ctr.Image, ":", 2)[1],
			NewTag: strings.SplitN(newImageRef, ":", 2)[1],
			OldRef: ctr.Image,
			NewRef: newImageRef,
		}); err != nil {
			log.Warnf("GitOps push failed for %s: %v", ctr.Name, err)
		}
	}

	// Send update notification
	if u.notifier != nil {
		u.notifier.NotifyUpdate(result)
	}

	return result
}

// findNewerTag queries the registry for all tags and finds the best candidate
// based on semver rules and the configured strategy.
//
// Returns the full new image reference (e.g. "mysql:8.0.46"), whether a newer
// tag was found, and any error.
func (u *Updater) findNewerTag(ctx context.Context, imageRef string, strategy string) (string, bool, error) {
	// Parse current tag
	parts := strings.SplitN(imageRef, ":", 2)
	imageName := parts[0]
	currentTag := "latest"
	if len(parts) == 2 {
		currentTag = parts[1]
	}

	currentVer, err := semver.Parse(currentTag)
	if err != nil {
		return "", false, fmt.Errorf("current tag %q is not semver: %w", currentTag, err)
	}

	// List all tags from registry
	tags, err := u.registry.ListTags(ctx, imageRef)
	if err != nil {
		return "", false, fmt.Errorf("listing tags: %w", err)
	}

	// Parse all tags as semver, filter invalid ones
	var candidates []semver.Version
	for _, tag := range tags {
		v, err := semver.Parse(tag)
		if err != nil {
			continue // skip non-semver tags
		}
		candidates = append(candidates, v)
	}

	// Find best candidate by strategy
	best := semver.FilterByStrategy(currentVer, candidates, strategy)
	if best == nil {
		return "", false, nil // no newer version found
	}

	newRef := imageName + ":" + best.Original
	return newRef, true, nil
}

// execLifecycleHook runs a lifecycle hook command inside a container.
// The command is read from the specified label. Errors are logged but do not
// stop the update process (matching Watchtower behavior).
func (u *Updater) execLifecycleHook(ctx context.Context, ctr docker.ContainerInfo, label string, timeout time.Duration) {
	cmd, ok := ctr.Labels[label]
	if !ok || cmd == "" {
		return
	}

	log.Infof("Executing %s hook for container %s: %s", label, ctr.Name, cmd)
	output, err := u.docker.ExecCommand(ctx, ctr.ID, cmd, timeout)
	if err != nil {
		log.Errorf("Lifecycle hook %s failed for %s: %v (output: %s)", label, ctr.Name, err, output)
	} else {
		log.Debugf("Lifecycle hook %s output for %s: %s", label, ctr.Name, output)
	}
}

// execLifecycleHookByID is like execLifecycleHook but takes a container ID directly.
// Used for post-update hooks which run in the new container.
func (u *Updater) execLifecycleHookByID(ctx context.Context, containerID string, ctr docker.ContainerInfo, label string, timeout time.Duration) {
	cmd, ok := ctr.Labels[label]
	if !ok || cmd == "" {
		return
	}

	log.Infof("Executing %s hook for container %s: %s", label, ctr.Name, cmd)
	output, err := u.docker.ExecCommand(ctx, containerID, cmd, timeout)
	if err != nil {
		log.Errorf("Lifecycle hook %s failed for %s: %v (output: %s)", label, ctr.Name, err, output)
	} else {
		log.Debugf("Lifecycle hook %s output for %s: %s", label, ctr.Name, output)
	}
}

// getHookTimeout reads a timeout override from a container label.
// The label value is in minutes. Returns the default if not set.
func (u *Updater) getHookTimeout(ctr docker.ContainerInfo, label string, defaultTimeout time.Duration) time.Duration {
	val, ok := ctr.Labels[label]
	if !ok || val == "" {
		return defaultTimeout
	}

	minutes, err := strconv.Atoi(val)
	if err != nil {
		return defaultTimeout
	}

	if minutes == 0 {
		return 0 // disable timeout
	}

	return time.Duration(minutes) * time.Minute
}

// orderByDependencies sorts containers so that dependencies are updated first.
// Uses the com.updock.depends-on label to build a dependency graph.
func (u *Updater) orderByDependencies(ctx context.Context, containers []docker.ContainerInfo) []docker.ContainerInfo {
	nameMap := make(map[string]int) // name -> index in containers
	for i, c := range containers {
		nameMap[c.Name] = i
	}

	// Build adjacency list
	deps := make(map[string][]string)
	for _, c := range containers {
		d, err := u.docker.GetDependencies(ctx, c.ID)
		if err == nil && len(d) > 0 {
			deps[c.Name] = d
		}
	}

	// Topological sort (Kahn's algorithm)
	inDegree := make(map[string]int)
	for _, c := range containers {
		inDegree[c.Name] = 0
	}
	for _, depList := range deps {
		for _, dep := range depList {
			if _, ok := nameMap[dep]; ok {
				inDegree[dep]++
			}
		}
	}

	var queue []string
	for name, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, name)
		}
	}

	var ordered []docker.ContainerInfo
	visited := make(map[string]bool)

	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]

		if visited[name] {
			continue
		}
		visited[name] = true

		if idx, ok := nameMap[name]; ok {
			ordered = append(ordered, containers[idx])
		}

		for dependent, depList := range deps {
			for _, dep := range depList {
				if dep == name {
					inDegree[dependent]--
					if inDegree[dependent] == 0 {
						queue = append(queue, dependent)
					}
				}
			}
		}
	}

	// Add any containers not in the dependency graph
	for _, c := range containers {
		if !visited[c.Name] {
			ordered = append(ordered, c)
		}
	}

	return ordered
}

// History returns a copy of the update history.
// The history is capped at 1000 entries.
func (u *Updater) History() []UpdateResult {
	u.mu.RLock()
	defer u.mu.RUnlock()

	h := make([]UpdateResult, len(u.history))
	copy(h, u.history)
	return h
}
