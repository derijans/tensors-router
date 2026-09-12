package routinggroups

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type laneTables struct {
	groups       string
	members      string
	memberColumn string
}

var imageLaneTables = laneTables{groups: "routing_groups", members: "routing_group_members", memberColumn: "image_id"}
var textLaneTables = laneTables{groups: "routing_text_groups", members: "routing_text_group_members", memberColumn: "model_id"}

type laneMember struct {
	NodeID             string
	ModelID            string
	RestoreAfterBorrow bool
}

type laneGroup struct {
	ID      string
	Members []laneMember
}

func (store *Store) laneGroup(ctx context.Context, tables laneTables, member laneMember) (laneGroup, bool, error) {
	if store == nil {
		return laneGroup{}, false, nil
	}
	member = normalizeLaneMember(member)
	if member.NodeID == "" || member.ModelID == "" {
		return laneGroup{}, false, nil
	}
	var groupID string
	query := fmt.Sprintf(`SELECT group_id FROM %s WHERE node_id = ? AND %s = ?`, tables.members, tables.memberColumn)
	err := store.reader.QueryRowContext(ctx, query, member.NodeID, member.ModelID).Scan(&groupID)
	if err == sql.ErrNoRows {
		return laneGroup{}, false, nil
	}
	if err != nil {
		return laneGroup{}, false, err
	}
	members, err := store.laneMembersOf(ctx, tables, groupID)
	if err != nil {
		return laneGroup{}, false, err
	}
	return laneGroup{ID: groupID, Members: members}, true, nil
}

func (store *Store) laneGroups(ctx context.Context, tables laneTables) ([]laneGroup, error) {
	if store == nil {
		return nil, nil
	}
	query := fmt.Sprintf(`SELECT group_id, node_id, %s, restore_after_borrow FROM %s ORDER BY group_id, node_id, %s`, tables.memberColumn, tables.members, tables.memberColumn)
	rows, err := store.reader.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string][]laneMember{}
	var order []string
	for rows.Next() {
		var groupID string
		var member laneMember
		var restoreAfterBorrow int
		if err := rows.Scan(&groupID, &member.NodeID, &member.ModelID, &restoreAfterBorrow); err != nil {
			return nil, err
		}
		member.RestoreAfterBorrow = restoreAfterBorrow != 0
		if _, seen := byID[groupID]; !seen {
			order = append(order, groupID)
		}
		byID[groupID] = append(byID[groupID], member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	groups := make([]laneGroup, 0, len(order))
	for _, groupID := range order {
		groups = append(groups, laneGroup{ID: groupID, Members: byID[groupID]})
	}
	return groups, nil
}

func (store *Store) setLaneGroup(ctx context.Context, tables laneTables, anchor laneMember, members []laneMember) (laneGroup, error) {
	if store == nil {
		return laneGroup{}, fmt.Errorf("routing group store is not configured")
	}
	anchor = normalizeLaneMember(anchor)
	if anchor.NodeID == "" || anchor.ModelID == "" {
		return laneGroup{}, fmt.Errorf("anchor node_id and %s are required", tables.memberColumn)
	}
	wanted := dedupeLaneMembersFirstWins(append([]laneMember{anchor}, members...))

	transaction, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return laneGroup{}, err
	}
	defer func() { _ = transaction.Rollback() }()

	groupID, err := laneGroupIDForAnchor(ctx, transaction, tables, anchor)
	if err != nil {
		return laneGroup{}, err
	}
	if _, err := transaction.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE group_id = ?`, tables.members), groupID); err != nil {
		return laneGroup{}, err
	}
	if len(wanted) < 2 {
		if err := deleteUndersizedLaneGroups(ctx, transaction, tables); err != nil {
			return laneGroup{}, err
		}
		if err := transaction.Commit(); err != nil {
			return laneGroup{}, err
		}
		return laneGroup{}, nil
	}
	if _, err := transaction.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO %s(id) VALUES (?) ON CONFLICT(id) DO NOTHING`, tables.groups), groupID); err != nil {
		return laneGroup{}, err
	}
	insertMember := fmt.Sprintf(
		`INSERT INTO %s(node_id, %s, group_id, restore_after_borrow) VALUES (?, ?, ?, ?)
		 ON CONFLICT(node_id, %s) DO UPDATE SET group_id = excluded.group_id, restore_after_borrow = excluded.restore_after_borrow`,
		tables.members, tables.memberColumn, tables.memberColumn)
	for _, member := range wanted {
		if _, err := transaction.ExecContext(ctx, insertMember, member.NodeID, member.ModelID, groupID, restoreAfterBorrowColumn(member.RestoreAfterBorrow)); err != nil {
			return laneGroup{}, err
		}
	}
	if err := deleteUndersizedLaneGroups(ctx, transaction, tables); err != nil {
		return laneGroup{}, err
	}
	if err := transaction.Commit(); err != nil {
		return laneGroup{}, err
	}
	return laneGroup{ID: groupID, Members: wanted}, nil
}

