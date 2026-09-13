package githubapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"reviewstats/internal/repository"
)

func TestLoadReviewActivityNormalizesOuterPage(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := cutoff.Add(time.Hour)
	firstSubmittedAt := cutoff.Add(time.Minute)
	secondSubmittedAt := firstSubmittedAt.Add(time.Minute)
	thirdSubmittedAt := secondSubmittedAt.Add(time.Minute)
	fourthSubmittedAt := thirdSubmittedAt.Add(time.Minute)
	fifthSubmittedAt := fourthSubmittedAt.Add(time.Minute)

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload := decodeGraphQLRequest(t, request)
		for _, want := range []string{"query ReviewActivity", "first: 20", "reviews(first: 100)"} {
			if !strings.Contains(payload.Query, want) {
				t.Errorf("query = %q, want fragment %q", payload.Query, want)
			}
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

		writeOuterPage(writer, []any{
			newPullRequestNode(
				42,
				"alice",
				updatedAt,
				[]any{
					newReviewNode("bob", "approved", &firstSubmittedAt),
					newReviewNode("carol", "changes_requested", &secondSubmittedAt),
					newReviewNode("dana", "commented", &thirdSubmittedAt),
					newReviewNode("erin", "dismissed", &fourthSubmittedAt),
					newReviewNode("frank", "pending", nil),
					newReviewNode("", "approved", &fifthSubmittedAt),
				},
				false,
				nil,
			),
		}, false, nil)
	}))
	defer server.Close()

	client := newActivityTestClient(t, server)
	got, err := client.LoadReviewActivity(context.Background(), activityTestRepository(), cutoff)
	if err != nil {
		t.Fatalf("LoadReviewActivity() error = %v", err)
	}

	want := ReviewActivity{
		PullRequests: []PullRequest{{Number: 42, Author: "alice", UpdatedAt: updatedAt}},
		ReviewsByPull: map[int][]Review{
			42: {
				{PullRequestNumber: 42, Reviewer: "bob", State: "APPROVED", SubmittedAt: firstSubmittedAt},
				{PullRequestNumber: 42, Reviewer: "carol", State: "CHANGES_REQUESTED", SubmittedAt: secondSubmittedAt},
				{PullRequestNumber: 42, Reviewer: "dana", State: "COMMENTED", SubmittedAt: thirdSubmittedAt},
				{PullRequestNumber: 42, Reviewer: "erin", State: "DISMISSED", SubmittedAt: fourthSubmittedAt},
			},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadReviewActivity() = %#v, want %#v", got, want)
	}
}

