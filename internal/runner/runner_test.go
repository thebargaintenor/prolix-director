package runner

import "testing"

func TestOS_ExecuteStreaming_Succeeds(t *testing.T) {
	r := &OS{}
	if err := r.ExecuteStreaming("echo", "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOS_ExecuteStreaming_ReturnsErrorOnFailure(t *testing.T) {
	r := &OS{}
	if err := r.ExecuteStreaming("false"); err == nil {
		t.Error("expected error for failing command")
	}
}
