package analytics

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	Period24Hours = "24h"
	Period7Days   = "7d"
	Period30Days  = "30d"
	Period90Days  = "90d"
	PeriodAll     = "all"
)

func QueryFromValues(values url.Values, now time.Time) (Query, error) {
	query := Query{
		Period:  strings.TrimSpace(values.Get("period")),
		NodeID:  strings.TrimSpace(values.Get("node_id")),
		ModelID: strings.TrimSpace(values.Get("model_id")),
		Section: strings.TrimSpace(values.Get("section")),
	}
	if query.Period == "" {
		query.Period = Period24Hours
	}
	if !validPeriod(query.Period) {
		return Query{}, fmt.Errorf("analytics period is invalid")
	}
	if !validSectionFilter(query.Section) {
		return Query{}, fmt.Errorf("analytics section is invalid")
	}
	var err error
	query.StartMS, err = optionalInt64(values.Get("start_ms"))
	if err != nil {
		return Query{}, fmt.Errorf("analytics start_ms is invalid")
	}
	query.EndMS, err = optionalInt64(values.Get("end_ms"))
	if err != nil {
		return Query{}, fmt.Errorf("analytics end_ms is invalid")
	}
	return NormalizeQuery(query, now)
}

func NormalizeQuery(query Query, now time.Time) (Query, error) {
	if query.Period == "" {
		query.Period = Period24Hours
	}
	query.NodeID = strings.TrimSpace(query.NodeID)
	query.ModelID = strings.TrimSpace(query.ModelID)
	query.Section = strings.TrimSpace(query.Section)
	if query.EndMS == 0 {
		query.EndMS = now.UnixMilli()
	}
	if query.StartMS == 0 && query.Period != PeriodAll {
		query.StartMS = query.EndMS - periodDuration(query.Period).Milliseconds()
	}
	if query.StartMS < 0 {
		query.StartMS = 0
	}
	if query.EndMS < query.StartMS {
		return Query{}, fmt.Errorf("analytics end_ms must be greater than start_ms")
	}
	return query, nil
}

func Granularity(query Query) string {
	if query.Period == Period24Hours {
		return "hour"
	}
	if query.EndMS > query.StartMS && query.EndMS-query.StartMS <= (48*time.Hour).Milliseconds() {
		return "hour"
	}
	return "day"
}

func optionalInt64(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	return strconv.ParseInt(value, 10, 64)
}

func validPeriod(value string) bool {
	switch value {
	case Period24Hours, Period7Days, Period30Days, Period90Days, PeriodAll:
		return true
	default:
		return false
	}
}

func validSectionFilter(value string) bool {
	switch value {
	case "", SectionLLM, SectionEmbed, SectionImage, SectionVoice, SectionMusic:
		return true
	default:
		return false
	}
}

func periodDuration(period string) time.Duration {
	switch period {
	case Period7Days:
		return 7 * 24 * time.Hour
	case Period30Days:
		return 30 * 24 * time.Hour
	case Period90Days:
		return 90 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}
