package routinggroups

import (
	"context"
	"database/sql"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "routinggroups" }

func (SchemaModule) Version() int { return 2 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS routing_groups (
			id TEXT PRIMARY KEY NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (unixepoch())
		)`,
		`CREATE TABLE IF NOT EXISTS routing_group_members (
			node_id TEXT NOT NULL,
			image_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			PRIMARY KEY (node_id, image_id),
			FOREIGN KEY (group_id) REFERENCES routing_groups(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS routing_group_members_group_idx ON routing_group_members (group_id)`,
		`CREATE TABLE IF NOT EXISTS routing_text_groups (
			id TEXT PRIMARY KEY NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (unixepoch())
		)`,
		`CREATE TABLE IF NOT EXISTS routing_text_group_members (
			node_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			PRIMARY KEY (node_id, model_id),
			FOREIGN KEY (group_id) REFERENCES routing_text_groups(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS routing_text_group_members_group_idx ON routing_text_group_members (group_id)`,
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (SchemaModule) ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error) {
	groups, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "routing_groups",
		Table:        "routing_groups",
		Columns:      []string{"id", "created_at"},
	})
	if err != nil {
		return 0, err
	}
	members, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "routing_group_members",
		Table:        "routing_group_members",
		Columns:      []string{"node_id", "image_id", "group_id"},
	})
	if err != nil {
		return 0, err
	}
	textGroups, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "routing_text_groups",
		Table:        "routing_text_groups",
		Columns:      []string{"id", "created_at"},
	})
	if err != nil {
		return 0, err
	}
	textMembers, err := routerstore.CopyRows(ctx, tx, routerstore.CopySpec{
		LegacySchema: legacySchema,
		LegacyTable:  "routing_text_group_members",
		Table:        "routing_text_group_members",
		Columns:      []string{"node_id", "model_id", "group_id"},
	})
	if err != nil {
		return 0, err
	}
	return groups + members + textGroups + textMembers, nil
}
