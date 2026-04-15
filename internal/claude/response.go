package claude

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Response struct {
	Result           string          `json:"result"`
	IsError          bool            `json:"is_error"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

type ImplOutput struct {
	PRNumber           int    `json:"PR_number"`
	MRNumber           int    `json:"MR_number"`
	ClarifyingQuestion string `json:"clarifying_question"`
}

type RateLimitError struct {
	ResetTime    time.Time
	WaitDuration time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited until %s", e.ResetTime.Format(time.Kitchen))
}

func ParseResponse(data []byte) (*Response, error) {
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &r, nil
}

func (r *Response) HasStructuredOutput() bool {
	return len(r.StructuredOutput) > 0 && string(r.StructuredOutput) != "null"
}

func (r *Response) ParseImplOutput() (*ImplOutput, error) {
	if !r.HasStructuredOutput() {
		return nil, nil
	}
	var out ImplOutput
	if err := json.Unmarshal(r.StructuredOutput, &out); err != nil {
		return nil, fmt.Errorf("parse impl output: %w", err)
	}
	return &out, nil
}

func (o *ImplOutput) NumberForProvider(provider string) int {
	if provider == "github" {
		return o.PRNumber
	}
	return o.MRNumber
}

func ParseRateLimitError(result string) (*RateLimitError, bool) {
	if !strings.Contains(result, "You've hit your limit") {
		return nil, false
	}
	lower := strings.ToLower(result)
	idx := strings.Index(lower, "resets ")
	if idx == -1 {
		return &RateLimitError{WaitDuration: time.Hour}, true
	}
	after := result[idx+len("resets "):]
	after = strings.Map(func(r rune) rune {
		if r == '(' || r == ')' || r == ',' {
			return -1
		}
		return r
	}, after)
	after = strings.TrimSpace(after)

	now := time.Now()
	for _, format := range []string{"3:04 PM", "3:04PM", "15:04", "3:04:05 PM"} {
		if t, err := time.ParseInLocation(format, after, time.Local); err == nil {
			t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, time.Local)
			if !t.After(now) {
				t = t.Add(24 * time.Hour)
			}
			return &RateLimitError{ResetTime: t, WaitDuration: t.Sub(now) + 60*time.Second}, true
		}
	}
	return &RateLimitError{WaitDuration: time.Hour}, true
}
