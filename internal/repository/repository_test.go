package repository

import (
	"context"
	"os"
	"os/exec"
	"testing"
)

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Repository
	}{
		{
			name:  "owner slash repo",
			input: "acme/frontend",
			want: Repository{
				Owner: "acme",
				Name:  "frontend",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("Parse() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "empty value", input: ""},
		{name: "owner only", input: "owner"},
		{name: "extra path segment", input: "owner/repo/extra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := Parse(tt.input); err == nil {
				t.Fatal("Parse() error = nil, want error")
			}
		})
	}
}

func TestRepositoryString(t *testing.T) {
	t.Parallel()

	repository := Repository{Owner: "acme", Name: "frontend"}

	if got := repository.String(); got != "acme/frontend" {
		t.Fatalf("String() = %q, want %q", got, "acme/frontend")
	}
}

func TestFromRemoteURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Repository
	}{
		{
			name:  "SSH",
			input: "git@github.com:acme/frontend.git",
			want:  Repository{Owner: "acme", Name: "frontend"},
		},
		{
			name:  "HTTPS",
			input: "https://github.com/acme/website.git",
			want:  Repository{Owner: "acme", Name: "website"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := FromRemoteURL(tt.input)
			if err != nil {
				t.Fatalf("FromRemoteURL() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("FromRemoteURL() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestFromRemoteURLRejectsUnsupportedURLs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "GitLab remote", input: "https://gitlab.com/acme/frontend.git"},
		{name: "malformed SSH remote", input: "git@github.com/acme/frontend.git"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := FromRemoteURL(tt.input); err == nil {
				t.Fatal("FromRemoteURL() error = nil, want error")
			}
		})
	}
}

func TestCurrentReadsOriginURL(t *testing.T) {
	repositoryDirectory := t.TempDir()
	runGit(t, repositoryDirectory, "init")
	runGit(t, repositoryDirectory, "remote", "add", "origin", "git@github.com:acme/frontend.git")

	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}

	if err := os.Chdir(repositoryDirectory); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(originalDirectory); err != nil {
			t.Errorf("os.Chdir() cleanup error = %v", err)
		}
	})

	got, err := Current(context.Background())
	if err != nil {
		t.Fatalf("Current() error = %v", err)
	}

	want := Repository{Owner: "acme", Name: "frontend"}
	if got != want {
		t.Fatalf("Current() = %#v, want %#v", got, want)
	}
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()

	command := exec.Command("git", append([]string{"-C", directory}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v error = %v, output = %s", args, err, output)
	}
}
