package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/wyrd-company/lore/internal/indexing"
)

const endpoint = "https://ai-gateway.vercel.sh/v1/embeddings"
const maxBatchSize = 32

type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("AI_GATEWAY_API_KEY is required")
	}
	return &Client{apiKey: apiKey, httpClient: &http.Client{Timeout: 45 * time.Second}}, nil
}

func (c *Client) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	result := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += maxBatchSize {
		end := min(start+maxBatchSize, len(inputs))
		batch, err := c.embedBatch(ctx, inputs[start:end])
		if err != nil {
			return nil, err
		}
		result = append(result, batch...)
	}
	return result, nil
}

func (c *Client) embedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	payload, err := json.Marshal(map[string]any{
		"model": indexing.Model, "input": inputs, "dimensions": indexing.Dimensions,
	})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	delay := 250 * time.Millisecond
	for attempt := 1; attempt <= 4; attempt++ {
		vectors, retryAfter, retryable, err := c.request(ctx, payload, len(inputs))
		if err == nil {
			return vectors, nil
		}
		if !retryable || attempt == 4 {
			return nil, err
		}
		if retryAfter > delay {
			delay = retryAfter
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, 4*time.Second)
	}
	return nil, fmt.Errorf("embedding retry loop exhausted")
}

func (c *Client) request(ctx context.Context, payload []byte, expected int) ([][]float32, time.Duration, bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, 0, false, err
	}
	request.Header.Set("Authorization", "Bearer "+c.apiKey)
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, true, fmt.Errorf("request embeddings: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 16<<10))
		retryable := response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusConflict ||
			response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500
		return nil, parseRetryAfter(response.Header.Get("Retry-After")), retryable,
			fmt.Errorf("AI Gateway returned %s: %s", response.Status, string(bytes.TrimSpace(body)))
	}
	var result struct {
		Data []struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<20)).Decode(&result); err != nil {
		return nil, 0, true, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(result.Data) != expected {
		return nil, 0, true, fmt.Errorf("AI Gateway returned %d embeddings for %d inputs", len(result.Data), expected)
	}
	vectors := make([][]float32, expected)
	seen := make([]bool, expected)
	for _, item := range result.Data {
		if item.Index < 0 || item.Index >= expected || seen[item.Index] || len(item.Embedding) != indexing.Dimensions {
			return nil, 0, false, fmt.Errorf("AI Gateway returned an invalid embedding at index %d with %d dimensions", item.Index, len(item.Embedding))
		}
		seen[item.Index] = true
		vectors[item.Index] = item.Embedding
	}
	return vectors, 0, false, nil
}

func parseRetryAfter(value string) time.Duration {
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}
