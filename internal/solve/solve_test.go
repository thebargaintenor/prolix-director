package solve

import (
	"fmt"
	"strings"
	"testing"

	"github.com/thebargaintenor/prolix-director/internal/pipeline"
)

// --- fakes ---

type fakeMainClient struct {
	runResponses    []*RunResult
	resumeResponses []*RunResult
	runIdx          int
	resumeIdx       int
	runPrompts      []string
	resumePrompts   []string
}

func (f *fakeMainClient) RunWithRetry(prompt, schema string, p func(string) (string, error)) (*RunResult, error) {
	f.runPrompts = append(f.runPrompts, prompt)
	if f.runIdx < len(f.runResponses) {
		r := f.runResponses[f.runIdx]
		f.runIdx++
		return r, nil
	}
	return nil, fmt.Errorf("no run response configured")
}

func (f *fakeMainClient) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*RunResult, error) {
	f.resumePrompts = append(f.resumePrompts, prompt)
	if f.resumeIdx < len(f.resumeResponses) {
		r := f.resumeResponses[f.resumeIdx]
		f.resumeIdx++
		return r, nil
	}
	return nil, fmt.Errorf("no resume response configured")
}

func (f *fakeMainClient) ResumeWithRetryErr(prompt, schema string, p func(string) (string, error)) error {
	_, err := f.ResumeWithRetry(prompt, schema, p)
	return err
}

type fakePipeline struct {
	watchCalls int
	err        error
}

func (f *fakePipeline) Watch(prNum int, cl pipeline.Claude) error {
	f.watchCalls++
	return f.err
}

type fakeReviewer struct {
	resumePrompts []string
}

func (f *fakeReviewer) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*RunResult, error) {
	f.resumePrompts = append(f.resumePrompts, prompt)
	return &RunResult{}, nil
}

// --- tests ---

func TestSolver_Phase1_CreatesPR(t *testing.T) {
	prompter := func(q string) (string, error) { return "1", nil } // Phase 6: cleanup
	mainClaude := &fakeMainClient{
		runResponses:    []*RunResult{{PRNum: 42}},
		resumeResponses: []*RunResult{{}}, // phase 4 address comments
	}
	reviewer := &fakeReviewer{}
	pl := &fakePipeline{}

	s := New(
		Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		mainClaude, reviewer, pl, prompter,
	)

	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.runPrompts) == 0 {
		t.Error("expected Phase 1 to call Run")
	}
}

func TestSolver_Phase1_ClarifyingQuestionLoop(t *testing.T) {
	callCount := 0
	prompter := func(q string) (string, error) {
		callCount++
		if callCount == 1 {
			return "the deadline is Friday", nil
		}
		return "1", nil
	}
	mainClaude := &fakeMainClient{
		runResponses: []*RunResult{
			{Question: "What is the deadline?"},
		},
		resumeResponses: []*RunResult{
			{PRNum: 42},
			{},
		},
	}
	reviewer := &fakeReviewer{}
	pl := &fakePipeline{}

	s := New(
		Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		mainClaude, reviewer, pl, prompter,
	)

	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.resumePrompts) == 0 {
		t.Error("expected resume call after clarifying question")
	}
}

