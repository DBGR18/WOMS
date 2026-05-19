package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetryDependencyEventuallySucceeds(t *testing.T) {
	attempts := 0
	err := retryDependency(context.Background(), "test", time.Millisecond, func(context.Context) error {
		attempts++
		if attempts < 3 {
			return errors.New("not ready")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("retryDependency returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRetryDependencyStopsWhenContextExpires(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	err := retryDependency(ctx, "test", time.Millisecond, func(context.Context) error {
		return errors.New("not ready")
	})
	if err == nil {
		t.Fatal("retryDependency returned nil, want timeout error")
	}
}
