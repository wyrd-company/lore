package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wyrd-company/lore/internal/annotations"
	"github.com/wyrd-company/lore/internal/projects"
	"github.com/wyrd-company/lore/internal/retrieval"
	"github.com/wyrd-company/lore/internal/synchronization"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func (c *Client) CreateProject(ctx context.Context, request projects.CreateRequest) (projects.Project, error) {
	var project projects.Project
	if err := c.doJSON(ctx, http.MethodPost, "/api/projects", request, &project); err != nil {
		return project, err
	}
	return project, nil
}

func (c *Client) doJSON(ctx context.Context, method, path string, bodyValue, result any) error {
	body, err := json.Marshal(bodyValue)
	if err != nil {
		return fmt.Errorf("encode request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("request Lore: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		var problem struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(limited, &problem) == nil && problem.Detail != "" {
			return fmt.Errorf("Lore server returned %s: %s", response.Status, problem.Detail)
		}
		return fmt.Errorf("Lore server returned %s", response.Status)
	}
	if result == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(result); err != nil {
		return fmt.Errorf("decode Lore response: %w", err)
	}
	return nil
}

func New(baseURL, token string) (*Client, error) {
	return newClient(baseURL, token, true)
}

func NewViewer(baseURL string) (*Client, error) {
	return newClient(baseURL, "", false)
}

func newClient(baseURL, token string, requireToken bool) (*Client, error) {
	baseURL, _, err := NormalizeServerURL(baseURL)
	if err != nil {
		return nil, err
	}
	if requireToken && token == "" {
		return nil, fmt.Errorf("Lore ingest token is required")
	}
	return &Client{baseURL: baseURL, token: token, httpClient: &http.Client{Timeout: 60 * time.Second}}, nil
}

func NormalizeServerURL(baseURL string) (normalized string, assumedHTTP bool, err error) {
	baseURL = strings.TrimSpace(baseURL)
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
		assumedHTTP = true
	}
	parsed, parseErr := url.Parse(baseURL)
	if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", false, fmt.Errorf("invalid Lore server URL %q", baseURL)
	}
	return strings.TrimRight(baseURL, "/"), assumedHTTP, nil
}

func (c *Client) ExportAnnotations(ctx context.Context, project string, after int64) (annotations.Export, error) {
	var export annotations.Export
	endpoint := c.baseURL + "/api/projects/" + url.PathEscape(project) + "/annotations/export?after=" + fmt.Sprint(after)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return export, fmt.Errorf("create annotation export request: %w", err)
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return export, fmt.Errorf("export annotations from Lore: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		var problem struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(limited, &problem) == nil && problem.Detail != "" {
			return export, fmt.Errorf("Lore server returned %s: %s", response.Status, problem.Detail)
		}
		return export, fmt.Errorf("Lore server returned %s", response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&export); err != nil {
		return export, fmt.Errorf("decode annotation export: %w", err)
	}
	return export, nil
}

func (c *Client) Search(ctx context.Context, project string, searchRequest retrieval.Request) (retrieval.Response, error) {
	var result retrieval.Response
	query := url.Values{}
	query.Set("q", searchRequest.Query)
	for _, sourceType := range searchRequest.Filters.SourceTypes {
		query.Add("sourceType", sourceType)
	}
	for _, repository := range searchRequest.Filters.Repositories {
		query.Add("repository", repository)
	}
	for _, branch := range searchRequest.Filters.Branches {
		query.Add("branch", branch)
	}
	for _, tag := range searchRequest.Filters.Tags {
		query.Add("tag", tag)
	}
	if searchRequest.Filters.CreatedFrom != nil {
		query.Set("createdFrom", searchRequest.Filters.CreatedFrom.Format(time.RFC3339))
	}
	if searchRequest.Filters.CreatedTo != nil {
		query.Set("createdTo", searchRequest.Filters.CreatedTo.Format(time.RFC3339))
	}
	if searchRequest.Limit > 0 {
		query.Set("limit", fmt.Sprint(searchRequest.Limit))
	}
	endpoint := c.baseURL + "/api/projects/" + url.PathEscape(project) + "/search?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return result, fmt.Errorf("create search request: %w", err)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return result, fmt.Errorf("search Lore: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		var problem struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(limited, &problem) == nil && problem.Detail != "" {
			return result, fmt.Errorf("Lore server returned %s: %s", response.Status, problem.Detail)
		}
		return result, fmt.Errorf("Lore server returned %s", response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&result); err != nil {
		return result, fmt.Errorf("decode search response: %w", err)
	}
	return result, nil
}

func (c *Client) Synchronize(ctx context.Context, manifest synchronization.Manifest) (synchronization.Result, error) {
	body, err := json.Marshal(manifest)
	if err != nil {
		return synchronization.Result{}, fmt.Errorf("encode synchronization manifest: %w", err)
	}
	endpoint := c.baseURL + "/api/projects/" + url.PathEscape(manifest.Project) + "/synchronizations"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return synchronization.Result{}, fmt.Errorf("create synchronization request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return synchronization.Result{}, fmt.Errorf("synchronize with Lore: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		limited, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		var problem struct {
			Detail string `json:"detail"`
		}
		if json.Unmarshal(limited, &problem) == nil && problem.Detail != "" {
			return synchronization.Result{}, fmt.Errorf("Lore server returned %s: %s", response.Status, problem.Detail)
		}
		return synchronization.Result{}, fmt.Errorf("Lore server returned %s", response.Status)
	}
	var result synchronization.Result
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return synchronization.Result{}, fmt.Errorf("decode synchronization result: %w", err)
	}
	return result, nil
}
