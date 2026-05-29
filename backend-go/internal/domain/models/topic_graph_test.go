package models

import (
	"encoding/json"
	"testing"
)

func TestMetadataMapValueEncodesJSON(t *testing.T) {
	metadata := MetadataMap{
		"country": "瑞士",
		"domains": []string{
			"货币政策",
			"金融稳定",
		},
	}

	value, err := metadata.Value()
	if err != nil {
		t.Fatalf("Value() error = %v", err)
	}

	encoded, ok := value.(string)
	if !ok {
		t.Fatalf("Value() type = %T, want string", value)
	}

	var decoded map[string]any
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil {
		t.Fatalf("Value() produced invalid JSON: %v", err)
	}
	if decoded["country"] != "瑞士" {
		t.Fatalf("country = %v, want 瑞士", decoded["country"])
	}
}

func TestMetadataMapScanDecodesJSON(t *testing.T) {
	var metadata MetadataMap
	if err := metadata.Scan([]byte(`{"role":"行长","domains":["宏观经济分析"]}`)); err != nil {
		t.Fatalf("Scan() error = %v", err)
	}

	if metadata["role"] != "行长" {
		t.Fatalf("role = %v, want 行长", metadata["role"])
	}
}
