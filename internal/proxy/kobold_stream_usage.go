package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const (
	koboldNativeStreamPath           = "/api/extra/generate/stream"
	koboldPerfPath                   = "/api/extra/perf"
	koboldPerfResponseLimit          = 64 * 1024
	koboldPerfRequestDeadline        = 5 * time.Second
	koboldShortestCrediblePromptTime = time.Millisecond
)

type koboldGenerationReport struct {
	TotalGenerations   int64   `json:"total_gens"`
	InputTokens        int64   `json:"last_input_count"`
	OutputTokens       int64   `json:"last_token_count"`
	PromptSeconds      float64 `json:"last_process_time"`
	PromptTokensPerSec float64 `json:"last_process_speed"`
}

type koboldUsageTrailer struct {
	Usage koboldUsage `json:"usage"`
}

type koboldUsage struct {
	PromptTokens          int64   `json:"prompt_tokens"`
	CompletionTokens      int64   `json:"completion_tokens"`
	PromptTokensPerSecond float64 `json:"prompt_tokens_per_second,omitempty"`
}

type koboldPerfReader struct {
	client     *http.Client
	backendURL *url.URL
}

type koboldStreamUsageReporter func(ctx context.Context, source io.ReadCloser, perf koboldPerfReader, generationsBefore int64) io.ReadCloser

func routerReportsKoboldStreamUsage(path string, backendMode string) bool {
	return backendMode == BackendModeKobold && path == koboldNativeStreamPath
}

func koboldStreamUsageReporterFor(path string, backendMode string, body []byte) (koboldStreamUsageReporter, bool) {
	switch {
	case routerReportsKoboldStreamUsage(path, backendMode):
		return appendKoboldNativeStreamUsage, true
	case routerSuppliesKoboldOpenAIStreamUsage(path, backendMode, body):
		return supplyKoboldOpenAIStreamUsage, true
	default:
		return nil, false
	}
}

func forwardReportingKoboldStreamUsage(ctx context.Context, perf koboldPerfReader, report koboldStreamUsageReporter, send func() (*http.Response, error)) (*http.Response, error) {
	before, baselineErr := perf.read(ctx)
	response, err := send()
	if baselineErr != nil || err != nil || response == nil || response.StatusCode != http.StatusOK || !isEventStream(response.Header) {
		return response, err
	}
	response.Body = report(ctx, response.Body, perf, before.TotalGenerations)
	return response, nil
}

func appendKoboldNativeStreamUsage(ctx context.Context, source io.ReadCloser, perf koboldPerfReader, generationsBefore int64) io.ReadCloser {
	return &koboldUsageAppender{
		ctx:               ctx,
		source:            source,
		perf:              perf,
		generationsBefore: generationsBefore,
	}
}

func (perf koboldPerfReader) read(ctx context.Context) (koboldGenerationReport, error) {
	ctx, cancel := context.WithTimeout(ctx, koboldPerfRequestDeadline)
	defer cancel()
	target := *perf.backendURL
	target.Path = joinPath(target.Path, koboldPerfPath)
	target.RawQuery = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return koboldGenerationReport{}, err
	}
	response, err := perf.client.Do(request)
	if err != nil {
		return koboldGenerationReport{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return koboldGenerationReport{}, fmt.Errorf("kobold perf returned status %d", response.StatusCode)
	}
	var report koboldGenerationReport
	if err := json.NewDecoder(io.LimitReader(response.Body, koboldPerfResponseLimit)).Decode(&report); err != nil {
		return koboldGenerationReport{}, err
	}
	return report, nil
}

func (perf koboldPerfReader) usageOfSingleGenerationSince(ctx context.Context, generationsBefore int64) (koboldUsage, bool) {
	after, err := perf.read(ctx)
	if err != nil || !after.provesSingleGenerationSince(generationsBefore) {
		return koboldUsage{}, false
	}
	return koboldUsage{
		PromptTokens:          after.InputTokens,
		CompletionTokens:      after.OutputTokens,
		PromptTokensPerSecond: after.promptTokensPerSecond(),
	}, true
}

func serverSentDataEvent(payload any) []byte {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil
	}
	event := make([]byte, 0, len(encoded)+8)
	event = append(event, "data: "...)
	event = append(event, encoded...)
	return append(event, "\n\n"...)
}

type koboldUsageAppender struct {
	ctx               context.Context
	source            io.ReadCloser
	perf              koboldPerfReader
	generationsBefore int64
	lastByte          byte
	trailer           *bytes.Reader
}

func (appender *koboldUsageAppender) Read(p []byte) (int, error) {
	if appender.trailer != nil {
		return appender.trailer.Read(p)
	}
	read, err := appender.source.Read(p)
	if read > 0 {
		appender.lastByte = p[read-1]
	}
	if err != io.EOF {
		return read, err
	}
	appender.trailer = bytes.NewReader(appender.usageEvent())
	if read > 0 {
		return read, nil
	}
	return appender.trailer.Read(p)
}

func (appender *koboldUsageAppender) Close() error {
	return appender.source.Close()
}

func (appender *koboldUsageAppender) usageEvent() []byte {
	usage, ok := appender.perf.usageOfSingleGenerationSince(appender.ctx, appender.generationsBefore)
	if !ok {
		return nil
	}
	event := serverSentDataEvent(koboldUsageTrailer{Usage: usage})
	if event == nil || appender.lastByte == 0 || appender.lastByte == '\n' {
		return event
	}
	return append([]byte{'\n'}, event...)
}

func (report koboldGenerationReport) promptTokensPerSecond() float64 {
	if time.Duration(report.PromptSeconds*float64(time.Second)) < koboldShortestCrediblePromptTime {
		return 0
	}
	return report.PromptTokensPerSec
}

func (report koboldGenerationReport) provesSingleGenerationSince(generationsBefore int64) bool {
	return report.TotalGenerations == generationsBefore+1 && report.OutputTokens > 0
}
