package content

import "syntopica-backend/internal/platform/aisettings"

func GetFirecrawlConfig() (*FirecrawlConfig, error) {
	settingsData, _, err := aisettings.LoadFirecrawlConfig()
	if err != nil {
		return nil, err
	}

	firecrawlData := settingsData
	if len(firecrawlData) == 0 {
		legacy, _, legacyErr := aisettings.LoadSummaryConfig()
		if legacyErr != nil {
			return nil, legacyErr
		}
		if nested, ok := legacy["firecrawl"].(map[string]interface{}); ok {
			firecrawlData = nested
		} else {
			return nil, ErrFirecrawlConfigMissing
		}
	}

	config := &FirecrawlConfig{
		APIUrl:           getStringValue(firecrawlData, "api_url"),
		APIKey:           getStringValue(firecrawlData, "api_key"),
		Enabled:          getBoolValue(firecrawlData, "enabled"),
		Mode:             getStringValue(firecrawlData, "mode"),
		Timeout:          getIntValue(firecrawlData, "timeout"),
		MaxContentLength: getIntValue(firecrawlData, "max_content_length"),
	}

	if config.Timeout <= 0 {
		config.Timeout = 60
	}
	if config.MaxContentLength <= 0 {
		config.MaxContentLength = 50000
	}

	return config, nil
}

func GetFirecrawlAPIKey(data map[string]interface{}) string {
	return getStringValue(data, "api_key")
}
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func getBoolValue(m map[string]interface{}, key string) bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return b
		}
		switch v := val.(type) {
		case float64:
			return v != 0
		case int:
			return v != 0
		}
	}
	return false
}

func getIntValue(m map[string]interface{}, key string) int {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return int(v)
		case int:
			return v
		}
	}
	return 0
}

var (
	ErrInvalidSettingsFormat  = &FirecrawlError{Message: "invalid settings format"}
	ErrFirecrawlConfigMissing = &FirecrawlError{Message: "firecrawl configuration missing"}
)

type FirecrawlError struct {
	Message string
}

func (e *FirecrawlError) Error() string {
	return e.Message
}
