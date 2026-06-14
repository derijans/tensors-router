package proxy

import (
	"fmt"
	"strings"
	"time"

	routerbenchmark "tensors-router/internal/benchmark"
)

const (
	defaultBenchmarkTimeout = 30 * time.Minute
	maxBenchmarkTimeout     = 2 * time.Hour
	maxBenchmarkIterations  = 20
)

func normalizeBenchmarkRequest(request routerbenchmark.RunRequest) (routerbenchmark.RunRequest, error) {
	request.NodeID = strings.TrimSpace(request.NodeID)
	request.ModelID = strings.TrimSpace(request.ModelID)
	request.Type = strings.TrimSpace(request.Type)
	if request.Type == "" {
		request.Type = routerbenchmark.TypeGeneral
	}
	if request.Type != routerbenchmark.TypeGeneral && request.Type != routerbenchmark.TypeSection {
		return routerbenchmark.RunRequest{}, fmt.Errorf("benchmark type must be general or section")
	}
	if request.Iterations < 1 {
		request.Iterations = 1
	}
	if request.Iterations > maxBenchmarkIterations {
		request.Iterations = maxBenchmarkIterations
	}
	sections, err := normalizedSectionList(request.Sections)
	if err != nil {
		return routerbenchmark.RunRequest{}, err
	}
	request.Sections = sections
	return request, nil
}

func expandBenchmarkSections(request routerbenchmark.RunRequest) []string {
	if request.Type == routerbenchmark.TypeGeneral {
		return append([]string{}, routerbenchmark.OrderedSections...)
	}
	return request.Sections
}

func normalizedSectionList(sections []string) ([]string, error) {
	if len(sections) == 0 {
		return append([]string{}, routerbenchmark.OrderedSections...), nil
	}
	seen := map[string]struct{}{}
	values := make([]string, 0, len(sections))
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if section == routerbenchmark.SectionAll {
			return append([]string{}, routerbenchmark.OrderedSections...), nil
		}
		if !validBenchmarkSection(section) {
			return nil, fmt.Errorf("unknown benchmark section %q", section)
		}
		if _, ok := seen[section]; ok {
			continue
		}
		seen[section] = struct{}{}
		values = append(values, section)
	}
	if len(values) == 0 {
		return append([]string{}, routerbenchmark.OrderedSections...), nil
	}
	return values, nil
}

func validBenchmarkSection(section string) bool {
	for _, value := range routerbenchmark.OrderedSections {
		if section == value {
			return true
		}
	}
	return false
}

func benchmarkTimeout(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultBenchmarkTimeout
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > maxBenchmarkTimeout {
		return maxBenchmarkTimeout
	}
	return timeout
}

func textBenchmarkBody(modelID string) string {
	return fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Say ok."}],"max_tokens":1,"stream":false}`, modelID)
}

func embeddingsBenchmarkBody(modelID string) string {
	return fmt.Sprintf(`{"model":%q,"input":"benchmark"}`, modelID)
}

func imageBenchmarkBody(modelID string) string {
	return fmt.Sprintf(`{"model":%q,"prompt":"benchmark","width":64,"height":64,"steps":1,"n":1}`, modelID)
}

func voiceBenchmarkBody(modelID string) string {
	return fmt.Sprintf(`{"model":%q,"input":"benchmark","voice":"alloy","response_format":"wav"}`, modelID)
}

func musicBenchmarkBody(modelID string) string {
	return fmt.Sprintf(`{"model":%q,"prompt":"benchmark","seconds":1}`, modelID)
}
