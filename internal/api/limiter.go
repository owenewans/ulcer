package api

import (
	"sync"
	"time"
)

type loginLimiter struct {
	mu           sync.Mutex
	failures     int
	blockedUntil time.Time
}

func (l *loginLimiter) allowed(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return !now.Before(l.blockedUntil)
}

func (l *loginLimiter) failed(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures++
	if l.failures >= 5 {
		delay := time.Duration(l.failures-4) * 30 * time.Second
		if delay > 5*time.Minute {
			delay = 5 * time.Minute
		}
		l.blockedUntil = now.Add(delay)
	}
}

func (l *loginLimiter) succeeded() {
	l.mu.Lock()
	l.failures = 0
	l.blockedUntil = time.Time{}
	l.mu.Unlock()
}
