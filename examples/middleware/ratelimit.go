package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/cexll/agentsdk-go/pkg/middleware"
)

// rateLimitMiddleware enforces a lightweight token bucket plus a concurrency gate.
type rateLimitMiddleware struct {
	ratePerSec float64
	burst      float64
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
	concurrent chan struct{}
}

func newRateLimitMiddleware(rps, burst, maxConcurrent int) *rateLimitMiddleware {
	if rps <= 0 {
		rps = 5
	}
	if burst <= 0 {
		burst = rps
	}
	if maxConcurrent <= 0 {
		maxConcurrent = 2
	}
	return &rateLimitMiddleware{
		ratePerSec: float64(rps),
		burst:      float64(burst),
		tokens:     float64(burst),
		lastRefill: time.Now(),
		concurrent: make(chan struct{}, maxConcurrent),
	}
}

func (m *rateLimitMiddleware) Name() string { return "ratelimit" }

func (m *rateLimitMiddleware) BeforeAgent(ctx context.Context, st *middleware.State) error {
	if err := m.waitForToken(ctx); err != nil {
		return err
	}
	select {
	case m.concurrent <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return errors.New("ratelimit: concurrent limit reached")
	}
}

func (m *rateLimitMiddleware) AfterAgent(_ context.Context, _ *middleware.State) error {
	select {
	case <-m.concurrent:
	default:
	}
	return nil
}

func (m *rateLimitMiddleware) BeforeModel(context.Context, *middleware.State) error { return nil }
func (m *rateLimitMiddleware) AfterModel(context.Context, *middleware.State) error  { return nil }
func (m *rateLimitMiddleware) BeforeTool(context.Context, *middleware.State) error  { return nil }
func (m *rateLimitMiddleware) AfterTool(context.Context, *middleware.State) error   { return nil }

func (m *rateLimitMiddleware) waitForToken(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if m.tryConsume() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (m *rateLimitMiddleware) tryConsume() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(m.lastRefill).Seconds()
	if elapsed > 0 {
		m.tokens += elapsed * m.ratePerSec
		if m.tokens > m.burst {
			m.tokens = m.burst
		}
		m.lastRefill = now
	}
	if m.tokens < 1 {
		return false
	}
	m.tokens -= 1
	return true
}
