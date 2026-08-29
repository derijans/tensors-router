package routinggroups

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const databaseFilename = "routing-groups.sqlite"

// Member identifies one image model on one node. Members are declared by an
// operator and are not required to share a name, a config hash, or even a
// checkpoint with each other.
type Member struct {
	NodeID  string `json:"node_id"`
	ImageID string `json:"image_id"`
}

type Group struct {
	ID      string   `json:"id"`
	Members []Member `json:"members"`
}

type Store struct {
	database *sql.DB
}

func NewStore(directory string) (*Store, error) {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return nil, fmt.Errorf("routing group store dir is required")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", filepath.Join(directory, databaseFilename))
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	store := &Store{database: database}
	if err := store.initialize(); err != nil {
		_ = database.Close()
		return nil, err
	}
	return store, nil
}

// The member primary key is the pair identifying a model rather than the group,
// so the schema itself makes it impossible for one model to sit in two groups and
// for routing to face a choice it has no way to resolve.
func (store *Store) initialize() error {
	_, err := store.database.Exec(`
		PRAGMA busy_timeout = 5000;
		PRAGMA journal_mode = WAL;
		PRAGMA foreign_keys = ON;
		CREATE TABLE IF NOT EXISTS routing_groups (
			id TEXT PRIMARY KEY NOT NULL,
			created_at INTEGER NOT NULL DEFAULT (unixepoch())
		);
		CREATE TABLE IF NOT EXISTS routing_group_members (
			node_id TEXT NOT NULL,
			image_id TEXT NOT NULL,
			group_id TEXT NOT NULL,
			PRIMARY KEY (node_id, image_id),
			FOREIGN KEY (group_id) REFERENCES routing_groups(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS routing_group_members_group_idx ON routing_group_members (group_id);`)
	return err
}

func (store *Store) Close() error {
	if store == nil || store.database == nil {
		return nil
	}
	return store.database.Close()
}

func (store *Store) Group(ctx context.Context, member Member) (Group, bool, error) {
	if store == nil {
		return Group{}, false, nil
	}
	member = normalizeMember(member)
	if member.NodeID == "" || member.ImageID == "" {
		return Group{}, false, nil
	}
	var groupID string
	err := store.database.QueryRowContext(ctx,
		`SELECT group_id FROM routing_group_members WHERE node_id = ? AND image_id = ?`,
		member.NodeID, member.ImageID).Scan(&groupID)
	if err == sql.ErrNoRows {
		return Group{}, false, nil
	}
	if err != nil {
		return Group{}, false, err
	}
	members, err := store.membersOf(ctx, groupID)
	if err != nil {
		return Group{}, false, err
	}
	return Group{ID: groupID, Members: members}, true, nil
}

