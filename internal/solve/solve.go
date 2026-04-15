package solve

import (
	"fmt"

	"github.com/thebargaintenor/prolix-director/internal/pipeline"
)

type Config struct {
	IssueNum     string
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

type RunResult struct {
	PRNum    int
	MRNum    int
	Question string
}

func (r *RunResult) numberForProvider(provider string) int {
	if provider == "github" {
		return r.PRNum
	}
	return r.MRNum
}

type MainClient interface {
	RunWithRetry(prompt, schema string, prompter func(string) (string, error)) (*RunResult, error)
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*RunResult, error)
}

type ReviewClient interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) (*RunResult, error)
}

type pipelineMonitor interface {
	Watch(prNum int, cl pipeline.Claude) error
}

type Solver struct {
	config   Config
	main     MainClient
	reviewer ReviewClient
	pipeline pipelineMonitor
	prompter func(string) (string, error)
}

func New(cfg Config, main MainClient, reviewer ReviewClient, pm pipelineMonitor, prompter func(string) (string, error)) *Solver {
	return &Solver{
		config:   cfg,
		main:     main,
		reviewer: reviewer,
		pipeline: pm,
		prompter: prompter,
	}
}

func (s *Solver) Run() error {
	mrOrPR := "PR"
	if s.config.GitProvider == "gitlab" {
		mrOrPR = "MR"
	}

	schema := fmt.Sprintf(`{"type":"object","properties":{"%s_number":{"type":"integer"},"clarifying_question":{"type":"string"}}}`, mrOrPR)

	fmt.Println("===== Phase 1: Implementation =====")
	prompt := fmt.Sprintf(
		"/test-driven-development Work on %s issue %s. If you don't have enough context to create a %s, output a clarifying question via the `clarifying question` JSON key. Otherwise, create a %s and output the number via the `%s number` JSON key",
		s.config.GitProvider, s.config.IssueNum, mrOrPR, mrOrPR, mrOrPR,
	)

	result, err := s.main.RunWithRetry(prompt, schema, s.prompter)
	if err != nil {
		return err
	}

	result, err = s.clarifyingLoop(result, schema, mrOrPR)
	if err != nil {
		return err
	}

	prNum := result.numberForProvider(s.config.GitProvider)

	fmt.Println("===== Phase 2: Pipeline loop 1 =====")
	if err := s.runPipeline(prNum); err != nil {
		return err
	}

	fmt.Println("===== Phase 3: AI Code Review =====")
	reviewPrompt := fmt.Sprintf("/code-review %s %d Leave your review as a comment on the %s", mrOrPR, prNum, mrOrPR)
	if _, err := s.reviewer.ResumeWithRetry(reviewPrompt, "", s.prompter); err != nil {
		return err
	}

	fmt.Println("===== Phase 4: Address CR comments =====")
	addressPrompt := fmt.Sprintf("Read the review comments on %s %d and make changes accordingly", mrOrPR, prNum)
	if _, err := s.main.ResumeWithRetry(addressPrompt, "", s.prompter); err != nil {
		return err
	}

	fmt.Println("===== Phase 5: Pipeline loop 2 =====")
	if err := s.runPipeline(prNum); err != nil {
		return err
	}

	fmt.Println("===== Phase 6: Human Review Loop =====")
	return s.humanReviewLoop(prNum, mrOrPR)
}

func (s *Solver) clarifyingLoop(result *RunResult, schema, mrOrPR string) (*RunResult, error) {
	for result.numberForProvider(s.config.GitProvider) == 0 {
		if result.Question == "" {
			var err error
			result, err = s.main.ResumeWithRetry(
				fmt.Sprintf("Your last response didn't provide a %s number or a clarifying question.", mrOrPR),
				schema, s.prompter,
			)
			if err != nil {
				return nil, err
			}
			continue
		}
		fmt.Println("===== Clarification Question Asked =====")
		fmt.Println(result.Question)
		answer, err := s.prompter("> ")
		if err != nil {
			return nil, err
		}
		result, err = s.main.ResumeWithRetry(
			fmt.Sprintf("The user has answered your clarifying question with %s", answer),
			schema, s.prompter,
		)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (s *Solver) runPipeline(prNum int) error {
	if s.config.SkipPipeline {
		fmt.Println("--skip-pipeline was set. Skipping pipeline loop...")
		return nil
	}
	return s.pipeline.Watch(prNum, &pipelineClaudeAdapter{s.main, s.prompter})
}

func (s *Solver) humanReviewLoop(prNum int, mrOrPR string) error {
	for {
		fmt.Println("Please provide your code review.\n1. Cleanup\n2. Address comments\n3. Rewrite Issue")
		answer, err := s.prompter("> ")
		if err != nil {
			return err
		}
		switch answer {
		case "1":
			return nil
		case "2":
			prompt := fmt.Sprintf("Read the review latest comments on %s %d and make changes accordingly", mrOrPR, prNum)
			if _, err := s.main.ResumeWithRetry(prompt, "", s.prompter); err != nil {
				return err
			}
			if err := s.runPipeline(prNum); err != nil {
				return err
			}
		case "3":
			prompt := fmt.Sprintf("The user has decided to close the %s. Leave a summary of this session as a comment on issue %s", mrOrPR, s.config.IssueNum)
			if _, err := s.main.ResumeWithRetry(prompt, "", s.prompter); err != nil {
				return err
			}
			return nil
		default:
			fmt.Println("Not a valid choice")
		}
	}
}

type pipelineClaudeAdapter struct {
	client   MainClient
	prompter func(string) (string, error)
}

func (a *pipelineClaudeAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) error {
	_, err := a.client.ResumeWithRetry(prompt, schema, p)
	return err
}
