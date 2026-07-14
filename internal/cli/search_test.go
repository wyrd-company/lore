package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/wyrd-company/lore/internal/retrieval"
)

func TestSearchCommandReturnsCompleteJSONResponse(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if request.URL.Path != "/api/projects/lore/search" || query.Get("q") != "hybrid project knowledge" {
			t.Errorf("request URL = %s", request.URL.String())
		}
		assertSearchValues(t, query["sourceType"], []string{"task", "note", "repository"})
		assertSearchValues(t, query["repository"], []string{"wyrd-company/lore"})
		assertSearchValues(t, query["branch"], []string{"main"})
		assertSearchValues(t, query["tag"], []string{"search", "architecture"})
		assertSearchValues(t, query["createdFrom"], []string{"2026-01-01T00:00:00Z"})
		assertSearchValues(t, query["createdTo"], []string{"2026-12-31T23:59:59Z"})
		assertSearchValues(t, query["limit"], []string{"7"})
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(retrieval.Response{
			Query: query.Get("q"), Modes: retrieval.Modes{Keyword: true},
			Warnings: []string{"keyword only"}, Results: []retrieval.DocumentResult{},
		})
	}))
	t.Cleanup(server.Close)
	var output, errors bytes.Buffer
	err := New(&output, &errors).Run(context.Background(), []string{
		"search", "--config", filepath.Join(t.TempDir(), "missing.yml"),
		"--server", server.URL, "--project", "lore",
		"--source-type", "task,note", "--source-type", "repository",
		"--repository", "wyrd-company/lore", "--branch", "main",
		"--tag", "search,architecture", "--created-from", "2026-01-01T00:00:00Z",
		"--created-to", "2026-12-31T23:59:59Z", "--limit", "7",
		"hybrid", "project", "knowledge",
	})
	if err != nil {
		t.Fatal(err)
	}
	if errors.Len() != 0 {
		t.Fatalf("stderr = %q", errors.String())
	}
	var response retrieval.Response
	if err := json.Unmarshal(output.Bytes(), &response); err != nil {
		t.Fatalf("decode output: %v\n%s", err, output.String())
	}
	if response.Query != "hybrid project knowledge" || !response.Modes.Keyword || len(response.Warnings) != 1 || response.Results == nil {
		t.Fatalf("response = %#v", response)
	}
}

func TestSearchCommandValidatesRequiredInputs(t *testing.T) {
	t.Setenv("LORE_PROJECT", "")
	missingConfig := filepath.Join(t.TempDir(), "missing.yml")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "project", args: []string{"search", "--config", missingConfig, "knowledge"}, want: "--project or LORE_PROJECT is required"},
		{name: "query", args: []string{"search", "--config", missingConfig, "--project", "lore"}, want: "search query is required"},
		{name: "limit", args: []string{"search", "--config", missingConfig, "--project", "lore", "--limit", "0", "knowledge"}, want: "--limit must be a positive integer"},
		{name: "created from", args: []string{"search", "--config", missingConfig, "--project", "lore", "--created-from", "yesterday", "knowledge"}, want: "--created-from must be RFC3339"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			err := New(&output, &output).Run(context.Background(), test.args)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func assertSearchValues(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("query values = %#v, want %#v", got, want)
	}
}
