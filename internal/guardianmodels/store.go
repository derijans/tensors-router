package guardianmodels

import (
	"context"
	"database/sql"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "guardianmodels" }

func (SchemaModule) Version() int { return 1 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS guardian_models (
		scope TEXT NOT NULL,
		node_id TEXT NOT NULL DEFAULT '',
		model_id TEXT NOT NULL DEFAULT '',
		guardian_node_id TEXT NOT NULL,
		guardian_model_id TEXT NOT NULL,
		PRIMARY KEY (scope, node_id, model_id)
	)`)
	return err
}

func (SchemaModule) ImportLegacy(context.Context, *sql.Tx, string) (int64, error) {
	return 0, nil
}

type Store struct {
	writer *sql.DB
	reader *sql.DB
}

func NewStore(writer *sql.DB, reader *sql.DB) *Store {
	return &Store{writer: writer, reader: reader}
}

func (store *Store) Assignments(ctx context.Context) ([]Assignment, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.reader.QueryContext(ctx, `SELECT scope, node_id, model_id, guardian_node_id, guardian_model_id
		FROM guardian_models ORDER BY CASE scope WHEN 'cluster' THEN 0 WHEN 'node' THEN 1 ELSE 2 END, node_id, model_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var assignments []Assignment
	for rows.Next() {
		var assignment Assignment
		if err := rows.Scan(&assignment.Scope, &assignment.NodeID, &assignment.ModelID, &assignment.Guardian.NodeID, &assignment.Guardian.ModelID); err != nil {
			return nil, err
		}
		assignments = append(assignments, assignment)
	}
	return assignments, rows.Err()
}

func (store *Store) Set(ctx context.Context, assignment Assignment) error {
	assignment, err := assignment.normalized()
	if err != nil {
		return err
	}
	_, err = store.writer.ExecContext(ctx, `INSERT INTO guardian_models (scope, node_id, model_id, guardian_node_id, guardian_model_id)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(scope, node_id, model_id) DO UPDATE SET guardian_node_id = excluded.guardian_node_id, guardian_model_id = excluded.guardian_model_id`,
		assignment.Scope, assignment.NodeID, assignment.ModelID, assignment.Guardian.NodeID, assignment.Guardian.ModelID)
	return err
}

func (store *Store) Clear(ctx context.Context, scope Scope, nodeID string, modelID string) error {
	_, err := store.writer.ExecContext(ctx, `DELETE FROM guardian_models WHERE scope = ? AND node_id = ? AND model_id = ?`, scope, nodeID, modelID)
	return err
}
