# agentsdk-go

Minimal scaffold for the agentsdk-go project described in `docs/architecture.md`. The tree follows the reference architecture from section 3.3, ensuring every package has a placeholder implementation so future work can focus on behavior rather than wiring.

## Layout

- `pkg/`: Runtime packages grouped by responsibility (agent core, tools, models, session, events, workflow, evals, security)
- `cmd/agentctl`: CLI entry points (`main`, `run`, `serve`, `config`)
- `examples/`:
  - Learning path that climbs from minimal `agent.Run()` to streaming, tools, and HTTP servers.
  - Primary samples: `basic`, `simple-stream`, `tool-basic`, `tool-stream`, `http-simple`, `http-stream`, `http-full`.
  - See `examples/README.md` for a concise walkthrough and run commands.
- `docs/`: Architecture + onboarding material plus ADR seeds
- `tests/`: Unit, integration, and end-to-end harness directories awaiting test cases

## Next Steps

1. Flesh out the interfaces with real logic and add unit coverage per package.
2. Implement at least one model adapter (Anthropic/OpenAI) end to end.
3. Bring `agentctl` commands to life and connect them to the workflow/session subsystems.
