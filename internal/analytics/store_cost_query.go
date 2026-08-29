package analytics

import (
	"context"
	"time"
)

// CostSample is the aggregated form of every successful request recorded for one
// model, reduced to the sums an ordinary least squares fit needs. Only the sums
// leave the store: raw rows never cross a node boundary.
type CostSample struct {
	NodeID          string
	ModelID         string
	Section         string
	Count           int64
	SumWork         float64
	SumDuration     float64
	SumWorkDuration float64
	SumWorkSquared  float64
}

type LoadCostSample struct {
	NodeID         string
	ConfigFilename string
	Count          int64
	SumDuration    float64
}

// imageWorkExpression is the pre-dispatch size of an image job. Every factor is
// cast to REAL first: the squared term reaches ~10^15 for a single large request
// and would overflow SQLite integer arithmetic once summed over a day of traffic.
const imageWorkExpression = `CAST(image_steps AS REAL) * CAST(image_width AS REAL) * CAST(image_height AS REAL) * CAST(MAX(image_count, 1) AS REAL)`

// ImageWork is the same quantity computed in Go, for a request that has not run
// yet. The fit and the live estimate have to agree exactly or a prediction is
// made in different units from the coefficients it uses, so the two definitions
// are kept side by side and pinned together by TestImageWorkMatchesTheFitExpression.
// A request that does not state its size returns zero, which leaves it unpriced.
func ImageWork(event Event) float64 {
	if event.ImageSteps <= 0 || event.ImageWidth <= 0 || event.ImageHeight <= 0 {
		return 0
	}
	count := event.ImageCount
	if count < 1 {
		count = 1
	}
	return float64(event.ImageSteps) * float64(event.ImageWidth) * float64(event.ImageHeight) * float64(count)
}

// CostSamples reads raw rows rather than rollups. Rollups keep totals but discard
// the per-request work and duration pairing that a fit needs, so the window has
// to stay inside analytics.raw_retention to return anything.
func (store *Store) CostSamples(ctx context.Context, section string, window time.Duration, now time.Time) ([]CostSample, []LoadCostSample, error) {
	if store == nil {
		return nil, nil, nil
	}
	if err := store.Flush(ctx); err != nil {
		return nil, nil, err
	}
	since := now.Add(-window).UnixMilli()
	samples, err := store.requestCostSamples(ctx, section, since)
	if err != nil {
		return nil, nil, err
	}
	loads, err := store.loadCostSamples(ctx, section, since)
	if err != nil {
		return nil, nil, err
	}
	return samples, loads, nil
}

func (store *Store) requestCostSamples(ctx context.Context, section string, since int64) ([]CostSample, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT model_id, COUNT(*),
			SUM(work), SUM(duration), SUM(work * duration), SUM(work * work)
		FROM (
			SELECT model_id, CAST(duration_ms AS REAL) AS duration, `+imageWorkExpression+` AS work
			FROM analytics_events
			WHERE event_type = ? AND success = 1 AND section = ? AND finished_at >= ?
				AND duration_ms > 0 AND image_steps > 0 AND image_width > 0 AND image_height > 0
		)
		GROUP BY model_id`, EventTypeRequest, section, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []CostSample
	for rows.Next() {
		sample := CostSample{NodeID: store.nodeID, Section: section}
		if err := rows.Scan(
			&sample.ModelID,
			&sample.Count,
			&sample.SumWork,
			&sample.SumDuration,
			&sample.SumWorkDuration,
			&sample.SumWorkSquared,
		); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}

func (store *Store) loadCostSamples(ctx context.Context, section string, since int64) ([]LoadCostSample, error) {
	rows, err := store.db.QueryContext(ctx, `SELECT config_filename, COUNT(*), SUM(CAST(duration_ms AS REAL))
		FROM analytics_events
		WHERE event_type = ? AND success = 1 AND section = ? AND finished_at >= ?
			AND duration_ms > 0 AND config_filename <> ''
		GROUP BY config_filename`, EventTypeModelLoad, section, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var samples []LoadCostSample
	for rows.Next() {
		sample := LoadCostSample{NodeID: store.nodeID}
		if err := rows.Scan(&sample.ConfigFilename, &sample.Count, &sample.SumDuration); err != nil {
			return nil, err
		}
		samples = append(samples, sample)
	}
	return samples, rows.Err()
}
