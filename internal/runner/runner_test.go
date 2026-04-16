package runner

import "testing"

func TestOS_ExecuteVisible_Succeeds(t *testing.T) {
	r := &OS{}
	if err := r.ExecuteVisible("echo", "hello"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOS_ExecuteVisible_ReturnsErrorOnFailure(t *testing.T) {
	r := &OS{}
	if err := r.ExecuteVisible("false"); err == nil {
		t.Error("expected error for failing command")
	}
}
