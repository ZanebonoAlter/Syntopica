package repository

import (
	"strings"
	"testing"
	"time"

	"syntopica-backend/internal/models"
)

// Unit tests for the pure helpers of the description-repair storage seam
// (board-health-triage-advice seam B1): the three closed-set description
// rules and the opaque source-version token. No DB.

func TestValidateRepairDescription(t *testing.T) {
	const label = "上海"
	cases := []struct {
		name    string
		label   string
		input   string
		wantErr error
		want    string
	}{
		{name: "empty string", label: label, input: "", wantErr: ErrRepairDescriptionEmpty},
		{name: "ascii spaces only", label: label, input: "   ", wantErr: ErrRepairDescriptionEmpty},
		{name: "tab only", label: label, input: "\t\t", wantErr: ErrRepairDescriptionEmpty},
		{name: "full-width space only", label: label, input: "\u3000", wantErr: ErrRepairDescriptionEmpty},
		{name: "mixed whitespace only", label: label, input: " \t\u3000\n", wantErr: ErrRepairDescriptionEmpty},
		{name: "499 runes allowed", label: label, input: strings.Repeat("字", 499), want: strings.Repeat("字", 499)},
		{name: "500 runes allowed", label: label, input: strings.Repeat("字", 500), want: strings.Repeat("字", 500)},
		{name: "501 runes rejected", label: label, input: strings.Repeat("字", 501), wantErr: ErrRepairDescriptionTooLong},
		{name: "repeats label rejected", label: label, input: "上海", wantErr: ErrRepairDescriptionRepeatsLabel},
		{name: "repeats label after trim rejected", label: label, input: "  上海  ", wantErr: ErrRepairDescriptionRepeatsLabel},
		// The three-rule closed set: a single separator, a single token and
		// case variants stay legal — no extra blacklist may be added.
		{name: "single separator legal", label: label, input: "|", want: "|"},
		{name: "other single token legal", label: label, input: "直辖市", want: "直辖市"},
		{name: "case variant of ascii label legal", label: "OpenAI", input: "openai company", want: "openai company"},
		{name: "leading/trailing whitespace trimmed", label: label, input: "  中国东部沿海城市  ", want: "中国东部沿海城市"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateRepairDescription(tc.label, tc.input)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Fatalf("ValidateRepairDescription(%q) err = %v, want %v", tc.input, err, tc.wantErr)
				}
				if got != "" {
					t.Fatalf("ValidateRepairDescription(%q) on error returned %q, want empty", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateRepairDescription(%q) unexpected err = %v", tc.input, err)
			}
			if got != tc.want {
				t.Fatalf("ValidateRepairDescription(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestAuxiliaryVersionToken(t *testing.T) {
	base := models.SemanticLabel{
		ID: 7, Label: "上海", Description: "旧描述", Status: "active",
		UpdatedAt: time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC),
	}
	if got := AuxiliaryVersionToken(&base); !strings.HasPrefix(got, "v1_") || len(got) != len("v1_")+40 {
		t.Fatalf("AuxiliaryVersionToken format = %q (len %d), want v1_ + 40 hex chars", got, len(got))
	}
	if AuxiliaryVersionToken(&base) != AuxiliaryVersionToken(&base) {
		t.Fatal("token must be deterministic for the same label state")
	}

	mutations := map[string]func(*models.SemanticLabel){
		"label":       func(l *models.SemanticLabel) { l.Label = "上海滩" },
		"description": func(l *models.SemanticLabel) { l.Description = "新描述" },
		"status":      func(l *models.SemanticLabel) { l.Status = "disabled" },
		"updated_at":  func(l *models.SemanticLabel) { l.UpdatedAt = base.UpdatedAt.Add(time.Second) },
		"id":          func(l *models.SemanticLabel) { l.ID = 8 },
		"utc shift":   func(l *models.SemanticLabel) { l.UpdatedAt = base.UpdatedAt.Add(8 * time.Hour) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			mutated := base
			mutate(&mutated)
			if AuxiliaryVersionToken(&mutated) == AuxiliaryVersionToken(&base) {
				t.Fatalf("token must change when %s changes", name)
			}
		})
	}
}
