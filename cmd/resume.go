package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebargaintenor/prolix-director/internal/claude"
	"github.com/thebargaintenor/prolix-director/internal/config"
	"github.com/thebargaintenor/prolix-director/internal/envfile"
	"github.com/thebargaintenor/prolix-director/internal/git"
	"github.com/thebargaintenor/prolix-director/internal/pipeline"
	"github.com/thebargaintenor/prolix-director/internal/runner"
	"github.com/thebargaintenor/prolix-director/internal/solve"
)

type resumeConfig struct {
	IssueNum     string
	Branch       string
	PRNum        int
	MainBranch   string
	GitProvider  string
	SkipPipeline bool
}

func parseResumeFlags(args []string, defaultConfig *config.Config) (resumeConfig, error) {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	branch := fs.String("branch", "", "existing branch to attach to")
	prNum := fs.Int("pr", 0, "existing PR number (optional)")
	mainBranch := fs.String("main-branch", "trunk", "main branch name")
	skipPipeline := fs.Bool("skip-pipeline", false, "skip pipeline monitoring")
	gitProvider := fs.String("git-provider", "", "git provider: github or gitlab")

	if err := fs.Parse(args); err != nil {
		return resumeConfig{}, err
	}

	if *branch == "" {
		return resumeConfig{}, fmt.Errorf("--branch is required")
	}

	if *gitProvider == "" && defaultConfig.GitProvider != "" {
		*gitProvider = defaultConfig.GitProvider
	}
	if *gitProvider == "" {
		return resumeConfig{}, fmt.Errorf("--git-provider is required (github or gitlab)")
	}
	if *gitProvider != "github" && *gitProvider != "gitlab" {
		return resumeConfig{}, fmt.Errorf("unsupported git provider %q; use github or gitlab", *gitProvider)
	}

	if fs.NArg() != 1 {
		return resumeConfig{}, fmt.Errorf("usage: prolix resume [flags] ISSUE_NUM")
	}

	return resumeConfig{
		IssueNum:     fs.Arg(0),
		Branch:       *branch,
		PRNum:        *prNum,
		MainBranch:   *mainBranch,
		GitProvider:  *gitProvider,
		SkipPipeline: *skipPipeline,
	}, nil
}

func RunResume(args []string) error {
	envPath := filepath.Join(os.Getenv("HOME"), ".claude", ".env")
	if vars, loadErr := envfile.Load(envPath); loadErr == nil {
		envfile.Apply(vars)
	}

	appConfig := config.Load()

	parsedConfig, err := parseResumeFlags(args, appConfig)
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

	wt := git.New(ex, basePath, parsedConfig.Branch, parsedConfig.MainBranch)
	fmt.Println("Attaching to worktree")
	if err := wt.Attach(); err != nil {
		return fmt.Errorf("attach worktree: %w", err)
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

	return s.Resume(parsedConfig.PRNum)
}
