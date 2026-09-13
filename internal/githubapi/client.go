package githubapi

import (
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
	baseURL    *url.URL
	token      string
	httpClient *http.Client
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
		baseURL:    parsedBaseURL,
		token:      token,
		httpClient: httpClient,
	}, nil
}

func (c *Client) get(ctx context.Context, path string, query url.Values, destination any) (http.Header, error) {
	requestURL := c.baseURL.ResolveReference(&url.URL{Path: path})
	requestURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create GitHub API request: %w", err)
	}

	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("X-GitHub-Api-Version", "2026-03-10")
	request.Header.Set("User-Agent", "reviewstats")

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("send GitHub API request: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return response.Header, c.responseError(response)
	}

	if destination == nil {
		return response.Header, nil
	}

	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		return response.Header, fmt.Errorf("decode GitHub API response: %w", err)
	}

	return response.Header, nil
}

func (c *Client) responseError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodySize))

	var apiError struct {
		Message string `json:"message"`
	}

	if err := json.Unmarshal(body, &apiError); err == nil && apiError.Message != "" {
		message := apiError.Message
		if c.token != "" {
			message = strings.ReplaceAll(message, c.token, "[REDACTED]")
		}

		return fmt.Errorf("GitHub API request failed: %s: %s", response.Status, message)
	}

	return fmt.Errorf("GitHub API request failed: %s", response.Status)
}
