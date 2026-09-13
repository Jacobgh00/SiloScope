package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"reviewstats/internal/githubapi"
	"reviewstats/internal/repository"
)

func TestRunRejectsUnknownFlag(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--wat"}, &stdout, &stderr)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	if stderr.String() == "" {
		t.Fatal("stderr is empty, want usage error")
	}
}

func TestRunPrintsHelpWithSuccess(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--help"}, &stdout, &stderr)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0", exitCode)
	}

	if !strings.Contains(stderr.String(), "-repo") || !strings.Contains(stderr.String(), "-since") {
		t.Fatalf("help output = %q, want -repo and -since flags", stderr.String())
	}
}

func TestRunWithDependenciesUsesExplicitRepository(t *testing.T) {
	t.Parallel()

	client := &fakeGitHubClient{}
	dependencies := testDependencies(client)
	currentRepoCalls := 0
	dependencies.currentRepo = func(context.Context) (repository.Repository, error) {
		currentRepoCalls++
		return repository.Repository{}, errors.New("current repository should not be used")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}

	if currentRepoCalls != 0 {
		t.Fatalf("current repository calls = %d, want 0", currentRepoCalls)
	}

	if got := client.collaboratorsRepository; got != (repository.Repository{Owner: "acme", Name: "frontend"}) {
		t.Fatalf("collaborator repository = %#v, want acme/frontend", got)
	}
}

func TestRunWithDependenciesUsesCurrentRepositoryWhenRepoFlagIsAbsent(t *testing.T) {
	t.Parallel()

	client := &fakeGitHubClient{}
	dependencies := testDependencies(client)
	dependencies.currentRepo = func(context.Context) (repository.Repository, error) {
		return repository.Repository{Owner: "current", Name: "repository"}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(context.Background(), nil, &stdout, &stderr, dependencies)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}

	if got := client.collaboratorsRepository; got != (repository.Repository{Owner: "current", Name: "repository"}) {
		t.Fatalf("collaborator repository = %#v, want current/repository", got)
	}
}

func TestRunWithDependenciesUsesDefaultSincePeriod(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
	client := &fakeGitHubClient{}
	dependencies := testDependencies(client)
	dependencies.now = func() time.Time { return now }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}

	want := now.AddDate(0, 0, -30)
	if !client.pullsSince.Equal(want) {
		t.Fatalf("pull request cutoff = %v, want %v", client.pullsSince, want)
	}
}

func TestRunWithDependenciesRejectsInvalidRepository(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "invalid"},
		&stdout,
		&stderr,
		testDependencies(&fakeGitHubClient{}),
	)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	if stderr.String() == "" {
		t.Fatal("stderr is empty, want repository error")
	}
}

func TestRunWithDependenciesRejectsInvalidSincePeriod(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend", "--since", "0d"},
		&stdout,
		&stderr,
		testDependencies(&fakeGitHubClient{}),
	)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	if stderr.String() == "" {
		t.Fatal("stderr is empty, want period error")
	}
}

func TestRunWithDependenciesReturnsConfigurationErrorWhenAuthenticationIsMissing(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies(&fakeGitHubClient{})
	dependencies.resolveToken = func(context.Context) (string, error) {
		return "", errors.New("authentication unavailable")
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	if stderr.String() == "" {
		t.Fatal("stderr is empty, want authentication error")
	}
}

func TestRunWithDependenciesReturnsRuntimeErrorForGitHubFailure(t *testing.T) {
	t.Parallel()

	client := &fakeGitHubClient{collaboratorsError: errors.New("GitHub unavailable")}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend"},
		&stdout,
		&stderr,
		testDependencies(client),
	)

	if exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	if stderr.String() == "" {
		t.Fatal("stderr is empty, want GitHub error")
	}
}

func TestRunWithDependenciesRendersDeduplicatedRetrospective(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	client := &fakeGitHubClient{
		collaborators: []githubapi.Collaborator{
			{Login: "alice"},
			{Login: "bob"},
		},
		pulls: []githubapi.PullRequest{
			{Number: 10, Author: "bob"},
		},
		reviewsByPull: map[int][]githubapi.Review{
			10: {
				{PullRequestNumber: 10, Reviewer: "alice", State: "COMMENTED", SubmittedAt: cutoff},
				{PullRequestNumber: 10, Reviewer: "alice", State: "APPROVED", SubmittedAt: cutoff.Add(time.Minute)},
			},
		},
	}
	dependencies := testDependencies(client)
	dependencies.now = func() time.Time { return cutoff.AddDate(0, 0, 30) }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend", "--since", "2026-09-01"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}

	if !strings.Contains(stdout.String(), "alice     1") || !strings.Contains(stdout.String(), "bob       0") {
		t.Fatalf("report =\n%s\nwant Alice to have 1 review and Bob to have 0", stdout.String())
	}

	if got := matrixRowFields(stdout.String(), "bob"); !reflect.DeepEqual(got, []string{"bob", "1", "-"}) {
		t.Fatalf("bob matrix row = %#v, want %#v", got, []string{"bob", "1", "-"})
	}
}

