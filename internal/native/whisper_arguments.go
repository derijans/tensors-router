package native

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"tensors-router/internal/catalog"
)

var whisperCPPOptionFlags = map[string]string{
	"whispercpp_processors":                  "--processors",
	"whispercpp_offset_t":                    "--offset-t",
	"whispercpp_offset_n":                    "--offset-n",
	"whispercpp_duration":                    "--duration",
	"whispercpp_max_context":                 "--max-context",
	"whispercpp_max_len":                     "--max-len",
	"whispercpp_split_on_word":               "--split-on-word",
	"whispercpp_best_of":                     "--best-of",
	"whispercpp_beam_size":                   "--beam-size",
	"whispercpp_audio_ctx":                   "--audio-ctx",
	"whispercpp_word_threshold":              "--word-thold",
	"whispercpp_entropy_threshold":           "--entropy-thold",
	"whispercpp_logprob_threshold":           "--logprob-thold",
	"whispercpp_no_speech_threshold":         "--no-speech-thold",
	"whispercpp_debug":                       "--debug-mode",
	"whispercpp_translate":                   "--translate",
	"whispercpp_diarize":                     "--diarize",
	"whispercpp_tiny_diarize":                "--tinydiarize",
	"whispercpp_no_fallback":                 "--no-fallback",
	"whispercpp_language":                    "--language",
	"whispercpp_detect_language":             "--detect-language",
	"whispercpp_prompt":                      "--prompt",
	"whispercpp_carry_initial_prompt":        "--carry-initial-prompt",
	"whispercpp_openvino_device":             "--ov-e-device",
	"whispercpp_dtw":                         "--dtw",
	"whispercpp_suppress_non_speech":         "--suppress-nst",
	"whispercpp_print_colors":                "--print-colors",
	"whispercpp_print_special":               "--print-special",
	"whispercpp_print_realtime":              "--print-realtime",
	"whispercpp_print_progress":              "--print-progress",
	"whispercpp_no_timestamps":               "--no-timestamps",
	"whispercpp_vad":                         "--vad",
	"whispercpp_vad_model":                   "--vad-model",
	"whispercpp_vad_threshold":               "--vad-threshold",
	"whispercpp_vad_min_speech_duration_ms":  "--vad-min-speech-duration-ms",
	"whispercpp_vad_min_silence_duration_ms": "--vad-min-silence-duration-ms",
	"whispercpp_vad_max_speech_duration_s":   "--vad-max-speech-duration-s",
	"whispercpp_vad_speech_pad_ms":           "--vad-speech-pad-ms",
	"whispercpp_vad_samples_overlap":         "--vad-samples-overlap",
}

func whisperCPPArguments(metadata catalog.RuntimeConfig, target launchTarget) ([]string, error) {
	modelPath := strings.TrimSpace(metadata.WhisperModel)
	if modelPath == "" {
		return nil, fmt.Errorf("whisper.cpp config has no whispermodel")
	}
	args := []string{"--host", target.host, "--port", target.port, "--model", modelPath}
	appendIntArg(&args, "--threads", metadata.Threads)
	appendIntArg(&args, "--device", nonNegative(metadata.MainGPU))
	appendOptionalBoolArg(&args, "--flash-attn", "--no-flash-attn", metadata.FlashAttention)
	appendFlag(&args, "--no-gpu", metadata.UseCPU)
	keys := make([]string, 0, len(whisperCPPOptionFlags))
	for key := range whisperCPPOptionFlags {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		appendWhisperOption(&args, whisperCPPOptionFlags[key], metadata.WhisperCPPOptions[key])
	}
	if enabled, ok := metadata.WhisperCPPOptions["whispercpp_language_probabilities"].(bool); ok && !enabled {
		args = append(args, "--no-language-probabilities")
	}
	return args, nil
}

func appendWhisperOption(args *[]string, flag string, value any) {
	switch typed := value.(type) {
	case bool:
		appendFlag(args, flag, typed)
	case string:
		appendStringArg(args, flag, typed)
	case float64:
		*args = append(*args, flag, strconv.FormatFloat(typed, 'f', -1, 64))
	}
}
