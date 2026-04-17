package cmd

import (
	"testing"

	"github.com/thebargaintenor/prolix-director/internal/config"
)

func TestParseResumeFlags_RequiresBranch(t *testing.T) {
	_, err := parseResumeFlags([]string{"--git-provider", "github", "5"}, &config.Config{})
	if err == nil {
		t.Error("expected error when --branch not set")
	}
}

func TestParseResumeFlags_RequiresIssueNum(t *testing.T) {
	_, err := parseResumeFlags([]string{"--branch", "agent-issue-5", "--git-provider", "github"}, &config.Config{})
	if err == nil {
		t.Error("expected error when issue number not set")
	}
}

func TestParseResumeFlags_ParsesAllFlags(t *testing.T) {
	cfg, err := parseResumeFlags([]string{
		"--branch", "agent-issue-5",
		"--pr", "42",
		"--git-provider", "github",
		"5",
	}, &config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Branch != "agent-issue-5" {
		t.Errorf("Branch: want agent-issue-5, got %q", cfg.Branch)
	}
	if cfg.PRNum != 42 {
		t.Errorf("PRNum: want 42, got %d", cfg.PRNum)
	}
	if cfg.IssueNum != "5" {
		t.Errorf("IssueNum: want 5, got %q", cfg.IssueNum)
	}
	if cfg.GitProvider != "github" {
		t.Errorf("GitProvider: want github, got %q", cfg.GitProvider)
	}
}

func TestParseResumeFlags_PRIsOptional(t *testing.T) {
	cfg, err := parseResumeFlags([]string{
		"--branch", "agent-issue-5",
		"--git-provider", "github",
		"5",
	}, &config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.PRNum != 0 {
		t.Errorf("PRNum: want 0 when not set, got %d", cfg.PRNum)
	}
}

func TestParseResumeFlags_UsesGitProviderFromConfig(t *testing.T) {
	cfg, err := parseResumeFlags([]string{"--branch", "agent-issue-5", "5"}, &config.Config{GitProvider: "gitlab"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitProvider != "gitlab" {
		t.Errorf("expected git provider from config, got %q", cfg.GitProvider)
	}
}

func TestParseResumeFlags_RejectsUnsupportedProvider(t *testing.T) {
	_, err := parseResumeFlags([]string{"--branch", "agent-issue-5", "--git-provider", "bitbucket", "5"}, &config.Config{})
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestParseResumeFlags_DefaultMainBranch(t *testing.T) {
	cfg, err := parseResumeFlags([]string{"--branch", "agent-issue-5", "--git-provider", "github", "5"}, &config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MainBranch != "trunk" {
		t.Errorf("expected default main branch 'trunk', got %q", cfg.MainBranch)
	}
}
