package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/cexll/agentsdk-go/pkg/agent"
	"github.com/cexll/agentsdk-go/pkg/middleware"
)

// securityMiddleware performs a small set of synchronous checks to highlight
// where input and output validation would live inside middleware hooks.
type securityMiddleware struct {
	blocked []string
	logger  *slog.Logger
}

func newSecurityMiddleware(blocked []string, logger *slog.Logger) middleware.Middleware {
	if len(blocked) == 0 {
		blocked = []string{"drop table", "rm -rf", "system.exit"}
	}
	return &securityMiddleware{blocked: blocked, logger: logger}
}

func (m *securityMiddleware) Name() string { return "security" }

func (m *securityMiddleware) BeforeAgent(_ context.Context, st *middleware.State) error {
	ctx, _ := st.Agent.(*agent.Context)
	if ctx == nil {
		return errors.New("security: missing agent context")
	}
	prompt := readString(ctx.Values, promptKey)
	if prompt == "" {
		return errors.New("security: prompt is empty")
	}
	if hit := m.detect(prompt); hit != "" {
		return fmt.Errorf("security: prompt contains blocked phrase %q", hit)
	}
	st.Values[securityFlagsKey] = []string{}
	noteFlag(st, "prompt validated")
	return nil
}

func (m *securityMiddleware) BeforeModel(_ context.Context, st *middleware.State) error {
	m.logger.Debug("prompt accepted", "request_id", readString(st.Values, requestIDKey))
	return nil
}

func (m *securityMiddleware) AfterModel(_ context.Context, st *middleware.State) error {
	out, _ := st.ModelOutput.(*agent.ModelOutput)
	if out != nil {
		if hit := m.detect(out.Content); hit != "" {
			return fmt.Errorf("security: model output blocked phrase %q", hit)
		}
	}
	return nil
}

func (m *securityMiddleware) BeforeTool(_ context.Context, st *middleware.State) error {
	call, _ := st.ToolCall.(agent.ToolCall)
	query, _ := call.Input["query"].(string)
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("security: tool %s missing query", call.Name)
	}
	noteFlag(st, "tool params ok")
	return nil
}

func (m *securityMiddleware) AfterTool(_ context.Context, st *middleware.State) error {
	res, _ := st.ToolResult.(agent.ToolResult)
	if hit := m.detect(res.Output); hit != "" {
		return fmt.Errorf("security: tool %s output blocked phrase %q", res.Name, hit)
	}
	return nil
}

func (m *securityMiddleware) AfterAgent(_ context.Context, st *middleware.State) error {
	flags, _ := st.Values[securityFlagsKey].([]string)
	m.logger.Info("security review passed", "request_id", readString(st.Values, requestIDKey), "flags", flags)
	return nil
}

func (m *securityMiddleware) detect(s string) string {
	text := strings.ToLower(s)
	for _, blocked := range m.blocked {
		if strings.Contains(text, strings.ToLower(blocked)) {
			return blocked
		}
	}
	return ""
}

func noteFlag(st *middleware.State, msg string) {
	flags, _ := st.Values[securityFlagsKey].([]string)
	flags = append(flags, msg)
	st.Values[securityFlagsKey] = flags
}
