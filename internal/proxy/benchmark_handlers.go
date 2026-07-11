package proxy

import (
	"encoding/json"
	"net/http"

	routerbenchmark "tensors-router/internal/benchmark"
	"tensors-router/internal/openai"
)

func (service *Service) handleBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	if service.rejectModelLoadWhileDraining(w) {
		return
	}
	var request routerbenchmark.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	record, err := service.runBenchmark(r.Context(), request, false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}

func (service *Service) handleNodeBenchmarkRun(w http.ResponseWriter, r *http.Request) {
	if service.rejectModelLoadWhileDraining(w) {
		return
	}
	var request routerbenchmark.RunRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		openai.WriteError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	record, err := service.runBenchmark(r.Context(), request, true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}

func (service *Service) handleBenchmarks(w http.ResponseWriter, r *http.Request) {
	if !service.siteControlAllowed() {
		openai.WriteError(w, http.StatusNotFound, "not_found", "endpoint not found")
		return
	}
	record, err := service.benchmarkRecord(r.Context(), r.URL.Query(), false)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}

func (service *Service) handleNodeBenchmarks(w http.ResponseWriter, r *http.Request) {
	record, err := service.benchmarkRecord(r.Context(), r.URL.Query(), true)
	if err != nil {
		openai.WriteError(w, http.StatusBadRequest, "benchmark_error", err.Error())
		return
	}
	openai.WriteJSON(w, http.StatusOK, record)
}
