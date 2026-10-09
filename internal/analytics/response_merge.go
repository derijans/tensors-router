package analytics

import (
	"slices"
	"sort"
	"strings"
)

const mergedRecentLimit = 100

type responseMerger struct {
	merged   Response
	timeline map[int64]*Timeline
	sections map[string]*SectionUsage
	models   map[string]*ModelUsage
	nodes    map[string]*NodeUsage
	nodeIDs  map[string]struct{}
	modelIDs map[string]struct{}
}

func Merge(responses ...Response) Response {
	merger := responseMerger{
		merged:   Response{Filters: emptyFilters(), Recent: []RecentEvent{}, Versions: []VersionUsage{}},
		timeline: map[int64]*Timeline{},
		sections: map[string]*SectionUsage{},
		models:   map[string]*ModelUsage{},
		nodes:    map[string]*NodeUsage{},
		nodeIDs:  map[string]struct{}{},
		modelIDs: map[string]struct{}{},
	}
	for _, response := range responses {
		merger.add(response)
	}
	return merger.result()
}

func (merger *responseMerger) add(response Response) {
	merger.addHeader(response)
	addFilterValues(merger.nodeIDs, response.Filters.NodeIDs)
	addFilterValues(merger.modelIDs, response.Filters.ModelIDs)
	addSummary(&merger.merged.Summary, response.Summary)
	for _, item := range response.Timeline {
		existing := merger.timeline[item.BucketStart]
		if existing == nil {
			existing = &Timeline{BucketStart: item.BucketStart, Sections: []TimelineSection{}}
			merger.timeline[item.BucketStart] = existing
		}
		addTimeline(existing, item)
	}
	for _, item := range response.Sections {
		mergeKeyed(merger.sections, item.Section, item, addSection)
	}
	for _, item := range response.Models {
		mergeKeyed(merger.models, item.NodeID+"\x00"+item.ModelID, item, addModel)
	}
	for _, item := range response.Nodes {
		mergeKeyed(merger.nodes, item.NodeID, item, addNode)
	}
	merger.merged.Recent = append(merger.merged.Recent, response.Recent...)
	merger.merged.Versions = append(merger.merged.Versions, response.Versions...)
	merger.merged.NodeErrors = append(merger.merged.NodeErrors, response.NodeErrors...)
}

func (merger *responseMerger) addHeader(response Response) {
	merged := &merger.merged
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
}

func (merger *responseMerger) result() Response {
	merged := merger.merged
	merged.Timeline = mapValues(merger.timeline)
	merged.Sections = mapValues(merger.sections)
	merged.Models = mapValues(merger.models)
	merged.Nodes = mapValues(merger.nodes)
	sortMergedUsage(&merged)
	sortMergedHistory(&merged)
	merged.Filters = Filters{
		NodeIDs:  sortedFilterValues(merger.nodeIDs),
		ModelIDs: sortedFilterValues(merger.modelIDs),
	}
	if merged.Summary.RequestCount > 0 {
		merged.Summary.FailureCount = merged.Summary.RequestCount - merged.Summary.SuccessCount
	}
	return merged
}

func sortMergedUsage(merged *Response) {
	sort.Slice(merged.Timeline, func(left, right int) bool {
		return merged.Timeline[left].BucketStart < merged.Timeline[right].BucketStart
	})
	sort.Slice(merged.Sections, func(left, right int) bool {
		return merged.Sections[left].RequestCount > merged.Sections[right].RequestCount
	})
	sort.Slice(merged.Models, func(left, right int) bool {
		if merged.Models[left].RequestCount == merged.Models[right].RequestCount {
			return merged.Models[left].ModelID < merged.Models[right].ModelID
		}
		return merged.Models[left].RequestCount > merged.Models[right].RequestCount
	})
	sort.Slice(merged.Nodes, func(left, right int) bool {
		return merged.Nodes[left].NodeID < merged.Nodes[right].NodeID
	})
}

