package content

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.opentelemetry.io/otel"
	otelCodes "go.opentelemetry.io/otel/codes"
	"io"
	"net/http"
	"strings"
	"time"

	"syntopica-backend/internal/platform/tracing"
)

type FirecrawlConfig struct {
	APIUrl           string `json:"api_url"`
	APIKey           string `json:"api_key"`
	Enabled          bool   `json:"enabled"`
	Mode             string `json:"mode"`
	Timeout          int    `json:"timeout"`
	MaxContentLength int    `json:"max_content_length"`
}

type FirecrawlService struct {
	config *FirecrawlConfig
	client *http.Client
}

type ScrapeResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Content    string `json:"content"`
		HTML       string `json:"html"`
		Markdown   string `json:"markdown"`
		Screenshot string `json:"screenshot,omitempty"`
		Metadata   struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			Language    string `json:"language"`
			SourceURL   string `json:"sourceURL"`
		} `json:"metadata"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

func NewFirecrawlService(config *FirecrawlConfig) *FirecrawlService {
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = 60
	}

	return &FirecrawlService{
		config: config,
		client: &http.Client{
			Timeout: time.Duration(timeout) * time.Second,
		},
	}
}

func (s *FirecrawlService) ScrapePage(ctx context.Context, url string) (result *ScrapeResponse, err error) {
	_, span := otel.Tracer(tracing.ServiceName).Start(ctx, "FirecrawlService.ScrapePage")
	defer span.End()
	defer func() {
		if err != nil {
			span.SetStatus(otelCodes.Error, "error")
			span.RecordError(err)
		}
	}()
	/*line backend-go/internal/domain/contentprocessing/firecrawl_service.go:60:2*/ requestBody := map[string]interface{}{
		"url": url,
	}

	if s.config.Mode == "scrape" {
		requestBody["formats"] = []string{"markdown", "html"}
	}

	if s.config.Timeout > 0 {
		requestBody["timeout"] = s.config.Timeout * 1000
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	endpoint := buildFirecrawlEndpoint(s.config.APIUrl, "/v1/scrape")
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	httpResp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer func() { _ = httpResp.Body.Close() }()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		err := fmt.Errorf("firecrawl API error: %s", string(respBody))
		return nil, err
	}

	var scrapeResp ScrapeResponse
	if err := json.Unmarshal(respBody, &scrapeResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if !scrapeResp.Success {
		err := fmt.Errorf("firecrawl scrape failed: %s", scrapeResp.Error)
		return nil, err
	}

	maxLength := s.config.MaxContentLength
	if maxLength <= 0 {
		maxLength = 50000
	}

	if len(scrapeResp.Data.Markdown) > maxLength {
		scrapeResp.Data.Markdown = scrapeResp.Data.Markdown[:maxLength]
	}

	return &scrapeResp, nil
}

func buildFirecrawlEndpoint(baseURL, path string) string {
	trimmedBaseURL := strings.TrimRight(baseURL, "/")
	if strings.HasSuffix(trimmedBaseURL, "/v1") && strings.HasPrefix(path, "/v1/") {
		path = strings.TrimPrefix(path, "/v1")
	}

	return trimmedBaseURL + path
}

func (s *FirecrawlService) ShouldUseFirecrawl(globalEnabled, feedEnabled bool, articleContent string) bool {
	if !s.config.Enabled || !globalEnabled {
		return false
	}

	if !feedEnabled {
		return false
	}

	if len(articleContent) > 2000 {
		return false
	}

	return true
}
