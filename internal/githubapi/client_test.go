package githubapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewClientUsesGitHubDefaults(t *testing.T) {
	t.Parallel()

	client := NewClient("test-token", nil)

	if got := client.baseURL.String(); got != "https://api.github.com/" {
		t.Fatalf("base URL = %q, want %q", got, "https://api.github.com/")
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

func TestClientGetSendsGitHubRequestContract(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %q, want %q", request.Method, http.MethodGet)
		}

		if request.URL.Path != "/repos/acme/frontend" {
			t.Errorf("path = %q, want %q", request.URL.Path, "/repos/acme/frontend")
		}

		if got := request.URL.Query().Get("state"); got != "all" {
			t.Errorf("state query = %q, want %q", got, "all")
		}

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

		writer.Header().Set("X-RateLimit-Remaining", "4999")
		_ = json.NewEncoder(writer).Encode(struct {
			Login string `json:"login"`
		}{Login: "octocat"})
	}))
	defer server.Close()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	var destination struct {
		Login string `json:"login"`
	}

	headers, err := client.get(
		context.Background(),
		"/repos/acme/frontend",
		url.Values{"state": {"all"}},
		&destination,
	)
	if err != nil {
		t.Fatalf("get() error = %v", err)
	}

	if destination.Login != "octocat" {
		t.Fatalf("decoded login = %q, want %q", destination.Login, "octocat")
	}

	if got := headers.Get("X-RateLimit-Remaining"); got != "4999" {
		t.Fatalf("response header = %q, want %q", got, "4999")
	}
}

func TestClientGetReturnsTokenSafeAPIError(t *testing.T) {
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

	_, err = client.get(context.Background(), "/repos/acme/missing", nil, &struct{}{})
	if err == nil {
		t.Fatal("get() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "404 Not Found") {
		t.Fatalf("get() error = %q, want HTTP status", err)
	}

	if !strings.Contains(err.Error(), "Repository unavailable") {
		t.Fatalf("get() error = %q, want GitHub message", err)
	}

	if strings.Contains(err.Error(), "test-token") {
		t.Fatalf("get() error = %q, must not contain token", err)
	}
}
