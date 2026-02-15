package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
}

func NewLimiter() *Limiter {
	return &Limiter{
		buckets: map[string][]time.Time{},
	}
}

type Result struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

func (l *Limiter) Allow(key string, limit int, window time.Duration, now time.Time) Result {
	l.mu.Lock()
	defer l.mu.Unlock()

	if limit <= 0 {
		return Result{Allowed: true}
	}
	cutoff := now.Add(-window)
	history := l.buckets[key]
	trimmed := history[:0]
	for _, ts := range history {
		if !ts.Before(cutoff) {
			trimmed = append(trimmed, ts)
		}
	}
	history = trimmed

	result := Result{
		Allowed: len(history) < limit,
		Limit:   limit,
	}
	if !result.Allowed {
		result.Remaining = 0
		result.ResetAt = history[0].Add(window)
		l.buckets[key] = history
		return result
	}

	history = append(history, now)
	l.buckets[key] = history
	result.Remaining = limit - len(history)
	result.ResetAt = history[0].Add(window)
	return result
}
