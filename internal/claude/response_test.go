package claude

import (
	"testing"
	"time"
)

func TestParseResponse_PRNumber(t *testing.T) {
	data := []byte(`{"result":"done","is_error":false,"structured_output":{"PR_number":42}}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.IsError {
		t.Error("expected no error")
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NumberForProvider("github") != 42 {
		t.Errorf("expected PR 42, got %d", out.NumberForProvider("github"))
	}
}

func TestParseResponse_MRNumber(t *testing.T) {
	data := []byte(`{"result":"done","is_error":false,"structured_output":{"MR_number":7}}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.NumberForProvider("gitlab") != 7 {
		t.Errorf("expected MR 7, got %d", out.NumberForProvider("gitlab"))
	}
}

func TestParseResponse_ClarifyingQuestion(t *testing.T) {
	data := []byte(`{"result":"?","is_error":false,"structured_output":{"clarifying_question":"What is the deadline?"}}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ClarifyingQuestion != "What is the deadline?" {
		t.Errorf("unexpected question: %q", out.ClarifyingQuestion)
	}
}

func TestParseResponse_NoStructuredOutput(t *testing.T) {
	data := []byte(`{"result":"thinking","is_error":false}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HasStructuredOutput() {
		t.Error("expected no structured output")
	}
	out, err := resp.ParseImplOutput()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != nil {
		t.Error("expected nil output")
	}
}

func TestParseResponse_ErrorFlag(t *testing.T) {
	data := []byte(`{"result":"rate limit hit","is_error":true}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.IsError {
		t.Error("expected is_error=true")
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	_, err := ParseResponse([]byte(`not json`))
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestImplOutput_NumberForProvider_GitHub(t *testing.T) {
	o := &ImplOutput{PRNumber: 10, MRNumber: 5}
	if o.NumberForProvider("github") != 10 {
		t.Errorf("expected 10, got %d", o.NumberForProvider("github"))
	}
}

func TestImplOutput_NumberForProvider_GitLab(t *testing.T) {
	o := &ImplOutput{PRNumber: 10, MRNumber: 5}
	if o.NumberForProvider("gitlab") != 5 {
		t.Errorf("expected 5, got %d", o.NumberForProvider("gitlab"))
	}
}

func TestParseRateLimitError_Detected(t *testing.T) {
	result := "You've hit your limit. Your limit resets 3:00 PM"
	rl, ok := ParseRateLimitError(result)
	if !ok {
		t.Fatal("expected rate limit detected")
	}
	if rl.WaitDuration <= 0 {
		t.Error("expected positive wait duration")
	}
}

func TestParseRateLimitError_NotDetected(t *testing.T) {
	_, ok := ParseRateLimitError("Some other error occurred")
	if ok {
		t.Error("expected no rate limit detection")
	}
}

func TestParseRateLimitError_FallbackOnUnparsableTime(t *testing.T) {
	result := "You've hit your limit. Resets soon."
	rl, ok := ParseRateLimitError(result)
	if !ok {
		t.Fatal("expected rate limit detected")
	}
	if rl.WaitDuration != time.Hour {
		t.Errorf("expected 1h fallback, got %v", rl.WaitDuration)
	}
}

func TestParseResponse_NullStructuredOutput(t *testing.T) {
	data := []byte(`{"result":"ok","is_error":false,"structured_output":null}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.HasStructuredOutput() {
		t.Error("expected HasStructuredOutput=false for null")
	}
}

func TestParseResponse_MalformedStructuredOutput(t *testing.T) {
	data := []byte(`{"result":"ok","is_error":false,"structured_output":"not-an-object"}`)
	resp, err := ParseResponse(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.HasStructuredOutput() {
		t.Fatal("expected HasStructuredOutput=true")
	}
	_, err = resp.ParseImplOutput()
	if err == nil {
		t.Error("expected error for malformed structured_output")
	}
}
