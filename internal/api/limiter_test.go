package api

import (
	"testing"
	"time"
)

func TestLoginLimiterBlocksAfterFiveFailures(t *testing.T) {
	var limiter loginLimiter
	now := time.Unix(1_700_000_000, 0)
	for range 4 {
		limiter.failed(now)
	}
	if !limiter.allowed(now) {
		t.Fatal("limiter blocked before the fifth failure")
	}
	limiter.failed(now)
	if limiter.allowed(now) {
		t.Fatal("limiter did not block after the fifth failure")
	}
	if !limiter.allowed(now.Add(31 * time.Second)) {
		t.Fatal("limiter did not release after its delay")
	}
	limiter.succeeded()
	if !limiter.allowed(now) {
		t.Fatal("successful authentication did not reset limiter")
	}
}
