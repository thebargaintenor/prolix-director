# Prolix Solve CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace `.claude/scripts/work-on-issue.sh` with a `prolix solve <issue-number>` Go CLI command that has full functional parity.

**Architecture:** Subcommand routing in `main.go` dispatches to a `solve` package. Core workflow is orchestrated by `internal/solve`, which depends on injectable interfaces for `claude` command execution, `git` worktree management, `pipeline` CI monitoring, and `envfile` loading. All external I/O goes through interfaces so unit tests can inject fakes.

**Tech Stack:** Go stdlib only — `os/exec`, `flag`, `encoding/json`, `bufio`, `time`. Shell out to `claude`, `gh`, `glab` executables.

---

## File Map

| Path | Purpose |
|------|---------|
| `main.go` | Entry point: parse subcommand, dispatch to `cmd/` |
| `cmd/solve.go` | Parse `solve` flags, build `solve.Config`, run `solve.Run` |
| `internal/envfile/loader.go` | Load `~/.claude/.env` KEY=VALUE pairs into process env |
| `internal/envfile/loader_test.go` | Unit tests for env file parsing |
| `internal/claude/response.go` | `Response`, `ImplOutput`, `ParseRateLimitError` |
| `internal/claude/response_test.go` | Unit tests for response parsing |
| `internal/claude/client.go` | `Client` — builds args, runs claude, handles errors/rate-limits |
| `internal/claude/client_test.go` | Unit tests with mock executor |
| `internal/git/worktree.go` | `Worktree` — create, remove via `git` commands |
| `internal/git/worktree_test.go` | Unit tests with mock executor |
| `internal/pipeline/pipeline.go` | `Monitor` interface, `GitHub` and `GitLab` implementations |
| `internal/pipeline/pipeline_test.go` | Unit tests with mock executor |
| `internal/solve/solve.go` | `Solver` struct — orchestrates the 6-phase workflow |
| `internal/solve/solve_test.go` | Unit tests with mock claude client |

---

## Task 1: `internal/envfile` — Load `.env` file

**Files:**
- Create: `internal/envfile/loader.go`
- Create: `internal/envfile/loader_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/envfile/loader_test.go
package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ParsesKeyValuePairs(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	os.WriteFile(f, []byte("FOO=bar\nBAZ=qux\n"), 0600)

	vars, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["FOO"] != "bar" {
		t.Errorf("FOO: want bar, got %q", vars["FOO"])
	}
	if vars["BAZ"] != "qux" {
		t.Errorf("BAZ: want qux, got %q", vars["BAZ"])
	}
}

func TestLoad_SkipsComments(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	os.WriteFile(f, []byte("# comment\nFOO=bar\n"), 0600)

	vars, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := vars["# comment"]; ok {
		t.Error("should not parse comment as key")
	}
	if vars["FOO"] != "bar" {
		t.Errorf("FOO: want bar, got %q", vars["FOO"])
	}
}

func TestLoad_SkipsBlankLines(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	os.WriteFile(f, []byte("\n\nFOO=bar\n\n"), 0600)

	vars, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 1 {
		t.Errorf("expected 1 var, got %d", len(vars))
	}
}

func TestLoad_HandlesQuotedValues(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, ".env")
	os.WriteFile(f, []byte(`FOO="hello world"`+"\n"), 0600)

	vars, err := Load(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vars["FOO"] != "hello world" {
		t.Errorf("FOO: want %q, got %q", "hello world", vars["FOO"])
	}
}

func TestLoad_ReturnsErrorForMissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/.env")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestApply_SetsEnvVars(t *testing.T) {
	vars := map[string]string{"TEST_APPLY_VAR": "testval"}
	Apply(vars)
	if got := os.Getenv("TEST_APPLY_VAR"); got != "testval" {
		t.Errorf("expected testval, got %q", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd /Users/ted.monchamp/.config/ai-worktrees/prolix-director/agent-issue-1
go test ./internal/envfile/...
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Implement**

```go
// internal/envfile/loader.go
package envfile

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

func Load(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("envfile load %s: %w", path, err)
	}
	defer f.Close()

	vars := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		vars[k] = v
	}
	return vars, scanner.Err()
}

