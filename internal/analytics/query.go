package analytics

import (
	"fmt"
	"net/url"
	"sort"
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

func Merge(responses ...Response) Response {
	merged := Response{}
	timelineByBucket := map[int64]*Timeline{}
	sectionsByName := map[string]*SectionUsage{}
	modelsByKey := map[string]*ModelUsage{}
	nodesByName := map[string]*NodeUsage{}

	for _, response := range responses {
		merged.Enabled = merged.Enabled || response.Enabled
		if merged.From == 0 || (response.From > 0 && response.From < merged.From) {
			merged.From = response.From
		}
		if response.To > merged.To {
			merged.To = response.To
		}
		if merged.Granularity == "" {
			merged.Granularity = response.Granularity
		}
		addSummary(&merged.Summary, response.Summary)
		for _, item := range response.Timeline {
			existing := timelineByBucket[item.BucketStart]
			if existing == nil {
				itemCopy := item
				timelineByBucket[item.BucketStart] = &itemCopy
				continue
			}
			addTimeline(existing, item)
		}
		for _, item := range response.Sections {
			existing := sectionsByName[item.Section]
			if existing == nil {
				itemCopy := item
				sectionsByName[item.Section] = &itemCopy
				continue
			}
			addSection(existing, item)
		}
		for _, item := range response.Models {
			key := item.NodeID + "\x00" + item.ModelID
			existing := modelsByKey[key]
			if existing == nil {
				itemCopy := item
				modelsByKey[key] = &itemCopy
				continue
			}
			addModel(existing, item)
		}
		for _, item := range response.Nodes {
			existing := nodesByName[item.NodeID]
			if existing == nil {
				itemCopy := item
				nodesByName[item.NodeID] = &itemCopy
				continue
			}
			addNode(existing, item)
		}
		merged.Recent = append(merged.Recent, response.Recent...)
		merged.NodeErrors = append(merged.NodeErrors, response.NodeErrors...)
	}

	merged.Timeline = mapValues(timelineByBucket)
	sort.Slice(merged.Timeline, func(left, right int) bool {
		return merged.Timeline[left].BucketStart < merged.Timeline[right].BucketStart
	})
	merged.Sections = mapValues(sectionsByName)
	sort.Slice(merged.Sections, func(left, right int) bool {
		return merged.Sections[left].RequestCount > merged.Sections[right].RequestCount
	})
	merged.Models = mapValues(modelsByKey)
	sort.Slice(merged.Models, func(left, right int) bool {
		if merged.Models[left].RequestCount == merged.Models[right].RequestCount {
			return merged.Models[left].ModelID < merged.Models[right].ModelID
		}
		return merged.Models[left].RequestCount > merged.Models[right].RequestCount
	})
	merged.Nodes = mapValues(nodesByName)
	sort.Slice(merged.Nodes, func(left, right int) bool {
		return merged.Nodes[left].NodeID < merged.Nodes[right].NodeID
	})
	sort.Slice(merged.Recent, func(left, right int) bool {
		return merged.Recent[left].FinishedAt > merged.Recent[right].FinishedAt
	})
	if len(merged.Recent) > 100 {
		merged.Recent = merged.Recent[:100]
	}
	if merged.Summary.RequestCount > 0 {
		merged.Summary.FailureCount = merged.Summary.RequestCount - merged.Summary.SuccessCount
	}
	return merged
}

func addSummary(left *Summary, right Summary) {
	previousRequests := left.RequestCount
	previousLoads := left.LoadCount
	left.RequestCount += right.RequestCount
	left.SuccessCount += right.SuccessCount
	left.FailureCount += right.FailureCount
	left.InputTokens += right.InputTokens
	left.OutputTokens += right.OutputTokens
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.AudioSeconds += right.AudioSeconds
	left.AudioTokens += right.AudioTokens
	left.LoadCount += right.LoadCount
	left.AverageDuration = weightedAverage(previousRequests, left.AverageDuration, right.RequestCount, right.AverageDuration)
	left.AverageTokensPS = weightedAverage(previousRequests, left.AverageTokensPS, right.RequestCount, right.AverageTokensPS)
	left.AverageLoadMS = weightedAverage(previousLoads, left.AverageLoadMS, right.LoadCount, right.AverageLoadMS)
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPercent = maxFloat64(left.VRAMPeakPercent, right.VRAMPeakPercent)
	left.VRAMTotalMB = maxInt64(left.VRAMTotalMB, right.VRAMTotalMB)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func addTimeline(left *Timeline, right Timeline) {
	left.RequestCount += right.RequestCount
	left.InputTokens += right.InputTokens
	left.OutputTokens += right.OutputTokens
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.AudioSeconds += right.AudioSeconds
	left.LoadCount += right.LoadCount
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPct = maxFloat64(left.VRAMPeakPct, right.VRAMPeakPct)
	left.VRAMTotalMB = maxInt64(left.VRAMTotalMB, right.VRAMTotalMB)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func addSection(left *SectionUsage, right SectionUsage) {
	left.RequestCount += right.RequestCount
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.AudioSeconds += right.AudioSeconds
	left.LoadCount += right.LoadCount
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPct = maxFloat64(left.VRAMPeakPct, right.VRAMPeakPct)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func addModel(left *ModelUsage, right ModelUsage) {
	previousLoads := left.LoadCount
	left.RequestCount += right.RequestCount
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.AudioSeconds += right.AudioSeconds
	left.LoadCount += right.LoadCount
	left.AverageLoadMS = weightedAverage(previousLoads, left.AverageLoadMS, right.LoadCount, right.AverageLoadMS)
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPct = maxFloat64(left.VRAMPeakPct, right.VRAMPeakPct)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func addNode(left *NodeUsage, right NodeUsage) {
	previousLoads := left.LoadCount
	left.RequestCount += right.RequestCount
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.AudioSeconds += right.AudioSeconds
	left.LoadCount += right.LoadCount
	left.AverageLoadMS = weightedAverage(previousLoads, left.AverageLoadMS, right.LoadCount, right.AverageLoadMS)
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPct = maxFloat64(left.VRAMPeakPct, right.VRAMPeakPct)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func weightedAverage(leftCount int64, leftValue float64, rightCount int64, rightValue float64) float64 {
	total := leftCount + rightCount
	if total == 0 {
		return 0
	}
	return ((leftValue * float64(leftCount)) + (rightValue * float64(rightCount))) / float64(total)
}

func mapValues[K comparable, V any](values map[K]*V) []V {
	result := make([]V, 0, len(values))
	for _, value := range values {
		result = append(result, *value)
	}
	return result
}

func maxFloat64(left float64, right float64) float64 {
	if right > left {
		return right
	}
	return left
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