func TestRunWithDependenciesIntegration(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	var requestedPaths []string
	var requestedPathsMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestedPathsMu.Lock()
		requestedPaths = append(requestedPaths, request.URL.Path)
		requestedPathsMu.Unlock()

		if got := request.URL.Query().Get("per_page"); got != "100" {
			t.Errorf("per_page query = %q, want %q", got, "100")
		}
		if got := request.URL.Query().Get("page"); got != "1" {
			t.Errorf("page query = %q, want %q", got, "1")
		}

		switch request.URL.Path {

		case "/repos/acme/frontend/collaborators":
			_ = json.NewEncoder(writer).Encode([]map[string]string{
				{"login": "alice"},
				{"login": "bob"},
				{"login": "charlie"},
			})

		case "/repos/acme/frontend/pulls":
			if got := request.URL.Query().Get("state"); got != "all" {
				t.Errorf("state query = %q, want %q", got, "all")
			}
			if got := request.URL.Query().Get("sort"); got != "updated" {
				t.Errorf("sort query = %q, want %q", got, "updated")
			}
			if got := request.URL.Query().Get("direction"); got != "desc" {
				t.Errorf("direction query = %q, want %q", got, "desc")
			}
			_ = json.NewEncoder(writer).Encode([]map[string]any{
				{
					"number":     101,
					"user":       map[string]string{"login": "author-a"},
					"updated_at": cutoff.Add(2 * time.Hour),
				},
				{
					"number":     102,
					"user":       map[string]string{"login": "author-b"},
					"updated_at": cutoff.Add(time.Hour),
				},
			})

		case "/repos/acme/frontend/pulls/101/reviews":
			_ = json.NewEncoder(writer).Encode([]map[string]any{
				{"state": "APPROVED", "submitted_at": cutoff.Add(time.Hour), "user": map[string]string{"login": "alice"}},
				{"state": "COMMENTED", "submitted_at": cutoff.Add(2 * time.Hour), "user": map[string]string{"login": "bob"}},
				{"state": "APPROVED", "submitted_at": cutoff.Add(3 * time.Hour), "user": map[string]string{"login": "bob"}},
				{"state": "APPROVED", "submitted_at": cutoff.Add(time.Hour), "user": map[string]string{"login": "author-a"}},
				{"state": "APPROVED", "submitted_at": cutoff.Add(-time.Second), "user": map[string]string{"login": "eve"}},
			})

		case "/repos/acme/frontend/pulls/102/reviews":
			_ = json.NewEncoder(writer).Encode([]map[string]any{
				{"state": "CHANGES_REQUESTED", "submitted_at": cutoff.Add(time.Hour), "user": map[string]string{"login": "alice"}},
			})

		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	client, err := githubapi.NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	dependencies := testDependencies(client)
	dependencies.now = func() time.Time { return cutoff.AddDate(0, 0, 30) }

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"--repo", "acme/frontend", "--since", "2026-09-01"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr = %q", exitCode, stderr.String())
	}

	if !strings.Contains(stdout.String(), "alice     2") ||
		!strings.Contains(stdout.String(), "bob       1") ||
		!strings.Contains(stdout.String(), "charlie   0") {
		t.Fatalf("report =\n%s\nwant participation alice=2, bob=1, charlie=0", stdout.String())
	}

	if strings.Contains(stdout.String(), "eve") {
		t.Fatalf("report =\n%s\nmust not include old reviewer eve", stdout.String())
	}

	if got := matrixRowFields(stdout.String(), "author-a"); !reflect.DeepEqual(got, []string{"author-a", "1", "1", "0"}) {
		t.Fatalf("author-a matrix row = %#v, want %#v", got, []string{"author-a", "1", "1", "0"})
	}

	if got := matrixRowFields(stdout.String(), "author-b"); !reflect.DeepEqual(got, []string{"author-b", "1", "0", "0"}) {
		t.Fatalf("author-b matrix row = %#v, want %#v", got, []string{"author-b", "1", "0", "0"})
	}

	requestedPathsMu.Lock()
	gotPaths := append([]string(nil), requestedPaths...)
	requestedPathsMu.Unlock()
	wantPaths := []string{
		"/repos/acme/frontend/collaborators",
		"/repos/acme/frontend/pulls",
		"/repos/acme/frontend/pulls/101/reviews",
		"/repos/acme/frontend/pulls/102/reviews",
	}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("requested paths = %#v, want %#v", gotPaths, wantPaths)
	}
}

type fakeGitHubClient struct {
	collaborators           []githubapi.Collaborator
	collaboratorsError      error
	collaboratorsRepository repository.Repository
	pulls                   []githubapi.PullRequest
	pullsError              error
	pullsSince              time.Time
	reviewsByPull           map[int][]githubapi.Review
	reviewsErrorByPull      map[int]error
}

func (client *fakeGitHubClient) ListCollaborators(
	_ context.Context,
	repo repository.Repository,
) ([]githubapi.Collaborator, error) {
	client.collaboratorsRepository = repo

	return client.collaborators, client.collaboratorsError
}

func (client *fakeGitHubClient) ListPullRequestsUpdatedSince(
	_ context.Context,
	_ repository.Repository,
	since time.Time,
) ([]githubapi.PullRequest, error) {
	client.pullsSince = since

	return client.pulls, client.pullsError
}

func (client *fakeGitHubClient) ListReviews(
	_ context.Context,
	_ repository.Repository,
	pullRequestNumber int,
) ([]githubapi.Review, error) {
	if err := client.reviewsErrorByPull[pullRequestNumber]; err != nil {
		return nil, err
	}

	return client.reviewsByPull[pullRequestNumber], nil
}

func testDependencies(client githubClient) dependencies {
	return dependencies{
		now: func() time.Time {
			return time.Date(2026, 9, 12, 22, 0, 0, 0, time.UTC)
		},
		currentRepo: func(context.Context) (repository.Repository, error) {
			return repository.Repository{Owner: "current", Name: "repository"}, nil
		},
		resolveToken: func(context.Context) (string, error) {
			return "test-token", nil
		},
		newClient: func(string) githubClient {
			return client
		},
	}
}

func matrixRowFields(report, author string) []string {
	inMatrix := false
	for _, line := range strings.Split(report, "\n") {
		if line == "Review matrix" {
			inMatrix = true
			continue
		}

		if !inMatrix {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == author {
			return fields
		}
	}

	return nil
}
