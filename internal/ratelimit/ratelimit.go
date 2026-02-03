package ratelimit

import (
	"sync"
	"time"
)

// Limiter defines the rate limiting interface
type Limiter interface {
	// Allow checks if the action is allowed for the given key
	// Returns true if allowed, false if rate limited
	Allow(key string, limit int, window time.Duration) bool

	// Remaining returns the number of remaining requests for the key
	Remaining(key string, limit int, window time.Duration) int

	// RetryAfter returns the duration until the rate limit resets
	RetryAfter(key string, window time.Duration) time.Duration
}

// MemoryLimiter is an in-memory rate limiter implementation
type MemoryLimiter struct {
	mu      sync.RWMutex
	buckets map[string]*bucket
}

type bucket struct {
	count     int
	resetTime time.Time
}

// NewMemoryLimiter creates a new in-memory rate limiter
func NewMemoryLimiter() *MemoryLimiter {
	return &MemoryLimiter{
		buckets: make(map[string]*bucket),
	}
}

func (l *MemoryLimiter) Allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]

	if !ok || now.After(b.resetTime) {
		l.buckets[key] = &bucket{
			count:     1,
			resetTime: now.Add(window),
		}
		return true
	}

	if b.count >= limit {
		return false
	}

	b.count++
	return true
}

func (l *MemoryLimiter) Remaining(key string, limit int, window time.Duration) int {
	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now()
	b, ok := l.buckets[key]

	if !ok || now.After(b.resetTime) {
		return limit
	}

	remaining := limit - b.count
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (l *MemoryLimiter) RetryAfter(key string, window time.Duration) time.Duration {
	l.mu.RLock()
	defer l.mu.RUnlock()

	now := time.Now()
	b, ok := l.buckets[key]

	if !ok || now.After(b.resetTime) {
		return 0
	}

	return b.resetTime.Sub(now)
}

// Cleanup removes expired buckets to prevent memory leaks
func (l *MemoryLimiter) Cleanup() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	for key, b := range l.buckets {
		if now.After(b.resetTime) {
			delete(l.buckets, key)
		}
	}
}

// StartCleanup starts a background goroutine to periodically clean up expired buckets
func (l *MemoryLimiter) StartCleanup(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			l.Cleanup()
		}
	}()
}

// Ensure MemoryLimiter implements Limiter
var _ Limiter = (*MemoryLimiter)(nil)
