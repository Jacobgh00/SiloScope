package githubapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClientUsesGitHubDefaults(t *testing.T) {
	t.Parallel()

	client := NewClient("test-token", nil)

	if got := client.graphqlURL.String(); got != "https://api.github.com/graphql" {
		t.Fatalf("GraphQL URL = %q, want %q", got, "https://api.github.com/graphql")
	}

	if got := client.httpClient.Timeout; got != 20*time.Second {
		t.Fatalf("HTTP timeout = %v, want %v", got, 20*time.Second)
	}
}

func TestNewClientWithBaseURLRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	if _, err := NewClientWithBaseURL("test-token", nil, "://invalid"); err == nil {
		t.Fatal("NewClientWithBaseURL() error = nil, want error")
	}
}

func TestClientGraphQLSendsGitHubRequestContract(t *testing.T) {
	t.Parallel()

	const query = "query Viewer { viewer { login } }"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", request.Method, http.MethodPost)
		}

		if request.URL.Path != "/graphql" {
			t.Errorf("path = %q, want %q", request.URL.Path, "/graphql")
		}

		if got := request.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		assertGitHubHeaders(t, request)

		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatalf("decode GraphQL request = %v", err)
		}

		if payload.Query != query {
			t.Errorf("query = %q, want %q", payload.Query, query)
		}

		if got := payload.Variables["owner"]; got != "acme" {
			t.Errorf("owner variable = %#v, want %q", got, "acme")
		}

		_ = json.NewEncoder(writer).Encode(map[string]any{
			"data": map[string]any{
				"viewer": map[string]string{"login": "octocat"},
			},
		})
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	var destination struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}

	err = client.graphql(
		context.Background(),
		query,
		map[string]string{"owner": "acme"},
		&destination,
	)
	if err != nil {
		t.Fatalf("graphql() error = %v", err)
	}

	if destination.Viewer.Login != "octocat" {
		t.Fatalf("decoded login = %q, want %q", destination.Viewer.Login, "octocat")
	}
}

func TestClientGraphQLReturnsTokenSafeEnvelopeError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"errors": []map[string]string{{"message": "bad credentials for test-token"}},
		})
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	err = client.graphql(context.Background(), "query { viewer { login } }", nil, &struct{}{})
	if err == nil {
		t.Fatal("graphql() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "GitHub GraphQL request failed") {
		t.Fatalf("graphql() error = %q, want GraphQL error context", err)
	}

	if strings.Contains(err.Error(), "test-token") {
		t.Fatalf("graphql() error = %q, must not contain token", err)
	}
}

func TestClientGraphQLReturnsTokenSafeAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
		_, _ = writer.Write([]byte(`{"message":"Repository unavailable for test-token"}`))
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	err = client.graphql(context.Background(), "query { viewer { login } }", nil, &struct{}{})
	if err == nil {
		t.Fatal("graphql() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("graphql() error = %q, want HTTP status", err)
	}

	if !strings.Contains(err.Error(), "Repository unavailable") {
		t.Fatalf("graphql() error = %q, want GitHub message", err)
	}

	if strings.Contains(err.Error(), "test-token") {
		t.Fatalf("graphql() error = %q, must not contain token", err)
	}
}

func assertGitHubHeaders(t *testing.T, request *http.Request) {
	t.Helper()

	for header, want := range map[string]string{
		"Accept":               "application/vnd.github+json",
		"Authorization":        "Bearer test-token",
		"X-GitHub-Api-Version": "2026-03-10",
		"User-Agent":           "reviewstats",
	} {
		if got := request.Header.Get(header); got != want {
			t.Errorf("%s header = %q, want %q", header, got, want)
		}
	}
}
