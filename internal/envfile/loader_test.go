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
