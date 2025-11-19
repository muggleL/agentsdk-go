package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/cexll/agentsdk-go/pkg/middleware"
)

type monitoringMiddleware struct {
	threshold time.Duration
	logger    *slog.Logger
	metrics   *metricsRegistry
}

type metricsRegistry struct {
	mu         sync.Mutex
	totalRuns  int
	slowRuns   int
	maxLatency time.Duration
	lastRun    time.Duration
}

func newMonitoringMiddleware(threshold time.Duration, logger *slog.Logger) *monitoringMiddleware {
	return &monitoringMiddleware{
		threshold: threshold,
		logger:    logger,
		metrics:   &metricsRegistry{},
	}
}

func (m *monitoringMiddleware) Name() string { return "monitoring" }

func (m *monitoringMiddleware) BeforeAgent(_ context.Context, st *middleware.State) error {
	st.Values["monitoring.start"] = time.Now()
	return nil
}

func (m *monitoringMiddleware) BeforeModel(_ context.Context, st *middleware.State) error {
	st.Values[fmt.Sprintf("monitoring.iter.%d", st.Iteration)] = time.Now()
	return nil
}

func (m *monitoringMiddleware) AfterModel(_ context.Context, st *middleware.State) error {
	start := nowOr(st.Values[fmt.Sprintf("monitoring.iter.%d", st.Iteration)], time.Now())
	latency := time.Since(start)
	if latency > m.threshold {
		m.logger.Warn("slow model iteration", "request_id", readString(st.Values, requestIDKey), "iteration", st.Iteration, "latency", latency)
	}
	return nil
}

func (m *monitoringMiddleware) BeforeTool(_ context.Context, st *middleware.State) error {
	st.Values[fmt.Sprintf("monitoring.tool.%d", st.Iteration)] = time.Now()
	return nil
}

func (m *monitoringMiddleware) AfterTool(_ context.Context, st *middleware.State) error {
	latency := time.Since(nowOr(st.Values[fmt.Sprintf("monitoring.tool.%d", st.Iteration)], time.Now()))
	if latency > m.threshold {
		m.logger.Warn("slow tool call", "request_id", readString(st.Values, requestIDKey), "latency", latency)
	}
	return nil
}

func (m *monitoringMiddleware) AfterAgent(_ context.Context, st *middleware.State) error {
	started := nowOr(st.Values["monitoring.start"], time.Now())
	latency := time.Since(started)
	slow := latency > m.threshold
	m.metrics.record(latency, slow)
	if slow {
		m.logger.Info("request flagged as slow", "request_id", readString(st.Values, requestIDKey), "latency", latency)
	}
	return nil
}

func (reg *metricsRegistry) record(latency time.Duration, slow bool) {
	reg.mu.Lock()
	defer reg.mu.Unlock()
	reg.totalRuns++
	reg.lastRun = latency
	if latency > reg.maxLatency {
		reg.maxLatency = latency
	}
	if slow {
		reg.slowRuns++
	}
}

func (m *monitoringMiddleware) Snapshot() (total int, slow int, max time.Duration, last time.Duration) {
	m.metrics.mu.Lock()
	defer m.metrics.mu.Unlock()
	return m.metrics.totalRuns, m.metrics.slowRuns, m.metrics.maxLatency, m.metrics.lastRun
}
