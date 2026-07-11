package benchmark

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"tensors-router/internal/atomicfile"
)

const HistoryLimit = 25

type Store struct {
	mu       sync.Mutex
	path     string
	snapshot atomic.Pointer[storeFile]
	persist  func(storeFile) error
}

type storeFile struct {
	Version int                     `json:"version"`
	Records map[string]storedRecord `json:"records"`
}

type storedRecord struct {
	Record
	CurrentOptions map[string]json.RawMessage `json:"current_options,omitempty"`
}

type ModelKey struct {
	NodeID  string
	ModelID string
}

func NewStore(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("benchmark store dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	store := &Store{path: filepath.Join(dir, "benchmarks.json")}
	file, err := loadStoreFile(store.path)
	if err != nil {
		return nil, err
	}
	store.persist = store.persistStoreFile
	store.snapshot.Store(&file)
	return store, nil
}

func (store *Store) Record(nodeID string, modelID string) (Record, bool, error) {
	if store == nil {
		return Record{}, false, nil
	}
	stored, ok := store.snapshot.Load().Records[recordKey(nodeID, modelID)]
	if !ok {
		return Record{}, false, nil
	}
	return cloneRecord(stored.Record), true, nil
}

func (store *Store) ModelBenchmarks(keys []ModelKey) map[ModelKey]ModelBenchmark {
	result := make(map[ModelKey]ModelBenchmark, len(keys))
	if store == nil {
		return result
	}
	snapshot := store.snapshot.Load()
	for _, key := range keys {
		stored, ok := snapshot.Records[recordKey(key.NodeID, key.ModelID)]
		if !ok {
			continue
		}
		result[key] = modelBenchmarkFromRecord(stored.Record)
	}
	return result
}

func (store *Store) ModelBenchmark(nodeID string, modelID string) (ModelBenchmark, bool, error) {
	record, ok, err := store.Record(nodeID, modelID)
	if err != nil || !ok {
		return ModelBenchmark{}, ok, err
	}
	return ModelBenchmark{
		Latest:   record.Latest,
		Sections: record.Sections,
	}, true, nil
}

func (store *Store) SaveRun(nodeID string, modelID string, runType string, sections []Summary, options map[string]json.RawMessage) (Record, error) {
	if store == nil {
		return Record{}, fmt.Errorf("benchmark store is not configured")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	file := cloneStoreFile(*store.snapshot.Load())
	key := recordKey(nodeID, modelID)
	stored := file.Records[key]
	stored.NodeID = strings.TrimSpace(nodeID)
	stored.ModelID = strings.TrimSpace(modelID)
	if stored.Sections == nil {
		stored.Sections = map[string]Summary{}
	}
	changes := optionChanges(stored.CurrentOptions, options)
	for index := range sections {
		sections[index].OptionChanges = changes
		stored.Sections[sections[index].Section] = sections[index]
		stored.History = append(stored.History, sections[index])
	}
	latest := aggregateSummary(runType, sections)
	latest.OptionChanges = changes
	stored.Latest = &latest
	stored.CurrentOptions = cloneOptions(options)
	if len(stored.History) > HistoryLimit {
		stored.History = append([]Summary{}, stored.History[len(stored.History)-HistoryLimit:]...)
	}
	file.Records[key] = stored
	if err := store.persist(file); err != nil {
		return Record{}, err
	}
	store.snapshot.Store(&file)
	return cloneRecord(stored.Record), nil
}

func loadStoreFile(path string) (storeFile, error) {
	file := storeFile{
		Version: 1,
		Records: map[string]storedRecord{},
	}
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return file, nil
		}
		return file, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return file, nil
	}
	if err := json.Unmarshal(content, &file); err != nil {
		return storeFile{}, err
	}
	if file.Records == nil {
		file.Records = map[string]storedRecord{}
	}
	if file.Version == 0 {
		file.Version = 1
	}
	return file, nil
}

