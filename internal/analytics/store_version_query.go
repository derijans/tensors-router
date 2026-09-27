package analytics

import "context"

// queryVersions reads raw events only: rollups keep totals, not the build that
// recorded them, so versions cover the raw retention window at most.
func (store *Store) queryVersions(ctx context.Context, query Query) ([]VersionUsage, error) {
	where, args := eventWhere(query)
	rows, err := store.reader.QueryContext(ctx, `SELECT node_id, router_version, MIN(started_at), MAX(finished_at), COUNT(*)
		FROM analytics_events `+where+`
		GROUP BY node_id, router_version
		ORDER BY node_id, MIN(started_at)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := []VersionUsage{}
	for rows.Next() {
		var usage VersionUsage
		if err := rows.Scan(&usage.NodeID, &usage.RouterVersion, &usage.FirstSeen, &usage.LastSeen, &usage.EventCount); err != nil {
			return nil, err
		}
		versions = append(versions, usage)
	}
	return versions, rows.Err()
}