func TestLoadReviewActivityFollowsOuterCursorAndStopsAtOlderPullRequest(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	firstPage := make([]any, 0, 20)
	for index := 0; index < 20; index++ {
		firstPage = append(firstPage, newPullRequestNode(
			100-index,
			"author",
			cutoff.Add(time.Duration(20-index)*time.Minute),
			nil,
			false,
			nil,
		))
	}

	var requestedAfters []any
	var requestedAftersMu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload := decodeGraphQLRequest(t, request)
		requestedAftersMu.Lock()
		requestedAfters = append(requestedAfters, payload.Variables["after"])
		requestCount := len(requestedAfters)
		requestedAftersMu.Unlock()

		switch requestCount {
		case 1:
			writeOuterPage(writer, firstPage, true, "page-1")
		case 2:
			writeOuterPage(writer, []any{
				newPullRequestNode(80, "author", cutoff, nil, false, nil),
				newPullRequestNode(79, "author", cutoff.Add(-time.Second), nil, false, nil),
			}, true, "page-2")
		default:
			t.Errorf("unexpected GraphQL request %d", len(requestedAfters))
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := newActivityTestClient(t, server)
	activity, err := client.LoadReviewActivity(context.Background(), activityTestRepository(), cutoff)
	if err != nil {
		t.Fatalf("LoadReviewActivity() error = %v", err)
	}

	requestedAftersMu.Lock()
	gotAfters := append([]any(nil), requestedAfters...)
	requestedAftersMu.Unlock()
	if want := []any{nil, "page-1"}; !reflect.DeepEqual(gotAfters, want) {
		t.Fatalf("request cursors = %#v, want %#v", gotAfters, want)
	}

	if len(activity.PullRequests) != 21 {
		t.Fatalf("pull request count = %d, want 21", len(activity.PullRequests))
	}

	if got := activity.PullRequests[len(activity.PullRequests)-1].Number; got != 80 {
		t.Fatalf("last pull request number = %d, want 80", got)
	}
}

func TestLoadReviewActivityFollowsReviewContinuationCursor(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	updatedAt := cutoff.Add(time.Hour)
	firstSubmittedAt := cutoff.Add(time.Minute)
	secondSubmittedAt := firstSubmittedAt.Add(time.Minute)
	thirdSubmittedAt := secondSubmittedAt.Add(time.Minute)
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		payload := decodeGraphQLRequest(t, request)
		requestCount++

		switch requestCount {
		case 1:
			if _, ok := payload.Variables["number"]; ok {
				t.Error("outer query unexpectedly included a pull request number")
			}
			if !strings.Contains(payload.Query, "query ReviewActivity") {
				t.Errorf("query = %q, want outer activity query", payload.Query)
			}
			writeOuterPage(writer, []any{
				newPullRequestNode(
					42,
					"alice",
					updatedAt,
					[]any{newReviewNode("bob", "approved", &firstSubmittedAt)},
					true,
					"reviews-1",
				),
			}, false, nil)
		case 2:
			if got := payload.Variables["number"]; got != float64(42) {
				t.Errorf("number variable = %#v, want 42", got)
			}
			if got := payload.Variables["after"]; got != "reviews-1" {
				t.Errorf("after variable = %#v, want %q", got, "reviews-1")
			}
			if !strings.Contains(payload.Query, "reviews(first: 100, after: $after)") {
				t.Errorf("query = %q, want review continuation query", payload.Query)
			}
			writeReviewPage(writer, []any{newReviewNode("carol", "changes_requested", &secondSubmittedAt)}, true, "reviews-2")
		case 3:
			if got := payload.Variables["number"]; got != float64(42) {
				t.Errorf("number variable = %#v, want 42", got)
			}
			if got := payload.Variables["after"]; got != "reviews-2" {
				t.Errorf("after variable = %#v, want %q", got, "reviews-2")
			}
			writeReviewPage(writer, []any{newReviewNode("dana", "dismissed", &thirdSubmittedAt)}, false, nil)
		default:
			t.Errorf("unexpected GraphQL request %d", requestCount)
			http.Error(writer, "unexpected request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := newActivityTestClient(t, server)
	activity, err := client.LoadReviewActivity(context.Background(), activityTestRepository(), cutoff)
	if err != nil {
		t.Fatalf("LoadReviewActivity() error = %v", err)
	}

	want := []Review{
		{PullRequestNumber: 42, Reviewer: "bob", State: "APPROVED", SubmittedAt: firstSubmittedAt},
		{PullRequestNumber: 42, Reviewer: "carol", State: "CHANGES_REQUESTED", SubmittedAt: secondSubmittedAt},
		{PullRequestNumber: 42, Reviewer: "dana", State: "DISMISSED", SubmittedAt: thirdSubmittedAt},
	}
	if got := activity.ReviewsByPull[42]; !reflect.DeepEqual(got, want) {
		t.Fatalf("reviews = %#v, want %#v", got, want)
	}
}

func TestLoadReviewActivityIncludesPullRequestNumberAndRedactsTokenForReviewContinuationErrors(t *testing.T) {
	t.Parallel()

	cutoff := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		decodeGraphQLRequest(t, request)
		requestCount++

		if requestCount == 1 {
			writeOuterPage(writer, []any{
				newPullRequestNode(42, "alice", cutoff, nil, true, "reviews-1"),
			}, false, nil)
			return
		}

		_ = json.NewEncoder(writer).Encode(map[string]any{
			"errors": []map[string]string{{"message": "unavailable for test-token"}},
		})
	}))
	defer server.Close()

	client := newActivityTestClient(t, server)
	_, err := client.LoadReviewActivity(context.Background(), activityTestRepository(), cutoff)
	if err == nil {
		t.Fatal("LoadReviewActivity() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "pull request #42") {
		t.Fatalf("LoadReviewActivity() error = %q, want pull request number", err)
	}

	if strings.Contains(err.Error(), "test-token") {
		t.Fatalf("LoadReviewActivity() error = %q, must not contain token", err)
	}
}

func TestLoadReviewActivityWrapsOuterQueryErrors(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		decodeGraphQLRequest(t, request)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"errors": []map[string]string{{"message": "repository unavailable for test-token"}},
		})
	}))
	defer server.Close()

	client := newActivityTestClient(t, server)
	_, err := client.LoadReviewActivity(context.Background(), activityTestRepository(), time.Now())
	if err == nil {
		t.Fatal("LoadReviewActivity() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "load review activity") {
		t.Fatalf("LoadReviewActivity() error = %q, want outer operation context", err)
	}

	if strings.Contains(err.Error(), "test-token") {
		t.Fatalf("LoadReviewActivity() error = %q, must not contain token", err)
	}
}

