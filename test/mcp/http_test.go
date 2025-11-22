package mcp_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cexll/agentsdk-go/pkg/mcp"
)

func TestHTTPTransportHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Test") != "ok" {
			t.Errorf("missing propagated header")
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{"jsonrpc":"2.0","id":"1","result":{}}`)); err != nil {
			t.Fatal(err)
		}
	}))
	defer server.Close()

	transport, err := mcp.NewHTTPTransport(mcp.HTTPOptions{URL: server.URL, Headers: map[string]string{"X-Test": "ok"}})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	if _, err := transport.Call(context.Background(), &mcp.Request{ID: "1", Method: "ping"}); err != nil {
		t.Fatalf("call failed: %v", err)
	}
}
