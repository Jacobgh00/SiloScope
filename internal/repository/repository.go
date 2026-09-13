package repository

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type Repository struct {
	Owner string
	Name  string
}

func (r Repository) String() string {
	return r.Owner + "/" + r.Name
}

func Parse(value string) (Repository, error) {
	parts := strings.Split(value, "/")

	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Repository{}, fmt.Errorf("repository must be in owner/name form")
	}

	if strings.TrimSpace(parts[0]) != parts[0] || strings.TrimSpace(parts[1]) != parts[1] {
		return Repository{}, fmt.Errorf("repository must be in owner/name form")
	}

	return Repository{Owner: parts[0], Name: parts[1]}, nil
}

func FromRemoteURL(value string) (Repository, error) {
	const sshPrefix = "git@github.com:"

	if strings.HasPrefix(value, sshPrefix) {
		return parseRemotePath(strings.TrimPrefix(value, sshPrefix))
	}

	parsed, err := url.Parse(value)

	if err != nil {
		return Repository{}, fmt.Errorf("parse remote URL: %w", err)
	}

	if parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return Repository{}, fmt.Errorf("remote URL must point to github.com over HTTPS or use GitHub SSH")
	}

	return parseRemotePath(strings.TrimPrefix(parsed.Path, "/"))
}

func Current(ctx context.Context) (Repository, error) {
	output, err := exec.CommandContext(ctx, "git", "config", "--get", "remote.origin.url").Output()

	if err != nil {
		return Repository{}, fmt.Errorf("read remote.origin.url: %w", err)
	}

	repository, err := FromRemoteURL(strings.TrimSpace(string(output)))

	if err != nil {
		return Repository{}, fmt.Errorf("parse remote.origin.url: %w", err)
	}

	return repository, nil
}

func parseRemotePath(value string) (Repository, error) {
	return Parse(strings.TrimSuffix(value, ".git"))
}
