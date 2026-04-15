package claude

import (
	"fmt"
	"testing"
	"time"
)

type mockExecutor struct {
	responses [][]byte
	errors    []error
	calls     [][]string
	callIdx   int
}

func (m *mockExecutor) Execute(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	i := m.callIdx
	m.callIdx++
	if i < len(m.responses) {
		return m.responses[i], nil
	}
	if i < len(m.errors) {
		return nil, m.errors[i]
	}
	return nil, fmt.Errorf("no response configured for call %d", i)
}

func noopSleep(d time.Duration) {}

func noopPrompter(q string) (string, error) { return "y", nil }

func TestClient_Run_UsesSessionIDFlag(t *testing.T) {
	mock := &mockExecutor{
		responses: [][]byte{[]byte(`{"result":"ok","is_error":false}`)},
	}
	c := newTestClient(mock, "sess-abc", "model-x")
	_, err := c.Run("do the thing", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := mock.calls[0]
	assertFlagValue(t, args, "--session-id", "sess-abc")
	assertFlagValue(t, args, "--model", "model-x")
	assertContains(t, args, "--dangerously-skip-permissions")
	assertContains(t, args, "-p")
}

func TestClient_Resume_UsesResumeFlag(t *testing.T) {
	mock := &mockExecutor{
		responses: [][]byte{[]byte(`{"result":"ok","is_error":false}`)},
	}
	c := newTestClient(mock, "sess-abc", "model-x")
	_, err := c.Resume("continue", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	args := mock.calls[0]
	assertFlagValue(t, args, "--resume", "sess-abc")
	for _, a := range args {
		if a == "--session-id" {
			t.Error("Resume must not use --session-id")
		}
	}
}

func TestClient_Run_IncludesSchema(t *testing.T) {
	mock := &mockExecutor{
		responses: [][]byte{[]byte(`{"result":"ok","is_error":false}`)},
	}
	c := newTestClient(mock, "sid", "model")
	schema := `{"type":"object"}`
	_, err := c.Run("prompt", schema)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertFlagValue(t, mock.calls[0], "--json-schema", schema)
}

func TestClient_Run_NoSchemaSkipsFlag(t *testing.T) {
	mock := &mockExecutor{
		responses: [][]byte{[]byte(`{"result":"ok","is_error":false}`)},
	}
	c := newTestClient(mock, "sid", "model")
	_, err := c.Run("prompt", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range mock.calls[0] {
		if a == "--json-schema" {
			t.Error("should not include --json-schema when schema is empty")
		}
	}
}

func TestClient_RunWithRetry_RetriesAfterRateLimit(t *testing.T) {
	rateLimitResp := []byte(`{"result":"You've hit your limit. resets 3:00 PM","is_error":true}`)
	successResp := []byte(`{"result":"done","is_error":false}`)
	mock := &mockExecutor{
		responses: [][]byte{rateLimitResp, successResp},
	}
	c := newTestClient(mock, "sid", "model")
	resp, err := c.RunWithRetry("prompt", "", noopPrompter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Error("expected success after retry")
	}
	if len(mock.calls) != 2 {
		t.Errorf("expected 2 calls, got %d", len(mock.calls))
	}
	// Second call must use --resume
	assertFlagValue(t, mock.calls[1], "--resume", "sid")
}

func TestClient_RunWithRetry_AbortsOnUserN(t *testing.T) {
	errorResp := []byte(`{"result":"some other error","is_error":true}`)
	mock := &mockExecutor{
		responses: [][]byte{errorResp},
	}
	c := newTestClient(mock, "sid", "model")
	prompter := func(q string) (string, error) { return "n", nil }
	_, err := c.RunWithRetry("prompt", "", prompter)
	if err == nil {
		t.Error("expected error when user aborts")
	}
}

// helpers

func newTestClient(exec Executor, sessionID, model string) *Client {
	return &Client{
		executor:  exec,
		sessionID: sessionID,
		model:     model,
		sleep:     noopSleep,
	}
}

func assertContains(t *testing.T, args []string, target string) {
	t.Helper()
	for _, a := range args {
		if a == target {
			return
		}
	}
	t.Errorf("args %v do not contain %q", args, target)
}

func assertFlagValue(t *testing.T, args []string, flag, value string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return
		}
	}
	t.Errorf("args %v do not contain sequence %q %q", args, flag, value)
}
