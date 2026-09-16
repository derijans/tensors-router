package cluster

import (
	"encoding/json"
	"testing"
)

func eligibleTextModel() Model {
	model := testModel("llama", "node-a", "hash", "config", SourceLocal)
	model.HasLLM = true
	model.Capabilities.Context = 8192
	return model
}

func withOption(model Model, key string, value string) Model {
	clone := map[string]json.RawMessage{}
	for k, v := range model.Options {
		clone[k] = v
	}
	clone[key] = json.RawMessage(value)
	model.Options = clone
	return model
}

func TestTextGroupEligibleAcceptsAnOrdinarySerialModel(t *testing.T) {
	if !TextGroupEligible(eligibleTextModel()) {
		t.Fatal("an ordinary serial text model was rejected")
	}
}

func TestConcurrentConfigIsNotGroupable(t *testing.T) {
	if TextGroupEligible(withOption(eligibleTextModel(), "parallel", "4")) {
		t.Fatal("a config with parallel: 4 was accepted as groupable")
	}
	vllmConcurrent := withOption(eligibleTextModel(), "vllm", `{"settings":{"max_number_sequences":8}}`)
	if TextGroupEligible(vllmConcurrent) {
		t.Fatal("a vLLM config with max_number_sequences: 8 was accepted as groupable")
	}
}

func TestSingleSlotConfigsRemainGroupable(t *testing.T) {
	singleParallel := withOption(eligibleTextModel(), "parallel", "1")
	if !TextGroupEligible(singleParallel) {
		t.Fatal("parallel: 1 was rejected as concurrent")
	}
	vllmSingle := withOption(eligibleTextModel(), "vllm", `{"settings":{"max_number_sequences":1}}`)
	if !TextGroupEligible(vllmSingle) {
		t.Fatal("max_number_sequences: 1 was rejected as concurrent")
	}
}

func TestUnknownContextIsNotGroupable(t *testing.T) {
	model := eligibleTextModel()
	model.Capabilities.Context = 0
	if TextGroupEligible(model) {
		t.Fatal("a model with no stated context window was accepted as groupable")
	}
}
