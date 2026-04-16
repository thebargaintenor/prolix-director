package config

import "os"

type Config struct {
	CodeGenModel  string
	ReviewerModel string
	GitProvider   string
}

func Load() *Config {
	appConfig := Config{
		CodeGenModel:  os.Getenv("CODE_GEN_MODEL"),
		ReviewerModel: os.Getenv("REVIEWER_MODEL"),
		GitProvider:   os.Getenv("GIT_PROVIDER"),
	}

	if appConfig.CodeGenModel == "" {
		appConfig.CodeGenModel = "claude-sonnet-4-6"
	}
	if appConfig.ReviewerModel == "" {
		appConfig.ReviewerModel = "claude-opus-4-6"
	}

	return &appConfig
}