func TestSolver_Phase6_AddressComments(t *testing.T) {
	callCount := 0
	prompter := func(q string) (string, error) {
		callCount++
		if callCount == 1 {
			return "2", nil
		}
		return "1", nil
	}
	mainClaude := &fakeMainClient{
		runResponses: []*RunResult{{PRNum: 42}},
		resumeResponses: []*RunResult{
			{},
			{},
		},
	}
	reviewer := &fakeReviewer{}
	pl := &fakePipeline{}

	s := New(
		Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		mainClaude, reviewer, pl, prompter,
	)

	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSolver_Phase6_RewriteIssue(t *testing.T) {
	prompter := func(q string) (string, error) { return "3", nil }
	mainClaude := &fakeMainClient{
		runResponses: []*RunResult{{PRNum: 42}},
		resumeResponses: []*RunResult{
			{},
			{},
		},
	}
	reviewer := &fakeReviewer{}
	pl := &fakePipeline{}

	s := New(
		Config{IssueNum: "5", GitProvider: "github", MainBranch: "trunk"},
		mainClaude, reviewer, pl, prompter,
	)

	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSolver_SkipsPipelineWhenConfigured(t *testing.T) {
	prompter := func(q string) (string, error) { return "1", nil }
	mainClaude := &fakeMainClient{
		runResponses:    []*RunResult{{PRNum: 42}},
		resumeResponses: []*RunResult{{}},
	}
	pl := &fakePipeline{}

	s := New(
		Config{IssueNum: "5", GitProvider: "github", SkipPipeline: true},
		mainClaude, &fakeReviewer{}, pl, prompter,
	)

	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pl.watchCalls != 0 {
		t.Errorf("expected 0 pipeline calls, got %d", pl.watchCalls)
	}
}

func TestSolver_Resume_WithPRNum_SkipsToPhase6(t *testing.T) {
	prompter := func(q string) (string, error) { return "1", nil } // Phase 6: cleanup
	mainClaude := &fakeMainClient{}
	s := New(
		Config{IssueNum: "5", GitProvider: "github", SkipPipeline: true},
		mainClaude, &fakeReviewer{}, &fakePipeline{}, prompter,
	)

	if err := s.Resume(42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.runPrompts) != 0 {
		t.Errorf("expected no RunWithRetry calls when starting at Phase 6, got %v", mainClaude.runPrompts)
	}
}

func TestSolver_Resume_WithoutPRNum_BehavesIdenticallyToRun(t *testing.T) {
	prompter := func(q string) (string, error) { return "1", nil }
	mainClaude := &fakeMainClient{
		runResponses:    []*RunResult{{PRNum: 42}},
		resumeResponses: []*RunResult{{}},
	}
	s := New(
		Config{IssueNum: "5", GitProvider: "github", SkipPipeline: true},
		mainClaude, &fakeReviewer{}, &fakePipeline{}, prompter,
	)

	if err := s.Resume(0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Resume(0) delegates to Run(), which uses RunWithRetry for Phase 1
	if len(mainClaude.runPrompts) == 0 {
		t.Error("expected Phase 1 RunWithRetry call when no PR provided (via Run delegation)")
	}
	// No duplicate code path — only one runPrompt should exist
	if len(mainClaude.runPrompts) > 1 {
		t.Errorf("expected exactly 1 run call, got %d — possible code duplication", len(mainClaude.runPrompts))
	}
}

func TestSolver_Resume_WithPRNum_CallsHumanReviewLoop(t *testing.T) {
	callCount := 0
	prompter := func(q string) (string, error) {
		callCount++
		if callCount == 1 {
			return "2", nil // address comments
		}
		return "1", nil // cleanup
	}
	mainClaude := &fakeMainClient{
		resumeResponses: []*RunResult{{}, {}},
	}
	s := New(
		Config{IssueNum: "5", GitProvider: "github", SkipPipeline: true},
		mainClaude, &fakeReviewer{}, &fakePipeline{}, prompter,
	)

	if err := s.Resume(42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.resumePrompts) == 0 {
		t.Error("expected resume calls from human review loop")
	}
}

func TestSolver_GitLabUsesMRLabel(t *testing.T) {
	prompter := func(q string) (string, error) { return "1", nil }
	mainClaude := &fakeMainClient{
		runResponses:    []*RunResult{{MRNum: 7}},
		resumeResponses: []*RunResult{{}},
	}
	pl := &fakePipeline{}

	s := New(
		Config{IssueNum: "5", GitProvider: "gitlab", SkipPipeline: true},
		mainClaude, &fakeReviewer{}, pl, prompter,
	)

	if err := s.Run(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mainClaude.runPrompts) == 0 {
		t.Fatal("expected a run prompt")
	}
	prompt := mainClaude.runPrompts[0]
	if !strings.Contains(prompt, "MR") {
		t.Errorf("expected MR label in prompt for gitlab provider, got: %q", prompt)
	}
}
