package datamigrate

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
)

type PostgresWriter struct {
	db    *gorm.DB
	sqlDB *sql.DB
}

func NewPostgresWriter(db *gorm.DB) (*PostgresWriter, error) {
	if db == nil {
		return nil, fmt.Errorf("postgres database is required")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("postgres db handle: %w", err)
	}

	return &PostgresWriter{db: db, sqlDB: sqlDB}, nil
}

func (w *PostgresWriter) WithDB(db *gorm.DB) (*PostgresWriter, error) {
	return NewPostgresWriter(db)
}

func (w *PostgresWriter) ExistingTables(ctx context.Context) (map[string]bool, error) {
	var tables []string
	if err := w.db.WithContext(ctx).
		Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name").
		Scan(&tables).Error; err != nil {
		return nil, fmt.Errorf("list postgres tables: %w", err)
	}

	result := make(map[string]bool, len(tables))
	for _, table := range tables {
		result[table] = true
	}

	return result, nil
}

func (w *PostgresWriter) TableColumns(ctx context.Context, table string) ([]string, error) {
	var columns []string
	if err := w.db.WithContext(ctx).Raw(`
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ?
		ORDER BY ordinal_position
	`, table).Scan(&columns).Error; err != nil {
		return nil, fmt.Errorf("read postgres columns for %s: %w", table, err)
	}
	return columns, nil
}

func (w *PostgresWriter) TableColumnTypes(ctx context.Context, table string) (map[string]string, error) {
	type columnInfo struct {
		Name string
		Type string
	}

	var columns []columnInfo
	if err := w.db.WithContext(ctx).Raw(`
		SELECT column_name AS name, data_type AS type
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = ?
	`, table).Scan(&columns).Error; err != nil {
		return nil, fmt.Errorf("read postgres column types for %s: %w", table, err)
	}

	result := make(map[string]string, len(columns))
	for _, column := range columns {
		result[column.Name] = strings.ToLower(column.Type)
	}

	return result, nil
}

func (w *PostgresWriter) TruncateTables(ctx context.Context, specs []TableSpec) error {
	tables := make([]string, 0, len(specs))
	for i := len(specs) - 1; i >= 0; i-- {
		tables = append(tables, quoteIdentifier(specs[i].Name))
	}

	if len(tables) == 0 {
		return nil
	}

	statement := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	if err := w.db.WithContext(ctx).Exec(statement).Error; err != nil {
		return fmt.Errorf("truncate target tables: %w", err)
	}

	return nil
}

func (w *PostgresWriter) ImportTable(ctx context.Context, spec TableSpec, sourceColumns []string, stream func(func(map[string]any) error) error) (int64, error) {
	targetColumns, err := w.TableColumns(ctx, spec.Name)
	if err != nil {
		return 0, err
	}

	sharedColumns, includeEmbeddingVector, err := validateImportColumns(spec, sourceColumns, targetColumns)
	if err != nil {
		return 0, err
	}

	columnTypes, err := w.TableColumnTypes(ctx, spec.Name)
	if err != nil {
		return 0, err
	}

	insertColumns := make([]string, 0, len(sharedColumns)+1)
	valueExprs := make([]string, 0, len(sharedColumns)+1)
	for _, column := range sharedColumns {
		insertColumns = append(insertColumns, column)
		valueExprs = append(valueExprs, "?")
	}

	if includeEmbeddingVector {
		insertColumns = append(insertColumns, "embedding")
		valueExprs = append(valueExprs, "CAST(? AS vector)")
	}

	if len(insertColumns) == 0 {
		return 0, fmt.Errorf("no shared columns found for table %s", spec.Name)
	}

	statement := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		quoteIdentifier(spec.Name),
		joinQuoted(insertColumns),
		strings.Join(valueExprs, ", "),
	)

	var imported int64
	err = stream(func(row map[string]any) error {
		args := make([]any, 0, len(insertColumns)+1)
		for _, column := range sharedColumns {
			normalized, err := normalizeInsertValue(row[column], columnTypes[column])
			if err != nil {
				return fmt.Errorf("normalize %s.%s: %w", spec.Name, column, err)
			}
			args = append(args, normalized)
		}
		if includeEmbeddingVector {
			args = append(args, row["vector"])
		}

		if err := w.db.WithContext(ctx).Exec(statement, args...).Error; err != nil {
			return fmt.Errorf("insert into %s: %w", spec.Name, err)
		}
		imported++
		return nil
	})
	if err != nil {
		return imported, err
	}

	return imported, nil
}

