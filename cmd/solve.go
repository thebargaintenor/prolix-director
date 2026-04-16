package cmd

import (
	"bufio"
	"crypto/rand"
	"flag"
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

type solveConfig struct {
	IssueNum     string
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

func parseFlags(args []string, defaultConfig *config.Config) (solveConfig, error) {
	fs := flag.NewFlagSet("solve", flag.ContinueOnError)
	mainBranch := fs.String("main-branch", "trunk", "main branch name")
	skipPipeline := fs.Bool("skip-pipeline", false, "skip pipeline monitoring")
	gitProvider := fs.String("git-provider", "", "git provider: github or gitlab")

	if err := fs.Parse(args); err != nil {
		return solveConfig{}, err
	}

	// Set default git provider from config ONLY if not provided in flags
	if *gitProvider == "" && defaultConfig.GitProvider != "" {
		*gitProvider = defaultConfig.GitProvider
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
	envPath := filepath.Join(os.Getenv("HOME"), ".claude", ".env")
	if vars, loadErr := envfile.Load(envPath); loadErr == nil {
		envfile.Apply(vars)
	}

	appConfig := config.Load()

	parsedConfig, err := parseFlags(args, appConfig)
	if err != nil {
		return err
	}

	mainSessionID, err := newUUID()
	if err != nil {
		return fmt.Errorf("Error generating main session id: %w", err)
	}
	reviewerSessionID, err := newUUID()
	if err != nil {
		return fmt.Errorf("Error generating reviewer session id: %w", err)
	}
	fmt.Printf("Main session ID: %s\n", mainSessionID)
	fmt.Printf("Reviewer session ID: %s\n", reviewerSessionID)

	ex := &runner.OS{}
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}
	projectName := filepath.Base(wd)
	basePath := filepath.Join(os.Getenv("HOME"), ".config", "ai-worktrees", projectName)
	branch := fmt.Sprintf("agent-issue-%s", parsedConfig.IssueNum)

	wt := git.New(ex, basePath, branch, parsedConfig.MainBranch)
	fmt.Println("Creating worktree")
	if err := wt.Create(); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	if err := os.Chdir(wt.Path()); err != nil {
		_ = wt.Remove()
		return fmt.Errorf("chdir to worktree: %w", err)
	}
	defer func() {
		os.Chdir(wd) //nolint: errcheck
		if rmErr := wt.Remove(); rmErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", rmErr)
		}
	}()

	prompter := stdinPrompter()
	mainClaude := claude.NewDefault(mainSessionID, appConfig.CodeGenModel)
	reviewerClaude := claude.NewDefault(reviewerSessionID, appConfig.ReviewerModel)

	var monitor pipeline.Monitor
	if parsedConfig.GitProvider == "github" {
		monitor = pipeline.NewGitHub(ex, 3, prompter)
	} else {
		monitor = pipeline.NewGitLab(ex, prompter)
	}

	s := solve.New(
		solve.Config{
			IssueNum:     parsedConfig.IssueNum,
			MainBranch:   parsedConfig.MainBranch,
			GitProvider:  parsedConfig.GitProvider,
			SkipPipeline: parsedConfig.SkipPipeline,
		},
		&mainAdapter{mainClaude},
		&reviewAdapter{reviewerClaude},
		monitor,
		prompter,
	)

	return s.Run()
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
