package config

import (
	"testing"

	"github.com/thebargaintenor/prolix-director/internal/envfile"
)

func TestLoad_IncludesAllEnvValues(t *testing.T) {
	testEnv := map[string]string{
		"CODE_GEN_MODEL": "test-code-gen-model",
		"REVIEWER_MODEL": "test-reviewer-model",
		"GIT_PROVIDER":   "test-git-provider",
	}

	envfile.Apply(testEnv)

	cfg := Load()
	if cfg.CodeGenModel != "test-code-gen-model" {
		t.Errorf("CodeGenModel: want test-code-gen-model, got %q", cfg.CodeGenModel)
	}
	if cfg.ReviewerModel != "test-reviewer-model" {
		t.Errorf("ReviewerModel: want test-reviewer-model, got %q", cfg.ReviewerModel)
	}
	if cfg.GitProvider != "test-git-provider" {
		t.Errorf("GitProvider: want test-git-provider, got %q", cfg.GitProvider)
	}
}

func TestLoad_UsesDefaults(t *testing.T) {
	testEnv := map[string]string{
		"CODE_GEN_MODEL": "",
		"REVIEWER_MODEL": "",
		"GIT_PROVIDER":   "",
	}

	envfile.Apply(testEnv)

	cfg := Load()
	if cfg.CodeGenModel != "claude-sonnet-4-6" {
		t.Errorf("CodeGenModel: want claude-sonnet-4-6, got %q", cfg.CodeGenModel)
	}
	if cfg.ReviewerModel != "claude-opus-4-6" {
		t.Errorf("ReviewerModel: want claude-opus-4-6, got %q", cfg.ReviewerModel)
	}
	if cfg.GitProvider != "" {
		t.Errorf("GitProvider: want empty string, got %q", cfg.GitProvider)
	}
}
