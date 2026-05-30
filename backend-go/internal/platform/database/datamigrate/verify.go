package datamigrate

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"
)

type Verifier struct {
	Source verificationSource
	Target verificationTarget
	Now    func() time.Time
}

type verificationSource interface {
	ExistingTables(ctx context.Context) (map[string]bool, error)
	CountRows(ctx context.Context, table string) (int64, error)
	SampleRows(ctx context.Context, spec TableSpec, limit int) ([]map[string]any, error)
}

type verificationTarget interface {
	ExistingTables(ctx context.Context) (map[string]bool, error)
	CountRows(ctx context.Context, table string) (int64, error)
	LoadSequenceState(ctx context.Context, spec TableSpec) (SequenceState, error)
	SampleRows(ctx context.Context, spec TableSpec, limit int) ([]map[string]any, error)
	LoadEmbeddingChecks(ctx context.Context, spec TableSpec, limit int) ([]EmbeddingVectorCheck, error)
}

func (v *Verifier) Verify(ctx context.Context, specs []TableSpec) (*VerificationReport, error) {
	if v == nil || v.Source == nil || v.Target == nil {
		return nil, fmt.Errorf("both source and target verifiers are required")
	}

	sourceTables, err := v.Source.ExistingTables(ctx)
	if err != nil {
		return nil, err
	}
	targetTables, err := v.Target.ExistingTables(ctx)
	if err != nil {
		return nil, err
	}

	now := time.Now
	if v.Now != nil {
		now = v.Now
	}

	report := &VerificationReport{CheckedAt: now()}
	for _, spec := range specs {
		if !sourceTables[spec.Name] || !targetTables[spec.Name] {
			continue
		}

		sourceCount, err := v.Source.CountRows(ctx, spec.Name)
		if err != nil {
			return nil, err
		}
		targetCount, err := v.Target.CountRows(ctx, spec.Name)
		if err != nil {
			return nil, err
		}

		report.CountChecks = append(report.CountChecks, TableCountCheck{
			Table:       spec.Name,
			SourceCount: sourceCount,
			TargetCount: targetCount,
		})
		report.CheckedTables = append(report.CheckedTables, spec.Name)

		if spec.PrimaryKey != "" {
			state, err := v.Target.LoadSequenceState(ctx, spec)
			if err != nil {
				return nil, err
			}
			if state.Sequence != "" {
				report.SequenceStates = append(report.SequenceStates, state)
			}
		}

		if len(spec.SampleColumns) > 0 && spec.PrimaryKey != "" {
			samples, err := v.compareSampleRows(ctx, spec, verificationSampleLimit)
			if err != nil {
				return nil, err
			}
			report.SampleChecks = append(report.SampleChecks, samples...)
		}

		if spec.Name == "topic_tag_embeddings" {
			embeddingChecks, err := v.compareEmbeddingVectors(ctx, spec, verificationSampleLimit)
			if err != nil {
				return nil, err
			}
			report.EmbeddingChecks = append(report.EmbeddingChecks, embeddingChecks...)
		}
	}

	if err := VerifyCountsMatch(report.CountChecks); err != nil {
		return report, err
	}

	for _, state := range report.SequenceStates {
		if err := VerifySequenceIsResetAfterImport(state); err != nil {
			return report, err
		}
	}

	for _, check := range report.CountChecks {
		if check.Table == "topic_tag_embeddings" {
			if err := VerifyEmbeddingRowCountMatches(check.SourceCount, check.TargetCount); err != nil {
				return report, err
			}
		}
	}

	if err := VerifyEmbeddingVectorValuesPresent(report.EmbeddingChecks); err != nil {
		return report, err
	}

	if err := VerifySampleRowsMatch(report.SampleChecks); err != nil {
		return report, err
	}

	return report, nil
}

func VerifyEmbeddingVectorValuesPresent(checks []EmbeddingVectorCheck) error {
	problems := make([]string, 0)
	for _, check := range checks {
		if strings.TrimSpace(check.SourceVector) == "" {
			continue
		}
		if strings.TrimSpace(check.TargetVector) == "" {
			problems = append(problems, fmt.Sprintf("%s[%v] missing target embedding vector", check.Table, check.PrimaryKey))
			continue
		}

		sourceNormalized, err := normalizeVectorLiteral(check.SourceVector)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s[%v] invalid source vector: %v", check.Table, check.PrimaryKey, err))
			continue
		}
		targetNormalized, err := normalizeVectorLiteral(check.TargetVector)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s[%v] invalid target vector: %v", check.Table, check.PrimaryKey, err))
			continue
		}
		if sourceNormalized != targetNormalized {
			problems = append(problems, fmt.Sprintf("%s[%v] target embedding mismatch", check.Table, check.PrimaryKey))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("embedding vector verification failed: %s", strings.Join(problems, "; "))
	}

	return nil
}

