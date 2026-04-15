package cmd

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thebargaintenor/prolix-director/internal/claude"
	"github.com/thebargaintenor/prolix-director/internal/envfile"
	"github.com/thebargaintenor/prolix-director/internal/git"
	"github.com/thebargaintenor/prolix-director/internal/pipeline"
	"github.com/thebargaintenor/prolix-director/internal/solve"
)

type solveConfig struct {
	IssueNum     string
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

func parseFlags(args []string) (solveConfig, error) {
	fs := flag.NewFlagSet("solve", flag.ContinueOnError)
	mainBranch := fs.String("main-branch", "trunk", "main branch name")
	skipPipeline := fs.Bool("skip-pipeline", false, "skip pipeline monitoring")
	gitProvider := fs.String("git-provider", "", "git provider: github or gitlab")

	if err := fs.Parse(args); err != nil {
		return solveConfig{}, err
	}
	if *gitProvider == "" {
		return solveConfig{}, fmt.Errorf("--git-provider is required (github or gitlab)")
	}
	if *gitProvider != "github" && *gitProvider != "gitlab" {
		return solveConfig{}, fmt.Errorf("unsupported git provider %q; use github or gitlab", *gitProvider)
	}
	if fs.NArg() != 1 {
		return solveConfig{}, fmt.Errorf("usage: prolix solve [flags] ISSUE_NUM")
	}
	return solveConfig{
		IssueNum:     fs.Arg(0),
		MainBranch:   *mainBranch,
		GitProvider:  *gitProvider,
		SkipPipeline: *skipPipeline,
	}, nil
}

func RunSolve(args []string) error {
	cfg, err := parseFlags(args)
	if err != nil {
		return err
	}

	envPath := filepath.Join(os.Getenv("HOME"), ".claude", ".env")
	if vars, loadErr := envfile.Load(envPath); loadErr == nil {
		envfile.Apply(vars)
	}

	codeModel := os.Getenv("CODE_GEN_MODEL")
	if codeModel == "" {
		codeModel = "claude-opus-4-6"
	}
	reviewerModel := os.Getenv("REVIEWER_MODEL")
	if reviewerModel == "" {
		reviewerModel = "claude-sonnet-4-6"
	}

	mainSessionID := newUUID()
	reviewerSessionID := newUUID()
	fmt.Printf("Main session ID: %s\n", mainSessionID)
	fmt.Printf("Reviewer session ID: %s\n", reviewerSessionID)

	executor := &osExecutor{}
	projectName := filepath.Base(mustGetwd())
	basePath := filepath.Join(os.Getenv("HOME"), ".config", "ai-worktrees", projectName)
	branch := fmt.Sprintf("agent-issue-%s", cfg.IssueNum)

	wt := git.New(executor, basePath, branch, cfg.MainBranch)
	fmt.Println("Creating worktree")
	if err := wt.Create(); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	defer func() {
		if rmErr := wt.Remove(); rmErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", rmErr)
		}
	}()

	prompter := stdinPrompter()
	mainClaude := claude.NewDefault(mainSessionID, codeModel)
	reviewerClaude := claude.NewDefault(reviewerSessionID, reviewerModel)

	var monitor pipeline.Monitor
	if cfg.GitProvider == "github" {
		monitor = pipeline.NewGitHub(executor, 3, prompter)
	} else {
		monitor = pipeline.NewGitLab(executor, prompter)
	}

	s := solve.New(
		solve.Config{
			IssueNum:     cfg.IssueNum,
			MainBranch:   cfg.MainBranch,
			GitProvider:  cfg.GitProvider,
			SkipPipeline: cfg.SkipPipeline,
		},
		&mainAdapter{mainClaude, prompter},
		&reviewAdapter{reviewerClaude, prompter},
		monitor,
		prompter,
	)

	return s.Run()
}

type osExecutor struct{}

func (e *osExecutor) Execute(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

type mainAdapter struct {
	client   *claude.Client
	prompter func(string) (string, error)
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
	client   *claude.Client
	prompter func(string) (string, error)
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

func newUUID() string {
	out, err := exec.Command("uuidgen").Output()
	if err != nil {
		return fmt.Sprintf("%d", os.Getpid())
	}
	return strings.TrimSpace(string(out))
}

func mustGetwd() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return wd
}