func (store *Store) persistStoreFile(file storeFile) error {
	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(store.path, body, 0o600)
}

func modelBenchmarkFromRecord(record Record) ModelBenchmark {
	cloned := cloneRecord(record)
	return ModelBenchmark{Latest: cloned.Latest, Sections: cloned.Sections}
}

func cloneStoreFile(file storeFile) storeFile {
	cloned := storeFile{Version: file.Version, Records: make(map[string]storedRecord, len(file.Records))}
	for key, stored := range file.Records {
		cloned.Records[key] = storedRecord{
			Record:         cloneRecord(stored.Record),
			CurrentOptions: cloneOptions(stored.CurrentOptions),
		}
	}
	return cloned
}

func cloneRecord(record Record) Record {
	cloned := record
	if record.Latest != nil {
		latest := cloneSummary(*record.Latest)
		cloned.Latest = &latest
	}
	if record.Sections != nil {
		cloned.Sections = make(map[string]Summary, len(record.Sections))
		for key, summary := range record.Sections {
			cloned.Sections[key] = cloneSummary(summary)
		}
	}
	cloned.History = make([]Summary, len(record.History))
	for index := range record.History {
		cloned.History[index] = cloneSummary(record.History[index])
	}
	return cloned
}

func cloneSummary(summary Summary) Summary {
	cloned := summary
	cloned.Metrics = append([]Metric{}, summary.Metrics...)
	cloned.OptionChanges = make([]OptionChange, len(summary.OptionChanges))
	for index, change := range summary.OptionChanges {
		cloned.OptionChanges[index] = OptionChange{
			Key:      change.Key,
			Kind:     change.Kind,
			Previous: cloneRaw(change.Previous),
			Current:  cloneRaw(change.Current),
		}
	}
	return cloned
}

func aggregateSummary(runType string, sections []Summary) Summary {
	latest := Summary{
		Type:    runType,
		Section: SectionAll,
		Status:  StatusSkipped,
	}
	if len(sections) == 0 {
		return latest
	}
	latest.RunID = sections[0].RunID
	latest.StartedAt = sections[0].StartedAt
	latest.FinishedAt = sections[0].FinishedAt
	success := 0
	failed := 0
	skipped := 0
	for _, section := range sections {
		if section.StartedAt < latest.StartedAt {
			latest.StartedAt = section.StartedAt
		}
		if section.FinishedAt > latest.FinishedAt {
			latest.FinishedAt = section.FinishedAt
		}
		switch section.Status {
		case StatusSuccess:
			success++
		case StatusFailed:
			failed++
		default:
			skipped++
		}
	}
	latest.DurationMS = latest.FinishedAt - latest.StartedAt
	switch {
	case failed == 0 && success > 0:
		latest.Status = StatusSuccess
	case failed > 0 && success > 0:
		latest.Status = StatusPartial
	case failed > 0:
		latest.Status = StatusFailed
	default:
		latest.Status = StatusSkipped
	}
	latest.Metrics = []Metric{
		durationMetric(MetricTotalRunMS, latest.DurationMS),
		countMetric(MetricSectionsTotal, len(sections)),
		countMetric(MetricSectionsSuccess, success),
		countMetric(MetricSectionsFailed, failed),
		countMetric(MetricSectionsSkipped, skipped),
	}
	latest.Metrics = append(latest.Metrics, aggregateStartupMetrics(sections)...)
	return latest
}

