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
	w.executor.Execute("git", "push") // best-effort, mirrors `git push || true`
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
