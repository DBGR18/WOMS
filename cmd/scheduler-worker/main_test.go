package main

import (
	"context"
	"errors"
	"testing"
	"time"

	womslock "github.com/d11nn/woms/internal/lock"
)

func TestScheduleLineLockKeyScopesByProductionLine(t *testing.T) {
	if got := scheduleLineLockKey("A"); got != "woms:locks:schedule-line:A" {
		t.Fatalf("unexpected line A key %q", got)
	}
	if scheduleLineLockKey("A") == scheduleLineLockKey("B") {
		t.Fatal("different production lines must use different Redis lock keys")
	}
}

func TestAcquireLineLockRetriesContentionUntilAvailable(t *testing.T) {
	provider := &retryLockProvider{failures: 2}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	lineLock, err := acquireLineLock(ctx, provider, "woms:locks:schedule-line:A", time.Second)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	if lineLock == nil {
		t.Fatal("expected lock")
	}
	if provider.attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", provider.attempts)
	}
}

func TestAcquireLineLockStopsOnNonContentionError(t *testing.T) {
	expected := errors.New("redis unavailable")
	provider := &retryLockProvider{err: expected}
	_, err := acquireLineLock(context.Background(), provider, "woms:locks:schedule-line:A", time.Second)
	if !errors.Is(err, expected) {
		t.Fatalf("expected %v, got %v", expected, err)
	}
	if provider.attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", provider.attempts)
	}
}

type retryLockProvider struct {
	failures int
	attempts int
	err      error
}

func (p *retryLockProvider) Acquire(context.Context, string, time.Duration) (womslock.Lock, error) {
	p.attempts++
	if p.err != nil {
		return nil, p.err
	}
	if p.attempts <= p.failures {
		return nil, womslock.ErrNotAcquired
	}
	return noopLock{}, nil
}

type noopLock struct{}

func (noopLock) Refresh(context.Context, time.Duration) error { return nil }
func (noopLock) Release(context.Context) error                { return nil }
