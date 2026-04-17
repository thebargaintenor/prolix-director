package cmd

import (
	"bufio"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thebargaintenor/prolix-director/internal/claude"
	"github.com/thebargaintenor/prolix-director/internal/config"
	"github.com/thebargaintenor/prolix-director/internal/envfile"
	"github.com/thebargaintenor/prolix-director/internal/git"
	"github.com/thebargaintenor/prolix-director/internal/pipeline"
	"github.com/thebargaintenor/prolix-director/internal/runner"
	"github.com/thebargaintenor/prolix-director/internal/solve"
)

func loadEnvAndConfig() *config.Config {
	envPath := filepath.Join(os.Getenv("HOME"), ".claude", ".env")
	if vars, loadErr := envfile.Load(envPath); loadErr == nil {
		envfile.Apply(vars)
	}
	return config.Load()
}

func worktreeBasePath() (wd, basePath string, err error) {
	wd, err = os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("getwd: %w", err)
	}
	projectName := filepath.Base(wd)
	basePath = filepath.Join(os.Getenv("HOME"), ".config", "ai-worktrees", projectName)
	return wd, basePath, nil
}

func newWorktree(ex git.Executor, basePath, branch, mainBranch string) *git.Worktree {
	return git.New(ex, basePath, branch, mainBranch)
}

func newSolver(appConfig *config.Config, ex *runner.OS, cfg solve.Config) (*solve.Solver, error) {
	mainSessionID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("Error generating main session id: %w", err)
	}
	reviewerSessionID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("Error generating reviewer session id: %w", err)
	}
	fmt.Printf("Main session ID: %s\n", mainSessionID)
	fmt.Printf("Reviewer session ID: %s\n", reviewerSessionID)

	prompter := stdinPrompter()
	mainClaude := claude.NewDefault(mainSessionID, appConfig.CodeGenModel)
	reviewerClaude := claude.NewDefault(reviewerSessionID, appConfig.ReviewerModel)

	var monitor pipeline.Monitor
	if cfg.GitProvider == "github" {
		monitor = pipeline.NewGitHub(ex, 3, prompter)
	} else {
		monitor = pipeline.NewGitLab(ex, prompter)
	}

	return solve.New(cfg, &mainAdapter{mainClaude}, &reviewAdapter{reviewerClaude}, monitor, prompter), nil
}

type mainAdapter struct {
	client *claude.Client
}

func (a *mainAdapter) RunWithRetry(prompt, schema string, p func(string) (string, error)) (*solve.RunResult, error) {
	resp, err := a.client.RunWithRetry(prompt, schema, p)
	if err != nil {
		return nil, err
	}
	return toRunResult(resp)
}

func (a *mainAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*solve.RunResult, error) {
	resp, err := a.client.ResumeWithRetry(prompt, schema, p)
	if err != nil {
		return nil, err
	}
	return toRunResult(resp)
}

type reviewAdapter struct {
	client *claude.Client
}

func (a *reviewAdapter) ResumeWithRetry(prompt, schema string, p func(string) (string, error)) (*solve.RunResult, error) {
	resp, err := a.client.ResumeWithRetry(prompt, schema, p)
	if err != nil {
		return nil, err
	}
	return toRunResult(resp)
}

func toRunResult(resp *claude.Response) (*solve.RunResult, error) {
	out, err := resp.ParseImplOutput()
	if err != nil {
		return nil, err
	}
	if out == nil {
		return &solve.RunResult{}, nil
	}
	return &solve.RunResult{
		PRNum:    out.PRNumber,
		MRNum:    out.MRNumber,
		Question: out.ClarifyingQuestion,
	}, nil
}

func stdinPrompter() func(string) (string, error) {
	reader := bufio.NewReader(os.Stdin)
	return func(question string) (string, error) {
		fmt.Print(question)
		line, err := reader.ReadString('\n')
		return strings.TrimSpace(line), err
	}
}

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate uuid: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
