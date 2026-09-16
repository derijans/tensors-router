package routinggroups

import (
	"context"
	"database/sql"
	"fmt"

	"tensors-router/internal/routerstore"
)

type SchemaModule struct{}

var _ routerstore.Module = SchemaModule{}

func (SchemaModule) Name() string { return "routinggroups" }

func (SchemaModule) Version() int { return 4 }

func (SchemaModule) Migrate(ctx context.Context, db *sql.DB) error {
	for _, table := range linkTablesByLane {
		if _, err := db.ExecContext(ctx, createLinkTableStatement(table)); err != nil {
			return err
		}
	}
	transaction, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()
	for _, lane := range symmetricGroupLanes {
		if _, err := convertGroupPairsToLinksInBothDirections(ctx, transaction, "main", lane); err != nil {
			return err
		}
		if err := dropSymmetricGroupTables(ctx, transaction, lane); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (SchemaModule) ImportLegacy(ctx context.Context, tx *sql.Tx, legacySchema string) (int64, error) {
	var imported int64
	for _, lane := range symmetricGroupLanes {
		converted, err := convertGroupPairsToLinksInBothDirections(ctx, tx, legacySchema, lane)
		if err != nil {
			return 0, err
		}
		imported += converted
	}
	return imported, nil
}

func createLinkTableStatement(table string) string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		owner_node_id TEXT NOT NULL,
		owner_model_id TEXT NOT NULL,
		helper_node_id TEXT NOT NULL,
		helper_model_id TEXT NOT NULL,
		load_if_unloaded INTEGER NOT NULL DEFAULT 1,
		restore_after_borrow INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (owner_node_id, owner_model_id, helper_node_id, helper_model_id)
	)`, table)
}

type symmetricGroupLane struct {
	groupsTable  string
	membersTable string
	modelColumn  string
	linksTable   string
}

var symmetricGroupLanes = []symmetricGroupLane{
	{groupsTable: "routing_groups", membersTable: "routing_group_members", modelColumn: "image_id", linksTable: linkTablesByLane[ImageLane]},
	{groupsTable: "routing_text_groups", membersTable: "routing_text_group_members", modelColumn: "model_id", linksTable: linkTablesByLane[TextLane]},
}

func convertGroupPairsToLinksInBothDirections(ctx context.Context, tx *sql.Tx, schema string, lane symmetricGroupLane) (int64, error) {
	present, err := routerstore.TableExists(ctx, tx, schema, lane.membersTable)
	if err != nil || !present {
		return 0, err
	}
	restoreColumns, err := routerstore.SharedColumns(ctx, tx, schema, lane.membersTable, []string{"restore_after_borrow"})
	if err != nil {
		return 0, err
	}
	helperRestore := "0"
	if len(restoreColumns) == 1 {
		helperRestore = "helper.restore_after_borrow"
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(
		`INSERT INTO main.%[1]s (owner_node_id, owner_model_id, helper_node_id, helper_model_id, load_if_unloaded, restore_after_borrow)
		 SELECT owner.node_id, owner.%[3]s, helper.node_id, helper.%[3]s, 1, %[4]s
		 FROM %[2]s.%[5]s AS owner
		 JOIN %[2]s.%[5]s AS helper ON helper.group_id = owner.group_id AND helper.node_id <> owner.node_id
		 WHERE true
		 ON CONFLICT DO NOTHING`,
		lane.linksTable, schema, lane.modelColumn, helperRestore, lane.membersTable))
	if err != nil {
		return 0, fmt.Errorf("convert %s to links: %w", lane.membersTable, err)
	}
	return result.RowsAffected()
}

func dropSymmetricGroupTables(ctx context.Context, tx *sql.Tx, lane symmetricGroupLane) error {
	for _, table := range []string{lane.membersTable, lane.groupsTable} {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS main.%s`, table)); err != nil {
			return err
		}
	}
	return nil
}
