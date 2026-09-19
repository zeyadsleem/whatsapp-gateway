package ratelimit

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*entry
	rps     rate.Limit
	burst   int
	idleTTL time.Duration
}

type entry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func New(rps float64, burst int) *Limiter {
	return &Limiter{
		buckets: make(map[string]*entry),
		rps:     rate.Limit(rps),
		burst:   burst,
		idleTTL: 10 * time.Minute,
	}
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	e, ok := l.buckets[key]
	if !ok {
		e = &entry{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[key] = e
	}
	e.lastSeen = time.Now()
	l.mu.Unlock()
	return e.limiter.Allow()
}

func (l *Limiter) CleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.cleanup()
		}
	}
}

func (l *Limiter) cleanup() {
	cutoff := time.Now().Add(-l.idleTTL)
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, e := range l.buckets {
		if e.lastSeen.Before(cutoff) {
			delete(l.buckets, key)
		}
	}
}