func TestLoadReviewActivityRejectsMissingOuterCursor(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		decodeGraphQLRequest(t, request)
		writeOuterPage(writer, nil, true, nil)
	}))
	defer server.Close()

	client := newActivityTestClient(t, server)
	_, err := client.LoadReviewActivity(context.Background(), activityTestRepository(), time.Now())
	if err == nil {
		t.Fatal("LoadReviewActivity() error = nil, want error")
	}

	for _, want := range []string{"load review activity", "next page without a cursor"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("LoadReviewActivity() error = %q, want %q", err, want)
		}
	}
}

type graphQLRequestPayload struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

func decodeGraphQLRequest(t *testing.T, request *http.Request) graphQLRequestPayload {
	t.Helper()

	if request.Method != http.MethodPost {
		t.Errorf("method = %q, want %q", request.Method, http.MethodPost)
	}

	if request.URL.Path != "/graphql" {
		t.Errorf("path = %q, want %q", request.URL.Path, "/graphql")
	}

	var payload graphQLRequestPayload
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		t.Fatalf("decode GraphQL request = %v", err)
	}

	return payload
}

func newActivityTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	return client
}

func activityTestRepository() repository.Repository {
	return repository.Repository{Owner: "acme", Name: "frontend"}
}

func newPullRequestNode(
	number int,
	author string,
	updatedAt time.Time,
	reviews []any,
	reviewsHasNextPage bool,
	reviewsEndCursor any,
) map[string]any {
	return map[string]any{
		"number":    number,
		"updatedAt": updatedAt,
		"author": map[string]string{
			"login": author,
		},
		"reviews": map[string]any{
			"nodes": reviews,
			"pageInfo": map[string]any{
				"hasNextPage": reviewsHasNextPage,
				"endCursor":   reviewsEndCursor,
			},
		},
	}
}

func newReviewNode(author, state string, submittedAt *time.Time) map[string]any {
	var reviewAuthor any
	if author != "" {
		reviewAuthor = map[string]string{"login": author}
	}

	return map[string]any{
		"author":      reviewAuthor,
		"state":       state,
		"submittedAt": submittedAt,
	}
}

func writeOuterPage(writer http.ResponseWriter, nodes []any, hasNextPage bool, endCursor any) {
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"data": map[string]any{
			"repository": map[string]any{
				"pullRequests": map[string]any{
					"nodes": nodes,
					"pageInfo": map[string]any{
						"hasNextPage": hasNextPage,
						"endCursor":   endCursor,
					},
				},
			},
		},
	})
}

func writeReviewPage(writer http.ResponseWriter, nodes []any, hasNextPage bool, endCursor any) {
	_ = json.NewEncoder(writer).Encode(map[string]any{
		"data": map[string]any{
			"repository": map[string]any{
				"pullRequest": map[string]any{
					"reviews": map[string]any{
						"nodes": nodes,
						"pageInfo": map[string]any{
							"hasNextPage": hasNextPage,
							"endCursor":   endCursor,
						},
					},
				},
			},
		},
	})
}