func aggregateStartupMetrics(sections []Summary) []Metric {
	loadMS, hasLoad := sectionMetricDuration(sections, SectionRuntime, MetricModelLoadMS)
	firstCompletionMS, hasFirstCompletion := sectionMetricDuration(sections, SectionLLM, MetricRequestMS)
	metrics := []Metric{}
	if hasLoad {
		metrics = append(metrics, durationMetric(MetricModelLoadMS, loadMS))
	}
	if hasFirstCompletion {
		metrics = append(metrics, durationMetric(MetricFirstCompletionMS, firstCompletionMS))
	}
	if hasLoad && hasFirstCompletion {
		metrics = append(metrics, durationMetric(MetricTotalStartMS, loadMS+firstCompletionMS))
	}
	if tokensPerSecond, ok := sectionMetricValue(sections, SectionLLM, MetricTokensPerSecond); ok {
		metrics = append(metrics, valueMetric(MetricTokensPerSecond, tokensPerSecond, "tokens/s"))
	}
	return metrics
}

func sectionMetricDuration(sections []Summary, section string, name string) (int64, bool) {
	for _, summary := range sections {
		if summary.Section != section {
			continue
		}
		for _, metric := range summary.Metrics {
			if metric.Name == name && metric.Status == StatusSuccess {
				return metric.DurationMS, true
			}
		}
	}
	return 0, false
}

func sectionMetricValue(sections []Summary, section string, name string) (float64, bool) {
	for _, summary := range sections {
		if summary.Section != section {
			continue
		}
		for _, metric := range summary.Metrics {
			if metric.Name == name && metric.Status == StatusSuccess {
				return metric.Value, true
			}
		}
	}
	return 0, false
}

func durationMetric(name string, durationMS int64) Metric {
	return Metric{
		Name:       name,
		Status:     StatusSuccess,
		DurationMS: durationMS,
		Unit:       "ms",
	}
}

func countMetric(name string, value int) Metric {
	return Metric{
		Name:   name,
		Status: StatusSuccess,
		Value:  float64(value),
		Unit:   "count",
	}
}

func valueMetric(name string, value float64, unit string) Metric {
	return Metric{
		Name:   name,
		Status: StatusSuccess,
		Value:  value,
		Unit:   unit,
	}
}

func optionChanges(previous map[string]json.RawMessage, current map[string]json.RawMessage) []OptionChange {
	if previous == nil {
		return []OptionChange{}
	}
	keys := map[string]struct{}{}
	for key := range previous {
		keys[key] = struct{}{}
	}
	for key := range current {
		keys[key] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)
	changes := make([]OptionChange, 0)
	for _, key := range ordered {
		left, leftOK := previous[key]
		right, rightOK := current[key]
		if leftOK && rightOK && sameRawJSON(left, right) {
			continue
		}
		change := OptionChange{Key: key}
		switch {
		case leftOK && rightOK:
			change.Kind = "changed"
			change.Previous = cloneRaw(left)
			change.Current = cloneRaw(right)
		case leftOK:
			change.Kind = "removed"
			change.Previous = cloneRaw(left)
		default:
			change.Kind = "added"
			change.Current = cloneRaw(right)
		}
		changes = append(changes, change)
	}
	return changes
}

func cloneOptions(options map[string]json.RawMessage) map[string]json.RawMessage {
	cloned := map[string]json.RawMessage{}
	for key, value := range options {
		cloned[key] = cloneRaw(value)
	}
	return cloned
}

func cloneRaw(value json.RawMessage) json.RawMessage {
	if value == nil {
		return nil
	}
	cloned := make(json.RawMessage, len(value))
	copy(cloned, value)
	return cloned
}

func sameRawJSON(left json.RawMessage, right json.RawMessage) bool {
	leftCompact, leftErr := compactRawJSON(left)
	rightCompact, rightErr := compactRawJSON(right)
	if leftErr == nil && rightErr == nil {
		return bytes.Equal(leftCompact, rightCompact)
	}
	return bytes.Equal(left, right)
}

func compactRawJSON(value json.RawMessage) ([]byte, error) {
	var buffer bytes.Buffer
	if err := json.Compact(&buffer, value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func recordKey(nodeID string, modelID string) string {
	return strings.TrimSpace(nodeID) + "\x00" + strings.TrimSpace(modelID)
}
