package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	routerbenchmark "tensors-router/internal/benchmark"
)

type benchmarkCommand struct {
	URL     string
	Token   string
	JSON    bool
	Request routerbenchmark.RunRequest
}

func runBenchmark(args []string) error {
	command, err := parseBenchmarkCommand(args)
	if err != nil {
		return err
	}
	record, err := executeBenchmarkCommand(context.Background(), command)
	if err != nil {
		return err
	}
	if command.JSON {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(record)
	}
	printBenchmarkRecord(record)
	return nil
}

func parseBenchmarkCommand(args []string) (benchmarkCommand, error) {
	flags := flag.NewFlagSet("benchmark", flag.ContinueOnError)
	modelID := flags.String("model", "", "model id")
	benchmarkType := flags.String("type", routerbenchmark.TypeGeneral, "benchmark type")
	sections := flags.String("sections", routerbenchmark.SectionAll, "comma separated sections")
	nodeID := flags.String("node", "", "node id")
	routerURL := flags.String("url", "http://127.0.0.1:8080", "router url")
	token := flags.String("token", "", "router bearer token")
	jsonOutput := flags.Bool("json", false, "print json")
	timeoutSeconds := flags.Int("timeout", 1800, "timeout in seconds")
	iterations := flags.Int("iterations", 1, "iterations per section")
	if err := flags.Parse(args); err != nil {
		return benchmarkCommand{}, err
	}
	request := routerbenchmark.RunRequest{
		NodeID:         strings.TrimSpace(*nodeID),
		ModelID:        strings.TrimSpace(*modelID),
		Type:           strings.TrimSpace(*benchmarkType),
		Sections:       splitBenchmarkSections(*sections),
		Iterations:     *iterations,
		TimeoutSeconds: *timeoutSeconds,
	}
	if request.ModelID == "" {
		return benchmarkCommand{}, fmt.Errorf("--model is required")
	}
	return benchmarkCommand{
		URL:     strings.TrimRight(strings.TrimSpace(*routerURL), "/"),
		Token:   strings.TrimSpace(*token),
		JSON:    *jsonOutput,
		Request: request,
	}, nil
}

func executeBenchmarkCommand(ctx context.Context, command benchmarkCommand) (routerbenchmark.Record, error) {
	body, err := json.Marshal(command.Request)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	timeout := time.Duration(command.Request.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Minute
	}
	requestContext, cancel := context.WithTimeout(ctx, timeout+30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestContext, http.MethodPost, command.URL+"/router/v1/benchmarks/run", bytes.NewReader(body))
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if command.Token != "" {
		request.Header.Set("Authorization", "Bearer "+command.Token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	defer response.Body.Close()
	content, err := io.ReadAll(response.Body)
	if err != nil {
		return routerbenchmark.Record{}, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return routerbenchmark.Record{}, fmt.Errorf("benchmark failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(content)))
	}
	var record routerbenchmark.Record
	if err := json.Unmarshal(content, &record); err != nil {
		return routerbenchmark.Record{}, err
	}
	return record, nil
}

func printBenchmarkRecord(record routerbenchmark.Record) {
	if record.Latest != nil {
		fmt.Printf("benchmark %s/%s %s %s\n", record.NodeID, record.ModelID, record.Latest.Status, benchmarkMetricLine(*record.Latest))
	}
	for _, section := range routerbenchmark.OrderedSections {
		summary, ok := record.Sections[section]
		if !ok {
			continue
		}
		fmt.Printf("  %s: %s %s", section, summary.Status, benchmarkMetricLine(summary))
		if summary.Error != "" {
			fmt.Printf(" %s", summary.Error)
		}
		fmt.Println()
	}
}

func benchmarkMetricLine(summary routerbenchmark.Summary) string {
	parts := []string{fmt.Sprintf("duration=%dms", summary.DurationMS)}
	for _, name := range []string{
		routerbenchmark.MetricModelLoadMS,
		routerbenchmark.MetricTotalStartMS,
		routerbenchmark.MetricTokensPerSecond,
		routerbenchmark.MetricCompletionTokens,
		routerbenchmark.MetricRequestMS,
	} {
		if metric, ok := benchmarkMetricByName(summary, name); ok {
			parts = append(parts, formatBenchmarkMetric(metric))
		}
	}
	return strings.Join(parts, " ")
}

func benchmarkMetricByName(summary routerbenchmark.Summary, name string) (routerbenchmark.Metric, bool) {
	for _, metric := range summary.Metrics {
		if metric.Name == name {
			return metric, true
		}
	}
	return routerbenchmark.Metric{}, false
}

func formatBenchmarkMetric(metric routerbenchmark.Metric) string {
	if metric.Unit == "ms" || metric.DurationMS > 0 {
		return fmt.Sprintf("%s=%dms", metric.Name, metric.DurationMS)
	}
	if metric.Unit != "" {
		return fmt.Sprintf("%s=%.2f %s", metric.Name, metric.Value, metric.Unit)
	}
	return fmt.Sprintf("%s=%.2f", metric.Name, metric.Value)
}

func splitBenchmarkSections(value string) []string {
	parts := strings.Split(value, ",")
	sections := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			sections = append(sections, part)
		}
	}
	return sections
}
