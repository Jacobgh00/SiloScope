package githubapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"reviewstats/internal/repository"
)

func TestListPullRequestsUpdatedSinceUsesExpectedQueryAndCapturesAuthors(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertPullRequestRequest(t, request, "1")

		_ = json.NewEncoder(writer).Encode([]pullResponse{
			newPullResponse(20, "alice", "open", since.Add(2*time.Hour)),
			newPullResponse(19, "bob", "closed", since),
		})
	}))
	defer server.Close()

	client := newPullRequestTestClient(t, server)

	got, err := client.ListPullRequestsUpdatedSince(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
		since,
	)
	if err != nil {
		t.Fatalf("ListPullRequestsUpdatedSince() error = %v", err)
	}

	want := []PullRequest{
		{Number: 20, Author: "alice", UpdatedAt: since.Add(2 * time.Hour)},
		{Number: 19, Author: "bob", UpdatedAt: since},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListPullRequestsUpdatedSince() = %#v, want %#v", got, want)
	}
}

func TestListPullRequestsUpdatedSinceFollowsFullPages(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	firstPage := make([]pullResponse, 100)
	for index := range firstPage {
		firstPage[index] = newPullResponse(
			200-index,
			"author",
			"open",
			since.Add(time.Duration(101-index)*time.Minute),
		)
	}
	secondPage := []pullResponse{
		newPullResponse(100, "author", "closed", since.Add(time.Minute)),
		newPullResponse(99, "author", "open", since),
	}

	var requestedPages []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page := request.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		assertPullRequestRequest(t, request, page)

		switch page {
		case "1":
			_ = json.NewEncoder(writer).Encode(firstPage)
		case "2":
			_ = json.NewEncoder(writer).Encode(secondPage)
		default:
			http.Error(writer, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := newPullRequestTestClient(t, server)

	got, err := client.ListPullRequestsUpdatedSince(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
		since,
	)
	if err != nil {
		t.Fatalf("ListPullRequestsUpdatedSince() error = %v", err)
	}

	if want := []string{"1", "2"}; !reflect.DeepEqual(requestedPages, want) {
		t.Fatalf("requested pages = %#v, want %#v", requestedPages, want)
	}

	if len(got) != 102 {
		t.Fatalf("pull request count = %d, want %d", len(got), 102)
	}

	if got[0].Number != 200 || got[99].Number != 101 || got[100].Number != 100 || got[101].Number != 99 {
		t.Fatalf("pull request order = %#v, want API update order", []int{got[0].Number, got[99].Number, got[100].Number, got[101].Number})
	}
}

func TestListPullRequestsUpdatedSinceStopsAtOlderPullRequest(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	firstPage := make([]pullResponse, 100)
	for index := 0; index < 99; index++ {
		firstPage[index] = newPullResponse(
			100-index,
			"author",
			"open",
			since.Add(time.Duration(100-index)*time.Minute),
		)
	}
	firstPage[99] = newPullResponse(1, "author", "closed", since.Add(-time.Second))

	var requestedPages []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page := request.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		assertPullRequestRequest(t, request, page)

		if page != "1" {
			http.Error(writer, "unexpected page", http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(writer).Encode(firstPage)
	}))
	defer server.Close()

	client := newPullRequestTestClient(t, server)

	got, err := client.ListPullRequestsUpdatedSince(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
		since,
	)
	if err != nil {
		t.Fatalf("ListPullRequestsUpdatedSince() error = %v", err)
	}

	if want := []string{"1"}; !reflect.DeepEqual(requestedPages, want) {
		t.Fatalf("requested pages = %#v, want %#v", requestedPages, want)
	}

	if len(got) != 99 {
		t.Fatalf("pull request count = %d, want %d", len(got), 99)
	}

	if got[len(got)-1].Number != 2 {
		t.Fatalf("last pull request number = %d, want %d", got[len(got)-1].Number, 2)
	}
}

type pullResponse struct {
	Number    int       `json:"number"`
	User      pullUser  `json:"user"`
	UpdatedAt time.Time `json:"updated_at"`
	State     string    `json:"state"`
}

type pullUser struct {
	Login string `json:"login"`
}

func newPullResponse(number int, author, state string, updatedAt time.Time) pullResponse {
	return pullResponse{
		Number:    number,
		User:      pullUser{Login: author},
		UpdatedAt: updatedAt,
		State:     state,
	}
}

func newPullRequestTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	return client
}

func assertPullRequestRequest(t *testing.T, request *http.Request, wantPage string) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %q, want %q", request.Method, http.MethodGet)
	}

	if request.URL.Path != "/repos/acme/frontend/pulls" {
		t.Errorf("path = %q, want %q", request.URL.Path, "/repos/acme/frontend/pulls")
	}

	for key, want := range map[string]string{
		"state":     "all",
		"sort":      "updated",
		"direction": "desc",
		"per_page":  "100",
		"page":      wantPage,
	} {
		if got := request.URL.Query().Get(key); got != want {
			t.Errorf("%s query = %q, want %q", key, got, want)
		}
	}
}
