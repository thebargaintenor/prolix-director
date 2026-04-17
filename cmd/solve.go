package cmd

import (
	"flag"
	"fmt"
	"os"

	"github.com/thebargaintenor/prolix-director/internal/config"
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
	appConfig := loadEnvAndConfig()

	parsedConfig, err := parseFlags(args, appConfig)
	if err != nil {
		return err
	}

	ex := &runner.OS{}
	wd, basePath, err := worktreeBasePath()
	if err != nil {
		return err
	}
	branch := fmt.Sprintf("agent-issue-%s", parsedConfig.IssueNum)
	wt := newWorktree(ex, basePath, branch, parsedConfig.MainBranch)

	fmt.Println("Creating worktree")
	if err := wt.Create(); err != nil {
		return fmt.Errorf("create worktree: %w", err)
	}
	if err := os.Chdir(wt.Path()); err != nil {
		_ = wt.Remove()
		return fmt.Errorf("chdir to worktree: %w", err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: chdir: %v\n", err)
		}
		if rmErr := wt.Remove(); rmErr != nil {
			fmt.Fprintf(os.Stderr, "cleanup warning: %v\n", rmErr)
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

	return s.Run()
}
