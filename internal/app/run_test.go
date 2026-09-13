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

func TestRunWithDependenciesRejectsUnexpectedPositionalArguments(t *testing.T) {
	t.Parallel()

	dependencies := testDependencies(&fakeGitHubClient{})
	dependencies.currentRepo = func(context.Context) (repository.Repository, error) {
		t.Fatal("current repository lookup must not run")
		return repository.Repository{}, nil
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	exitCode := runWithDependencies(
		context.Background(),
		[]string{"ignored-argument"},
		&stdout,
		&stderr,
		dependencies,
	)

	if exitCode != 2 {
		t.Fatalf("exit code = %d, want 2", exitCode)
	}

	if !strings.Contains(stderr.String(), "unexpected arguments: ignored-argument") {
		t.Fatalf("stderr = %q, want positional argument error", stderr.String())
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

	if got := client.activityRepository; got != (repository.Repository{Owner: "acme", Name: "frontend"}) {
		t.Fatalf("activity repository = %#v, want acme/frontend", got)
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

	if got := client.activityRepository; got != (repository.Repository{Owner: "current", Name: "repository"}) {
		t.Fatalf("activity repository = %#v, want current/repository", got)
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
	if !client.activitySince.Equal(want) {
		t.Fatalf("activity cutoff = %v, want %v", client.activitySince, want)
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

	client := &fakeGitHubClient{activityError: errors.New("GitHub unavailable")}
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
		activity: githubapi.ReviewActivity{
			PullRequests: []githubapi.PullRequest{
				{Number: 10, Author: "bob"},
			},
			ReviewsByPull: map[int][]githubapi.Review{
				10: {
					{PullRequestNumber: 10, Reviewer: "alice", State: "COMMENTED", SubmittedAt: cutoff},
					{PullRequestNumber: 10, Reviewer: "alice", State: "APPROVED", SubmittedAt: cutoff.Add(time.Minute)},
				},
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

	if got := participationRowFields(stdout.String(), "alice"); !reflect.DeepEqual(got, []string{"alice", "1"}) {
		t.Fatalf("alice participation row = %#v, want %#v", got, []string{"alice", "1"})
	}

	if got := participationRowFields(stdout.String(), "bob"); !reflect.DeepEqual(got, []string{"bob", "0"}) {
		t.Fatalf("bob participation row = %#v, want %#v", got, []string{"bob", "0"})
	}

	if got := matrixRowFields(stdout.String(), "bob"); !reflect.DeepEqual(got, []string{"bob", "1"}) {
		t.Fatalf("bob matrix row = %#v, want %#v", got, []string{"bob", "1"})
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

		if request.Method != http.MethodPost || request.URL.Path != "/graphql" {
			http.NotFound(writer, request)
			return
		}

		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode GraphQL request = %v", err)
		}

		if !strings.Contains(payload.Query, "ReviewActivity") {
			t.Errorf("query = %q, want ReviewActivity query", payload.Query)
		}
		if got := payload.Variables["owner"]; got != "acme" {
			t.Errorf("owner variable = %#v, want %q", got, "acme")
		}
		if got := payload.Variables["name"]; got != "frontend" {
			t.Errorf("name variable = %#v, want %q", got, "frontend")
		}
		if got := payload.Variables["after"]; got != nil {
			t.Errorf("after variable = %#v, want nil", got)
		}

		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"repository": map[string]any{
					"pullRequests": map[string]any{
						"nodes": []map[string]any{
							{
								"number":    101,
								"updatedAt": cutoff.Add(2 * time.Hour),
								"author":    map[string]string{"login": "author-a"},
								"reviews": map[string]any{
									"nodes": []map[string]any{
										{"state": "APPROVED", "submittedAt": cutoff.Add(time.Hour), "author": map[string]string{"login": "alice"}},
										{"state": "COMMENTED", "submittedAt": cutoff.Add(2 * time.Hour), "author": map[string]string{"login": "bob"}},
										{"state": "APPROVED", "submittedAt": cutoff.Add(3 * time.Hour), "author": map[string]string{"login": "bob"}},
										{"state": "APPROVED", "submittedAt": cutoff.Add(time.Hour), "author": map[string]string{"login": "author-a"}},
										{"state": "APPROVED", "submittedAt": cutoff.Add(-time.Second), "author": map[string]string{"login": "eve"}},
									},
									"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
								},
							},
							{
								"number":    102,
								"updatedAt": cutoff.Add(time.Hour),
								"author":    map[string]string{"login": "author-b"},
								"reviews": map[string]any{
									"nodes": []map[string]any{
										{"state": "CHANGES_REQUESTED", "submittedAt": cutoff.Add(time.Hour), "author": map[string]string{"login": "alice"}},
									},
									"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
								},
							},
						},
						"pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil},
					},
				},
			},
		})
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

	if !strings.Contains(stdout.String(), "Repository participants") {
		t.Fatalf("report =\n%s\nwant repository participant heading", stdout.String())
	}

	for participant, want := range map[string][]string{
		"alice":    {"alice", "2"},
		"author-a": {"author-a", "0"},
		"author-b": {"author-b", "0"},
		"bob":      {"bob", "1"},
	} {
		if got := participationRowFields(stdout.String(), participant); !reflect.DeepEqual(got, want) {
			t.Fatalf("%s participation row = %#v, want %#v", participant, got, want)
		}
	}

	if strings.Contains(stdout.String(), "eve") {
		t.Fatalf("report =\n%s\nmust not include reviewer with only an old review", stdout.String())
	}

	if got := matrixRowFields(stdout.String(), "author-a"); !reflect.DeepEqual(got, []string{"author-a", "1", "1"}) {
		t.Fatalf("author-a matrix row = %#v, want %#v", got, []string{"author-a", "1", "1"})
	}

	if got := matrixRowFields(stdout.String(), "author-b"); !reflect.DeepEqual(got, []string{"author-b", "1", "0"}) {
		t.Fatalf("author-b matrix row = %#v, want %#v", got, []string{"author-b", "1", "0"})
	}

	requestedPathsMu.Lock()
	gotPaths := append([]string(nil), requestedPaths...)
	requestedPathsMu.Unlock()
	wantPaths := []string{"/graphql"}
	if !reflect.DeepEqual(gotPaths, wantPaths) {
		t.Fatalf("requested paths = %#v, want %#v", gotPaths, wantPaths)
	}
}

type fakeGitHubClient struct {
	activity           githubapi.ReviewActivity
	activityError      error
	activityRepository repository.Repository
	activitySince      time.Time
}

func (client *fakeGitHubClient) LoadReviewActivity(
	_ context.Context,
	repo repository.Repository,
	since time.Time,
) (githubapi.ReviewActivity, error) {
	client.activityRepository = repo
	client.activitySince = since

	return client.activity, client.activityError
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

func participationRowFields(report, participant string) []string {
	inParticipation := false
	for _, line := range strings.Split(report, "\n") {
		if line == "Repository participants" {
			inParticipation = true
			continue
		}

		if line == "Review matrix" {
			return nil
		}

		if !inParticipation {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) > 0 && fields[0] == participant {
			return fields
		}
	}

	return nil
}
