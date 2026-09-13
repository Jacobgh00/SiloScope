package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResolveTokenPrefersGHToken(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("from-gh-cli")}

	got, err := ResolveToken(context.Background(), environment(map[string]string{
		"GH_TOKEN":     "from-gh-token",
		"GITHUB_TOKEN": "from-github-token",
	}), runner)

	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if got != "from-gh-token" {
		t.Fatalf("ResolveToken() = %q, want %q", got, "from-gh-token")
	}

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want none", runner.calls)
	}
}

func TestResolveTokenUsesGitHubTokenWhenGHTokenIsEmpty(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("from-gh-cli")}

	got, err := ResolveToken(context.Background(), environment(map[string]string{
		"GH_TOKEN":     "",
		"GITHUB_TOKEN": "from-github-token",
	}), runner)

	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if got != "from-github-token" {
		t.Fatalf("ResolveToken() = %q, want %q", got, "from-github-token")
	}

	if len(runner.calls) != 0 {
		t.Fatalf("runner calls = %#v, want none", runner.calls)
	}
}

func TestResolveTokenUsesGitHubCLIWhenEnvironmentTokensAreEmpty(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte("from-gh-cli\n")}

	got, err := ResolveToken(context.Background(), environment(map[string]string{
		"GH_TOKEN":     "",
		"GITHUB_TOKEN": "",
	}), runner)

	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if got != "from-gh-cli" {
		t.Fatalf("ResolveToken() = %q, want %q", got, "from-gh-cli")
	}

	assertGitHubCLICall(t, runner)
}

func TestResolveTokenTrimsGitHubCLIToken(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte(" \tfrom-gh-cli\n")}

	got, err := ResolveToken(context.Background(), environment(nil), runner)

	if err != nil {
		t.Fatalf("ResolveToken() error = %v", err)
	}

	if got != "from-gh-cli" {
		t.Fatalf("ResolveToken() = %q, want %q", got, "from-gh-cli")
	}

	assertGitHubCLICall(t, runner)
}

func TestResolveTokenRejectsEmptyGitHubCLIOutput(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{output: []byte(" \n\t")}

	if _, err := ResolveToken(context.Background(), environment(nil), runner); err == nil {
		t.Fatal("ResolveToken() error = nil, want error")
	}
}

func TestResolveTokenHidesGitHubCLIErrorDetails(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{err: errors.New("gh failed with secret-token")}

	_, err := ResolveToken(context.Background(), environment(nil), runner)

	if err == nil {
		t.Fatal("ResolveToken() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "GitHub authentication") {
		t.Fatalf("ResolveToken() error = %q, want user-facing authentication context", err)
	}

	if strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("ResolveToken() error = %q, must not contain command output", err)
	}
}

type fakeRunner struct {
	output []byte
	err    error
	calls  []commandCall
}

type commandCall struct {
	name string
	args []string
}

func (r *fakeRunner) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, commandCall{name: name, args: append([]string(nil), args...)})

	return r.output, r.err
}

func environment(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func assertGitHubCLICall(t *testing.T, runner *fakeRunner) {
	t.Helper()

	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %#v, want one call", runner.calls)
	}

	call := runner.calls[0]

	if call.name != "gh" || len(call.args) != 2 || call.args[0] != "auth" || call.args[1] != "token" {
		t.Fatalf("runner call = %#v, want gh auth token", call)
	}
}
