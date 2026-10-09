package native

import (
	"fmt"
	"strings"

	"tensors-router/internal/catalog"
)

const (
	gpuLayersFlag   = "--n-gpu-layers"
	deviceFlag      = "--device"
	splitModeFlag   = "--split-mode"
	tensorSplitFlag = "--tensor-split"
	mainGPUFlag     = "--main-gpu"
)

func llamaEmbeddingArguments(metadata catalog.RuntimeConfig, target launchTarget) ([]string, error) {
	modelPath := strings.TrimSpace(metadata.EmbeddingsModel)
	if modelPath == "" {
		return nil, fmt.Errorf("llama embeddings config has no embeddingsmodel")
	}
	if !metadata.RunEmbedSeparate {
		return nil, fmt.Errorf("llama embeddings config does not enable run_embed_separate")
	}
	args := []string{
		"--host", target.host,
		"--port", target.port,
		"--model", modelPath,
		"--alias", target.modelID,
		"--embeddings",
	}
	appendIntArg(&args, "--ctx-size", metadata.EmbeddingsMaxCtx)
	appendIntArg(&args, "--threads", metadata.Threads)
	appendIntArg(&args, "--threads-batch", metadata.BLASThreads)
	appendIntArg(&args, "--batch-size", metadata.BatchSize)
	appendIntArg(&args, "--ubatch-size", metadata.UBatchSize)
	if metadata.EmbeddingsGPU {
		args = append(args, gpuLayersFlag, "-1")
		appendStringArg(&args, deviceFlag, metadata.Device)
		appendStringArg(&args, splitModeFlag, metadata.SplitMode)
		appendStringArg(&args, tensorSplitFlag, metadata.TensorSplitValue())
		appendIntArg(&args, mainGPUFlag, nonNegative(metadata.MainGPU))
		appendStringArg(&args, "--rpc", metadata.RPCTargets)
	} else {
		args = append(args, deviceFlag, "none", gpuLayersFlag, "0")
	}
	if err := appendLlamaLoadArguments(&args, metadata); err != nil {
		return nil, err
	}
	appendStringArg(&args, "--pooling", metadata.Pooling)
	return args, nil
}

func identityArgs(_ catalog.RuntimeConfig, args []string) []string {
	return append([]string(nil), args...)
}

func embeddingExtraArgs(metadata catalog.RuntimeConfig, args []string) []string {
	owned := embeddingRuntimeOwnedArguments(metadata)
	filtered := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		argument := args[index]
		key := argument
		if separator := strings.IndexByte(key, '='); separator >= 0 {
			key = key[:separator]
		}
		takesValue, managed := owned[key]
		if !managed {
			filtered = append(filtered, argument)
			continue
		}
		if argument == key && takesValue && index+1 < len(args) {
			index++
		}
	}
	return filtered
}

func embeddingRuntimeOwnedArguments(metadata catalog.RuntimeConfig) map[string]bool {
	owned := map[string]bool{
		"--host": true, "--port": true, "--model": true, "--alias": true,
		"--embeddings": false, "--ctx-size": true,
		gpuLayersFlag: true, "--gpu-layers": true, "-ngl": true, "--no-gpu": false,
	}
	if strings.TrimSpace(metadata.Pooling) != "" {
		owned["--pooling"] = true
	}
	if !metadata.EmbeddingsGPU {
		addEmbeddingPlacementArguments(owned)
		return owned
	}
	if strings.TrimSpace(metadata.Device) != "" {
		owned[deviceFlag] = true
		owned["-dev"] = true
	}
	if strings.TrimSpace(metadata.SplitMode) != "" {
		owned[splitModeFlag] = true
		owned["-sm"] = true
	}
	if metadata.TensorSplitValue() != "" {
		owned[tensorSplitFlag] = true
		owned["-ts"] = true
	}
	if metadata.MainGPUSet && metadata.MainGPU >= 0 {
		owned[mainGPUFlag] = true
		owned["-mg"] = true
	}
	if strings.TrimSpace(metadata.RPCTargets) != "" {
		owned["--rpc"] = true
	}
	return owned
}

func addEmbeddingPlacementArguments(arguments map[string]bool) {
	for _, argument := range []string{deviceFlag, "-dev", splitModeFlag, "-sm", tensorSplitFlag, "-ts", mainGPUFlag, "-mg", "--rpc"} {
		arguments[argument] = true
	}
}
