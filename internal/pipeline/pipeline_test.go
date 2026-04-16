package pipeline

import (
	"fmt"
	"testing"
	"time"
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

type mockStreamingExecutor struct {
	responses []error
	calls     [][]string
	callIdx   int
}

func (m *mockStreamingExecutor) ExecuteStreaming(name string, args ...string) error {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	i := m.callIdx
	m.callIdx++
	if i < len(m.responses) {
		return m.responses[i]
	}
	return nil
}

func noopPrompter(q string) (string, error) { return "y", nil }

func TestGitHub_Watch_SuccessOnFirstTry(t *testing.T) {
	mock := &mockStreamingExecutor{
		responses: []error{nil},
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
	mock := &mockStreamingExecutor{
		responses: []error{
			fmt.Errorf("checks failed"),
			fmt.Errorf("checks failed"),
			nil,
		},
	}
	var claudePrompts []string
	fakeCl := &fakeClaude{onResume: func(p string) { claudePrompts = append(claudePrompts, p) }}
	m := NewGitHub(mock, 3, noopPrompter)
	if err := m.Watch(10, fakeCl); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(claudePrompts) != 2 {
		t.Errorf("expected 2 claude prompts for 2 failures, got %d", len(claudePrompts))
	}
}

func TestGitHub_Watch_PromptsUserAtMaxAttempts(t *testing.T) {
	mock := &mockStreamingExecutor{
		responses: []error{
			fmt.Errorf("fail"),
			fmt.Errorf("fail"),
			fmt.Errorf("fail"),
			nil,
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
	mock := &mockStreamingExecutor{
		responses: []error{
			fmt.Errorf("fail"),
			fmt.Errorf("fail"),
			fmt.Errorf("fail"),
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

func TestGitLab_Watch_SuccessOnFirstPoll(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{out: []byte(`{"head_pipeline":{"status":"success"}}`), err: nil},
		},
	}
	m := NewGitLab(mock, noopPrompter)
	if err := m.Watch(5, nopClaude{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertCallContains(t, mock.calls, []string{"glab", "mr", "view", "5", "--output", "json"})
}

func TestGitLab_Watch_FailedCallsClaude(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{out: []byte(`{"head_pipeline":{"status":"failed"}}`), err: nil},
			{out: []byte(`{"head_pipeline":{"status":"success"}}`), err: nil},
		},
	}
	var claudePrompts []string
	fakeCl := &fakeClaude{onResume: func(p string) { claudePrompts = append(claudePrompts, p) }}
	m := NewGitLab(mock, noopPrompter)
	m.sleep = func(time.Duration) {}
	if err := m.Watch(5, fakeCl); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(claudePrompts) != 1 {
		t.Errorf("expected 1 claude prompt, got %d", len(claudePrompts))
	}
}

func TestGitLab_Watch_CanceledPromptsUser(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{out: []byte(`{"head_pipeline":{"status":"canceled"}}`), err: nil},
			{out: []byte(`{"head_pipeline":{"status":"success"}}`), err: nil},
		},
	}
	var prompted bool
	prompter := func(q string) (string, error) {
		prompted = true
		return "y", nil
	}
	m := NewGitLab(mock, prompter)
	m.sleep = func(time.Duration) {}
	if err := m.Watch(5, nopClaude{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !prompted {
		t.Error("expected user prompt for canceled pipeline")
	}
}

func TestGitLab_Watch_CanceledUserQuitReturns(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{out: []byte(`{"head_pipeline":{"status":"canceled"}}`), err: nil},
		},
	}
	m := NewGitLab(mock, func(q string) (string, error) { return "n", nil })
	m.sleep = func(time.Duration) {}
	if err := m.Watch(5, nopClaude{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGitLab_Watch_SkippedPromptsUser(t *testing.T) {
	mock := &mockExecutor{
		responses: []executeResult{
			{out: []byte(`{"head_pipeline":{"status":"skipped"}}`), err: nil},
			{out: []byte(`{"head_pipeline":{"status":"success"}}`), err: nil},
		},
	}
	var prompted bool
	prompter := func(q string) (string, error) {
		prompted = true
		return "y", nil
	}
	m := NewGitLab(mock, prompter)
	m.sleep = func(time.Duration) {}
	if err := m.Watch(5, nopClaude{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !prompted {
		t.Error("expected user prompt for skipped pipeline")
	}
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