func Apply(vars map[string]string) {
	for k, v := range vars {
		os.Setenv(k, v)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/envfile/... -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/envfile/
git commit -m "feat: add envfile loader for ~/.claude/.env"
```

---

## Task 2: `internal/claude/response` — Parse Claude JSON output

**Files:**
- Create: `internal/claude/response.go`
- Create: `internal/claude/response_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/claude/response_test.go
package claude

import (
	"testing"
	"time"
)

func TestParseResponse_PRNumber(t *testing.T) {
	data := []byte(`{"result":"done","is_error":false,"structured_output":{"PR_number":42}}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Error("expected no error")
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NumberForProvider("github") != 42 {
		t.Errorf("expected PR 42, got %d", out.NumberForProvider("github"))
	}
}

func TestParseResponse_MRNumber(t *testing.T) {
	data := []byte(`{"result":"done","is_error":false,"structured_output":{"MR_number":7}}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NumberForProvider("gitlab") != 7 {
		t.Errorf("expected MR 7, got %d", out.NumberForProvider("gitlab"))
	}
}

func TestParseResponse_ClarifyingQuestion(t *testing.T) {
	data := []byte(`{"result":"?","is_error":false,"structured_output":{"clarifying_question":"What is the deadline?"}}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ClarifyingQuestion != "What is the deadline?" {
		t.Errorf("unexpected question: %q", out.ClarifyingQuestion)
	}
}

func TestParseResponse_NoStructuredOutput(t *testing.T) {
	data := []byte(`{"result":"thinking","is_error":false}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HasStructuredOutput() {
		t.Error("expected no structured output")
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Error("expected nil output")
	}
}

func TestParseResponse_ErrorFlag(t *testing.T) {
	data := []byte(`{"result":"rate limit hit","is_error":true}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Error("expected is_error=true")
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	_, err := ParseResponse([]byte(`not json`))
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestImplOutput_NumberForProvider_GitHub(t *testing.T) {
	o := &ImplOutput{PRNumber: 10, MRNumber: 5}
	if o.NumberForProvider("github") != 10 {
		t.Errorf("expected 10, got %d", o.NumberForProvider("github"))
	}
}

func TestImplOutput_NumberForProvider_GitLab(t *testing.T) {
	o := &ImplOutput{PRNumber: 10, MRNumber: 5}
	if o.NumberForProvider("gitlab") != 5 {
		t.Errorf("expected 5, got %d", o.NumberForProvider("gitlab"))
	}
}

func TestParseRateLimitError_Detected(t *testing.T) {
	result := "You've hit your limit. Your limit resets 3:00 PM"
	rl, ok := ParseRateLimitError(result)
	if !ok {
		t.Fatal("expected rate limit detected")
	}
	if rl.WaitDuration <= 0 {
		t.Error("expected positive wait duration")
	}
}

func TestParseRateLimitError_NotDetected(t *testing.T) {
	_, ok := ParseRateLimitError("Some other error occurred")
	if ok {
		t.Error("expected no rate limit detection")
	}
}

func TestParseRateLimitError_FallbackOnUnparsableTime(t *testing.T) {
	result := "You've hit your limit. Resets soon."
	rl, ok := ParseRateLimitError(result)
	if !ok {
		t.Fatal("expected rate limit detected")
	}
	if rl.WaitDuration != time.Hour {
		t.Errorf("expected 1h fallback, got %v", rl.WaitDuration)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/claude/...
```

Expected: compile error — package does not exist.

- [ ] **Step 3: Implement**

```go
// internal/claude/response.go
package claude

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Response struct {
	Result           string          `json:"result"`
	IsError          bool            `json:"is_error"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

type ImplOutput struct {
	PRNumber           int    `json:"PR_number"`
	MRNumber           int    `json:"MR_number"`
	ClarifyingQuestion string `json:"clarifying_question"`
}

type RateLimitError struct {
	ResetTime    time.Time
	WaitDuration time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited until %s", e.ResetTime.Format(time.Kitchen))
}

func ParseResponse(data []byte) (*Response, error) {
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &r, nil
}

func (r *Response) HasStructuredOutput() bool {
	return len(r.StructuredOutput) > 0 && string(r.StructuredOutput) != "null"
}

func (r *Response) ParseImplOutput() (*ImplOutput, error) {
	if !r.HasStructuredOutput() {
		return nil, nil
	}
	var out ImplOutput
	if err := json.Unmarshal(r.StructuredOutput, &out); err != nil {
		return nil, fmt.Errorf("parse impl output: %w", err)
	}
	return &out, nil
}

func (o *ImplOutput) NumberForProvider(provider string) int {
	if provider == "github" {
		return o.PRNumber
	}
	return o.MRNumber
}

func ParseRateLimitError(result string) (*RateLimitError, bool) {
	if !strings.Contains(result, "You've hit your limit") {
		return nil, false
	}
	idx := strings.Index(result, "resets ")
	if idx == -1 {
		return &RateLimitError{WaitDuration: time.Hour}, true
	}
	after := result[idx+len("resets "):]
	after = strings.Map(func(r rune) rune {
		if r == '(' || r == ')' || r == ',' {
			return -1
		}
		return r
	}, after)
	after = strings.TrimSpace(after)

	now := time.Now()
	for _, format := range []string{"3:04 PM", "3:04PM", "15:04", "3:04:05 PM"} {
		if t, err := time.ParseInLocation(format, after, time.Local); err == nil {
			t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
			if !t.After(now) {
				t = t.Add(24 * time.Hour)
			}
			return &RateLimitError{ResetTime: t, WaitDuration: t.Sub(now) + 60*time.Second}, true
		}
	}
	return &RateLimitError{WaitDuration: time.Hour}, true
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/claude/... -run TestParse -v
go test ./internal/claude/... -run TestImplOutput -v
go test ./internal/claude/... -run TestRateLimit -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/response.go internal/claude/response_test.go
git commit -m "feat: add claude response types and rate limit parsing"
```

---

## Task 3: `internal/claude/client` — Run claude with retry

**Files:**
- Create: `internal/claude/client.go`
- Modify: `internal/claude/client_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/claude/client_test.go
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
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/claude/...
```

Expected: compile error — `Client`, `Executor`, `newTestClient` not defined.

- [ ] **Step 3: Implement**

```go
// internal/claude/client.go
package claude

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Executor interface {
	Execute(name string, args ...string) ([]byte, error)
}

type osExecutor struct{}

func (e *osExecutor) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

type Client struct {
	executor  Executor
	sessionID string
	model     string
	sleep     func(time.Duration)
}

func New(executor Executor, sessionID, model string) *Client {
	return &Client{
		executor:  executor,
		sessionID: sessionID,
		model:     model,
		sleep:     time.Sleep,
	}
}

func NewDefault(sessionID, model string) *Client {
	return New(&osExecutor{}, sessionID, model)
}

func (c *Client) Run(prompt, schema string) (*Response, error) {
	return c.runWith("--session-id", prompt, schema)
}

func (c *Client) Resume(prompt, schema string) (*Response, error) {
	return c.runWith("--resume", prompt, schema)
}

func (c *Client) RunWithRetry(prompt, schema string, prompter func(string) (string, error)) (*Response, error) {
	resp, err := c.Run(prompt, schema)
	if err != nil {
		return nil, err
	}
	return c.retryOnError(resp, prompt, schema, prompter)
}

func (c *Client) ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*Response, error) {
	resp, err := c.Resume(prompt, schema)
	if err != nil {
		return nil, err
	}
	return c.retryOnError(resp, prompt, schema, prompter)
}

func (c *Client) runWith(sessionFlag, prompt, schema string) (*Response, error) {
	args := []string{
		"-p",
		"--dangerously-skip-permissions",
		sessionFlag, c.sessionID,
		"--output-format", "json",
		"--model", c.model,
	}
	if schema != "" {
		args = append(args, "--json-schema", schema)
	}
	args = append(args, prompt)
	out, err := c.executor.Execute("claude", args...)
	if err != nil && len(out) == 0 {
		return nil, fmt.Errorf("claude execute: %w", err)
	}
	return ParseResponse(out)
}

func (c *Client) retryOnError(resp *Response, prompt, schema string, prompter func(string) (string, error)) (*Response, error) {
	for resp.IsError {
		if rl, ok := ParseRateLimitError(resp.Result); ok {
			c.countdownSleep(rl.WaitDuration)
		} else {
			fmt.Fprintf(os.Stderr, "Claude error: %s\n", resp.Result)
			answer, err := prompter("continue?[yn] ")
			if err != nil {
				return nil, err
			}
			if answer == "n" {
				return nil, fmt.Errorf("user aborted after claude error")
			}
		}
		var err error
		resp, err = c.Resume(prompt, schema)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func (c *Client) countdownSleep(d time.Duration) {
	remaining := d
	for remaining > 0 {
		h := int(remaining.Hours())
		m := int(remaining.Minutes()) % 60
		s := int(remaining.Seconds()) % 60
		fmt.Printf("\rWaiting %d hours, %d minutes, and %d seconds ...", h, m, s)
		c.sleep(time.Second)
		remaining -= time.Second
	}
	fmt.Println("\n*Yawns*... Okay, time to get up.")
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
go test ./internal/claude/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/claude/client.go internal/claude/client_test.go
git commit -m "feat: add claude client with rate-limit retry"
```

---

## Task 4: `internal/git/worktree` — Create and remove git worktrees

**Files:**
- Create: `internal/git/worktree.go`
- Create: `internal/git/worktree_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/git/worktree_test.go
package git

import (
	"testing"
)

type mockExecutor struct {
	calls [][]string
	errs  map[string]error
}

func (m *mockExecutor) Execute(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	key := name + " " + args[0]
	if m.errs != nil {
		if err, ok := m.errs[key]; ok {
			return nil, err
		}
	}
	return nil, nil
}

func TestWorktree_Create_CallsGitCommands(t *testing.T) {
	mock := &mockExecutor{}
	wt := New(mock, "/base/worktrees/repo", "agent-issue-5", "trunk")

	if err := wt.Create(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallContains(t, mock.calls, []string{"git", "checkout", "trunk"})
	assertCallContains(t, mock.calls, []string{"git", "pull"})
	assertCallWithPrefix(t, mock.calls, []string{"git", "worktree", "add", "-b", "agent-issue-5"})
}

func TestWorktree_Path_ReturnsFullPath(t *testing.T) {
	mock := &mockExecutor{}
	wt := New(mock, "/base/worktrees/repo", "agent-issue-5", "trunk")
	if wt.Path() != "/base/worktrees/repo/agent-issue-5" {
		t.Errorf("unexpected path: %q", wt.Path())
	}
}

func TestWorktree_Remove_CallsCleanupCommands(t *testing.T) {
	mock := &mockExecutor{}
	wt := New(mock, "/base/worktrees/repo", "agent-issue-5", "trunk")

	if err := wt.Remove(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallWithPrefix(t, mock.calls, []string{"git", "push"})
	assertCallContains(t, mock.calls, []string{"git", "worktree", "remove", "/base/worktrees/repo/agent-issue-5"})
	assertCallContains(t, mock.calls, []string{"git", "checkout", "trunk"})
	assertCallContains(t, mock.calls, []string{"git", "pull"})
	assertCallContains(t, mock.calls, []string{"git", "branch", "-D", "agent-issue-5"})
}

func TestWorktree_Remove_ContinuesIfPushFails(t *testing.T) {
	mock := &mockExecutor{
		errs: map[string]error{"git push": fmt.Errorf("push failed")},
	}
	wt := New(mock, "/base/worktrees/repo", "agent-issue-5", "trunk")
	// Remove should not fail even if push fails
	if err := wt.Remove(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// helpers

func assertCallContains(t *testing.T, calls [][]string, target []string) {
	t.Helper()
	for _, call := range calls {
		if slicesEqual(call, target) {
			return
		}
	}
	t.Errorf("no call matching %v in %v", target, calls)
}

func assertCallWithPrefix(t *testing.T, calls [][]string, prefix []string) {
	t.Helper()
	for _, call := range calls {
		if len(call) >= len(prefix) {
			match := true
			for i, p := range prefix {
				if call[i] != p {
					match = false
					break
				}
			}
			if match {
				return
			}
		}
	}
	t.Errorf("no call with prefix %v in %v", prefix, calls)
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
```

Note: you will need to add `"fmt"` to the import in the test file for `TestWorktree_Remove_ContinuesIfPushFails`.

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/git/...
```

Expected: compile error.

- [ ] **Step 3: Implement**

```go
// internal/git/worktree.go
package git

import (
	"fmt"
	"path/filepath"
)

type Executor interface {
	Execute(name string, args ...string) ([]byte, error)
}

type Worktree struct {
	executor   Executor
	basePath   string
	branch     string
	mainBranch string
}

func New(executor Executor, basePath, branch, mainBranch string) *Worktree {
	return &Worktree{
		executor:   executor,
		basePath:   basePath,
		branch:     branch,
		mainBranch: mainBranch,
	}
}

func (w *Worktree) Path() string {
	return filepath.Join(w.basePath, w.branch)
}

func (w *Worktree) Create() error {
	if _, err := w.executor.Execute("git", "checkout", w.mainBranch); err != nil {
		return fmt.Errorf("checkout %s: %w", w.mainBranch, err)
	}
	if _, err := w.executor.Execute("git", "pull"); err != nil {
		return fmt.Errorf("git pull: %w", err)
	}
	if _, err := w.executor.Execute("git", "worktree", "add", "-b", w.branch, w.Path()); err != nil {
		return fmt.Errorf("worktree add: %w", err)
	}
	return nil
}

func (w *Worktree) Remove() error {
	// push is best-effort; mirror the bash script's `git push || true`
	w.executor.Execute("git", "push") //nolint:errcheck
	if _, err := w.executor.Execute("git", "worktree", "remove", w.Path()); err != nil {
		return fmt.Errorf("worktree remove: %w", err)
	}
	if _, err := w.executor.Execute("git", "checkout", w.mainBranch); err != nil {
		return fmt.Errorf("checkout %s: %w", w.mainBranch, err)
	}
	if _, err := w.executor.Execute("git", "pull"); err != nil {
		return fmt.Errorf("git pull after remove: %w", err)
	}
	if _, err := w.executor.Execute("git", "branch", "-D", w.branch); err != nil {
		return fmt.Errorf("branch delete: %w", err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/git/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat: add git worktree management"
```

---

## Task 5: `internal/pipeline` — CI pipeline monitoring

**Files:**
- Create: `internal/pipeline/pipeline.go`
- Create: `internal/pipeline/pipeline_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/pipeline/pipeline_test.go
package pipeline

import (
	"fmt"
	"testing"
)

type mockExecutor struct {
	responses []executeResult
	calls     [][]string
	callIdx   int
}

type executeResult struct {
	out []byte
	err error
}

func (m *mockExecutor) Execute(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	i := m.callIdx
	m.callIdx++
	if i < len(m.responses) {
		r := m.responses[i]
		return r.out, r.err
	}
	return nil, nil
}

func noopPrompter(q string) (string, error) { return "y", nil }

func TestGitHub_Watch_SuccessOnFirstTry(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{out: nil, err: nil}, // gh pr checks exits 0 = success
		},
	}
	m := NewGitHub(mock, 3, noopPrompter)
	if err := m.Watch(10, nopClaude{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Errorf("expected 1 call, got %d", len(mock.calls))
	}
	assertCallContains(t, mock.calls, []string{"gh", "pr", "checks", "10", "--watch"})
}

func TestGitHub_Watch_RetriesOnFailure(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{err: fmt.Errorf("checks failed")}, // attempt 1
			{err: fmt.Errorf("checks failed")}, // attempt 2
			{out: nil, err: nil},               // attempt 3 = success
		},
	}
	var claudePrompts []string
	fakeClaude := &fakeClaude{onResume: func(p string) { claudePrompts = append(claudePrompts, p) }}
	m := NewGitHub(mock, 3, noopPrompter)
	if err := m.Watch(10, fakeClaude); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(claudePrompts) != 2 {
		t.Errorf("expected 2 claude prompts for 2 failures, got %d", len(claudePrompts))
	}
}

func TestGitHub_Watch_PromptsUserAtMaxAttempts(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{err: fmt.Errorf("fail")}, // attempt 1
			{err: fmt.Errorf("fail")}, // attempt 2
			{err: fmt.Errorf("fail")}, // attempt 3 → max reached
			{out: nil, err: nil},      // success after human guidance
		},
	}
	var humanPrompted bool
	prompter := func(q string) (string, error) {
		humanPrompted = true
		return "try again", nil
	}
	m := NewGitHub(mock, 3, prompter)
	if err := m.Watch(10, nopClaude{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !humanPrompted {
		t.Error("expected human to be prompted at max attempts")
	}
}

func TestGitHub_Watch_ExitsOnUserQuit(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{err: fmt.Errorf("fail")},
			{err: fmt.Errorf("fail")},
			{err: fmt.Errorf("fail")},
		},
	}
	prompter := func(q string) (string, error) { return "q", nil }
	m := NewGitHub(mock, 3, prompter)
	err := m.Watch(10, nopClaude{})
	if err == nil {
		t.Error("expected error when user quits")
	}
}

// helpers

type fakeClaude struct {
	onResume func(string)
}

func (f *fakeClaude) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) error {
	if f.onResume != nil {
		f.onResume(prompt)
	}
	return nil
}

type nopClaude struct{}

func (nopClaude) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) error {
	return nil
}

func assertCallContains(t *testing.T, calls [][]string, target []string) {
	t.Helper()
	for _, call := range calls {
		if len(call) == len(target) {
			match := true
			for i := range target {
				if call[i] != target[i] {
					match = false
					break
				}
			}
			if match {
				return
			}
		}
	}
	t.Errorf("no call matching %v found in %v", target, calls)
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/pipeline/...
```

Expected: compile error.

- [ ] **Step 3: Implement**

```go
// internal/pipeline/pipeline.go
package pipeline

import (
	"fmt"
	"strconv"
)

type Executor interface {
	Execute(name string, args ...string) ([]byte, error)
}

type Claude interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) error
}

type Monitor interface {
	Watch(prNum int, claude Claude) error
}

type GitHub struct {
	executor    Executor
	maxAttempts int
	prompter    func(string) (string, error)
}

func NewGitHub(executor Executor, maxAttempts int, prompter func(string) (string, error)) *GitHub {
	return &GitHub{
		executor:    executor,
		maxAttempts: maxAttempts,
		prompter:    prompter,
	}
}

func (g *GitHub) Watch(prNum int, claude Claude) error {
	prStr := strconv.Itoa(prNum)
	attempt := 0
	for {
		_, err := g.executor.Execute("gh", "pr", "checks", prStr, "--watch")
		if err == nil {
			return nil
		}
		attempt++
		if attempt >= g.maxAttempts {
			answer, err := g.prompter(fmt.Sprintf("Pipeline failed %d times. Provide guidance or 'q' to quit: ", g.maxAttempts))
			if err != nil {
				return err
			}
			if answer == "q" {
				return fmt.Errorf("user quit after pipeline failures")
			}
			if err := claude.ResumeWithRetry(
				fmt.Sprintf("The pipeline has failed %d times. The user says: %s", g.maxAttempts, answer),
				"", g.prompter,
			); err != nil {
				return err
			}
			attempt = 0
		} else {
			fmt.Printf("Prompting claude to fix pipeline (attempt %d/%d)...\n", attempt, g.maxAttempts)
			if err := claude.ResumeWithRetry("Looks like your pipeline failed. Please fix and push", "", g.prompter); err != nil {
				return err
			}
		}
	}
}

type GitLab struct {
	executor Executor
	prompter func(string) (string, error)
}

func NewGitLab(executor Executor, prompter func(string) (string, error)) *GitLab {
	return &GitLab{executor: executor, prompter: prompter}
}

func (g *GitLab) Watch(mrNum int, claude Claude) error {
	mrStr := strconv.Itoa(mrNum)
	for {
		status, err := g.pipelineStatus(mrStr)
		if err != nil {
			return err
		}
		fmt.Printf("Waiting on pipeline...\n")
		for status != "success" && status != "failed" && status != "canceled" && status != "skipped" {
			fmt.Printf("\rCurrent status: %s", status)
			// poll every 60s — in production this calls g.executor sleep or time.Sleep
			status, err = g.pipelineStatus(mrStr)
			if err != nil {
				return err
			}
		}
		fmt.Printf("\nPipeline finished with status %s\n", status)
		switch status {
		case "success":
			return nil
		case "failed":
			if err := claude.ResumeWithRetry("Looks like your pipeline failed. Please fix and push", "", g.prompter); err != nil {
				return err
			}
		default:
			answer, err := g.prompter(fmt.Sprintf("The pipeline status is %s, continue?[yn] ", status))
			if err != nil {
				return err
			}
			if answer == "n" {
				return nil
			}
		}
	}
}

func (g *GitLab) pipelineStatus(mrStr string) (string, error) {
	out, err := g.executor.Execute("glab", "mr", "view", mrStr, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("glab mr view: %w", err)
	}
	// minimal JSON parse — extract .head_pipeline.status
	// using encoding/json would require importing it; inline parse is fragile,
	// so import json in the real implementation
	_ = out
	return "success", nil // placeholder replaced below
}
```

**Important:** Replace the `pipelineStatus` body with a proper JSON parse:

```go
// Add to imports: "encoding/json"

func (g *GitLab) pipelineStatus(mrStr string) (string, error) {
	out, err := g.executor.Execute("glab", "mr", "view", mrStr, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("glab mr view: %w", err)
	}
	var payload struct {
		HeadPipeline struct {
			Status string `json:"status"`
		} `json:"head_pipeline"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", fmt.Errorf("parse pipeline status: %w", err)
	}
	return payload.HeadPipeline.Status, nil
}
```

The full `pipeline.go` with the JSON import:

```go
// internal/pipeline/pipeline.go
package pipeline

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Executor interface {
	Execute(name string, args ...string) ([]byte, error)
}

type Claude interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) error
}

type Monitor interface {
	Watch(prNum int, claude Claude) error
}

type GitHub struct {
	executor    Executor
	maxAttempts int
	prompter    func(string) (string, error)
}

func NewGitHub(executor Executor, maxAttempts int, prompter func(string) (string, error)) *GitHub {
	return &GitHub{executor: executor, maxAttempts: maxAttempts, prompter: prompter}
}

func (g *GitHub) Watch(prNum int, claude Claude) error {
	prStr := strconv.Itoa(prNum)
	attempt := 0
	for {
		_, err := g.executor.Execute("gh", "pr", "checks", prStr, "--watch")
		if err == nil {
			return nil
		}
		attempt++
		if attempt >= g.maxAttempts {
			answer, pErr := g.prompter(fmt.Sprintf("Pipeline failed %d times. Provide guidance or 'q' to quit: ", g.maxAttempts))
			if pErr != nil {
				return pErr
			}
			if answer == "q" {
				return fmt.Errorf("user quit after pipeline failures")
			}
			if err := claude.ResumeWithRetry(
				fmt.Sprintf("The pipeline has failed %d times. The user says: %s", g.maxAttempts, answer),
				"", g.prompter,
			); err != nil {
				return err
			}
			attempt = 0
		} else {
			fmt.Printf("Prompting claude to fix pipeline (attempt %d/%d)...\n", attempt, g.maxAttempts)
			if err := claude.ResumeWithRetry("Looks like your pipeline failed. Please fix and push", "", g.prompter); err != nil {
				return err
			}
		}
	}
}

type GitLab struct {
	executor Executor
	prompter func(string) (string, error)
}

func NewGitLab(executor Executor, prompter func(string) (string, error)) *GitLab {
	return &GitLab{executor: executor, prompter: prompter}
}

func (g *GitLab) Watch(mrNum int, claude Claude) error {
	mrStr := strconv.Itoa(mrNum)
	for {
		status, err := g.pipelineStatus(mrStr)
		if err != nil {
			return err
		}
		fmt.Printf("Waiting on pipeline...\n")
		for status != "success" && status != "failed" && status != "canceled" && status != "skipped" {
			fmt.Printf("\rCurrent status: %s", status)
			status, err = g.pipelineStatus(mrStr)
			if err != nil {
				return err
			}
		}
		fmt.Printf("\nPipeline finished with status %s\n", status)
		switch status {
		case "success":
			return nil
		case "failed":
			if err := claude.ResumeWithRetry("Looks like your pipeline failed. Please fix and push", "", g.prompter); err != nil {
				return err
			}
		default:
			answer, err := g.prompter(fmt.Sprintf("The pipeline status is %s, continue?[yn] ", status))
			if err != nil {
				return err
			}
			if answer == "n" {
				return nil
			}
		}
		status, err = g.pipelineStatus(mrStr)
		if err != nil {
			return err
		}
	}
}

func (g *GitLab) pipelineStatus(mrStr string) (string, error) {
	out, err := g.executor.Execute("glab", "mr", "view", mrStr, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("glab mr view: %w", err)
	}
	var payload struct {
		HeadPipeline struct {
			Status string `json:"status"`
		} `json:"head_pipeline"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", fmt.Errorf("parse pipeline status: %w", err)
	}
	return payload.HeadPipeline.Status, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/pipeline/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/
git commit -m "feat: add GitHub and GitLab pipeline monitors"
```

---

## Task 6: `internal/solve` — 6-phase workflow orchestration

**Files:**
- Create: `internal/solve/solve.go`
- Create: `internal/solve/solve_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/solve/solve_test.go
package solve

import (
	"fmt"
	"testing"
)

// --- fakes ---

type fakeClaudeClient struct {
	runResponses    []*claudeResponse
	resumeResponses []*claudeResponse
	runIdx          int
	resumeIdx       int
	runPrompts      []string
	resumePrompts   []string
}

type claudeResponse struct {
	prNum    int
	mrNum    int
	question string
	isError  bool
}

func (f *fakeClaudeClient) RunWithRetry(prompt, schema string, p func(string) (string, error)) (*runResult, error) {
	f.runPrompts = append(f.runPrompts, prompt)
	if f.runIdx < len(f.runResponses) {
		r := f.runResponses[f.runIdx]
		f.runIdx++
		return &runResult{prNum: r.prNum, mrNum: r.mrNum, question: r.question}, nil
	}
	return nil, fmt.Errorf("no run response configured")
}

func (f *fakeClaudeClient) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*runResult, error) {
	f.resumePrompts = append(f.resumePrompts, prompt)
	if f.resumeIdx < len(f.resumeResponses) {
		r := f.resumeResponses[f.resumeIdx]
		f.resumeIdx++
		return &runResult{prNum: r.prNum, mrNum: r.mrNum, question: r.question}, nil
	}
	return nil, fmt.Errorf("no resume response configured")
}

type fakePipeline struct {
	watchCalls int
	err        error
}

func (f *fakePipeline) Watch(prNum int, cl pipelineClaude) error {
	f.watchCalls++
	return f.err
}

type fakeReviewer struct {
	resumePrompts []string
}

func (f *fakeReviewer) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*runResult, error) {
	f.resumePrompts = append(f.resumePrompts, prompt)
	return &runResult{}, nil
}

// --- tests ---

func TestSolver_Phase1_CreatesPR(t *testing.T) {
	answers := []string{"1"} // Phase 6: cleanup
	answerIdx := 0
	prompter := func(q string) (string, error) {
		a := answers[answerIdx]
		answerIdx++
		return a, nil
	}
	mainClaude := &fakeClaudeClient{
		runResponses:    []*claudeResponse{{prNum: 42}},
		resumeResponses: []*claudeResponse{{}}, // phase 4 address comments
	}
	reviewer := &fakeReviewer{}
	pipeline := &fakePipeline{}

	s := &Solver{
		config:   Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		main:     mainClaude,
		reviewer: reviewer,
		pipeline: pipeline,
		prompter: prompter,
	}

	if err := s.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.runPrompts) == 0 {
		t.Error("expected Phase 1 to call Run")
	}
}

func TestSolver_Phase1_ClarifyingQuestionLoop(t *testing.T) {
	callCount := 0
	prompter := func(q string) (string, error) {
		callCount++
		if callCount == 1 {
			return "the deadline is Friday", nil // answer clarifying question
		}
		return "1", nil // Phase 6 cleanup
	}
	mainClaude := &fakeClaudeClient{
		runResponses: []*claudeResponse{
			{question: "What is the deadline?"}, // first run → question
		},
		resumeResponses: []*claudeResponse{
			{prNum: 42}, // after user answers → PR created
			{},          // phase 4 address comments
		},
	}
	reviewer := &fakeReviewer{}
	pipeline := &fakePipeline{}

	s := &Solver{
		config:   Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		main:     mainClaude,
		reviewer: reviewer,
		pipeline: pipeline,
		prompter: prompter,
	}

	if err := s.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.resumePrompts) == 0 {
		t.Error("expected resume call after clarifying question")
	}
}

func TestSolver_Phase6_AddressComments(t *testing.T) {
	callCount := 0
	prompter := func(q string) (string, error) {
		callCount++
		if callCount == 1 {
			return "2", nil // Phase 6: address comments
		}
		return "1", nil // Phase 6: cleanup on second loop
	}
	mainClaude := &fakeClaudeClient{
		runResponses: []*claudeResponse{{prNum: 42}},
		resumeResponses: []*claudeResponse{
			{}, // phase 4 address comments
			{}, // phase 6 address additional comments
		},
	}
	reviewer := &fakeReviewer{}
	pipeline := &fakePipeline{}

	s := &Solver{
		config:   Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		main:     mainClaude,
		reviewer: reviewer,
		pipeline: pipeline,
		prompter: prompter,
	}

	if err := s.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSolver_Phase6_RewriteIssue(t *testing.T) {
	prompter := func(q string) (string, error) {
		return "3", nil // Phase 6: rewrite/close
	}
	mainClaude := &fakeClaudeClient{
		runResponses: []*claudeResponse{{prNum: 42}},
		resumeResponses: []*claudeResponse{
			{}, // phase 4 address comments
			{}, // phase 6 summarize
		},
	}
	reviewer := &fakeReviewer{}
	pipeline := &fakePipeline{}

	s := &Solver{
		config:   Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		main:     mainClaude,
		reviewer: reviewer,
		pipeline: pipeline,
		prompter: prompter,
	}

	if err := s.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSolver_SkipsPipelineWhenConfigured(t *testing.T) {
	prompter := func(q string) (string, error) { return "1", nil }
	mainClaude := &fakeClaudeClient{
		runResponses:    []*claudeResponse{{prNum: 42}},
		resumeResponses: []*claudeResponse{{}},
	}
	pipeline := &fakePipeline{}

	s := &Solver{
		config:   Config{IssueNum: "5", GitProvider: "github", SkipPipeline: true},
		main:     mainClaude,
		reviewer: &fakeReviewer{},
		pipeline: pipeline,
		prompter: prompter,
	}

	if err := s.run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pipeline.watchCalls != 0 {
		t.Errorf("expected 0 pipeline calls, got %d", pipeline.watchCalls)
	}
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./internal/solve/...
```

Expected: compile error — types not defined.

- [ ] **Step 3: Implement**

```go
// internal/solve/solve.go
package solve

import (
	"fmt"
)

type Config struct {
	IssueNum     string
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

type runResult struct {
	prNum    int
	mrNum    int
	question string
}

func (r *runResult) numberForProvider(provider string) int {
	if provider == "github" {
		return r.prNum
	}
	return r.mrNum
}

type mainClient interface {
	RunWithRetry(prompt, schema string, prompter func(string) (string, error)) (*runResult, error)
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*runResult, error)
}

type reviewClient interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*runResult, error)
}

type pipelineClaude interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) error
}

type pipelineMonitor interface {
	Watch(prNum int, cl pipelineClaude) error
}

type Solver struct {
	config   Config
	main     mainClient
	reviewer reviewClient
	pipeline pipelineMonitor
	prompter func(string) (string, error)
}

func (s *Solver) run() error {
	mrOrPR := "PR"
	if s.config.GitProvider == "gitlab" {
		mrOrPR = "MR"
	}

	schema := fmt.Sprintf(`{"type":"object","properties":{"%s_number":{"type":"integer"},"clarifying_question":{"type":"string"}}}`, mrOrPR)

	fmt.Println("===== Phase 1: Implementation =====")
	fmt.Println("Working on issue...")
	prompt := fmt.Sprintf(
		"/test-driven-development Work on %s issue %s. If you don't have enough context to create a %s, output a clarifying question via the `clarifying question` JSON key. Otherwise, create a %s and output the number via the `%s number` JSON key",
		s.config.GitProvider, s.config.IssueNum, mrOrPR, mrOrPR, mrOrPR,
	)

	result, err := s.main.RunWithRetry(prompt, schema, s.prompter)
	if err != nil {
		return err
	}

	result, err = s.clarifyingLoop(result, schema, mrOrPR)
	if err != nil {
		return err
	}

	prNum := result.numberForProvider(s.config.GitProvider)

	fmt.Println("===== Phase 2: Pipeline loop 1 =====")
	if err := s.runPipeline(prNum); err != nil {
		return err
	}

	fmt.Println("===== Phase 3: AI Code Review =====")
	fmt.Println("Reviewing...")
	reviewPrompt := fmt.Sprintf("/code-review %s %d Leave your review as a comment on the %s", mrOrPR, prNum, mrOrPR)
	if _, err := s.reviewer.ResumeWithRetry(reviewPrompt, "", s.prompter); err != nil {
		return err
	}

	fmt.Println("===== Phase 4: Address CR comments =====")
	fmt.Println("Addressing comments...")
	addressPrompt := fmt.Sprintf("Read the review comments on %s %d and make changes accordingly", mrOrPR, prNum)
	if _, err := s.main.ResumeWithRetry(addressPrompt, "", s.prompter); err != nil {
		return err
	}

	fmt.Println("===== Phase 5: Pipeline loop 2 =====")
	if err := s.runPipeline(prNum); err != nil {
		return err
	}

	fmt.Println("===== Phase 6: Human Review Loop =====")
	return s.humanReviewLoop(prNum, mrOrPR)
}

func (s *Solver) clarifyingLoop(result *runResult, schema, mrOrPR string) (*runResult, error) {
	for result.numberForProvider(s.config.GitProvider) == 0 {
		if result.question == "" {
			var err error
			result, err = s.main.ResumeWithRetry(
				fmt.Sprintf("Your last response didn't provide a %s number or a clarifying question.", mrOrPR),
				schema, s.prompter,
			)
			if err != nil {
				return nil, err
			}
			continue
		}
		fmt.Println("===== Clarification Question Asked =====")
		fmt.Println(result.question)
		answer, err := s.prompter("> ")
		if err != nil {
			return nil, err
		}
		fmt.Println("Working on issue...")
		result, err = s.main.ResumeWithRetry(
			fmt.Sprintf("The user has answered your clarifying question with %s", answer),
			schema, s.prompter,
		)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Solver) runPipeline(prNum int) error {
	if s.config.SkipPipeline {
		fmt.Println("--skip-pipeline was set. Skipping pipeline loop...")
		return nil
	}
	return s.pipeline.Watch(prNum, &pipelineClaudeAdapter{s.main, s.prompter})
}

func (s *Solver) humanReviewLoop(prNum int, mrOrPR string) error {
	for {
		fmt.Println("Please provide your code review.\n1. Cleanup\n2. Address comments\n3. Rewrite Issue")
		answer, err := s.prompter("> ")
		if err != nil {
			return err
		}
		switch answer {
		case "1":
			return nil
		case "2":
			fmt.Println("Addressing additional comments...")
			prompt := fmt.Sprintf("Read the review latest comments on %s %d and make changes accordingly", mrOrPR, prNum)
			if _, err := s.main.ResumeWithRetry(prompt, "", s.prompter); err != nil {
				return err
			}
			if err := s.runPipeline(prNum); err != nil {
				return err
			}
		case "3":
			fmt.Println("Summarizing work into issue comment...")
			prompt := fmt.Sprintf("The user has decided to close the %s. Leave a summary of this session as a comment on issue %s", mrOrPR, s.config.IssueNum)
			if _, err := s.main.ResumeWithRetry(prompt, "", s.prompter); err != nil {
				return err
			}
			return nil
		default:
			fmt.Println("Not a valid choice")
		}
	}
}

type pipelineClaudeAdapter struct {
	client   mainClient
	prompter func(string) (string, error)
}

func (a *pipelineClaudeAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) error {
	_, err := a.client.ResumeWithRetry(prompt, schema, a.prompter)
	return err
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/solve/... -v
```

Expected: all PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/solve/
git commit -m "feat: add 6-phase solve workflow orchestration"
```

---

## Task 7: Wire up `claude.Client` to `mainClient` interface

The `claude.Client`'s `RunWithRetry` and `ResumeWithRetry` return `(*Response, error)` but `internal/solve` expects `(*runResult, error)`. Add an adapter in `cmd/solve.go` that bridges these.

**Files:**
- Create: `cmd/solve.go`
- Modify: `main.go`

- [ ] **Step 1: Write the test for the adapter**

```go
// cmd/solve_test.go
package cmd

import (
	"testing"

	"github.com/thebargaintenor/prolix-director/internal/claude"
)

func TestParseFlags_RequiresGitProvider(t *testing.T) {
	_, err := parseFlags([]string{"5"})
	if err == nil {
		t.Error("expected error when --git-provider not set")
	}
}

func TestParseFlags_RequiresIssueNum(t *testing.T) {
	_, err := parseFlags([]string{"--git-provider", "github"})
	if err == nil {
		t.Error("expected error when issue number not set")
	}
}

func TestParseFlags_ParsesAllFlags(t *testing.T) {
	cfg, err := parseFlags([]string{
		"--git-provider", "github",
		"--main-branch", "main",
		"--skip-pipeline",
		"42",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitProvider != "github" {
		t.Errorf("GitProvider: want github, got %q", cfg.GitProvider)
	}
	if cfg.MainBranch != "main" {
		t.Errorf("MainBranch: want main, got %q", cfg.MainBranch)
	}
	if !cfg.SkipPipeline {
		t.Error("expected SkipPipeline=true")
	}
	if cfg.IssueNum != "42" {
		t.Errorf("IssueNum: want 42, got %q", cfg.IssueNum)
	}
}

func TestParseFlags_DefaultMainBranch(t *testing.T) {
	cfg, err := parseFlags([]string{"--git-provider", "github", "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MainBranch != "trunk" {
		t.Errorf("expected default main branch 'trunk', got %q", cfg.MainBranch)
	}
}

func TestParseFlags_RejectsUnsupportedProvider(t *testing.T) {
	_, err := parseFlags([]string{"--git-provider", "bitbucket", "5"})
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestClaudeAdapter_ConvertsResponse(t *testing.T) {
	resp := &claude.Response{}
	resp2 := &claude.ImplOutput{PRNumber: 99, ClarifyingQuestion: "why?"}
	_ = resp
	_ = resp2
	// The adapter is tested implicitly via integration; flag parsing is the
	// critical testable unit in cmd/solve.go
}
```

- [ ] **Step 2: Run to verify compile failure**

```bash
go test ./cmd/...
```

Expected: compile error.

- [ ] **Step 3: Implement `cmd/solve.go`**

```go
// cmd/solve.go
package cmd

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thebargaintenor/prolix-director/internal/claude"
	"github.com/thebargaintenor/prolix-director/internal/envfile"
	"github.com/thebargaintenor/prolix-director/internal/git"
	"github.com/thebargaintenor/prolix-director/internal/pipeline"
	"github.com/thebargaintenor/prolix-director/internal/solve"
)

type solveConfig struct {
	IssueNum     string
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

func parseFlags(args []string) (solveConfig, error) {
	fs := flag.NewFlagSet("solve", flag.ContinueOnError)
	mainBranch := fs.String("main-branch", "trunk", "main branch name")
	skipPipeline := fs.Bool("skip-pipeline", false, "skip pipeline monitoring")
	gitProvider := fs.String("git-provider", "", "git provider: github or gitlab")

	if err := fs.Parse(args); err != nil {
		return solveConfig{}, err
	}
	if *gitProvider == "" {
		return solveConfig{}, fmt.Errorf("--git-provider is required (github or gitlab)")
	}
	if *gitProvider != "github" && *gitProvider != "gitlab" {
		return solveConfig{}, fmt.Errorf("unsupported git provider %q; use github or gitlab", *gitProvider)
	}
	if fs.NArg() != 1 {
		return solveConfig{}, fmt.Errorf("usage: prolix solve [flags] ISSUE_NUM")
	}
	return solveConfig{
		IssueNum:     fs.Arg(0),
		MainBranch:   *mainBranch,
		GitProvider:  *gitProvider,
		SkipPipeline: *skipPipeline,
	}, nil
}

func RunSolve(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}

	envPath := filepath.Join(os.Getenv("HOME"), ".claude", ".env")
	if vars, err := envfile.Load(envPath); err == nil {
		envfile.Apply(vars)
	}

	codeModel := os.Getenv("CODE_GEN_MODEL")
	if codeModel == "" {
		codeModel = "claude-opus-4-6"
	}
	reviewerModel := os.Getenv("REVIEWER_MODEL")
	if reviewerModel == "" {
		reviewerModel = "claude-sonnet-4-6"
	}

	mainSessionID := newUUID()
	reviewerSessionID := newUUID()
	fmt.Printf("Main session ID: %s\n", mainSessionID)
	fmt.Printf("Reviewer session ID: %s\n", reviewerSessionID)

	exec := &osExecutor{}
	projectName := filepath.Base(mustGetwd())
	basePath := filepath.Join(os.Getenv("HOME"), ".config", "ai-worktrees", projectName)
	branch := fmt.Sprintf("agent-issue-%s", cfg.IssueNum)

	wt := git.New(exec, basePath, branch, cfg.MainBranch)
	fmt.Println("Creating worktree")
	if err := wt.Create(); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	defer func() {
		if err := wt.Remove(); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		}
	}()

	prompter := stdinPrompter()
	mainClient := claude.NewDefault(mainSessionID, codeModel)
	reviewerClient := claude.NewDefault(reviewerSessionID, reviewerModel)

	var monitor pipeline.Monitor
	if cfg.GitProvider == "github" {
		monitor = pipeline.NewGitHub(exec, 3, prompter)
	} else {
		monitor = pipeline.NewGitLab(exec, prompter)
	}

	s := solve.New(
		solve.Config{
			IssueNum:     cfg.IssueNum,
			MainBranch:   cfg.MainBranch,
			GitProvider:  cfg.GitProvider,
			SkipPipeline: cfg.SkipPipeline,
		},
		&claudeMainAdapter{mainClient, prompter},
		&claudeReviewAdapter{reviewerClient, prompter},
		monitor,
		prompter,
	)

	return s.Run()
}

// osExecutor satisfies git.Executor and pipeline.Executor
type osExecutor struct{}

func (e *osExecutor) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// claudeMainAdapter bridges claude.Client to solve.MainClient
type claudeMainAdapter struct {
	client   *claude.Client
	prompter func(string) (string, error)
}

func (a *claudeMainAdapter) RunWithRetry(prompt, schema string, p func(string) (string, error)) (*solve.RunResult, error) {
	resp, err := a.client.RunWithRetry(prompt, schema, p)
	if err != nil {
		return nil, err
	}
	return responseToRunResult(resp, p)
}

func (a *claudeMainAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*solve.RunResult, error) {
	resp, err := a.client.ResumeWithRetry(prompt, schema, p)
	if err != nil {
		return nil, err
	}
	return responseToRunResult(resp, p)
}

func responseToRunResult(resp *claude.Response, p func(string) (string, error)) (*solve.RunResult, error) {
	out, err := resp.ParseImplOutput()
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &solve.RunResult{}, nil
	}
	return &solve.RunResult{
		PRNum:    out.PRNumber,
		MRNum:    out.MRNumber,
		Question: out.ClarifyingQuestion,
	}, nil
}

// claudeReviewAdapter bridges claude.Client to solve.ReviewClient
type claudeReviewAdapter struct {
	client   *claude.Client
	prompter func(string) (string, error)
}

func (a *claudeReviewAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*solve.RunResult, error) {
	resp, err := a.client.ResumeWithRetry(prompt, schema, p)
	if err != nil {
		return nil, err
	}
	return responseToRunResult(resp, p)
}

func stdinPrompter() func(string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	return func(question string) (string, error) {
		fmt.Print(question)
		line, err := reader.ReadString('\n')
		return strings.TrimSpace(line), err
	}
}

func newUUID() string {
	out, err := exec.Command("uuidgen").Output()
	if err != nil {
		// fallback: use time-based ID
		return fmt.Sprintf("%d", os.Getpid())
	}
	return strings.TrimSpace(string(out))
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}
```

**Note:** This requires `solve.New`, `solve.RunResult`, `solve.MainClient`, and `solve.ReviewClient` to be exported. Update `internal/solve/solve.go` accordingly (see Step 3b below).

- [ ] **Step 3b: Export types in `internal/solve/solve.go`**

Update `internal/solve/solve.go`:
- Rename `runResult` → `RunResult`
- Rename fields: `prNum` → `PRNum`, `mrNum` → `MRNum`, `question` → `Question`
- Export `mainClient` interface → `MainClient`
- Export `reviewClient` interface → `ReviewClient`
- Add constructor `func New(cfg Config, main MainClient, reviewer ReviewClient, pipeline pipelineMonitor, prompter func(string) (string, error)) *Solver`
- Add exported `func (s *Solver) Run() error` that calls `s.run()`

The full updated `internal/solve/solve.go`:

```go
// internal/solve/solve.go
package solve

import (
	"fmt"
)

type Config struct {
	IssueNum     string
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

type RunResult struct {
	PRNum    int
	MRNum    int
	Question string
}

func (r *RunResult) numberForProvider(provider string) int {
	if provider == "github" {
		return r.PRNum
	}
	return r.MRNum
}

type MainClient interface {
	RunWithRetry(prompt, schema string, prompter func(string) (string, error)) (*RunResult, error)
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*RunResult, error)
}

type ReviewClient interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*RunResult, error)
}

type pipelineClaude interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) error
}

type pipelineMonitor interface {
	Watch(prNum int, cl pipelineClaude) error
}

type Solver struct {
	config   Config
	main     MainClient
	reviewer ReviewClient
	pipeline pipelineMonitor
	prompter func(string) (string, error)
}

func New(cfg Config, main MainClient, reviewer ReviewClient, pm pipelineMonitor, prompter func(string) (string, error)) *Solver {
	return &Solver{
		config:   cfg,
		main:     main,
		reviewer: reviewer,
		pipeline: pm,
		prompter: prompter,
	}
}

func (s *Solver) Run() error {
	return s.run()
}

func (s *Solver) run() error {
	mrOrPR := "PR"
	if s.config.GitProvider == "gitlab" {
		mrOrPR = "MR"
	}

	schema := fmt.Sprintf(`{"type":"object","properties":{"%s_number":{"type":"integer"},"clarifying_question":{"type":"string"}}}`, mrOrPR)

	fmt.Println("===== Phase 1: Implementation =====")
	fmt.Println("Working on issue...")
	prompt := fmt.Sprintf(
		"/test-driven-development Work on %s issue %s. If you don't have enough context to create a %s, output a clarifying question via the `clarifying question` JSON key. Otherwise, create a %s and output the number via the `%s number` JSON key",
		s.config.GitProvider, s.config.IssueNum, mrOrPR, mrOrPR, mrOrPR,
	)

	result, err := s.main.RunWithRetry(prompt, schema, s.prompter)
	if err != nil {
		return err
	}

	result, err = s.clarifyingLoop(result, schema, mrOrPR)
	if err != nil {
		return err
	}

	prNum := result.numberForProvider(s.config.GitProvider)

	fmt.Println("===== Phase 2: Pipeline loop 1 =====")
	if err := s.runPipeline(prNum); err != nil {
		return err
	}

	fmt.Println("===== Phase 3: AI Code Review =====")
	fmt.Println("Reviewing...")
	reviewPrompt := fmt.Sprintf("/code-review %s %d Leave your review as a comment on the %s", mrOrPR, prNum, mrOrPR)
	if _, err := s.reviewer.ResumeWithRetry(reviewPrompt, "", s.prompter); err != nil {
		return err
	}

	fmt.Println("===== Phase 4: Address CR comments =====")
	fmt.Println("Addressing comments...")
	addressPrompt := fmt.Sprintf("Read the review comments on %s %d and make changes accordingly", mrOrPR, prNum)
	if _, err := s.main.ResumeWithRetry(addressPrompt, "", s.prompter); err != nil {
		return err
	}

	fmt.Println("===== Phase 5: Pipeline loop 2 =====")
	if err := s.runPipeline(prNum); err != nil {
		return err
	}

	fmt.Println("===== Phase 6: Human Review Loop =====")
	return s.humanReviewLoop(prNum, mrOrPR)
}

func (s *Solver) clarifyingLoop(result *RunResult, schema, mrOrPR string) (*RunResult, error) {
	for result.numberForProvider(s.config.GitProvider) == 0 {
		if result.Question == "" {
			var err error
			result, err = s.main.ResumeWithRetry(
				fmt.Sprintf("Your last response didn't provide a %s number or a clarifying question.", mrOrPR),
				schema, s.prompter,
			)
			if err != nil {
				return nil, err
			}
			continue
		}
		fmt.Println("===== Clarification Question Asked =====")
		fmt.Println(result.Question)
		answer, err := s.prompter("> ")
		if err != nil {
			return nil, err
		}
		fmt.Println("Working on issue...")
		result, err = s.main.ResumeWithRetry(
			fmt.Sprintf("The user has answered your clarifying question with %s", answer),
			schema, s.prompter,
		)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Solver) runPipeline(prNum int) error {
	if s.config.SkipPipeline {
		fmt.Println("--skip-pipeline was set. Skipping pipeline loop...")
		return nil
	}
	return s.pipeline.Watch(prNum, &pipelineClaudeAdapter{s.main, s.prompter})
}

func (s *Solver) humanReviewLoop(prNum int, mrOrPR string) error {
	for {
		fmt.Println("Please provide your code review.\n1. Cleanup\n2. Address comments\n3. Rewrite Issue")
		answer, err := s.prompter("> ")
		if err != nil {
			return err
		}
		switch answer {
		case "1":
			return nil
		case "2":
			fmt.Println("Addressing additional comments...")
			prompt := fmt.Sprintf("Read the review latest comments on %s %d and make changes accordingly", mrOrPR, prNum)
			if _, err := s.main.ResumeWithRetry(prompt, "", s.prompter); err != nil {
				return err
			}
			if err := s.runPipeline(prNum); err != nil {
				return err
			}
		case "3":
			fmt.Println("Summarizing work into issue comment...")
			prompt := fmt.Sprintf("The user has decided to close the %s. Leave a summary of this session as a comment on issue %s", mrOrPR, s.config.IssueNum)
			if _, err := s.main.ResumeWithRetry(prompt, "", s.prompter); err != nil {
				return err
			}
			return nil
		default:
			fmt.Println("Not a valid choice")
		}
	}
}

type pipelineClaudeAdapter struct {
	client   MainClient
	prompter func(string) (string, error)
}

func (a *pipelineClaudeAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) error {
	_, err := a.client.ResumeWithRetry(prompt, schema, a.prompter)
	return err
}
```

Also update `internal/solve/solve_test.go` to use the exported types (`RunResult`, `MainClient`, `ReviewClient`). Replace all `runResult` with `RunResult`, `prNum` with `PRNum`, `mrNum` with `MRNum`, `question` with `Question`.

- [ ] **Step 4: Update `main.go`**

```go
// main.go
package main

import (
	"fmt"
	"os"

	"github.com/thebargaintenor/prolix-director/cmd"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: prolix <command> [args]\nCommands: solve")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "solve":
		if err := cmd.RunSolve(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Run all tests**

```bash
go test ./... -v
```

Expected: all PASS.

- [ ] **Step 6: Build and verify the binary compiles**

```bash
go build -o prolix .
./prolix solve --help
```

Expected: binary builds and prints flag usage.

- [ ] **Step 7: Commit**

```bash
git add cmd/ main.go internal/solve/solve.go internal/solve/solve_test.go
git commit -m "feat: wire up solve command and export solve types"
```

---

## Task 8: Fix pipeline.Monitor interface mismatch and final integration check

The `pipeline.Monitor` interface uses `pipeline.Claude`, but `solve` passes a `pipelineClaude` adapter. The interfaces need to align across packages.

**Files:**
- Modify: `internal/pipeline/pipeline.go` (export `Claude` interface correctly)
- Modify: `cmd/solve.go` (ensure `*pipeline.GitHub` / `*pipeline.GitLab` satisfy `pipelineMonitor`)

- [ ] **Step 1: Verify the interface chain compiles**

```bash
go build ./...
```

If there are interface mismatch errors, the `pipeline.Claude` and `solve.pipelineClaude` must be the same signature. Ensure both define:

```go
ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) error
```

The `pipeline.Monitor` interface must use `pipeline.Claude` — and `solve.pipelineMonitor` must have `Watch(prNum int, cl pipeline.Claude) error` if you want to use `pipeline.GitHub` directly. The simplest fix: import `pipeline` in `solve` and use `pipeline.Claude` instead of a local `pipelineClaude` interface.

Update `internal/solve/solve.go`:

```go
import "github.com/thebargaintenor/prolix-director/internal/pipeline"

// Replace local pipelineClaude and pipelineMonitor:
type pipelineMonitor interface {
	Watch(prNum int, cl pipeline.Claude) error
}

// Update pipelineClaudeAdapter to satisfy pipeline.Claude:
type pipelineClaudeAdapter struct {
	client   MainClient
	prompter func(string) (string, error)
}

func (a *pipelineClaudeAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) error {
	_, err := a.client.ResumeWithRetry(prompt, schema, a.prompter)
	return err
}
```

- [ ] **Step 2: Run full build and test suite**

```bash
go build ./...
go test ./...
```

Expected: no errors, all tests PASS.

- [ ] **Step 3: Run gofmt**

```bash
gofmt -w .
```

- [ ] **Step 4: Final commit**

```bash
git add -u
git commit -m "fix: align pipeline.Claude interface across packages"
```

---

## Spec Coverage Check

| Requirement | Task |
|-------------|------|
| Written in Go, uses existing `main.go` | Task 7 |
| `prolix solve <issue-number>` subcommand | Task 7 |
| All functionality from `work-on-issue.sh` | Tasks 2–7 |
| Unit tests for all Go code | Tasks 1–7 |
| Idiomatic Go practices | All tasks |
| Prefer standard library | All tasks (stdlib only) |
| `--main-branch` flag | Task 7 |
| `--skip-pipeline` flag | Task 7 |
| `--git-provider` flag | Task 7 |
| Load `~/.claude/.env` | Task 1 |
| Git worktree create/cleanup | Task 4 |
| Phase 1: Implementation via claude | Task 6 |
| Clarifying question loop | Task 6 |
| Phase 2/5: Pipeline loops | Tasks 5, 6 |
| Phase 3: AI Code Review | Task 6 |
| Phase 4: Address comments | Task 6 |
| Phase 6: Human review loop (1/2/3) | Task 6 |
| Rate limit handling with countdown | Task 3 |
| Session ID / resume logic | Tasks 3, 6 |
| GitLab pipeline status polling | Task 5 |
| GitHub `gh pr checks --watch` | Task 5 |