func (store *Store) Groups(ctx context.Context) ([]Group, error) {
	if store == nil {
		return nil, nil
	}
	rows, err := store.database.QueryContext(ctx,
		`SELECT group_id, node_id, image_id FROM routing_group_members ORDER BY group_id, node_id, image_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string][]Member{}
	var order []string
	for rows.Next() {
		var groupID string
		var member Member
		if err := rows.Scan(&groupID, &member.NodeID, &member.ImageID); err != nil {
			return nil, err
		}
		if _, seen := byID[groupID]; !seen {
			order = append(order, groupID)
		}
		byID[groupID] = append(byID[groupID], member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	groups := make([]Group, 0, len(order))
	for _, groupID := range order {
		groups = append(groups, Group{ID: groupID, Members: byID[groupID]})
	}
	return groups, nil
}

// SetGroup replaces whatever group the anchor belonged to with exactly the anchor
// plus the supplied members. A member named here leaves any other group it was in,
// so the one-group-per-model invariant holds without the caller having to unpick
// the previous arrangement. A group left with fewer than two members is deleted:
// a group of one has nowhere to offload to.
func (store *Store) SetGroup(ctx context.Context, anchor Member, members []Member) (Group, error) {
	if store == nil {
		return Group{}, fmt.Errorf("routing group store is not configured")
	}
	anchor = normalizeMember(anchor)
	if anchor.NodeID == "" || anchor.ImageID == "" {
		return Group{}, fmt.Errorf("anchor node_id and image_id are required")
	}
	wanted := dedupeMembers(append([]Member{anchor}, members...))

	transaction, err := store.database.BeginTx(ctx, nil)
	if err != nil {
		return Group{}, err
	}
	defer func() { _ = transaction.Rollback() }()

	groupID, err := groupIDForAnchor(ctx, transaction, anchor)
	if err != nil {
		return Group{}, err
	}
	if _, err := transaction.ExecContext(ctx, `DELETE FROM routing_group_members WHERE group_id = ?`, groupID); err != nil {
		return Group{}, err
	}
	if len(wanted) < 2 {
		if err := deleteUndersizedGroups(ctx, transaction); err != nil {
			return Group{}, err
		}
		if err := transaction.Commit(); err != nil {
			return Group{}, err
		}
		return Group{}, nil
	}
	if _, err := transaction.ExecContext(ctx,
		`INSERT INTO routing_groups(id) VALUES (?) ON CONFLICT(id) DO NOTHING`, groupID); err != nil {
		return Group{}, err
	}
	for _, member := range wanted {
		if _, err := transaction.ExecContext(ctx,
			`INSERT INTO routing_group_members(node_id, image_id, group_id) VALUES (?, ?, ?)
			 ON CONFLICT(node_id, image_id) DO UPDATE SET group_id = excluded.group_id`,
			member.NodeID, member.ImageID, groupID); err != nil {
			return Group{}, err
		}
	}
	if err := deleteUndersizedGroups(ctx, transaction); err != nil {
		return Group{}, err
	}
	if err := transaction.Commit(); err != nil {
		return Group{}, err
	}
	return Group{ID: groupID, Members: wanted}, nil
}

func (store *Store) DeleteGroup(ctx context.Context, member Member) error {
	if store == nil {
		return nil
	}
	_, err := store.SetGroup(ctx, member, nil)
	return err
}

func (store *Store) membersOf(ctx context.Context, groupID string) ([]Member, error) {
	rows, err := store.database.QueryContext(ctx,
		`SELECT node_id, image_id FROM routing_group_members WHERE group_id = ? ORDER BY node_id, image_id`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.NodeID, &member.ImageID); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func groupIDForAnchor(ctx context.Context, transaction *sql.Tx, anchor Member) (string, error) {
	var groupID string
	err := transaction.QueryRowContext(ctx,
		`SELECT group_id FROM routing_group_members WHERE node_id = ? AND image_id = ?`,
		anchor.NodeID, anchor.ImageID).Scan(&groupID)
	if err == nil {
		return groupID, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	return anchor.NodeID + "\x00" + anchor.ImageID, nil
}

// A member joining another group can leave its old one with a single model in it,
// which is no longer a group: there is nowhere to offload to, and leaving the row
// behind would let a later edit resurrect a peer that has since moved away.
func deleteUndersizedGroups(ctx context.Context, transaction *sql.Tx) error {
	if _, err := transaction.ExecContext(ctx,
		`DELETE FROM routing_group_members WHERE group_id IN (
			SELECT group_id FROM routing_group_members GROUP BY group_id HAVING COUNT(*) < 2
		)`); err != nil {
		return err
	}
	_, err := transaction.ExecContext(ctx,
		`DELETE FROM routing_groups WHERE id NOT IN (SELECT DISTINCT group_id FROM routing_group_members)`)
	return err
}

func normalizeMember(member Member) Member {
	return Member{NodeID: strings.TrimSpace(member.NodeID), ImageID: strings.TrimSpace(member.ImageID)}
}

func dedupeMembers(members []Member) []Member {
	seen := map[Member]struct{}{}
	result := make([]Member, 0, len(members))
	for _, member := range members {
		member = normalizeMember(member)
		if member.NodeID == "" || member.ImageID == "" {
			continue
		}
		if _, duplicate := seen[member]; duplicate {
			continue
		}
		seen[member] = struct{}{}
		result = append(result, member)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].NodeID != result[right].NodeID {
			return result[left].NodeID < result[right].NodeID
		}
		return result[left].ImageID < result[right].ImageID
	})
	return result
}
