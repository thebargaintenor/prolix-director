package cmd

import (
	"testing"

	"github.com/thebargaintenor/prolix-director/internal/config"
)

func TestParseFlags_RequiresGitProvider(t *testing.T) {
	_, err := parseFlags([]string{"5"}, &config.Config{})
	if err == nil {
		t.Error("expected error when --git-provider not set")
	}
}

func TestParseFlags_RequiresIssueNum(t *testing.T) {
	_, err := parseFlags([]string{"--git-provider", "github"}, &config.Config{})
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
	}, &config.Config{})
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
	cfg, err := parseFlags([]string{"--git-provider", "github", "5"}, &config.Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MainBranch != "trunk" {
		t.Errorf("expected default main branch 'trunk', got %q", cfg.MainBranch)
	}
}

func TestParseFlags_RejectsUnsupportedProvider(t *testing.T) {
	_, err := parseFlags([]string{"--git-provider", "bitbucket", "5"}, &config.Config{})
	if err == nil {
		t.Error("expected error for unsupported provider")
	}
}

func TestParseFlags_UsesGitProviderFromConfig(t *testing.T) {
	cfg, err := parseFlags([]string{"5"}, &config.Config{GitProvider: "gitlab"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GitProvider != "gitlab" {
		t.Errorf("expected git provider from config, got %q", cfg.GitProvider)
	}
}

func TestParseFlags_RejectsUnsupportedProviderInConfig(t *testing.T) {
	_, err := parseFlags([]string{"5"}, &config.Config{GitProvider: "bitbucket"})
	if err == nil {
		t.Error("expected error for unsupported provider in config")
	}
}
