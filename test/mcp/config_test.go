package mcp_test

import (
	"encoding/json"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/config"
)

func TestSettingsParseMCPSchema(t *testing.T) {
	raw := []byte(`{"model":"demo","mcp":{"servers":{"demo":{"type":"http","url":"https://example","headers":{"X-Test":"1"}}}}}`)
	var settings config.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if settings.MCP == nil || len(settings.MCP.Servers) != 1 {
		t.Fatalf("mcp servers not parsed: %+v", settings.MCP)
	}
	server := settings.MCP.Servers["demo"]
	if server.URL != "https://example" || server.Type != "http" {
		t.Fatalf("unexpected server config: %+v", server)
	}
}