func validateImportColumns(spec TableSpec, sourceColumns []string, targetColumns []string) ([]string, bool, error) {
	targetSet := make(map[string]bool, len(targetColumns))
	for _, column := range targetColumns {
		targetSet[column] = true
	}

	allowedMissing := make(map[string]bool, len(spec.AllowedMissingTargetColumns))
	for _, column := range spec.AllowedMissingTargetColumns {
		allowedMissing[column] = true
	}

	shared := make([]string, 0, len(sourceColumns))
	missing := make([]string, 0)
	for _, column := range sourceColumns {
		if targetSet[column] {
			shared = append(shared, column)
			continue
		}
		if allowedMissing[column] {
			continue
		}
		missing = append(missing, column)
	}

	includeEmbeddingVector := spec.Name == "topic_tag_embeddings" && targetSet["embedding"] && contains(sourceColumns, "vector")
	if len(missing) > 0 {
		return nil, false, fmt.Errorf("target schema drift for %s: missing target columns for source fields %s", spec.Name, strings.Join(missing, ", "))
	}
	if len(shared) == 0 {
		return nil, false, fmt.Errorf("no shared columns found for table %s", spec.Name)
	}

	return shared, includeEmbeddingVector, nil
}

func (w *PostgresWriter) CountRows(ctx context.Context, table string) (int64, error) {
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(table))
	if err := w.db.WithContext(ctx).Raw(query).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count postgres rows for %s: %w", table, err)
	}
	return count, nil
}

func (w *PostgresWriter) ResetSequences(ctx context.Context, specs []TableSpec) ([]SequenceState, error) {
	states := make([]SequenceState, 0, len(specs))
	for _, spec := range specs {
		if spec.PrimaryKey == "" {
			continue
		}

		maxID, err := w.maxID(ctx, spec)
		if err != nil {
			return nil, err
		}

		sequenceName, err := w.sequenceName(ctx, spec)
		if err != nil {
			return nil, err
		}
		if sequenceName == "" {
			continue
		}

		nextValue := maxID + 1
		if nextValue < 1 {
			nextValue = 1
		}

		if err := w.db.WithContext(ctx).Exec(
			"SELECT setval(pg_get_serial_sequence(?, ?), ?, false)",
			spec.Name,
			spec.PrimaryKey,
			nextValue,
		).Error; err != nil {
			return nil, fmt.Errorf("reset sequence for %s: %w", spec.Name, err)
		}

		state, err := w.LoadSequenceState(ctx, spec)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}

	return states, nil
}

func (w *PostgresWriter) LoadSequenceState(ctx context.Context, spec TableSpec) (SequenceState, error) {
	maxID, err := w.maxID(ctx, spec)
	if err != nil {
		return SequenceState{}, err
	}

	sequenceName, err := w.sequenceName(ctx, spec)
	if err != nil {
		return SequenceState{}, err
	}
	if sequenceName == "" {
		return SequenceState{Table: spec.Name, MaxID: maxID}, nil
	}

	query := fmt.Sprintf("SELECT last_value, is_called FROM %s", quoteQualifiedIdentifier(sequenceName))
	row := w.db.WithContext(ctx).Raw(query).Row()
	var lastValue int64
	var isCalled bool
	if err := row.Scan(&lastValue, &isCalled); err != nil {
		return SequenceState{}, fmt.Errorf("read sequence state for %s: %w", spec.Name, err)
	}

	nextValue := lastValue
	if isCalled {
		nextValue = lastValue + 1
	}

	return SequenceState{
		Table:     spec.Name,
		Sequence:  sequenceName,
		MaxID:     maxID,
		NextValue: nextValue,
	}, nil
}

func (w *PostgresWriter) SampleRows(ctx context.Context, spec TableSpec, limit int) ([]map[string]any, error) {
	targetColumns, err := w.TableColumns(ctx, spec.Name)
	if err != nil {
		return nil, err
	}

	columns, err := filterTargetColumns(spec, append([]string{spec.PrimaryKey}, spec.SampleColumns...), targetColumns)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT %d",
		joinQuoted(columns),
		quoteIdentifier(spec.Name),
		quoteIdentifier(spec.PrimaryKey),
		limit,
	)

	rows, err := w.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("query postgres samples for %s: %w", spec.Name, err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]map[string]any, 0, limit)
	for rows.Next() {
		row, err := scanRow(rows, columns)
		if err != nil {
			return nil, fmt.Errorf("scan postgres sample for %s: %w", spec.Name, err)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate postgres samples for %s: %w", spec.Name, err)
	}

	return result, nil
}

