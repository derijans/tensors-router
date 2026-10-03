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
	koboldNativeStreamPath    = "/api/extra/generate/stream"
	koboldPerfPath            = "/api/extra/perf"
	koboldPerfResponseLimit   = 64 * 1024
	koboldPerfRequestDeadline = 5 * time.Second
)

type koboldGenerationReport struct {
	TotalGenerations int64 `json:"total_gens"`
	InputTokens      int64 `json:"last_input_count"`
	OutputTokens     int64 `json:"last_token_count"`
}

type koboldPerfReader struct {
	client     *http.Client
	backendURL *url.URL
}

func routerReportsKoboldStreamUsage(path string, backendMode string) bool {
	return backendMode == BackendModeKobold && path == koboldNativeStreamPath
}

func forwardReportingKoboldStreamUsage(ctx context.Context, perf koboldPerfReader, send func() (*http.Response, error)) (*http.Response, error) {
	before, baselineErr := perf.read(ctx)
	response, err := send()
	if baselineErr != nil || err != nil || response == nil || response.StatusCode != http.StatusOK || !isEventStream(response.Header) {
		return response, err
	}
	response.Body = &koboldUsageAppender{
		ctx:               ctx,
		source:            response.Body,
		perf:              perf,
		generationsBefore: before.TotalGenerations,
	}
	return response, nil
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
	after, err := appender.perf.read(appender.ctx)
	if err != nil || !after.provesSingleGenerationSince(appender.generationsBefore) {
		return nil
	}
	payload, err := json.Marshal(map[string]map[string]int64{"usage": {
		"prompt_tokens":     after.InputTokens,
		"completion_tokens": after.OutputTokens,
	}})
	if err != nil {
		return nil
	}
	event := make([]byte, 0, len(payload)+10)
	if appender.lastByte != 0 && appender.lastByte != '\n' {
		event = append(event, '\n')
	}
	event = append(event, "data: "...)
	event = append(event, payload...)
	return append(event, "\n\n"...)
}

func (report koboldGenerationReport) provesSingleGenerationSince(generationsBefore int64) bool {
	return report.TotalGenerations == generationsBefore+1 && report.OutputTokens > 0
}
