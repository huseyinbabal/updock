// Package config provides centralized configuration management for Updock.
//
// Configuration values can be set via CLI flags, environment variables (prefixed
// with UPDOCK_), or a configuration file. Environment variables take the form
// UPDOCK_<UPPER_SNAKE_CASE>, for example UPDOCK_INTERVAL=10m.
//
// Sensitive values such as API tokens and webhook URLs support file references:
// if the value starts with "/" and points to an existing file, its contents are
// read and used as the actual value. This enables Docker secrets integration.
//
// # Example
//
//	UPDOCK_HTTP_API_TOKEN=/run/secrets/api_token
//	UPDOCK_WEBHOOK_URL=/run/secrets/webhook_url
package config

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Version is the current Updock version. It is set at build time via ldflags:
//
//	go build -ldflags "-X github.com/huseyinbabal/updock/internal/config.Version=1.0.0"
var Version = "dev"

// Config holds all application configuration for a running Updock instance.
// Each field is documented with its CLI flag, environment variable, and default value.
type Config struct {
	// DockerHost is the Docker daemon socket to connect to.
	// Supports both Unix sockets and TCP endpoints for remote hosts.
	//
	//   Flag: --docker-host
	//   Env:  UPDOCK_DOCKER_HOST
	//   Default: unix:///var/run/docker.sock
	DockerHost string `mapstructure:"docker_host"`

	// Interval is the polling interval between update checks.
	// Accepts Go duration strings such as "5m", "1h", "30s".
	// Mutually exclusive with Schedule.
	//
	//   Flag: --interval
	//   Env:  UPDOCK_INTERVAL
	//   Default: 5m
	Interval time.Duration `mapstructure:"interval"`

	// Schedule is a 6-field cron expression that defines when to check for updates.
	// When set, it overrides Interval. Uses second-level precision.
	//
	//   Flag: --schedule
	//   Env:  UPDOCK_SCHEDULE
	//   Example: "0 0 4 * * *" (every day at 04:00)
	Schedule string `mapstructure:"schedule"`

	// EnableLabel is the container label key used for opt-in monitoring.
	// Only relevant when MonitorAll is false.
	//
	//   Flag: --enable-label
	//   Env:  UPDOCK_ENABLE_LABEL
	//   Default: com.updock.enable
	EnableLabel string `mapstructure:"enable_label"`

	// MonitorAll controls whether all containers are monitored by default.
	// When true, all containers are monitored unless explicitly disabled via label.
	// When false, only containers with the enable label set to "true" are monitored.
	//
	//   Flag: --monitor-all
	//   Env:  UPDOCK_MONITOR_ALL
	//   Default: true
	MonitorAll bool `mapstructure:"monitor_all"`

	// CleanupImages removes old images after a container has been updated.
	// Prevents accumulation of orphaned images on the host.
	//
	//   Flag: --cleanup
	//   Env:  UPDOCK_CLEANUP_IMAGES
	//   Default: false
	CleanupImages bool `mapstructure:"cleanup_images"`

	// RemoveVolumes removes anonymous volumes when recreating a container.
	// Named volumes are never removed regardless of this setting.
	//
	//   Flag: --remove-volumes
	//   Env:  UPDOCK_REMOVE_VOLUMES
	//   Default: false
	RemoveVolumes bool `mapstructure:"remove_volumes"`

	// StopTimeout is the maximum time to wait for a container to stop gracefully
	// before it is forcefully killed.
	//
	//   Flag: --stop-timeout
	//   Env:  UPDOCK_STOP_TIMEOUT
	//   Default: 30s
	StopTimeout time.Duration `mapstructure:"stop_timeout"`

	// HTTPAddr is the address the built-in HTTP server listens on.
	// This serves both the Web UI dashboard and the REST API.
	//
	//   Flag: --http-addr
	//   Env:  UPDOCK_HTTP_ADDR
	//   Default: :8080
	HTTPAddr string `mapstructure:"http_addr"`

	// HTTPEnabled controls whether the HTTP server (Web UI + API) is started.
	//
	//   Flag: --http-enabled
	//   Env:  UPDOCK_HTTP_ENABLED
	//   Default: true
	HTTPEnabled bool `mapstructure:"http_enabled"`

	// HTTPAPIToken is a Bearer token required for all API requests.
	// When set, every request must include an "Authorization: Bearer <token>" header.
	// Supports file references for Docker secrets integration.
	//
	//   Flag: --http-api-token
	//   Env:  UPDOCK_HTTP_API_TOKEN
	//   Default: "" (no authentication)
	HTTPAPIToken string `mapstructure:"http_api_token"`

	// MetricsEnabled exposes a Prometheus-compatible /metrics endpoint.
	//
	//   Flag: --metrics
	//   Env:  UPDOCK_METRICS_ENABLED
	//   Default: true
	MetricsEnabled bool `mapstructure:"metrics_enabled"`

	// WebhookURL is a single webhook endpoint for update notifications.
	// For multiple endpoints, use NotificationURLs. Supports file references.
	//
	//   Flag: --webhook-url
	//   Env:  UPDOCK_WEBHOOK_URL
	//   Default: ""
	WebhookURL string `mapstructure:"webhook_url"`

	// NotificationURLs is a list of notification endpoints (space-separated in env).
	// Each URL receives a JSON POST with update details.
	//
	//   Env: UPDOCK_NOTIFICATION_URL
	//   Example: "https://hooks.slack.com/xxx https://discord.com/api/webhooks/yyy"
	NotificationURLs []string

	// NotificationTemplate is a Go text/template string for formatting notification
	// messages. Available fields: .ContainerName, .Image, .OldImageID, .NewImageID.
	//
	//   Flag: --notification-template
	//   Env:  UPDOCK_NOTIFICATION_TEMPLATE
	//   Default: built-in template
	NotificationTemplate string `mapstructure:"notification_template"`

	// DryRun enables monitor-only mode globally. Updock will check for updates
	// and send notifications, but will not pull images or restart containers.
	//
	//   Flag: --dry-run
	//   Env:  UPDOCK_DRY_RUN
	//   Default: false
	DryRun bool `mapstructure:"dry_run"`

	// RunOnce performs a single update check and exits immediately.
	// Useful for CI/CD pipelines or one-off checks.
	//
	//   Flag: --run-once
	//   Env:  UPDOCK_RUN_ONCE
	//   Default: false
	RunOnce bool `mapstructure:"run_once"`

	// LogLevel controls the verbosity of log output.
	// Valid values: panic, fatal, error, warn, info, debug, trace.
	//
	//   Flag: --log-level
	//   Env:  UPDOCK_LOG_LEVEL
	//   Default: info
	LogLevel string `mapstructure:"log_level"`

	// ContainerNames is a list of container names to monitor exclusively.
	// When non-empty, only these containers are checked for updates.
	// Set via positional arguments: updock nginx redis
	ContainerNames []string

	// DisableContainers is a comma-separated list of container names to exclude.
	// These containers are skipped even if they have the enable label.
	//
	//   Flag: --disable-containers
	//   Env:  UPDOCK_DISABLE_CONTAINERS
	//   Example: "watchtower,updock"
	DisableContainers []string `mapstructure:"disable_containers"`

	// IncludeStopped includes created and exited containers in the update check.
	//
	//   Flag: --include-stopped
	//   Env:  UPDOCK_INCLUDE_STOPPED
	//   Default: false
	IncludeStopped bool `mapstructure:"include_stopped"`

	// IncludeRestarting includes containers in "restarting" state.
	//
	//   Flag: --include-restarting
	//   Env:  UPDOCK_INCLUDE_RESTARTING
	//   Default: false
	IncludeRestarting bool `mapstructure:"include_restarting"`

	// ReviveStopped starts stopped containers if their image has been updated.
	// Only effective when IncludeStopped is true.
	//
	//   Flag: --revive-stopped
	//   Env:  UPDOCK_REVIVE_STOPPED
	//   Default: false
	ReviveStopped bool `mapstructure:"revive_stopped"`

	// NoPull disables pulling new images from the registry.
	// Updock will only check the local image cache for changes.
	// Useful when images are built directly on the Docker host.
	//
	//   Flag: --no-pull
	//   Env:  UPDOCK_NO_PULL
	//   Default: false
	NoPull bool `mapstructure:"no_pull"`

	// NoRestart prevents containers from being restarted after an update.
	// Useful when container lifecycle is managed by an external system (e.g. systemd).
	//
	//   Flag: --no-restart
	//   Env:  UPDOCK_NO_RESTART
	//   Default: false
	NoRestart bool `mapstructure:"no_restart"`

	// RollingRestart restarts containers one at a time instead of all at once.
	// Useful with lifecycle hooks to achieve zero-downtime deployments.
	//
	//   Flag: --rolling-restart
	//   Env:  UPDOCK_ROLLING_RESTART
	//   Default: false
	RollingRestart bool `mapstructure:"rolling_restart"`

	// LabelPrecedence makes container labels take precedence over CLI arguments.
	// When true, a container with com.updock.monitor-only=false will be updated
	// even if --dry-run is set globally.
	//
	//   Flag: --label-precedence
	//   Env:  UPDOCK_LABEL_PRECEDENCE
	//   Default: false
	LabelPrecedence bool `mapstructure:"label_precedence"`

	// LifecycleHooks enables execution of pre/post update commands inside containers.
	// Commands are specified via container labels (see Label constants).
	//
	//   Flag: --lifecycle-hooks
	//   Env:  UPDOCK_LIFECYCLE_HOOKS
	//   Default: false
	LifecycleHooks bool `mapstructure:"lifecycle_hooks"`

	// Scope limits this Updock instance to containers with a matching scope label.
	// Enables running multiple Updock instances on the same Docker host.
	// Use "none" to only match containers without a scope label.
	//
	//   Flag: --scope
	//   Env:  UPDOCK_SCOPE
	//   Default: "" (all containers regardless of scope)
	Scope string `mapstructure:"scope"`

	// TLSVerify enables TLS verification when connecting to the Docker daemon.
	//
	//   Flag: --tls-verify
	//   Env:  DOCKER_TLS_VERIFY
	//   Default: false
	TLSVerify bool `mapstructure:"tls_verify"`

	// DockerConfigPath is the path to a Docker config.json file for private
	// registry authentication. This file contains base64-encoded credentials.
	//
	//   Flag: --docker-config
	//   Env:  DOCKER_CONFIG
	//   Default: /config.json
	DockerConfigPath string `mapstructure:"docker_config"`

	// WarnOnHeadFailure controls when to warn about failed HEAD requests to registries.
	// "auto" warns only for known registries (e.g. Docker Hub) that may rate-limit.
	// "always" warns on every failure. "never" suppresses warnings.
	//
	//   Flag: --warn-on-head-failure
	//   Env:  UPDOCK_WARN_ON_HEAD_FAILURE
	//   Default: auto
	WarnOnHeadFailure string `mapstructure:"warn_on_head_failure"`

	// NoStartupMessage suppresses the startup notification message.
	//
	//   Flag: --no-startup-message
	//   Env:  UPDOCK_NO_STARTUP_MESSAGE
	//   Default: false
	NoStartupMessage bool `mapstructure:"no_startup_message"`

	// PolicyFile is the path to the updock.yml declarative policy file.
	// This file defines update strategies, maintenance windows, container
	// groups, and approval modes — the core differentiator from label-based tools.
	//
	//   Flag: --policy-file
	//   Env:  UPDOCK_POLICY_FILE
	//   Default: updock.yml
	PolicyFile string `mapstructure:"policy_file"`

	// AuditLogPath is the path where the audit log file is stored.
	// Every update attempt, rollback, and approval decision is recorded here.
	//
	//   Flag: --audit-log
	//   Env:  UPDOCK_AUDIT_LOG
	//   Default: /var/lib/updock/audit.json
	AuditLogPath string `mapstructure:"audit_log"`
}

