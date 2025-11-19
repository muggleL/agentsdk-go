package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/cexll/agentsdk-go/pkg/agent"
	"github.com/cexll/agentsdk-go/pkg/middleware"
)

// loggingMiddleware prints structured request/response logs and records
// elapsed time across every interception point.
type loggingMiddleware struct {
	logger *slog.Logger
}

func newLoggingMiddleware(logger *slog.Logger) middleware.Middleware {
	return &loggingMiddleware{logger: logger}
}

func (m *loggingMiddleware) Name() string { return "logging" }

func (m *loggingMiddleware) BeforeAgent(_ context.Context, st *middleware.State) error {
	reqID := genRequestID()
	if st.Values == nil {
		st.Values = map[string]any{}
	}
	st.Values[requestIDKey] = reqID
	st.Values[startedAtKey] = time.Now()
	m.logger.Info("agent request start", "request_id", reqID)
	return nil
}

func (m *loggingMiddleware) BeforeModel(_ context.Context, st *middleware.State) error {
	m.logger.Info("before model", "request_id", readString(st.Values, requestIDKey), "iteration", st.Iteration)
	return nil
}

func (m *loggingMiddleware) AfterModel(_ context.Context, st *middleware.State) error {
	out, _ := st.ModelOutput.(*agent.ModelOutput)
	reqID := readString(st.Values, requestIDKey)
	if out == nil {
		m.logger.Warn("model returned nil output", "request_id", reqID, "iteration", st.Iteration)
		return nil
	}
	m.logger.Info("after model", "request_id", reqID, "content", clampPreview(out.Content, 64), "tool_calls", len(out.ToolCalls))
	return nil
}

func (m *loggingMiddleware) BeforeTool(_ context.Context, st *middleware.State) error {
	call, _ := st.ToolCall.(agent.ToolCall)
	m.logger.Info("before tool", "request_id", readString(st.Values, requestIDKey), "tool", call.Name)
	return nil
}

func (m *loggingMiddleware) AfterTool(_ context.Context, st *middleware.State) error {
	res, _ := st.ToolResult.(agent.ToolResult)
	m.logger.Info("after tool", "request_id", readString(st.Values, requestIDKey), "tool", res.Name, "output", clampPreview(res.Output, 64))
	return nil
}

func (m *loggingMiddleware) AfterAgent(_ context.Context, st *middleware.State) error {
	started := nowOr(st.Values[startedAtKey], time.Now())
	elapsed := time.Since(started)
	flags, _ := st.Values[securityFlagsKey].([]string)
	m.logger.Info("agent request done", "request_id", readString(st.Values, requestIDKey), "iterations", st.Iteration+1, "elapsed", elapsed, "security_flags", flags)
	return nil
}