func VerifyCountsMatch(counts []TableCountCheck) error {
	mismatches := make([]string, 0)
	for _, check := range counts {
		if check.SourceCount == check.TargetCount {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("%s: source=%d target=%d", check.Table, check.SourceCount, check.TargetCount))
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("row count verification failed: %s", strings.Join(mismatches, "; "))
	}

	return nil
}

func VerifySequenceIsResetAfterImport(state SequenceState) error {
	if state.Sequence == "" {
		return nil
	}

	minNextValue := state.MaxID + 1
	if minNextValue < 1 {
		minNextValue = 1
	}
	if state.NextValue < minNextValue {
		return fmt.Errorf("sequence verification failed for %s: next=%d max_id=%d sequence=%s", state.Table, state.NextValue, state.MaxID, state.Sequence)
	}

	return nil
}

func VerifyEmbeddingRowCountMatches(sourceCount int64, targetCount int64) error {
	if sourceCount != targetCount {
		return fmt.Errorf("embedding row count verification failed: source=%d target=%d", sourceCount, targetCount)
	}

	return nil
}

func VerifySampleRowsMatch(checks []SampleCheck) error {
	mismatches := make([]string, 0)
	for _, check := range checks {
		if comparableValue(check.SourceValue) == comparableValue(check.TargetValue) {
			continue
		}
		if boolIntEquivalent(check.SourceValue, check.TargetValue) {
			continue
		}
		mismatches = append(mismatches, fmt.Sprintf("%s[%v].%s: source=%v target=%v", check.Table, check.PrimaryKey, check.Column, check.SourceValue, check.TargetValue))
	}

	if len(mismatches) > 0 {
		return fmt.Errorf("sample verification failed: %s", strings.Join(mismatches, "; "))
	}

	return nil
}

func boolIntEquivalent(source, target any) bool {
	sourceInt, sourceOk := toInt64(source)
	targetBool, targetOk := toBool(target)
	if sourceOk && targetOk {
		return (sourceInt == 1 && targetBool) || (sourceInt == 0 && !targetBool)
	}
	return false
}

func toInt64(v any) (int64, bool) {
	switch typed := v.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if typed > uint(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case float64:
		return int64(typed), true
	default:
		return 0, false
	}
}

func toBool(v any) (bool, bool) {
	switch typed := v.(type) {
	case bool:
		return typed, true
	default:
		return false, false
	}
}

func (v *Verifier) compareSampleRows(ctx context.Context, spec TableSpec, limit int) ([]SampleCheck, error) {
	sourceRows, err := v.Source.SampleRows(ctx, spec, limit)
	if err != nil {
		return nil, err
	}
	targetRows, err := v.Target.SampleRows(ctx, spec, limit)
	if err != nil {
		return nil, err
	}

	checks := make([]SampleCheck, 0)
	for i := 0; i < len(sourceRows) && i < len(targetRows); i++ {
		primaryKey := sourceRows[i][spec.PrimaryKey]
		for _, column := range spec.SampleColumns {
			if contains(spec.AllowedMissingTargetColumns, column) {
				if _, ok := targetRows[i][column]; !ok {
					continue
				}
			}
			checks = append(checks, SampleCheck{
				Table:       spec.Name,
				PrimaryKey:  primaryKey,
				Column:      column,
				SourceValue: sourceRows[i][column],
				TargetValue: targetRows[i][column],
			})
		}
	}

	return checks, nil
}

func (v *Verifier) compareEmbeddingVectors(ctx context.Context, spec TableSpec, limit int) ([]EmbeddingVectorCheck, error) {
	sourceRows, err := v.Source.SampleRows(ctx, spec, limit)
	if err != nil {
		return nil, err
	}
	targetChecks, err := v.Target.LoadEmbeddingChecks(ctx, spec, limit)
	if err != nil {
		return nil, err
	}

	byPrimaryKey := make(map[string]EmbeddingVectorCheck, len(targetChecks))
	for _, check := range targetChecks {
		byPrimaryKey[comparableValue(check.PrimaryKey)] = check
	}

	checks := make([]EmbeddingVectorCheck, 0, len(sourceRows))
	for _, sourceRow := range sourceRows {
		primaryKey := sourceRow[spec.PrimaryKey]
		key := comparableValue(primaryKey)
		check := byPrimaryKey[key]
		check.Table = spec.Name
		check.PrimaryKey = primaryKey
		check.SourceVector = fmt.Sprintf("%v", sourceRow["vector"])
		checks = append(checks, check)
	}

	return checks, nil
}

func normalizeVectorLiteral(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	trimmed = strings.TrimPrefix(trimmed, "[")
	trimmed = strings.TrimSuffix(trimmed, "]")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return "", nil
	}

	parts := strings.Split(trimmed, ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		clean := strings.TrimSpace(part)
		if clean == "" {
			return "", fmt.Errorf("empty vector element")
		}
		normalized = append(normalized, clean)
	}

	return strings.Join(normalized, ","), nil
}

func comparableValue(value any) string {
	switch typed := value.(type) {
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case *time.Time:
		if typed == nil {
			return "<nil>"
		}
		return typed.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprintf("%v", value)
	}
}
