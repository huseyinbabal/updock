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
	"github.com/huseyinbabal/updock/internal/notification"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/registry"
	"github.com/huseyinbabal/updock/internal/scheduler"
	"github.com/huseyinbabal/updock/internal/updater"
	log "github.com/sirupsen/logrus"
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
	viper.BindPFlag("docker_host", flags.Lookup("docker-host"))
	viper.BindPFlag("tls_verify", flags.Lookup("tls-verify"))
	viper.BindPFlag("docker_config", flags.Lookup("docker-config"))
	viper.BindPFlag("interval", flags.Lookup("interval"))
	viper.BindPFlag("schedule", flags.Lookup("schedule"))
	viper.BindPFlag("monitor_all", flags.Lookup("monitor-all"))
	viper.BindPFlag("disable_containers", flags.Lookup("disable-containers"))
	viper.BindPFlag("include_stopped", flags.Lookup("include-stopped"))
	viper.BindPFlag("include_restarting", flags.Lookup("include-restarting"))
	viper.BindPFlag("revive_stopped", flags.Lookup("revive-stopped"))
	viper.BindPFlag("scope", flags.Lookup("scope"))
	viper.BindPFlag("cleanup_images", flags.Lookup("cleanup"))
	viper.BindPFlag("remove_volumes", flags.Lookup("remove-volumes"))
	viper.BindPFlag("stop_timeout", flags.Lookup("stop-timeout"))
	viper.BindPFlag("dry_run", flags.Lookup("dry-run"))
	viper.BindPFlag("run_once", flags.Lookup("run-once"))
	viper.BindPFlag("no_pull", flags.Lookup("no-pull"))
	viper.BindPFlag("no_restart", flags.Lookup("no-restart"))
	viper.BindPFlag("rolling_restart", flags.Lookup("rolling-restart"))
	viper.BindPFlag("label_precedence", flags.Lookup("label-precedence"))
	viper.BindPFlag("lifecycle_hooks", flags.Lookup("lifecycle-hooks"))
	viper.BindPFlag("http_addr", flags.Lookup("http-addr"))
	viper.BindPFlag("http_enabled", flags.Lookup("http-enabled"))
	viper.BindPFlag("http_api_token", flags.Lookup("http-api-token"))
	viper.BindPFlag("metrics_enabled", flags.Lookup("metrics"))
	viper.BindPFlag("webhook_url", flags.Lookup("webhook-url"))
	viper.BindPFlag("notification_template", flags.Lookup("notification-template"))
	viper.BindPFlag("no_startup_message", flags.Lookup("no-startup-message"))
	viper.BindPFlag("log_level", flags.Lookup("log-level"))
	viper.BindPFlag("warn_on_head_failure", flags.Lookup("warn-on-head-failure"))
	viper.BindPFlag("policy_file", flags.Lookup("policy-file"))
	viper.BindPFlag("audit_log", flags.Lookup("audit-log"))

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
	level, err := log.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = log.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp: true,
	})

	log.Infof("Starting Updock %s", config.Version)

	if cfg.DryRun {
		log.Warn("Running in dry-run mode - no updates will be applied")
	}
	if cfg.RunOnce {
		log.Info("Running in run-once mode - will exit after single check")
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
	defer dockerClient.Close()

	// Verify Docker connection
	if err := dockerClient.Ping(ctx); err != nil {
		return fmt.Errorf("failed to connect to Docker daemon: %w", err)
	}
	log.Info("Connected to Docker daemon")

	// Load declarative policy file (updock.yml)
	spec, err := policy.LoadSpec(cfg.PolicyFile)
	if err != nil {
		return fmt.Errorf("failed to load policy file: %w", err)
	}
	log.Infof("Loaded policy file: %s (%d policies, %d container overrides, %d groups)",
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
		log.Infof("Run-once complete: %d checked, %d updated", len(results), updated)
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
		server := api.NewServer(dockerClient, upd, cfg)
		go func() {
			if err := server.Start(); err != nil && err != http.ErrServerClosed {
				log.Errorf("HTTP server error: %v", err)
			}
		}()
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.StopTimeout)
			defer shutdownCancel()
			server.Stop(shutdownCtx)
		}()
	}

	// Wait for shutdown signal
	sig := <-sigCh
	log.Infof("Received signal %s, shutting down...", sig)
	cancel()

	return nil
}
