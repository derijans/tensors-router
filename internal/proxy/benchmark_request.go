package proxy

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"mime/multipart"
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
	return fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"Count from one upward using comma separated numbers."}],"max_tokens":64,"temperature":0,"stream":false}`, modelID)
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

func transcriptionBenchmarkBody(modelID string) (string, string, error) {
	var content bytes.Buffer
	writer := multipart.NewWriter(&content)
	if err := writer.WriteField("model", modelID); err != nil {
		return "", "", err
	}
	audio, err := writer.CreateFormFile("file", "benchmark.wav")
	if err != nil {
		return "", "", err
	}
	if _, err := audio.Write(benchmarkSilenceWAV()); err != nil {
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	return writer.FormDataContentType(), content.String(), nil
}

func benchmarkSilenceWAV() []byte {
	const sampleRate = 16000
	const sampleCount = sampleRate / 10
	const headerSize = 44
	content := make([]byte, headerSize+sampleCount*2)
	copy(content[0:4], "RIFF")
	binary.LittleEndian.PutUint32(content[4:8], uint32(len(content)-8))
	copy(content[8:12], "WAVE")
	copy(content[12:16], "fmt ")
	binary.LittleEndian.PutUint32(content[16:20], 16)
	binary.LittleEndian.PutUint16(content[20:22], 1)
	binary.LittleEndian.PutUint16(content[22:24], 1)
	binary.LittleEndian.PutUint32(content[24:28], sampleRate)
	binary.LittleEndian.PutUint32(content[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(content[32:34], 2)
	binary.LittleEndian.PutUint16(content[34:36], 16)
	copy(content[36:40], "data")
	binary.LittleEndian.PutUint32(content[40:44], sampleCount*2)
	return content
}

func musicBenchmarkBody(modelID string) string {
	return fmt.Sprintf(`{"model":%q,"prompt":"benchmark","seconds":1}`, modelID)
}
