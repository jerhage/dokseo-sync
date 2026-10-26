package api

import (
	"sync"
	"time"
)

const (
	maxFailures   = 5
	failureWindow = 15 * time.Minute
)

type failureLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

func (l *failureLimiter) locked(username string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.prune(now)
	return len(l.failures[username]) >= maxFailures
}

func (l *failureLimiter) record(username string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.failures == nil {
		l.failures = make(map[string][]time.Time)
	}
	l.failures[username] = append(l.failures[username], now)
}

func (l *failureLimiter) prune(now time.Time) {
	for username, times := range l.failures {
		kept := times[:0]
		for _, at := range times {
			if now.Sub(at) < failureWindow {
				kept = append(kept, at)
			}
		}
		if len(kept) == 0 {
			delete(l.failures, username)
			continue
		}
		l.failures[username] = kept
	}
}
