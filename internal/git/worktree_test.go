package git

import (
	"fmt"
	"testing"
)

type mockExecutor struct {
	calls [][]string
	errs  map[string]error
}

func (m *mockExecutor) Execute(name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	if m.errs != nil {
		key := name
		if len(args) > 0 {
			key = name + " " + args[0]
		}
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
	if err := wt.Remove(); err != nil {
		t.Fatalf("Remove should succeed even when push fails, got: %v", err)
	}
}

func TestWorktree_Attach_AddsExistingBranchAsWorktree(t *testing.T) {
	mock := &mockExecutor{}
	wt := New(mock, "/base/worktrees/repo", "agent-issue-5", "trunk")

	if err := wt.Attach(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertCallContains(t, mock.calls, []string{"git", "worktree", "add", "/base/worktrees/repo/agent-issue-5", "agent-issue-5"})
}

func TestWorktree_Attach_DoesNotCheckoutMainBranch(t *testing.T) {
	mock := &mockExecutor{}
	wt := New(mock, "/base/worktrees/repo", "agent-issue-5", "trunk")

	if err := wt.Attach(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, call := range mock.calls {
		if len(call) >= 3 && call[0] == "git" && call[1] == "checkout" && call[2] == "trunk" {
			t.Error("Attach should not checkout main branch")
		}
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