func (store *Store) laneMembersOf(ctx context.Context, tables laneTables, groupID string) ([]laneMember, error) {
	query := fmt.Sprintf(`SELECT node_id, %s, restore_after_borrow FROM %s WHERE group_id = ? ORDER BY node_id, %s`, tables.memberColumn, tables.members, tables.memberColumn)
	rows, err := store.reader.QueryContext(ctx, query, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []laneMember
	for rows.Next() {
		var member laneMember
		var restoreAfterBorrow int
		if err := rows.Scan(&member.NodeID, &member.ModelID, &restoreAfterBorrow); err != nil {
			return nil, err
		}
		member.RestoreAfterBorrow = restoreAfterBorrow != 0
		members = append(members, member)
	}
	return members, rows.Err()
}

func restoreAfterBorrowColumn(value bool) int {
	if value {
		return 1
	}
	return 0
}

func laneGroupIDForAnchor(ctx context.Context, transaction *sql.Tx, tables laneTables, anchor laneMember) (string, error) {
	var groupID string
	query := fmt.Sprintf(`SELECT group_id FROM %s WHERE node_id = ? AND %s = ?`, tables.members, tables.memberColumn)
	err := transaction.QueryRowContext(ctx, query, anchor.NodeID, anchor.ModelID).Scan(&groupID)
	if err == nil {
		return groupID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	return anchor.NodeID + "\x00" + anchor.ModelID, nil
}

func deleteUndersizedLaneGroups(ctx context.Context, transaction *sql.Tx, tables laneTables) error {
	if _, err := transaction.ExecContext(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE group_id IN (
			SELECT group_id FROM %s GROUP BY group_id HAVING COUNT(*) < 2
		)`, tables.members, tables.members)); err != nil {
		return err
	}
	_, err := transaction.ExecContext(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE id NOT IN (SELECT DISTINCT group_id FROM %s)`, tables.groups, tables.members))
	return err
}

func normalizeLaneMember(member laneMember) laneMember {
	return laneMember{
		NodeID:             strings.TrimSpace(member.NodeID),
		ModelID:            strings.TrimSpace(member.ModelID),
		RestoreAfterBorrow: member.RestoreAfterBorrow,
	}
}

type laneMemberKey struct {
	NodeID  string
	ModelID string
}

// setLaneGroup prepends the anchor, so its RestoreAfterBorrow survives a duplicate.
func dedupeLaneMembersFirstWins(members []laneMember) []laneMember {
	byKey := map[laneMemberKey]laneMember{}
	var order []laneMemberKey
	for _, member := range members {
		member = normalizeLaneMember(member)
		if member.NodeID == "" || member.ModelID == "" {
			continue
		}
		key := laneMemberKey{NodeID: member.NodeID, ModelID: member.ModelID}
		if _, duplicate := byKey[key]; duplicate {
			continue
		}
		order = append(order, key)
		byKey[key] = member
	}
	result := make([]laneMember, 0, len(order))
	for _, key := range order {
		result = append(result, byKey[key])
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].NodeID != result[right].NodeID {
			return result[left].NodeID < result[right].NodeID
		}
		return result[left].ModelID < result[right].ModelID
	})
	return result
}
