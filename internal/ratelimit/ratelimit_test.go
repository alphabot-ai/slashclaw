package ratelimit

import (
	"testing"
	"time"
)

func TestMemoryLimiter_Allow(t *testing.T) {
	limiter := NewMemoryLimiter()

	// First request should be allowed
	if !limiter.Allow("test-key", 3, time.Hour) {
		t.Error("first request should be allowed")
	}

	// Second request should be allowed
	if !limiter.Allow("test-key", 3, time.Hour) {
		t.Error("second request should be allowed")
	}

	// Third request should be allowed
	if !limiter.Allow("test-key", 3, time.Hour) {
		t.Error("third request should be allowed")
	}

	// Fourth request should be denied (limit is 3)
	if limiter.Allow("test-key", 3, time.Hour) {
		t.Error("fourth request should be denied")
	}

	// Different key should be allowed
	if !limiter.Allow("other-key", 3, time.Hour) {
		t.Error("different key should be allowed")
	}
}

func TestMemoryLimiter_Remaining(t *testing.T) {
	limiter := NewMemoryLimiter()

	// Initially should have full limit
	if r := limiter.Remaining("test-key", 5, time.Hour); r != 5 {
		t.Errorf("Remaining = %d, want 5", r)
	}

	// After one request
	limiter.Allow("test-key", 5, time.Hour)
	if r := limiter.Remaining("test-key", 5, time.Hour); r != 4 {
		t.Errorf("Remaining = %d, want 4", r)
	}

	// After exhausting limit
	for i := 0; i < 4; i++ {
		limiter.Allow("test-key", 5, time.Hour)
	}
	if r := limiter.Remaining("test-key", 5, time.Hour); r != 0 {
		t.Errorf("Remaining = %d, want 0", r)
	}
}

func TestMemoryLimiter_RetryAfter(t *testing.T) {
	limiter := NewMemoryLimiter()

	// Before any requests, should be 0
	if r := limiter.RetryAfter("test-key", time.Hour); r != 0 {
		t.Errorf("RetryAfter = %v, want 0", r)
	}

	// After a request, should be positive
	limiter.Allow("test-key", 5, time.Hour)
	retryAfter := limiter.RetryAfter("test-key", time.Hour)
	if retryAfter <= 0 || retryAfter > time.Hour {
		t.Errorf("RetryAfter = %v, want > 0 and <= 1h", retryAfter)
	}
}

func TestMemoryLimiter_WindowReset(t *testing.T) {
	limiter := NewMemoryLimiter()

	// Use a very short window
	window := 50 * time.Millisecond

	// Exhaust the limit
	limiter.Allow("test-key", 1, window)
	if limiter.Allow("test-key", 1, window) {
		t.Error("should be rate limited")
	}

	// Wait for window to reset
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	if !limiter.Allow("test-key", 1, window) {
		t.Error("should be allowed after window reset")
	}
}

func TestMemoryLimiter_Cleanup(t *testing.T) {
	limiter := NewMemoryLimiter()

	// Add some entries with short window
	limiter.Allow("key1", 1, 10*time.Millisecond)
	limiter.Allow("key2", 1, time.Hour)

	// Wait for key1 to expire
	time.Sleep(20 * time.Millisecond)

	// Run cleanup
	limiter.Cleanup()

	// key1 should be gone (new bucket), key2 should remain
	if r := limiter.Remaining("key1", 1, time.Hour); r != 1 {
		t.Error("key1 should have been cleaned up and reset")
	}

	// key2 should still show used
	if r := limiter.Remaining("key2", 1, time.Hour); r != 0 {
		t.Errorf("key2 Remaining = %d, want 0 (still active)", r)
	}
}

func TestMemoryLimiter_Concurrent(t *testing.T) {
	limiter := NewMemoryLimiter()
	limit := 100
	done := make(chan bool, limit*2)

	// Launch concurrent requests
	for i := 0; i < limit*2; i++ {
		go func() {
			limiter.Allow("concurrent-key", limit, time.Hour)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < limit*2; i++ {
		<-done
	}

	// Should have exactly 0 remaining (limit exhausted)
	if r := limiter.Remaining("concurrent-key", limit, time.Hour); r != 0 {
		t.Errorf("Remaining = %d, want 0 after concurrent access", r)
	}
}