// Container label constants define the label keys that Updock reads from
// Docker containers to control per-container behavior.
//
// All labels use the "com.updock." prefix to avoid conflicts with other tools.
const (
	// LabelEnable opts a container in to monitoring when MonitorAll is false.
	//   Example: docker run --label com.updock.enable=true myimage
	LabelEnable = "com.updock.enable"

	// LabelDisable opts a container out of monitoring.
	//   Example: docker run --label com.updock.disable=true myimage
	LabelDisable = "com.updock.disable"

	// LabelScope assigns a container to a specific Updock instance scope.
	// Only the Updock instance with a matching --scope flag will monitor this container.
	//   Example: docker run --label com.updock.scope=production myimage
	LabelScope = "com.updock.scope"

	// LabelMonitorOnly prevents a container from being updated.
	// Updock will still check for updates and send notifications.
	//   Example: docker run --label com.updock.monitor-only=true myimage
	LabelMonitorOnly = "com.updock.monitor-only"

	// LabelNoPull prevents pulling a new image for this container.
	// Updates are only applied if the image changes in the local cache.
	//   Example: docker run --label com.updock.no-pull=true myimage
	LabelNoPull = "com.updock.no-pull"

	// LabelStopSignal overrides the signal sent to stop this container before an update.
	// Default is SIGTERM. Common alternatives: SIGHUP, SIGQUIT, SIGINT.
	//   Example: docker run --label com.updock.stop-signal=SIGHUP myimage
	LabelStopSignal = "com.updock.stop-signal"

	// LabelDependsOn declares dependencies between containers.
	// Dependent containers are stopped/started in the correct order during updates.
	// Multiple dependencies are comma-separated.
	//   Example: docker run --label com.updock.depends-on=mysql,redis myimage
	LabelDependsOn = "com.updock.depends-on"

	// LabelPreCheck is a shell command executed inside the container before each
	// update cycle. Requires --lifecycle-hooks to be enabled.
	//   Example: docker run --label com.updock.lifecycle.pre-check="/healthcheck.sh" myimage
	LabelPreCheck = "com.updock.lifecycle.pre-check"

	// LabelPreUpdate is a shell command executed inside the container before stopping
	// it for an update. Use this to gracefully drain connections or dump data.
	//   Example: docker run --label com.updock.lifecycle.pre-update="/dump-data.sh" myimage
	LabelPreUpdate = "com.updock.lifecycle.pre-update"

	// LabelPostUpdate is a shell command executed inside the new container after an
	// update. Use this to restore state or verify the new version.
	//   Example: docker run --label com.updock.lifecycle.post-update="/restore-data.sh" myimage
	LabelPostUpdate = "com.updock.lifecycle.post-update"

	// LabelPostCheck is a shell command executed inside the container after each
	// update cycle completes.
	//   Example: docker run --label com.updock.lifecycle.post-check="/send-heartbeat.sh" myimage
	LabelPostCheck = "com.updock.lifecycle.post-check"

	// LabelPreUpdateTimeout overrides the default 60-second timeout for the pre-update
	// lifecycle command. Value is in minutes. Set to "0" to disable the timeout.
	//   Example: docker run --label com.updock.lifecycle.pre-update-timeout=5 myimage
	LabelPreUpdateTimeout = "com.updock.lifecycle.pre-update-timeout"

	// LabelPostUpdateTimeout overrides the default 60-second timeout for the post-update
	// lifecycle command. Value is in minutes. Set to "0" to disable the timeout.
	//   Example: docker run --label com.updock.lifecycle.post-update-timeout=5 myimage
	LabelPostUpdateTimeout = "com.updock.lifecycle.post-update-timeout"
)

