package pipeline

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Executor interface {
	Execute(name string, args ...string) ([]byte, error)
}

type Claude interface {
	ResumeWithRetry(prompt, schema string, prompter func(string) (string, error)) error
}

type Monitor interface {
	Watch(prNum int, claude Claude) error
}

type GitHub struct {
	executor    Executor
	maxAttempts int
	prompter    func(string) (string, error)
}

func NewGitHub(executor Executor, maxAttempts int, prompter func(string) (string, error)) *GitHub {
	return &GitHub{executor: executor, maxAttempts: maxAttempts, prompter: prompter}
}

func (g *GitHub) Watch(prNum int, claude Claude) error {
	prStr := strconv.Itoa(prNum)
	attempt := 0
	for {
		_, err := g.executor.Execute("gh", "pr", "checks", prStr, "--watch")
		if err == nil {
			return nil
		}
		attempt++
		if attempt >= g.maxAttempts {
			answer, pErr := g.prompter(fmt.Sprintf("Pipeline failed %d times. Provide guidance or 'q' to quit: ", g.maxAttempts))
			if pErr != nil {
				return pErr
			}
			if answer == "q" {
				return fmt.Errorf("user quit after pipeline failures")
			}
			if err := claude.ResumeWithRetry(
				fmt.Sprintf("The pipeline has failed %d times. The user says: %s", g.maxAttempts, answer),
				"", g.prompter,
			); err != nil {
				return err
			}
			attempt = 0
		} else {
			fmt.Printf("Prompting claude to fix pipeline (attempt %d/%d)...\n", attempt, g.maxAttempts)
			if err := claude.ResumeWithRetry("Looks like your pipeline failed. Please fix and push", "", g.prompter); err != nil {
				return err
			}
		}
	}
}

type GitLab struct {
	executor Executor
	prompter func(string) (string, error)
}

func NewGitLab(executor Executor, prompter func(string) (string, error)) *GitLab {
	return &GitLab{executor: executor, prompter: prompter}
}

func (g *GitLab) Watch(mrNum int, claude Claude) error {
	mrStr := strconv.Itoa(mrNum)
	for {
		status, err := g.pipelineStatus(mrStr)
		if err != nil {
			return err
		}
		fmt.Printf("Waiting on pipeline...\n")
		for status != "success" && status != "failed" && status != "canceled" && status != "skipped" {
			fmt.Printf("\rCurrent status: %s", status)
			status, err = g.pipelineStatus(mrStr)
			if err != nil {
				return err
			}
		}
		fmt.Printf("\nPipeline finished with status %s\n", status)
		switch status {
		case "success":
			return nil
		case "failed":
			if err := claude.ResumeWithRetry("Looks like your pipeline failed. Please fix and push", "", g.prompter); err != nil {
				return err
			}
		default:
			answer, err := g.prompter(fmt.Sprintf("The pipeline status is %s, continue?[yn] ", status))
			if err != nil {
				return err
			}
			if answer == "n" {
				return nil
			}
		}
	}
}

func (g *GitLab) pipelineStatus(mrStr string) (string, error) {
	out, err := g.executor.Execute("glab", "mr", "view", mrStr, "--output", "json")
	if err != nil {
		return "", fmt.Errorf("glab mr view: %w", err)
	}
	var payload struct {
		HeadPipeline struct {
			Status string `json:"status"`
		} `json:"head_pipeline"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return "", fmt.Errorf("parse pipeline status: %w", err)
	}
	return payload.HeadPipeline.Status, nil
}
