package githubapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"reviewstats/internal/repository"
)

func TestListReviewsDecodesCompleteReviewRecords(t *testing.T) {
	t.Parallel()

	firstSubmittedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	secondSubmittedAt := firstSubmittedAt.Add(time.Minute)
	thirdSubmittedAt := secondSubmittedAt.Add(time.Minute)
	fourthSubmittedAt := thirdSubmittedAt.Add(time.Minute)
	fifthSubmittedAt := fourthSubmittedAt.Add(time.Minute)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertReviewRequest(t, request, "1")

		_ = json.NewEncoder(writer).Encode([]reviewAPIResponse{
			newReviewAPIResponse("alice", "approved", &firstSubmittedAt),
			newReviewAPIResponse("bob", "changes_requested", &secondSubmittedAt),
			newReviewAPIResponse("charlie", "commented", &thirdSubmittedAt),
			newReviewAPIResponse("dana", "dismissed", &fourthSubmittedAt),
			newReviewAPIResponse("erin", "pending", nil),
			newReviewAPIResponse("", "approved", &fifthSubmittedAt),
			newReviewAPIResponse("alice", "approved", &fifthSubmittedAt),
		})
	}))
	defer server.Close()

	client := newReviewTestClient(t, server)

	got, err := client.ListReviews(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
		42,
	)
	if err != nil {
		t.Fatalf("ListReviews() error = %v", err)
	}

	want := []Review{
		{PullRequestNumber: 42, Reviewer: "alice", State: "APPROVED", SubmittedAt: firstSubmittedAt},
		{PullRequestNumber: 42, Reviewer: "bob", State: "CHANGES_REQUESTED", SubmittedAt: secondSubmittedAt},
		{PullRequestNumber: 42, Reviewer: "charlie", State: "COMMENTED", SubmittedAt: thirdSubmittedAt},
		{PullRequestNumber: 42, Reviewer: "dana", State: "DISMISSED", SubmittedAt: fourthSubmittedAt},
		{PullRequestNumber: 42, Reviewer: "alice", State: "APPROVED", SubmittedAt: fifthSubmittedAt},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListReviews() = %#v, want %#v", got, want)
	}
}

func TestListReviewsIncludesPullRequestNumberInRequestErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		http.Error(writer, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newReviewTestClient(t, server)
	_, err := client.ListReviews(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
		42,
	)

	if err == nil {
		t.Fatal("ListReviews() error = nil, want error")
	}

	for _, want := range []string{"pull request #42", "503 Service Unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ListReviews() error = %q, want %q", err, want)
		}
	}
}

func TestListReviewsFollowsFullPages(t *testing.T) {
	t.Parallel()

	submittedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	firstPage := make([]reviewAPIResponse, 100)
	for index := range firstPage {
		firstPage[index] = newReviewAPIResponse("alice", "commented", &submittedAt)
	}

	var requestedPages []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page := request.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		assertReviewRequest(t, request, page)

		switch page {
		case "1":
			_ = json.NewEncoder(writer).Encode(firstPage)
		case "2":
			_ = json.NewEncoder(writer).Encode([]reviewAPIResponse{
				newReviewAPIResponse("dana", "dismissed", &submittedAt),
			})
		default:
			http.Error(writer, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := newReviewTestClient(t, server)

	got, err := client.ListReviews(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
		42,
	)
	if err != nil {
		t.Fatalf("ListReviews() error = %v", err)
	}

	if want := []string{"1", "2"}; !reflect.DeepEqual(requestedPages, want) {
		t.Fatalf("requested pages = %#v, want %#v", requestedPages, want)
	}

	if len(got) != 101 {
		t.Fatalf("review count = %d, want %d", len(got), 101)
	}

	if got[0].Reviewer != "alice" || got[100].Reviewer != "dana" || got[100].State != "DISMISSED" {
		t.Fatalf("reviews = %#v, want API order with DISMISSED review", []Review{got[0], got[100]})
	}
}

type reviewAPIResponse struct {
	State       string        `json:"state"`
	SubmittedAt *time.Time    `json:"submitted_at"`
	User        reviewAPIUser `json:"user"`
}

type reviewAPIUser struct {
	Login string `json:"login"`
}

func newReviewAPIResponse(login, state string, submittedAt *time.Time) reviewAPIResponse {
	return reviewAPIResponse{
		State:       state,
		SubmittedAt: submittedAt,
		User:        reviewAPIUser{Login: login},
	}
}

func newReviewTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	return client
}

func assertReviewRequest(t *testing.T, request *http.Request, wantPage string) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %q, want %q", request.Method, http.MethodGet)
	}

	if request.URL.Path != "/repos/acme/frontend/pulls/42/reviews" {
		t.Errorf("path = %q, want %q", request.URL.Path, "/repos/acme/frontend/pulls/42/reviews")
	}

	if got := request.URL.Query().Get("per_page"); got != "100" {
		t.Errorf("per_page query = %q, want %q", got, "100")
	}

	if got := request.URL.Query().Get("page"); got != wantPage {
		t.Errorf("page query = %q, want %q", got, wantPage)
	}
}
