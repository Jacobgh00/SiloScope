package githubapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"reviewstats/internal/repository"
)

func TestListCollaboratorsReturnsUniqueSortedLogins(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertCollaboratorRequest(t, request, "1")

		_ = json.NewEncoder(writer).Encode([]Collaborator{
			{Login: "charlie"},
			{Login: ""},
			{Login: "alice"},
			{Login: "charlie"},
		})
	}))
	defer server.Close()

	client := newCollaboratorTestClient(t, server)

	got, err := client.ListCollaborators(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
	)
	if err != nil {
		t.Fatalf("ListCollaborators() error = %v", err)
	}

	want := []Collaborator{{Login: "alice"}, {Login: "charlie"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListCollaborators() = %#v, want %#v", got, want)
	}
}

func TestListCollaboratorsFollowsFullPages(t *testing.T) {
	t.Parallel()

	firstPage := make([]Collaborator, 100)
	for index := range firstPage {
		firstPage[index] = Collaborator{Login: "bravo"}
	}

	var requestedPages []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		page := request.URL.Query().Get("page")
		requestedPages = append(requestedPages, page)
		assertCollaboratorRequest(t, request, page)

		switch page {
		case "1":
			_ = json.NewEncoder(writer).Encode(firstPage)
		case "2":
			_ = json.NewEncoder(writer).Encode([]Collaborator{
				{Login: "bravo"},
				{Login: "alpha"},
				{Login: ""},
			})
		default:
			http.Error(writer, "unexpected page", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := newCollaboratorTestClient(t, server)

	got, err := client.ListCollaborators(
		context.Background(),
		repository.Repository{Owner: "acme", Name: "frontend"},
	)
	if err != nil {
		t.Fatalf("ListCollaborators() error = %v", err)
	}

	if want := []string{"1", "2"}; !reflect.DeepEqual(requestedPages, want) {
		t.Fatalf("requested pages = %#v, want %#v", requestedPages, want)
	}

	want := []Collaborator{{Login: "alpha"}, {Login: "bravo"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ListCollaborators() = %#v, want %#v", got, want)
	}
}

func newCollaboratorTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()

	client, err := NewClientWithBaseURL("test-token", server.Client(), server.URL)
	if err != nil {
		t.Fatalf("NewClientWithBaseURL() error = %v", err)
	}

	return client
}

func assertCollaboratorRequest(t *testing.T, request *http.Request, wantPage string) {
	t.Helper()

	if request.Method != http.MethodGet {
		t.Errorf("method = %q, want %q", request.Method, http.MethodGet)
	}

	if request.URL.Path != "/repos/acme/frontend/collaborators" {
		t.Errorf("path = %q, want %q", request.URL.Path, "/repos/acme/frontend/collaborators")
	}

	if got := request.URL.Query().Get("per_page"); got != "100" {
		t.Errorf("per_page query = %q, want %q", got, "100")
	}

	if got := request.URL.Query().Get("page"); got != wantPage {
		t.Errorf("page query = %q, want %q", got, wantPage)
	}
}
