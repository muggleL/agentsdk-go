package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cexll/agentsdk-go/pkg/agent"
	"github.com/cexll/agentsdk-go/pkg/middleware"
)

const minimalConfig = "version: v0.0.1\ndescription: agentsdk-go middleware example\nenvironment: {}\n"

func main() {
	cfg := parseConfig()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	root, cleanup, err := resolveProjectRoot()
	if err != nil {
		log.Fatalf("init project root: %v", err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	monitorMW := newMonitoringMiddleware(cfg.slowThreshold, logger)
	middlewares := []middleware.Middleware{
		newLoggingMiddleware(logger),
		newRateLimitMiddleware(cfg.rps, cfg.burst, cfg.concurrent),
		newSecurityMiddleware(nil, logger),
		monitorMW,
	}
	chain := middleware.NewChain(middlewares, middleware.WithTimeout(cfg.middlewareTimeout))

	model := &demoModel{projectRoot: root}
	tools := &demoToolbox{latency: cfg.toolLatency, logger: logger}
	ag, err := agent.New(model, tools, agent.Options{
		MaxIterations: cfg.maxIterations,
		Timeout:       cfg.runTimeout,
		Middleware:    chain,
	})
	if err != nil {
		log.Fatalf("build agent: %v", err)
	}

	agentCtx := agent.NewContext()
	agentCtx.Values[promptKey] = cfg.prompt
	agentCtx.Values["project_root"] = root
	agentCtx.Values["request_owner"] = cfg.owner

	logger.Info("running middleware demo", "prompt", cfg.prompt)
	output, err := ag.Run(ctx, agentCtx)
	if err != nil {
		log.Fatalf("run agent: %v", err)
	}

	fmt.Println("\n===== Final Output =====")
	fmt.Println(output.Content)
	fmt.Println("\nTool Results:")
	for _, res := range agentCtx.ToolResults {
		fmt.Printf("- %s -> %s\n", res.Name, res.Output)
	}
	total, slow, maxLatency, lastLatency := monitorMW.Snapshot()
	logger.Info("metrics snapshot", "runs", total, "slow_runs", slow, "max_latency", maxLatency, "last_latency", lastLatency)
}

type runConfig struct {
	prompt            string
	owner             string
	rps               int
	burst             int
	concurrent        int
	slowThreshold     time.Duration
	toolLatency       time.Duration
	runTimeout        time.Duration
	middlewareTimeout time.Duration
	maxIterations     int
}

func parseConfig() runConfig {
	var cfg runConfig
	flag.StringVar(&cfg.prompt, "prompt", "分析 HTTP 日志并生成安全报告", "user prompt for the demo")
	flag.StringVar(&cfg.owner, "owner", "middleware-demo", "logical owner for logging")
	flag.IntVar(&cfg.rps, "rps", 5, "token bucket refill rate per second")
	flag.IntVar(&cfg.burst, "burst", 10, "token bucket burst size")
	flag.IntVar(&cfg.concurrent, "concurrent", 2, "maximum concurrent agent runs")
	flag.DurationVar(&cfg.slowThreshold, "slow-threshold", 250*time.Millisecond, "slow request threshold")
	flag.DurationVar(&cfg.toolLatency, "tool-latency", 150*time.Millisecond, "simulated tool latency")
	flag.DurationVar(&cfg.runTimeout, "timeout", 5*time.Second, "agent timeout")
	flag.DurationVar(&cfg.middlewareTimeout, "middleware-timeout", 2*time.Second, "per-hook timeout")
	flag.IntVar(&cfg.maxIterations, "max-iterations", 3, "max agent iterations")
	flag.Parse()
	return cfg
}

func resolveProjectRoot() (string, func(), error) {
	if root := strings.TrimSpace(os.Getenv("AGENTSDK_PROJECT_ROOT")); root != "" {
		return root, nil, nil
	}
	tmp, err := os.MkdirTemp("", "agentsdk-middleware-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	if err := scaffoldMinimalConfig(tmp); err != nil {
		cleanup()
		return "", nil, err
	}
	return tmp, cleanup, nil
}

func scaffoldMinimalConfig(root string) error {
	claudeDir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		return err
	}
	configPath := filepath.Join(claudeDir, "config.yaml")
	if _, err := os.Stat(configPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.WriteFile(configPath, []byte(minimalConfig), 0o644)
}

type demoModel struct {
	projectRoot string
}

func (m *demoModel) Generate(_ context.Context, c *agent.Context) (*agent.ModelOutput, error) {
	prompt := readString(c.Values, promptKey)
	switch c.Iteration {
	case 0:
		return &agent.ModelOutput{
			Content: fmt.Sprintf("收到指令：%s，准备分析项目 %s。", prompt, m.projectRoot),
			ToolCalls: []agent.ToolCall{{
				ID:   fmt.Sprintf("tool-%d", c.Iteration),
				Name: "observe_logs",
				Input: map[string]any{
					"query":        prompt,
					"project_root": m.projectRoot,
				},
			}},
		}, nil
	case 1:
		var summary string
		if len(c.ToolResults) > 0 {
			last := c.ToolResults[len(c.ToolResults)-1]
			summary = last.Output
		} else {
			summary = "工具未返回结果"
		}
		return &agent.ModelOutput{Content: fmt.Sprintf("安全报告：%s", summary), Done: true}, nil
	default:
		return &agent.ModelOutput{Content: "流程结束", Done: true}, nil
	}
}

type demoToolbox struct {
	latency time.Duration
	logger  *slog.Logger
}

func (t *demoToolbox) Execute(ctx context.Context, call agent.ToolCall, c *agent.Context) (agent.ToolResult, error) {
	switch call.Name {
	case "observe_logs":
		return t.runObservation(ctx, call, c)
	default:
		return agent.ToolResult{}, fmt.Errorf("unknown tool %s", call.Name)
	}
}

func (t *demoToolbox) runObservation(ctx context.Context, call agent.ToolCall, c *agent.Context) (agent.ToolResult, error) {
	select {
	case <-time.After(t.latency):
	case <-ctx.Done():
		return agent.ToolResult{}, ctx.Err()
	}
	query, _ := call.Input["query"].(string)
	root, _ := call.Input["project_root"].(string)
	result := fmt.Sprintf("已检查 %s 的最近 100 行日志，未发现高危操作；查询: %s", root, query)
	if c.Values == nil {
		c.Values = map[string]any{}
	}
	c.Values["tool.last_output"] = result
	t.logger.Info("tool finished", "tool", call.Name, "latency", t.latency)
	return agent.ToolResult{
		Name:   call.Name,
		Output: result,
		Metadata: map[string]any{
			"latency_ms": t.latency.Milliseconds(),
			"iteration":  c.Iteration,
		},
	}, nil
}
