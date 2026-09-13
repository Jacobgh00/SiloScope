package auth

import (
	"context"
	"errors"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Output(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Output(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func ResolveToken(ctx context.Context, getenv func(string) string, runner CommandRunner) (string, error) {
	if token := getenv("GH_TOKEN"); token != "" {
		return token, nil
	}

	if token := getenv("GITHUB_TOKEN"); token != "" {
		return token, nil
	}

	output, err := runner.Output(ctx, "gh", "auth", "token")

	if err != nil {
		return "", errors.New("GitHub authentication could not be resolved through gh CLI")
	}

	token := strings.TrimSpace(string(output))

	if token == "" {
		return "", errors.New("GitHub authentication token from gh CLI is empty")
	}

	return token, nil
}
