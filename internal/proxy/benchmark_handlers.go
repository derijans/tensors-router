package proxy

import (
	"encoding/json"
	"net/http"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/openai"
)

func (runner *benchmarkRunner) handleBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	if !runner.deps.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if runner.deps.rejectModelLoadWhileDraining(w) {
		return
	}
	var request routerbenchmark.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	record, err := runner.runBenchmark(r.Context(), request, false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}

func (runner *benchmarkRunner) handleNodeBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	if runner.deps.rejectModelLoadWhileDraining(w) {
		return
	}
	var request routerbenchmark.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	record, err := runner.runBenchmark(r.Context(), request, true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}

func (runner *benchmarkRunner) handleBenchmarks(w http.ResponseWriter, r *http.Request) {
	if !runner.deps.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	record, err := runner.benchmarkRecord(r.Context(), r.URL.Query(), false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}

func (runner *benchmarkRunner) handleNodeBenchmarks(w http.ResponseWriter, r *http.Request) {
	record, err := runner.benchmarkRecord(r.Context(), r.URL.Query(), true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}