func sortMergedHistory(merged *Response) {
	sort.Slice(merged.Recent, func(left, right int) bool {
		return merged.Recent[left].FinishedAt > merged.Recent[right].FinishedAt
	})
	if len(merged.Recent) > mergedRecentLimit {
		merged.Recent = merged.Recent[:mergedRecentLimit]
	}
	sort.SliceStable(merged.Versions, func(left, right int) bool {
		if merged.Versions[left].NodeID != merged.Versions[right].NodeID {
			return merged.Versions[left].NodeID < merged.Versions[right].NodeID
		}
		return merged.Versions[left].FirstSeen < merged.Versions[right].FirstSeen
	})
}

func mergeKeyed[T any](index map[string]*T, key string, item T, add func(*T, T)) {
	if existing := index[key]; existing != nil {
		add(existing, item)
		return
	}
	index[key] = &item
}

func addFilterValues(values map[string]struct{}, candidates []string) {
	for _, candidate := range candidates {
		if candidate != "" {
			values[candidate] = struct{}{}
		}
	}
}

func sortedFilterValues(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
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
	left.EmbeddingCount += right.EmbeddingCount
	left.AudioSeconds += right.AudioSeconds
	left.AudioTokens += right.AudioTokens
	left.LoadCount += right.LoadCount
	left.AverageDuration = weightedAverage(previousRequests, left.AverageDuration, right.RequestCount, right.AverageDuration)
	left.AverageTokensPS = weightedAverage(previousRequests, left.AverageTokensPS, right.RequestCount, right.AverageTokensPS)
	left.AveragePromptPS = weightedAverage(previousRequests, left.AveragePromptPS, right.RequestCount, right.AveragePromptPS)
	left.AverageLoadMS = weightedAverage(previousLoads, left.AverageLoadMS, right.LoadCount, right.AverageLoadMS)
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPercent = maxFloat64(left.VRAMPeakPercent, right.VRAMPeakPercent)
	left.VRAMTotalMB = maxInt64(left.VRAMTotalMB, right.VRAMTotalMB)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func addTimeline(left *Timeline, right Timeline) {
	left.AverageTokensPS = weightedAverage(left.TokensPSSamples, left.AverageTokensPS, right.TokensPSSamples, right.AverageTokensPS)
	left.TokensPSSamples += right.TokensPSSamples
	left.AveragePromptPS = weightedAverage(left.PromptPSSamples, left.AveragePromptPS, right.PromptPSSamples, right.AveragePromptPS)
	left.PromptPSSamples += right.PromptPSSamples
	left.RequestCount += right.RequestCount
	left.FailureCount += right.FailureCount
	left.Sections = mergeTimelineSections(left.Sections, right.Sections)
	left.InputTokens += right.InputTokens
	left.OutputTokens += right.OutputTokens
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.EmbeddingCount += right.EmbeddingCount
	left.AudioSeconds += right.AudioSeconds
	left.LoadCount += right.LoadCount
	left.VRAMPeakMB = maxInt64(left.VRAMPeakMB, right.VRAMPeakMB)
	left.VRAMPeakPct = maxFloat64(left.VRAMPeakPct, right.VRAMPeakPct)
	left.VRAMTotalMB = maxInt64(left.VRAMTotalMB, right.VRAMTotalMB)
	left.ModelVRAMMB = maxInt64(left.ModelVRAMMB, right.ModelVRAMMB)
}

func mergeTimelineSections(left []TimelineSection, right []TimelineSection) []TimelineSection {
	merged := make([]TimelineSection, 0, len(left)+len(right))
	merged = append(merged, left...)
	for _, section := range right {
		index := slices.IndexFunc(merged, func(existing TimelineSection) bool { return existing.Section == section.Section })
		if index < 0 {
			merged = append(merged, section)
			continue
		}
		merged[index].RequestCount += section.RequestCount
	}
	slices.SortFunc(merged, func(first TimelineSection, second TimelineSection) int {
		return strings.Compare(first.Section, second.Section)
	})
	return merged
}

func addSection(left *SectionUsage, right SectionUsage) {
	left.RequestCount += right.RequestCount
	left.TotalTokens += right.TotalTokens
	left.ImageCount += right.ImageCount
	left.EmbeddingCount += right.EmbeddingCount
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
	left.EmbeddingCount += right.EmbeddingCount
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
	left.EmbeddingCount += right.EmbeddingCount
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
