// Package api provides the HTTP server for Updock's REST API and Web UI dashboard.
//
// The server exposes the following endpoints:
//
// # Public Endpoints
//
//	GET  /            Web UI dashboard (embedded single-page application)
//	GET  /api/health  Health check (always accessible, no auth required)
//	GET  /metrics     Prometheus metrics (when enabled)
//
// # Authenticated Endpoints (require Bearer token when --http-api-token is set)
//
//	GET  /api/containers      List all running containers
//	GET  /api/containers/{id} Container details
//	POST /api/update          Trigger an update check
//	GET  /api/history         Update history
//	GET  /api/info            Version and configuration info
//
// # Authentication
//
// When an API token is configured (--http-api-token or UPDOCK_HTTP_API_TOKEN),
// all /api/* endpoints (except /api/health) require an Authorization header:
//
//	Authorization: Bearer <token>
//
// The token also supports query parameter authentication for browser access:
//
//	/api/containers?token=<token>
//
// # Example
//
//	curl -H "Authorization: Bearer mytoken" http://localhost:8080/api/update -X POST
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/updater"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// Server is the HTTP server that hosts the REST API and Web UI.
// It provides endpoints for container management, update triggering,
// and real-time status monitoring.
type Server struct {
	docker  *docker.Client
	updater *updater.Updater
	cfg     *config.Config
	mux     *http.ServeMux
	server  *http.Server
}

// NewServer creates a new API server with all routes configured.
// The server is not started until [Server.Start] is called.
func NewServer(dockerClient *docker.Client, upd *updater.Updater, cfg *config.Config) *Server {
	s := &Server{
		docker:  dockerClient,
		updater: upd,
		cfg:     cfg,
		mux:     http.NewServeMux(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes registers all HTTP handlers.
// Health and metrics endpoints are always accessible.
// API endpoints are protected by token auth when configured.
func (s *Server) setupRoutes() {
	// Public endpoints (no auth required)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)

	// Protected API endpoints
	s.mux.HandleFunc("GET /api/containers", s.withAuth(s.handleContainers))
	s.mux.HandleFunc("GET /api/containers/{id}", s.withAuth(s.handleContainerDetail))
	s.mux.HandleFunc("POST /api/update", s.withAuth(s.handleTriggerUpdate))
	s.mux.HandleFunc("GET /api/history", s.withAuth(s.handleHistory))
	s.mux.HandleFunc("GET /api/info", s.withAuth(s.handleInfo))
	s.mux.HandleFunc("GET /api/audit", s.withAuth(s.handleAuditLog))

	// Prometheus metrics endpoint
	if s.cfg.MetricsEnabled {
		s.mux.Handle("GET /metrics", promhttp.Handler())
	}

	// Web UI (embedded SPA dashboard)
	s.mux.HandleFunc("GET /", s.handleUI)
}

// withAuth wraps a handler with Bearer token authentication.
// If no token is configured, the handler is called directly.
// Supports both Authorization header and ?token= query parameter.
func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.HTTPAPIToken == "" {
			next(w, r)
			return
		}

		// Check Authorization header: "Bearer <token>"
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") && parts[1] == s.cfg.HTTPAPIToken {
				next(w, r)
				return
			}
		}

		// Check query parameter: ?token=<token>
		if r.URL.Query().Get("token") == s.cfg.HTTPAPIToken {
			next(w, r)
			return
		}

		writeError(w, http.StatusUnauthorized, "missing or invalid API token")
	}
}

// Start starts the HTTP server in blocking mode.
// Returns [http.ErrServerClosed] when shut down gracefully via [Server.Stop].
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:         s.cfg.HTTPAddr,
		Handler:      s.corsMiddleware(s.mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Infof("Web UI available at http://0.0.0.0%s", s.cfg.HTTPAddr)
	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the HTTP server, waiting for in-flight
// requests to complete within the given context deadline.
func (s *Server) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

// corsMiddleware adds Cross-Origin Resource Sharing headers to all responses.
// This allows the Web UI to be served from a different origin during development.
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// writeJSON serializes v as JSON and writes it to the response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// handleContainers returns a list of all containers visible to Updock.
//
//	GET /api/containers
//	Response: []docker.ContainerInfo
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	containers, err := s.docker.ListContainers(
		r.Context(),
		s.cfg.IncludeStopped,
		s.cfg.IncludeRestarting,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, containers)
}

// handleContainerDetail returns detailed information about a specific container.
//
//	GET /api/containers/{id}
//	Response: docker.ContainerInfo
func (s *Server) handleContainerDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "container ID required")
		return
	}

	info, err := s.docker.InspectContainer(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, info)
}

// handleTriggerUpdate manually triggers an update check cycle.
//
//	POST /api/update
//	Response: { "message": "...", "results": [...] }
func (s *Server) handleTriggerUpdate(w http.ResponseWriter, r *http.Request) {
	results, err := s.updater.Run(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Update check completed",
		"results": results,
	})
}

// handleHistory returns the update history (most recent 1000 entries).
//
//	GET /api/history
//	Response: []updater.UpdateResult
func (s *Server) handleHistory(w http.ResponseWriter, _ *http.Request) {
	history := s.updater.History()
	writeJSON(w, http.StatusOK, history)
}

// handleHealth is a health check endpoint that verifies Docker daemon connectivity.
// This endpoint is always accessible (no auth required).
//
//	GET /api/health
//	Response: { "status": "healthy" } or { "status": "unhealthy", "error": "..." }
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	err := s.docker.Ping(r.Context())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"error":  err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
	})
}

// handleInfo returns Updock version and current configuration.
//
//	GET /api/info
//	Response: { "version": "...", "interval": "...", ... }
func (s *Server) handleInfo(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":    config.Version,
		"interval":   s.cfg.Interval.String(),
		"schedule":   s.cfg.Schedule,
		"monitorAll": s.cfg.MonitorAll,
		"dryRun":     s.cfg.DryRun,
		"scope":      s.cfg.Scope,
	})
}

// handleAuditLog returns the audit log entries.
// Supports optional query parameters: container, type, limit.
//
//	GET /api/audit
//	GET /api/audit?container=nginx&limit=50
//	Response: []audit.Entry
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	auditLog := s.updater.AuditLog()
	if auditLog == nil {
		writeJSON(w, http.StatusOK, []struct{}{})
		return
	}

	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	container := r.URL.Query().Get("container")
	entries := auditLog.Query(container, "", limit)
	writeJSON(w, http.StatusOK, entries)
}

// handleUI serves the embedded Web UI dashboard.
// Only responds on the exact "/" path; all other paths return 404.
func (s *Server) handleUI(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}
