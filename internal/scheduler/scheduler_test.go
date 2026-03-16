package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/huseyinbabal/updock/internal/audit"
	"github.com/huseyinbabal/updock/internal/config"
	"github.com/huseyinbabal/updock/internal/docker"
	"github.com/huseyinbabal/updock/internal/mocks"
	"github.com/huseyinbabal/updock/internal/policy"
	"github.com/huseyinbabal/updock/internal/updater"
)

func newTestUpdater() *updater.Updater {
	cfg := &config.Config{MonitorAll: true}
	return updater.New(nil, nil, nil, cfg, policy.DefaultSpec(), audit.NewLog(""))
}

func TestNew(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 5*time.Minute, "")
	if s == nil {
		t.Fatal("expected non-nil scheduler")
	}
	if s.interval != 5*time.Minute {
		t.Errorf("expected 5m interval, got %v", s.interval)
	}
	if s.cronExpr != "" {
		t.Errorf("expected empty cron, got %q", s.cronExpr)
	}
}

func TestNew_WithCron(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 0, "0 0 * * * *")
	if s.cronExpr != "0 0 * * * *" {
		t.Errorf("expected cron expression, got %q", s.cronExpr)
	}
}

func TestStartInterval_AndStop(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 1*time.Hour, "") // long interval so ticker doesn't fire

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so initial run goroutine exits fast

	err := s.startInterval(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	s.Stop()
}

func TestStartCron_AndStop(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 0, "0 0 1 1 * *") // far-future cron

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.startCron(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	s.Stop()
}

func TestStartCron_InvalidExpr(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 0, "invalid cron")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.startCron(ctx)
	if err == nil {
		t.Error("expected error for invalid cron expression")
		s.Stop()
	}
}

func TestStop_ContextCancel(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 1*time.Hour, "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = s.startInterval(ctx)
	time.Sleep(20 * time.Millisecond)
	// No panic = success
}

func TestStop_NilCron(t *testing.T) {
	u := newTestUpdater()
	s := New(u, 1*time.Hour, "")
	// Stop without Start - cronJob is nil
	s.Stop()
}

func newMockUpdater(t *testing.T) *updater.Updater {
	mockDocker := mocks.NewMockDockerClient(t)
	mockDocker.EXPECT().ListContainers(mock.Anything, false, false).Return([]docker.ContainerInfo{}, nil).Maybe()
	cfg := &config.Config{MonitorAll: true}
	return updater.New(mockDocker, nil, nil, cfg, policy.DefaultSpec(), audit.NewLog(""))
}

func TestStart_IntervalMode(t *testing.T) {
	u := newMockUpdater(t)
	s := New(u, 1*time.Hour, "")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // let initial goroutine run
	s.Stop()
}

func TestStart_CronMode(t *testing.T) {
	u := newMockUpdater(t)
	s := New(u, 0, "0 0 1 1 * *") // far-future cron

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.Start(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	s.Stop()
}

func TestStart_CronInvalid(t *testing.T) {
	u := newMockUpdater(t)
	s := New(u, 0, "bad cron expr")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.Start(ctx)
	if err == nil {
		t.Error("expected error for invalid cron expression")
		s.Stop()
	}
}
