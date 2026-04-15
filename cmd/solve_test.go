package cmd

import (
	"testing"
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
