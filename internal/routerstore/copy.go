package routerstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

const legacySourceAlias = "legacy_source"

// CopySpec moves one table out of an attached legacy database. Columns absent
// from the legacy file are left to their defaults instead of failing the
// import, so a database written by an older router still copies cleanly.
type CopySpec struct {
	LegacySchema string
	LegacyTable  string
	Table        string
	Columns      []string
	Conflict     func(columns []string) string
}

func CopyRows(ctx context.Context, tx *sql.Tx, spec CopySpec) (int64, error) {
	present, err := legacyTableExists(ctx, tx, spec.LegacySchema, spec.LegacyTable)
	if err != nil {
		return 0, err
	}
	if !present {
		return 0, nil
	}
	columns, err := SharedColumns(ctx, tx, spec.LegacySchema, spec.LegacyTable, spec.Columns)
	if err != nil {
		return 0, err
	}
	if len(columns) == 0 {
		return 0, nil
	}
	list := quotedList(columns)
	statement := fmt.Sprintf(`INSERT INTO main.%s (%s) SELECT %s FROM %s.%s AS %s WHERE true`,
		quoteIdentifier(spec.Table), list, list, spec.LegacySchema, quoteIdentifier(spec.LegacyTable), legacySourceAlias)
	if spec.Conflict != nil {
		if clause := strings.TrimSpace(spec.Conflict(columns)); clause != "" {
			statement += " " + clause
		}
	}
	result, err := tx.ExecContext(ctx, statement)
	if err != nil {
		return 0, fmt.Errorf("copy %s: %w", spec.LegacyTable, err)
	}
	return result.RowsAffected()
}

func SharedColumns(ctx context.Context, tx *sql.Tx, schema string, table string, wanted []string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, fmt.Sprintf(`PRAGMA %s.table_info(%s)`, schema, quoteIdentifier(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	available := make(map[string]bool)
	for rows.Next() {
		var index int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&index, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		available[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	shared := make([]string, 0, len(wanted))
	for _, column := range wanted {
		if available[column] {
			shared = append(shared, column)
		}
	}
	return shared, nil
}

func legacyTableExists(ctx context.Context, tx *sql.Tx, schema string, table string) (bool, error) {
	var name string
	err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT name FROM %s.sqlite_master WHERE type = 'table' AND name = ?`, schema), table).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func quotedList(columns []string) string {
	quoted := make([]string, 0, len(columns))
	for _, column := range columns {
		quoted = append(quoted, quoteIdentifier(column))
	}
	return strings.Join(quoted, ", ")
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
