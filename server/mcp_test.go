package server

// MCP ingress tests: fail-closed mounting, bearer auth, and the paperclip
// intake round trip (task_create -> task_status -> task_list).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"artex-global/db"
)

func mcpPOST(t *testing.T, h http.Handler, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// jsonRPC builds a tools/call request frame for one tool invocation.
func jsonRPC(id int, tool string, args any) string {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": id, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args},
	})
	return string(b)
}

func rpcResult(t *testing.T, rec *httptest.ResponseRecorder) (string, bool) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("http %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Error != nil {
		t.Fatalf("rpc error: %s", out.Error.Message)
	}
	text := ""
	if len(out.Result.Content) > 0 {
		text = out.Result.Content[0].Text
	}
	return text, out.Result.IsError
}

func newMCPServer(t *testing.T) *Server {
	t.Helper()
	dsn, _, err := db.DSN()
	if err != nil {
		t.Skip("test postgres not configured")
	}
	pg, err := db.Open(dsn)
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { pg.Close() })
	td := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return &Server{ctx: ctx, m: &Manager{pg: pg, dir: td, tasks: map[string]*Task{}}}
}

func TestMCPDisabledWithoutToken(t *testing.T) {
	t.Setenv("ARTEX_MCP_TOKEN", "")
	s := &Server{}
	if h := s.mcpHandler(); h != nil {
		t.Fatal("handler mounted without a token; must stay invisible (fail closed)")
	}
}

func TestMCPAuthAndTaskIntake(t *testing.T) {
	t.Setenv("ARTEX_MCP_TOKEN", "test-token")
	s := newMCPServer(t)

	h := s.mcpHandler()
	if h == nil {
		t.Fatal("handler not mounted with ARTEX_MCP_TOKEN set")
	}

	// Wrong / missing token -> 401, never the tool surface.
	if rec := mcpPOST(t, h, "wrong", jsonRPC(1, "task_list", nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: http %d", rec.Code)
	}
	if rec := mcpPOST(t, h, "", jsonRPC(1, "task_list", nil)); rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing token: http %d", rec.Code)
	}

	autostart := false
	// create paused (autostart=false) so the test needs no LLM providers.
	rec := mcpPOST(t, h, "test-token", jsonRPC(1, "task_create", map[string]any{
		"description": "mcp fixture task", "goal": "prove the MCP intake works",
		"autostart": autostart,
	}))
	text, isErr := rpcResult(t, rec)
	if isErr || !strings.Contains(text, "task created") {
		t.Fatalf("task_create: isErr=%v %s", isErr, text)
	}
	var dto TaskDTO
	if err := json.Unmarshal([]byte(strings.TrimPrefix(text, "task created: ")), &dto); err != nil {
		t.Fatalf("task_create DTO: %v (%s)", err, text)
	}
	if dto.ID == "" {
		t.Fatal("task_create returned no id")
	}

	// status round trip.
	rec = mcpPOST(t, h, "test-token", jsonRPC(2, "task_status", map[string]any{"task_id": dto.ID}))
	text, isErr = rpcResult(t, rec)
	if isErr || !strings.Contains(text, "prove the MCP intake works") {
		t.Fatalf("task_status: isErr=%v %s", isErr, text)
	}

	// list contains the new task (search by the unique marker — the shared test
	// DB holds tasks from other suites).
	rec = mcpPOST(t, h, "test-token", jsonRPC(3, "task_list", map[string]any{"limit": 200}))
	text, isErr = rpcResult(t, rec)
	if isErr || !strings.Contains(text, "#"+dto.ID+" [") {
		t.Fatalf("task_list: isErr=%v %s", isErr, text)
	}

	// create without goal -> tool error, not a server error.
	rec = mcpPOST(t, h, "test-token", jsonRPC(4, "task_create", map[string]any{"description": "x"}))
	if _, isErr = rpcResult(t, rec); !isErr {
		t.Fatal("task_create without goal must be a tool error")
	}

	// browser Origin is always refused (DNS-rebinding guard).
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(jsonRPC(5, "task_list", nil)))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Origin", "http://evil.example")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("origin must be refused, got http %d", rec2.Code)
	}
}
