// Package main is the entry point for the Updock CLI.
//
// Updock is an automatic Docker container updater with a built-in Web UI.
// It monitors running containers for image updates, pulls new images,
// and recreates containers with the updated images while preserving
// all configuration.
//
// # Usage
//
//	updock [flags] [container names...]
//
// When container names are provided as positional arguments, only those
// containers are monitored. Otherwise all containers are monitored
// (subject to label filters).
//
// # Quick Start
//
//	docker run -d --name updock \
//	  -v /var/run/docker.sock:/var/run/docker.sock \
//	  -p 8080:8080 \
//	  updock
//
// Then open http://localhost:8080 for the dashboard.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/huseyinbabal/updock/internal/api"
	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/logger"
	"github.com/huseyinbabal/updock/internal/notification"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/registry"
	"github.com/huseyinbabal/updock/internal/scheduler"
	"github.com/huseyinbabal/updock/internal/updater"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "updock [flags] [container names...]",
		Short: "Updock - Automatic Docker container updater with Web UI",
		Long: `Updock automatically updates Docker containers when new images are available.
It monitors running containers, checks for image updates, and recreates
containers with the new image while preserving all configuration.

Features:
  - Automatic container updates with rollback on failure
  - Built-in Web UI dashboard at http://localhost:8080
  - REST API for integration and manual triggers
  - Prometheus metrics endpoint
  - Lifecycle hooks (pre/post update commands)
  - Container dependency ordering
  - Private registry authentication
  - Webhook notifications
  - Cron and interval scheduling

Container names can be passed as positional arguments to monitor only
specific containers. When no names are given, all containers are monitored.`,
		Version: config.Version,
		Args:    cobra.ArbitraryArgs,
		RunE:    run,
	}

	flags := rootCmd.Flags()

	// Docker connection
	flags.String("docker-host", "", "Docker daemon socket to connect to (default: unix:///var/run/docker.sock)")
	flags.Bool("tls-verify", false, "Use TLS and verify the Docker daemon certificate")
	flags.String("docker-config", "", "Path to Docker config.json for private registry auth (default: /config.json)")

	// Scheduling
	flags.Duration("interval", 0, "Polling interval between update checks (e.g. 5m, 1h)")
	flags.String("schedule", "", "Cron expression for update checks (6-field with seconds, overrides --interval)")

	// Container selection
	flags.Bool("monitor-all", true, "Monitor all containers by default")
	flags.Bool("label-enable", false, "Only monitor containers with com.updock.enable=true label")
	flags.StringSlice("disable-containers", nil, "Container names to exclude from monitoring (comma-separated)")
	flags.Bool("include-stopped", false, "Include stopped containers in update checks")
	flags.Bool("include-restarting", false, "Include restarting containers in update checks")
	flags.Bool("revive-stopped", false, "Start stopped containers if their image has been updated")
	flags.String("scope", "", "Limit monitoring to containers with matching com.updock.scope label")

	// Update behavior
	flags.Bool("cleanup", false, "Remove old images after updating containers")
	flags.Bool("remove-volumes", false, "Remove anonymous volumes when recreating containers")
	flags.Duration("stop-timeout", 0, "Timeout for stopping containers before forceful kill (default: 30s)")
	flags.Bool("dry-run", false, "Check for updates without applying them (monitor-only)")
	flags.Bool("run-once", false, "Run a single update check and exit")
	flags.Bool("no-pull", false, "Don't pull new images, only check local image cache")
	flags.Bool("no-restart", false, "Pull images but don't restart containers")
	flags.Bool("rolling-restart", false, "Restart containers one at a time (respects dependency order)")
	flags.Bool("label-precedence", false, "Container labels take precedence over CLI arguments")
	flags.Bool("lifecycle-hooks", false, "Enable pre/post update lifecycle hook commands")

	// HTTP server
	flags.String("http-addr", ":8080", "Address for the Web UI and API server")
	flags.Bool("http-enabled", true, "Enable the Web UI and REST API")
	flags.String("http-api-token", "", "Bearer token for API authentication (supports file references)")
	flags.Bool("metrics", true, "Enable Prometheus /metrics endpoint")

	// Notifications
	flags.String("webhook-url", "", "Webhook URL for update notifications (supports file references)")
	flags.String("notification-template", "", "Go template string for notification messages")
	flags.Bool("no-startup-message", false, "Don't send a notification when Updock starts")

	// Logging
	flags.String("log-level", "info", "Log level: panic, fatal, error, warn, info, debug, trace")
	flags.String("warn-on-head-failure", "auto", "When to warn on HEAD request failures: always, auto, never")

	// Updock-specific
	flags.String("policy-file", "updock.yml", "Path to updock.yml declarative policy file")
	flags.String("audit-log", "/var/lib/updock/audit.json", "Path to audit log file")

	// Bind all flags to viper for unified configuration
	binds := map[string]string{
		"docker_host":           "docker-host",
		"tls_verify":            "tls-verify",
		"docker_config":         "docker-config",
		"interval":              "interval",
		"schedule":              "schedule",
		"monitor_all":           "monitor-all",
		"disable_containers":    "disable-containers",
		"include_stopped":       "include-stopped",
		"include_restarting":    "include-restarting",
		"revive_stopped":        "revive-stopped",
		"scope":                 "scope",
		"cleanup_images":        "cleanup",
		"remove_volumes":        "remove-volumes",
		"stop_timeout":          "stop-timeout",
		"dry_run":               "dry-run",
		"run_once":              "run-once",
		"no_pull":               "no-pull",
		"no_restart":            "no-restart",
		"rolling_restart":       "rolling-restart",
		"label_precedence":      "label-precedence",
		"lifecycle_hooks":       "lifecycle-hooks",
		"http_addr":             "http-addr",
		"http_enabled":          "http-enabled",
		"http_api_token":        "http-api-token",
		"metrics_enabled":       "metrics",
		"webhook_url":           "webhook-url",
		"notification_template": "notification-template",
		"no_startup_message":    "no-startup-message",
		"log_level":             "log-level",
		"warn_on_head_failure":  "warn-on-head-failure",
		"policy_file":           "policy-file",
		"audit_log":             "audit-log",
	}
	for viperKey, flagName := range binds {
		_ = viper.BindPFlag(viperKey, flags.Lookup(flagName))
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	cfg := config.Load()

	// Set container names from positional arguments
	cfg.ContainerNames = args

	// --label-enable inverts the default: only monitor labeled containers
	labelEnable, _ := cmd.Flags().GetBool("label-enable")
	if labelEnable {
		cfg.MonitorAll = false
	}

	// Configure logging
	logger.Setup(cfg.LogLevel, false)

	logger.Info().Msgf("Starting Updock %s", config.Version)

	if cfg.DryRun {
		logger.Warn().Msg("Running in dry-run mode - no updates will be applied")
	}
	if cfg.RunOnce {
		logger.Info().Msg("Running in run-once mode - will exit after single check")
	}

	// Create context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Initialize Docker client
	dockerClient, err := docker.NewClient(cfg.DockerHost, cfg.TLSVerify)
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer func() { _ = dockerClient.Close() }()

	// Verify Docker connection
	if err := dockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}
	logger.Info().Msg("Connected to Docker daemon")

	// Load declarative policy file (updock.yml)
	spec, err := policy.LoadSpec(cfg.PolicyFile)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}
	logger.Info().Msgf("Loaded policy file: %s (%d policies, %d container overrides, %d groups)",
		cfg.PolicyFile, len(spec.Policies), len(spec.Containers), len(spec.Groups))

	// Initialize audit log
	auditLog := audit.NewLog(cfg.AuditLogPath)

	// Initialize components
	registryClient := registry.NewClient(cfg.DockerConfigPath)
	notifier := notification.NewNotifier(cfg.WebhookURL, cfg.NotificationURLs, cfg.NotificationTemplate)
	upd := updater.New(dockerClient, registryClient, notifier, cfg, spec, auditLog)

	// Send startup notification
	if notifier != nil && !cfg.NoStartupMessage {
		notifier.NotifyStartup(config.Version)
	}

	// Run-once mode: execute a single check and exit
	if cfg.RunOnce {
		results, err := upd.Run(ctx)
		if err != nil {
			return fmt.Errorf("update check failed: %w", err)
		}
		updated := 0
		for _, r := range results {
			if r.Updated {
				updated++
			}
		}
		logger.Info().Msgf("Run-once complete: %d checked, %d updated", len(results), updated)
		return nil
	}

	// Start scheduler for periodic checks
	sched := scheduler.New(upd, cfg.Interval, cfg.Schedule)
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}
	defer sched.Stop()

	// Start HTTP server (Web UI + API)
	if cfg.HTTPEnabled {
		server := api.NewServer(dockerClient, upd, cfg, spec)
		go func() {
			if err := server.Start(); err != nil && err != http.ErrServerClosed {
				logger.Error().Msgf("HTTP server error: %v", err)
			}
		}()
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.StopTimeout)
			defer shutdownCancel()
			_ = server.Stop(shutdownCtx)
		}()
	}

	// Wait for shutdown signal
	sig := <-sigCh
	logger.Info().Msgf("Received signal %s, shutting down...", sig)
	cancel()

	return nil
}