func (w *PostgresWriter) LoadEmbeddingChecks(ctx context.Context, spec TableSpec, limit int) ([]EmbeddingVectorCheck, error) {
	if spec.Name != "topic_tag_embeddings" || spec.PrimaryKey == "" {
		return nil, nil
	}
	targetColumns, err := w.TableColumns(ctx, spec.Name)
	if err != nil {
		return nil, err
	}

	targetSet := make(map[string]bool, len(targetColumns))
	for _, column := range targetColumns {
		targetSet[column] = true
	}

	legacyVectorExpr := "''"
	if targetSet["vector"] {
		legacyVectorExpr = "COALESCE(vector, '')"
	}
	embeddingExpr := "''"
	if targetSet["embedding"] {
		embeddingExpr = "COALESCE(embedding::text, '')"
	}

	query := fmt.Sprintf("SELECT %s, %s, %s FROM %s ORDER BY %s LIMIT %d",
		quoteIdentifier(spec.PrimaryKey),
		legacyVectorExpr,
		embeddingExpr,
		quoteIdentifier(spec.Name),
		quoteIdentifier(spec.PrimaryKey),
		limit,
	)

	rows, err := w.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("query embedding checks for %s: %w", spec.Name, err)
	}
	defer func() { _ = rows.Close() }()

	checks := make([]EmbeddingVectorCheck, 0, limit)
	for rows.Next() {
		var primaryKey any
		var legacyVector string
		var targetVector string
		if err := rows.Scan(&primaryKey, &legacyVector, &targetVector); err != nil {
			return nil, fmt.Errorf("scan embedding check for %s: %w", spec.Name, err)
		}
		checks = append(checks, EmbeddingVectorCheck{Table: spec.Name, PrimaryKey: primaryKey, LegacyVector: legacyVector, TargetVector: targetVector})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate embedding checks for %s: %w", spec.Name, err)
	}

	return checks, nil
}

func (w *PostgresWriter) maxID(ctx context.Context, spec TableSpec) (int64, error) {
	query := fmt.Sprintf("SELECT COALESCE(MAX(%s), 0) FROM %s", quoteIdentifier(spec.PrimaryKey), quoteIdentifier(spec.Name))
	var maxID int64
	if err := w.db.WithContext(ctx).Raw(query).Scan(&maxID).Error; err != nil {
		return 0, fmt.Errorf("max id for %s: %w", spec.Name, err)
	}
	return maxID, nil
}

func (w *PostgresWriter) sequenceName(ctx context.Context, spec TableSpec) (string, error) {
	var sequenceName sql.NullString
	row := w.db.WithContext(ctx).Raw("SELECT pg_get_serial_sequence(?, ?)", spec.Name, spec.PrimaryKey).Row()
	if err := row.Scan(&sequenceName); err != nil {
		return "", fmt.Errorf("resolve sequence for %s: %w", spec.Name, err)
	}
	if !sequenceName.Valid {
		return "", nil
	}
	return sequenceName.String, nil
}

func quoteQualifiedIdentifier(identifier string) string {
	parts := strings.Split(identifier, ".")
	for i, part := range parts {
		parts[i] = quoteIdentifier(part)
	}
	return strings.Join(parts, ".")
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func normalizeInsertValue(value any, targetType string) (any, error) {
	switch strings.ToLower(targetType) {
	case "boolean":
		return normalizeBoolValue(value)
	case "bool":
		return normalizeBoolValue(value)
	case "timestamp with time zone", "timestamp without time zone", "timestamp", "timestamptz":
		return normalizeTimestampValue(value)
	default:
		return sanitizeUTF8(value)
	}
}

func normalizeTimestampValue(value any) (any, error) {
	str, ok := value.(string)
	if !ok {
		return value, nil
	}
	if strings.TrimSpace(str) == "" {
		return nil, nil
	}
	return sanitizeUTF8(value)
}

func sanitizeUTF8(value any) (any, error) {
	str, ok := value.(string)
	if !ok {
		return value, nil
	}
	if !utf8.ValidString(str) {
		return strings.ToValidUTF8(str, "�"), nil
	}
	return str, nil
}

func normalizeBoolValue(value any) (any, error) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case int:
		return typed != 0, nil
	case int8:
		return typed != 0, nil
	case int16:
		return typed != 0, nil
	case int32:
		return typed != 0, nil
	case int64:
		return typed != 0, nil
	case uint:
		return typed != 0, nil
	case uint8:
		return typed != 0, nil
	case uint16:
		return typed != 0, nil
	case uint32:
		return typed != 0, nil
	case uint64:
		return typed != 0, nil
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(typed))
		switch trimmed {
		case "1", "true", "t":
			return true, nil
		case "0", "false", "f", "":
			return false, nil
		default:
			parsed, err := strconv.ParseBool(trimmed)
			if err != nil {
				return nil, fmt.Errorf("unsupported boolean string %q", typed)
			}
			return parsed, nil
		}
	default:
		return value, nil
	}
}

func filterTargetColumns(spec TableSpec, requested []string, targetColumns []string) ([]string, error) {
	targetSet := make(map[string]bool, len(targetColumns))
	for _, column := range targetColumns {
		targetSet[column] = true
	}

	allowedMissing := make(map[string]bool, len(spec.AllowedMissingTargetColumns))
	for _, column := range spec.AllowedMissingTargetColumns {
		allowedMissing[column] = true
	}

	selected := make([]string, 0, len(requested))
	missing := make([]string, 0)
	for _, column := range requested {
		if targetSet[column] {
			selected = append(selected, column)
			continue
		}
		if allowedMissing[column] {
			continue
		}
		missing = append(missing, column)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("target schema drift for %s samples: missing target columns %s", spec.Name, strings.Join(missing, ", "))
	}

	return selected, nil
}
