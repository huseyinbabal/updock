// Package scheduler manages periodic execution of update check cycles.
//
// It supports two scheduling modes:
//
//   - Interval-based: runs update checks at a fixed interval (e.g. every 5 minutes).
//   - Cron-based: runs update checks according to a 6-field cron expression
//     with second-level precision (e.g. "0 0 4 * * *" for daily at 04:00).
//
// The scheduler always runs an initial update check immediately on startup,
// then begins the periodic schedule. Both modes support graceful shutdown
// via context cancellation.
//
// # Cron Expression Format
//
// The cron expression uses 6 fields (with seconds):
//
//	┌──────────── second (0-59)
//	│ ┌────────── minute (0-59)
//	│ │ ┌──────── hour (0-23)
//	│ │ │ ┌────── day of month (1-31)
//	│ │ │ │ ┌──── month (1-12)
//	│ │ │ │ │ ┌── day of week (0-6, Sun=0)
//	│ │ │ │ │ │
//	* * * * * *
package scheduler

import (
	"context"
	"time"

	"github.com/huseyinbabal/updock/internal/logger"
	"github.com/huseyinbabal/updock/internal/metrics"
	"github.com/huseyinbabal/updock/internal/updater"
	"github.com/robfig/cron/v3"
)

// Scheduler manages periodic update check execution.
// It wraps either a time.Ticker (interval mode) or a cron.Cron (cron mode).
type Scheduler struct {
	updater  *updater.Updater
	interval time.Duration
	cronExpr string
	cronJob  *cron.Cron
	stopCh   chan struct{}
}

// New creates a new Scheduler. If cronExpr is non-empty, cron mode is used;
// otherwise interval mode is used with the given interval duration.
func New(u *updater.Updater, interval time.Duration, cronExpr string) *Scheduler {
	return &Scheduler{
		updater:  u,
		interval: interval,
		cronExpr: cronExpr,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduler. It immediately runs an initial update check
// in a separate goroutine, then starts the periodic schedule.
//
// The context is used for cancellation. When the context is cancelled or
// [Scheduler.Stop] is called, the scheduler stops scheduling new checks
// (in-flight checks are allowed to complete).
func (s *Scheduler) Start(ctx context.Context) error {
	// Run initial check immediately
	go func() {
		metrics.ScansTotal.Inc()
		if _, err := s.updater.Run(ctx); err != nil {
			logger.Error().Msgf("Initial update check failed: %v", err)
		}
	}()

	if s.cronExpr != "" {
		return s.startCron(ctx)
	}
	return s.startInterval(ctx)
}

// startCron initializes and starts a cron-based scheduler.
func (s *Scheduler) startCron(ctx context.Context) error {
	s.cronJob = cron.New(cron.WithSeconds())

	_, err := s.cronJob.AddFunc(s.cronExpr, func() {
		metrics.ScansTotal.Inc()
		if _, err := s.updater.Run(ctx); err != nil {
			logger.Error().Msgf("Scheduled update check failed: %v", err)
		}
	})
	if err != nil {
		return err
	}

	logger.Info().Msgf("Scheduler started with cron expression: %s", s.cronExpr)
	s.cronJob.Start()

	// Listen for shutdown signals
	go func() {
		select {
		case <-ctx.Done():
			s.cronJob.Stop()
		case <-s.stopCh:
			s.cronJob.Stop()
		}
	}()

	return nil
}

// startInterval initializes and starts an interval-based scheduler.
func (s *Scheduler) startInterval(ctx context.Context) error {
	logger.Info().Msgf("Scheduler started with interval: %s", s.interval)

	go func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				metrics.ScansTotal.Inc()
				if _, err := s.updater.Run(ctx); err != nil {
					logger.Error().Msgf("Scheduled update check failed: %v", err)
				}
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			}
		}
	}()

	return nil
}

// Stop signals the scheduler to stop scheduling new checks.
// Any in-flight update check will be allowed to complete.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	if s.cronJob != nil {
		s.cronJob.Stop()
	}
}
