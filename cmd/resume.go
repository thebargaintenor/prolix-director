package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/thebargaintenor/prolix-director/internal/config"
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
	appConfig := loadEnvAndConfig()

	parsedConfig, err := parseResumeFlags(args, appConfig)
	if err != nil {
		return err
	}

	ex := &runner.OS{}
	wd, basePath, err := worktreeBasePath()
	if err != nil {
		return err
	}
	wt := newWorktree(ex, basePath, parsedConfig.Branch, parsedConfig.MainBranch)

	fmt.Println("Attaching to worktree")
	if err := wt.Attach(); err != nil {
		return fmt.Errorf("attach worktree: %w", err)
	}
	if err := os.Chdir(wt.Path()); err != nil {
		_ = wt.Detach()
		return fmt.Errorf("chdir to worktree: %w", err)
	}
	defer func() {
		os.Chdir(wd) //nolint: errcheck
		if err := wt.Detach(); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", err)
		}
	}()

	s, err := newSolver(appConfig, ex, solve.Config{
		IssueNum:     parsedConfig.IssueNum,
		MainBranch:   parsedConfig.MainBranch,
		GitProvider:  parsedConfig.GitProvider,
		SkipPipeline: parsedConfig.SkipPipeline,
	})
	if err != nil {
		return err
	}

	return s.Resume(parsedConfig.PRNum)
}
