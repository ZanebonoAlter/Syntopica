package datamigrate

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type SQLiteReader struct {
	db    *gorm.DB
	sqlDB *sql.DB
}

func NewSQLiteReader(path string) (*SQLiteReader, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite source: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("sqlite db handle: %w", err)
	}

	return &SQLiteReader{db: db, sqlDB: sqlDB}, nil
}

func (r *SQLiteReader) Close() error {
	if r == nil || r.sqlDB == nil {
		return nil
	}
	return r.sqlDB.Close()
}

func (r *SQLiteReader) ExistingTables(ctx context.Context) (map[string]bool, error) {
	var tables []string
	if err := r.db.WithContext(ctx).
		Raw("SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name").
		Scan(&tables).Error; err != nil {
		return nil, fmt.Errorf("list sqlite tables: %w", err)
	}

	result := make(map[string]bool, len(tables))
	for _, table := range tables {
		result[table] = true
	}

	return result, nil
}

func (r *SQLiteReader) CountRows(ctx context.Context, table string) (int64, error) {
	var count int64
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", quoteIdentifier(table))
	if err := r.db.WithContext(ctx).Raw(query).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count sqlite rows for %s: %w", table, err)
	}
	return count, nil
}

func (r *SQLiteReader) TableColumns(ctx context.Context, table string) ([]string, error) {
	rows, err := r.db.WithContext(ctx).Raw(fmt.Sprintf("PRAGMA table_info(%s)", quoteIdentifier(table))).Rows()
	if err != nil {
		return nil, fmt.Errorf("read sqlite columns for %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()

	columns := make([]string, 0)
	for rows.Next() {
		var cid int
		var name string
		var dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan sqlite column metadata for %s: %w", table, err)
		}
		columns = append(columns, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite columns for %s: %w", table, err)
	}

	return columns, nil
}

func (r *SQLiteReader) ReadRows(ctx context.Context, spec TableSpec, handle func(map[string]any) error) (int64, []string, error) {
	columns, err := r.TableColumns(ctx, spec.Name)
	if err != nil {
		return 0, nil, err
	}

	query := fmt.Sprintf("SELECT * FROM %s", quoteIdentifier(spec.Name))
	if spec.PrimaryKey != "" {
		query += fmt.Sprintf(" ORDER BY %s", quoteIdentifier(spec.PrimaryKey))
	}

	rows, err := r.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return 0, nil, fmt.Errorf("query sqlite rows for %s: %w", spec.Name, err)
	}
	defer func() { _ = rows.Close() }()

	var count int64
	for rows.Next() {
		row, err := scanRow(rows, columns)
		if err != nil {
			return count, columns, fmt.Errorf("scan sqlite row for %s: %w", spec.Name, err)
		}
		if err := handle(row); err != nil {
			return count, columns, err
		}
		count++
	}

	if err := rows.Err(); err != nil {
		return count, columns, fmt.Errorf("iterate sqlite rows for %s: %w", spec.Name, err)
	}

	return count, columns, nil
}

func (r *SQLiteReader) SampleRows(ctx context.Context, spec TableSpec, limit int) ([]map[string]any, error) {
	columns := append([]string{spec.PrimaryKey}, spec.SampleColumns...)
	query := fmt.Sprintf("SELECT %s FROM %s ORDER BY %s LIMIT %d",
		joinQuoted(columns),
		quoteIdentifier(spec.Name),
		quoteIdentifier(spec.PrimaryKey),
		limit,
	)

	rows, err := r.db.WithContext(ctx).Raw(query).Rows()
	if err != nil {
		return nil, fmt.Errorf("query sqlite samples for %s: %w", spec.Name, err)
	}
	defer func() { _ = rows.Close() }()

	result := make([]map[string]any, 0, limit)
	for rows.Next() {
		row, err := scanRow(rows, columns)
		if err != nil {
			return nil, fmt.Errorf("scan sqlite sample for %s: %w", spec.Name, err)
		}
		result = append(result, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sqlite samples for %s: %w", spec.Name, err)
	}

	return result, nil
}

func scanRow(rows *sql.Rows, columns []string) (map[string]any, error) {
	values := make([]any, len(columns))
	pointers := make([]any, len(columns))
	for i := range values {
		pointers[i] = &values[i]
	}

	if err := rows.Scan(pointers...); err != nil {
		return nil, err
	}

	row := make(map[string]any, len(columns))
	for i, column := range columns {
		row[column] = normalizeDBValue(values[i])
	}

	return row, nil
}

func normalizeDBValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func joinQuoted(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}
