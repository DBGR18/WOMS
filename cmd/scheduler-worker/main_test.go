package main

import "testing"

func TestScheduleLineLockKeyScopesByProductionLine(t *testing.T) {
	if got := scheduleLineLockKey("A"); got != "woms:locks:schedule-line:A" {
		t.Fatalf("unexpected line A key %q", got)
	}
	if scheduleLineLockKey("A") == scheduleLineLockKey("B") {
		t.Fatal("different production lines must use different Redis lock keys")
	}
}
