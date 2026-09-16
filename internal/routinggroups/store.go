package routinggroups

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

type Lane string

const (
	ImageLane Lane = "image"
	TextLane  Lane = "text"
)

var linkTablesByLane = map[Lane]string{
	ImageLane: "routing_image_links",
	TextLane:  "routing_text_links",
}

func (lane Lane) linkTable() (string, error) {
	table, known := linkTablesByLane[lane]
	if !known {
		return "", fmt.Errorf("unknown routing lane %q", lane)
	}
	return table, nil
}

// Endpoint names one model on one node. Endpoints are declared by an operator and
// are not required to share a name, a config hash, or even a checkpoint.
type Endpoint struct {
	NodeID  string `json:"node_id"`
	ModelID string `json:"model_id"`
}

func (endpoint Endpoint) normalized() Endpoint {
	return Endpoint{NodeID: strings.TrimSpace(endpoint.NodeID), ModelID: strings.TrimSpace(endpoint.ModelID)}
}

func (endpoint Endpoint) blank() bool {
	return endpoint.NodeID == "" || endpoint.ModelID == ""
}

func (endpoint Endpoint) String() string {
	return endpoint.NodeID + "/" + endpoint.ModelID
}

func (endpoint Endpoint) sortKey() string {
	return endpoint.NodeID + "\x00" + endpoint.ModelID
}

type Link struct {
	Owner              Endpoint `json:"owner"`
	Helper             Endpoint `json:"helper"`
	LoadIfUnloaded     bool     `json:"load_if_unloaded"`
	RestoreAfterBorrow bool     `json:"restore_after_borrow"`
}

func (link Link) touches(endpoint Endpoint) bool {
	return link.Owner == endpoint || link.Helper == endpoint
}

type linkKey struct {
	Owner  Endpoint
	Helper Endpoint
}

type Store struct {
	writer *sql.DB
	reader *sql.DB
}

func NewStore(writer *sql.DB, reader *sql.DB) *Store {
	return &Store{writer: writer, reader: reader}
}

func (store *Store) Links(ctx context.Context, lane Lane) ([]Link, error) {
	if store == nil {
		return nil, nil
	}
	table, err := lane.linkTable()
	if err != nil {
		return nil, err
	}
	rows, err := store.reader.QueryContext(ctx, fmt.Sprintf(
		`SELECT owner_node_id, owner_model_id, helper_node_id, helper_model_id, load_if_unloaded, restore_after_borrow
		 FROM %s ORDER BY owner_node_id, owner_model_id, helper_node_id, helper_model_id`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []Link
	for rows.Next() {
		var link Link
		var loadIfUnloaded, restoreAfterBorrow int
		if err := rows.Scan(&link.Owner.NodeID, &link.Owner.ModelID, &link.Helper.NodeID, &link.Helper.ModelID, &loadIfUnloaded, &restoreAfterBorrow); err != nil {
			return nil, err
		}
		link.LoadIfUnloaded = loadIfUnloaded != 0
		link.RestoreAfterBorrow = restoreAfterBorrow != 0
		links = append(links, link)
	}
	return links, rows.Err()
}

func (store *Store) ReplaceLinksTouching(ctx context.Context, lane Lane, anchor Endpoint, links []Link) error {
	if store == nil {
		return fmt.Errorf("routing link store is not configured")
	}
	table, err := lane.linkTable()
	if err != nil {
		return err
	}
	anchor = anchor.normalized()
	if anchor.blank() {
		return fmt.Errorf("anchor node_id and model_id are required")
	}
	wanted, err := anchorLinks(anchor, links)
	if err != nil {
		return err
	}

	transaction, err := store.writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = transaction.Rollback() }()

	if _, err := transaction.ExecContext(ctx, fmt.Sprintf(
		`DELETE FROM %s WHERE (owner_node_id = ? AND owner_model_id = ?) OR (helper_node_id = ? AND helper_model_id = ?)`, table),
		anchor.NodeID, anchor.ModelID, anchor.NodeID, anchor.ModelID); err != nil {
		return err
	}
	insert := fmt.Sprintf(
		`INSERT INTO %s(owner_node_id, owner_model_id, helper_node_id, helper_model_id, load_if_unloaded, restore_after_borrow)
		 VALUES (?, ?, ?, ?, ?, ?)`, table)
	for _, link := range wanted {
		if _, err := transaction.ExecContext(ctx, insert,
			link.Owner.NodeID, link.Owner.ModelID, link.Helper.NodeID, link.Helper.ModelID,
			sqliteBool(link.LoadIfUnloaded), sqliteBool(link.RestoreAfterBorrow)); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func anchorLinks(anchor Endpoint, links []Link) ([]Link, error) {
	byKey := map[linkKey]Link{}
	for _, link := range links {
		link.Owner = link.Owner.normalized()
		link.Helper = link.Helper.normalized()
		if link.Owner.blank() || link.Helper.blank() {
			continue
		}
		if !link.touches(anchor) {
			return nil, fmt.Errorf("link %s -> %s does not involve %s", link.Owner, link.Helper, anchor)
		}
		if link.Owner.NodeID == link.Helper.NodeID {
			return nil, fmt.Errorf("model %s cannot lend work to a model on its own node", link.Owner.ModelID)
		}
		key := linkKey{Owner: link.Owner, Helper: link.Helper}
		if _, duplicate := byKey[key]; !duplicate {
			byKey[key] = link
		}
	}
	result := make([]Link, 0, len(byKey))
	for _, link := range byKey {
		result = append(result, link)
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Owner != result[right].Owner {
			return result[left].Owner.sortKey() < result[right].Owner.sortKey()
		}
		return result[left].Helper.sortKey() < result[right].Helper.sortKey()
	})
	return result, nil
}

func sqliteBool(value bool) int {
	if value {
		return 1
	}
	return 0
}