// ReadSecretFile reads a configuration value that may reference a file.
// If the value starts with "/" and the file exists, the file contents are
// returned (trimmed of whitespace). Otherwise the original value is returned.
//
// This enables Docker secrets integration:
//
//	UPDOCK_HTTP_API_TOKEN=/run/secrets/api_token
func ReadSecretFile(val string) string {
	if val == "" {
		return val
	}
	if strings.HasPrefix(val, "/") {
		data, err := os.ReadFile(val)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	return val
}

// Load reads configuration from environment variables, CLI flags (via viper bindings),
// and applies defaults. It returns a fully populated Config struct ready for use.
func Load() *Config {
	viper.SetDefault("docker_host", "unix:///var/run/docker.sock")
	viper.SetDefault("interval", 5*time.Minute)
	viper.SetDefault("schedule", "")
	viper.SetDefault("enable_label", LabelEnable)
	viper.SetDefault("monitor_all", true)
	viper.SetDefault("cleanup_images", false)
	viper.SetDefault("remove_volumes", false)
	viper.SetDefault("stop_timeout", 30*time.Second)
	viper.SetDefault("http_addr", ":8080")
	viper.SetDefault("http_enabled", true)
	viper.SetDefault("http_api_token", "")
	viper.SetDefault("metrics_enabled", true)
	viper.SetDefault("webhook_url", "")
	viper.SetDefault("notification_template", "")
	viper.SetDefault("dry_run", false)
	viper.SetDefault("run_once", false)
	viper.SetDefault("log_level", "info")
	viper.SetDefault("disable_containers", "")
	viper.SetDefault("include_stopped", false)
	viper.SetDefault("include_restarting", false)
	viper.SetDefault("revive_stopped", false)
	viper.SetDefault("no_pull", false)
	viper.SetDefault("no_restart", false)
	viper.SetDefault("rolling_restart", false)
	viper.SetDefault("label_precedence", false)
	viper.SetDefault("lifecycle_hooks", false)
	viper.SetDefault("scope", "")
	viper.SetDefault("tls_verify", false)
	viper.SetDefault("docker_config", "/config.json")
	viper.SetDefault("warn_on_head_failure", "auto")
	viper.SetDefault("no_startup_message", false)
	viper.SetDefault("policy_file", "updock.yml")
	viper.SetDefault("audit_log", "/var/lib/updock/audit.json")

	viper.SetEnvPrefix("UPDOCK")
	viper.AutomaticEnv()

	cfg := &Config{}
	cfg.DockerHost = viper.GetString("docker_host")
	cfg.Interval = viper.GetDuration("interval")
	cfg.Schedule = viper.GetString("schedule")
	cfg.EnableLabel = viper.GetString("enable_label")
	cfg.MonitorAll = viper.GetBool("monitor_all")
	cfg.CleanupImages = viper.GetBool("cleanup_images")
	cfg.RemoveVolumes = viper.GetBool("remove_volumes")
	cfg.StopTimeout = viper.GetDuration("stop_timeout")
	cfg.HTTPAddr = viper.GetString("http_addr")
	cfg.HTTPEnabled = viper.GetBool("http_enabled")
	cfg.HTTPAPIToken = ReadSecretFile(viper.GetString("http_api_token"))
	cfg.MetricsEnabled = viper.GetBool("metrics_enabled")
	cfg.WebhookURL = ReadSecretFile(viper.GetString("webhook_url"))
	cfg.NotificationTemplate = viper.GetString("notification_template")
	cfg.DryRun = viper.GetBool("dry_run")
	cfg.RunOnce = viper.GetBool("run_once")
	cfg.LogLevel = viper.GetString("log_level")
	cfg.IncludeStopped = viper.GetBool("include_stopped")
	cfg.IncludeRestarting = viper.GetBool("include_restarting")
	cfg.ReviveStopped = viper.GetBool("revive_stopped")
	cfg.NoPull = viper.GetBool("no_pull")
	cfg.NoRestart = viper.GetBool("no_restart")
	cfg.RollingRestart = viper.GetBool("rolling_restart")
	cfg.LabelPrecedence = viper.GetBool("label_precedence")
	cfg.LifecycleHooks = viper.GetBool("lifecycle_hooks")
	cfg.Scope = viper.GetString("scope")
	cfg.TLSVerify = viper.GetBool("tls_verify")
	cfg.DockerConfigPath = viper.GetString("docker_config")
	cfg.WarnOnHeadFailure = viper.GetString("warn_on_head_failure")
	cfg.NoStartupMessage = viper.GetBool("no_startup_message")
	cfg.PolicyFile = viper.GetString("policy_file")
	cfg.AuditLogPath = viper.GetString("audit_log")

	// Parse disable-containers comma-separated list
	dc := viper.GetString("disable_containers")
	if dc != "" {
		for _, name := range strings.Split(dc, ",") {
			name = strings.TrimSpace(name)
			if name != "" {
				cfg.DisableContainers = append(cfg.DisableContainers, name)
			}
		}
	}

	// Parse notification URLs (space-separated, mimics Watchtower's multi-URL support)
	notifURLs := viper.GetString("notification_url")
	if notifURLs != "" {
		for _, u := range strings.Fields(notifURLs) {
			u = strings.TrimSpace(u)
			if u != "" {
				cfg.NotificationURLs = append(cfg.NotificationURLs, u)
			}
		}
	}

	return cfg
}
