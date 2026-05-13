package content

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Crawl4AIClient struct {
	BaseURL    string
	APIToken   string
	HTTPClient *http.Client
}

type CrawlRequest struct {
	URL             string `json:"url"`
	OnlyMainContent bool   `json:"only_main_content"`
	WaitFor         string `json:"wait_for,omitempty"`
	Timeout         int    `json:"timeout"`
}

type CrawlResponse struct {
	URL         string  `json:"url"`
	Success     bool    `json:"success"`
	Markdown    string  `json:"markdown,omitempty"`
	HTML        string  `json:"html,omitempty"`
	Title       string  `json:"title,omitempty"`
	Error       string  `json:"error,omitempty"`
	ElapsedTime float64 `json:"elapsed_time"`
}

func NewCrawl4AIClient(baseURL string) *Crawl4AIClient {
	return &Crawl4AIClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (c *Crawl4AIClient) SetAPIToken(token string) {
	c.APIToken = token
}

func (c *Crawl4AIClient) CrawlURL(url string, onlyMainContent bool) (*CrawlResponse, error) {
	req := CrawlRequest{
		URL:             url,
		OnlyMainContent: onlyMainContent,
		Timeout:         30,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.BaseURL+"/crawl", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIToken != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIToken)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("crawl service returned status %d: %s", resp.StatusCode, string(body))
	}

	var crawlResp CrawlResponse
	if err := json.Unmarshal(body, &crawlResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &crawlResp, nil
}

func (c *Crawl4AIClient) HealthCheck() error {
	req, err := http.NewRequest("GET", c.BaseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	if c.APIToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIToken)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
