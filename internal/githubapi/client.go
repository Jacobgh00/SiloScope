package githubapi

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
)

const (
	defaultBaseURL   = "https://api.github.com/"
	defaultTimeout   = 20 * time.Second
	maxErrorBodySize = 32 * 1024
)

type Client struct {
	graphqlURL *url.URL
	token      string
	httpClient *http.Client
}

type graphQLRequest struct {
	Query     string `json:"query"`
	Variables any    `json:"variables"`
}

type graphQLResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []graphQLError  `json:"errors"`
}

type graphQLError struct {
	Message string `json:"message"`
}

func NewClient(token string, httpClient *http.Client) *Client {
	client, err := NewClientWithBaseURL(token, httpClient, defaultBaseURL)
	if err != nil {
		panic("invalid default GitHub API base URL")
	}

	return client
}

func NewClientWithBaseURL(token string, httpClient *http.Client, baseURL string) (*Client, error) {
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse GitHub API base URL: %w", err)
	}

	if parsedBaseURL.Scheme == "" || parsedBaseURL.Host == "" {
		return nil, fmt.Errorf("GitHub API base URL must be absolute")
	}

	if !strings.HasSuffix(parsedBaseURL.Path, "/") {
		parsedBaseURL.Path += "/"
	}

	if httpClient == nil {
		httpClient = &http.Client{Timeout: defaultTimeout}
	}

	return &Client{
		graphqlURL: parsedBaseURL.ResolveReference(&url.URL{Path: "/graphql"}),
		token:      token,
		httpClient: httpClient,
	}, nil
}

func (c *Client) graphql(ctx context.Context, query string, variables any, destination any) error {
	payload, err := json.Marshal(graphQLRequest{
		Query:     query,
		Variables: variables,
	})
	if err != nil {
		return fmt.Errorf("encode GitHub GraphQL request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.graphqlURL.String(),
		bytes.NewReader(payload),
	)
	if err != nil {
		return fmt.Errorf("create GitHub GraphQL request: %w", err)
	}

	c.setGitHubHeaders(request)
	request.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("send GitHub GraphQL request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return c.responseError(response)
	}

	var envelope graphQLResponse
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return fmt.Errorf("decode GitHub GraphQL response: %w", err)
	}

	if len(envelope.Errors) > 0 {
		messages := make([]string, 0, len(envelope.Errors))
		for _, graphQLError := range envelope.Errors {
			if graphQLError.Message != "" {
				messages = append(messages, graphQLError.Message)
			}
		}

		message := strings.Join(messages, "; ")
		if message == "" {
			message = "unknown GraphQL error"
		}

		return fmt.Errorf("GitHub GraphQL request failed: %s", c.redactToken(message))
	}

	if destination == nil {
		return nil
	}

	if len(envelope.Data) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data), []byte("null")) {
		return fmt.Errorf("GitHub GraphQL response did not contain data")
	}

	if err := json.Unmarshal(envelope.Data, destination); err != nil {
		return fmt.Errorf("decode GitHub GraphQL data: %w", err)
	}

	return nil
}

func (c *Client) responseError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodySize))

	var apiError struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Message != "" {
		return fmt.Errorf("GitHub API request failed: %s: %s", response.Status, c.redactToken(apiError.Message))
	}

	return fmt.Errorf("GitHub API request failed: %s", response.Status)
}

func (c *Client) setGitHubHeaders(request *http.Request) {
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "reviewstats")
}

func (c *Client) redactToken(value string) string {
	if c.token == "" {
		return value
	}

	return strings.ReplaceAll(value, c.token, "[REDACTED]")
}
