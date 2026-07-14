package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/wyrd-company/lore/internal/retrieval"
)

func TestSearchEncodesProjectQueryAndFilters(t *testing.T) {
	t.Parallel()
	createdFrom := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	createdTo := time.Date(2026, time.December, 31, 23, 59, 59, 0, time.UTC)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.EscapedPath() != "/api/projects/project%20one/search" {
			t.Errorf("request = %s %s", request.Method, request.URL.EscapedPath())
		}
		query := request.URL.Query()
		assertQueryValues(t, query["q"], []string{"project knowledge"})
		assertQueryValues(t, query["sourceType"], []string{"note", "repository"})
		assertQueryValues(t, query["repository"], []string{"wyrd-company/lore"})
		assertQueryValues(t, query["branch"], []string{"main"})
		assertQueryValues(t, query["tag"], []string{"search", "architecture"})
		assertQueryValues(t, query["createdFrom"], []string{createdFrom.Format(time.RFC3339)})
		assertQueryValues(t, query["createdTo"], []string{createdTo.Format(time.RFC3339)})
		assertQueryValues(t, query["limit"], []string{"7"})
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(retrieval.Response{Query: query.Get("q"), Results: []retrieval.DocumentResult{}})
	}))
	t.Cleanup(server.Close)

	api, err := NewViewer(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	response, err := api.Search(context.Background(), "project one", retrieval.Request{
		Query: "project knowledge",
		Filters: retrieval.Filters{
			SourceTypes: []string{"note", "repository"}, Repositories: []string{"wyrd-company/lore"},
			Branches: []string{"main"}, Tags: []string{"search", "architecture"},
			CreatedFrom: &createdFrom, CreatedTo: &createdTo,
		},
		Limit: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if response.Query != "project knowledge" || response.Results == nil {
		t.Fatalf("response = %#v", response)
	}
}

func assertQueryValues(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("query values = %#v, want %#v", got, want)
	}
}
