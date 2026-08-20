package backendreadiness

import (
	"strings"
	"testing"
)

// koboldReadyTranscript mirrors a successful text load captured from a production node.
const koboldReadyTranscript = `***
Welcome to KoboldCpp - Version 1.119
Loading Text Model: sha256:4c5e2db039e9325ac7724c8846c71356a24ad1cdfa28002d73ecb6be645f9675
Load Text Model OK: True
======
Active Modules: TextGeneration AdminControl
Inactive Modules: ImageGeneration VoiceRecognition MultimodalVision MultimodalAudio
Starting Kobold API on port 5001 at http://127.0.0.1:5001/api/
Please connect to custom endpoint at http://127.0.0.1:5001
`

// koboldAbortedTranscript mirrors the 628s failure: the load is interrupted and the
// process restarts serving no model at all. An HTTP probe sees only "inactive" here.
const koboldAbortedTranscript = `Loading Text Model: sha256:4c5e2db039e9325ac7724c8846c71356a24ad1cdfa28002d73ecb6be645f9675
Traceback (most recent call last):
  File "koboldcpp.py", line 11615, in main
    time.sleep(0.2)
KeyboardInterrupt
***
Welcome to KoboldCpp - Version 1.119
======
Active Modules: AdminControl
Inactive Modules: TextGeneration ImageGeneration VoiceRecognition MultimodalVision
Please connect to custom endpoint at http://127.0.0.1:5001
`

const nativeReadyTranscript = `0.00.173.480 I srv    load_model: loading model 'sha256:b60ae5ce2dd6a0b77f82cadf21def1f310a3e10cde380ad0081b07a9d416949d'
0.00.733.355 I srv    load_model: initializing, n_slots = 4, n_ctx_slot = 16128, kv_unified = 'true'
0.00.735.549 I srv  llama_server: model loaded
0.00.735.552 I srv  llama_server: listening on http://127.0.0.1:5002
`

const nativeOOMTranscript = `0.00.085.109 I srv    load_model: loading model 'sha256:b60ae5ce2dd6a0b77f82cadf21def1f310a3e10cde380ad0081b07a9d416949d'
0.06.575.846 E ggml_backend_cuda_buffer_type_alloc_buffer: allocating 694.64 MiB on device 0: cudaMalloc failed: out of memory
0.06.575.849 E ggml_gallocr_reserve_n_impl: failed to allocate CUDA0 buffer of size 728381824
0.06.682.049 E cmn  common_init_: failed to create context with model 'sha256:b60ae5ce'
0.06.682.760 E srv  llama_server: exiting due to model loading error
`

func TestScannerVerdicts(t *testing.T) {
	cases := []struct {
		name        string
		family      Family
		lane        Lane
		transcript  string
		verdict     Verdict
		reasonMatch string
	}{
		{"kobold text ready", FamilyKobold, LaneText, koboldReadyTranscript, Ready, ""},
		{"kobold aborted load", FamilyKobold, LaneText, koboldAbortedTranscript, Failed, "Inactive Modules: TextGeneration"},
		{"native ready", FamilyNative, LaneText, nativeReadyTranscript, Ready, ""},
		{"native cuda oom", FamilyNative, LaneText, nativeOOMTranscript, Failed, "cudaMalloc failed: out of memory"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			scanner := NewScanner(testCase.family, testCase.lane)
			result := scanner.Write([]byte(testCase.transcript))
			if result.Verdict != testCase.verdict {
				t.Fatalf("verdict = %v, want %v (reason %q)", result.Verdict, testCase.verdict, result.Reason)
			}
			if testCase.reasonMatch != "" && !strings.Contains(result.Reason, testCase.reasonMatch) {
				t.Fatalf("reason = %q, want it to contain %q", result.Reason, testCase.reasonMatch)
			}
		})
	}
}

// The aborted-load transcript must fail rather than hang, because the HTTP probe cannot
// distinguish it from a slow load. This is the 628-second stall in miniature.
func TestAbortedKoboldLoadIsFailedNotUndecided(t *testing.T) {
	scanner := NewScanner(FamilyKobold, LaneText)
	if result := scanner.Write([]byte(koboldAbortedTranscript)); result.Verdict != Failed {
		t.Fatalf("aborted load verdict = %v, want Failed", result.Verdict)
	}
}

// A ready banner for a different capability must not satisfy the lane we asked for.
func TestKoboldLaneIsolation(t *testing.T) {
	scanner := NewScanner(FamilyKobold, LaneImage)
	result := scanner.Write([]byte("Load Text Model OK: True\nActive Modules: TextGeneration AdminControl\n"))
	if result.Verdict != Undecided {
		t.Fatalf("image lane verdict = %v, want Undecided for a text-only banner", result.Verdict)
	}
	result = scanner.Write([]byte("Inactive Modules: ImageGeneration VectorEmbeddings\n"))
	if result.Verdict != Failed {
		t.Fatalf("image lane verdict = %v, want Failed once ImageGeneration is listed inactive", result.Verdict)
	}
}

func TestKoboldExplicitLoadFailure(t *testing.T) {
	scanner := NewScanner(FamilyKobold, LaneText)
	result := scanner.Write([]byte("Loading Text Model: x\nLoad Text Model OK: False\n"))
	if result.Verdict != Failed || !strings.Contains(result.Reason, "Load Text Model OK: False") {
		t.Fatalf("verdict = %v reason = %q", result.Verdict, result.Reason)
	}
}

// Output arrives in arbitrarily split chunks; a marker straddling a chunk boundary must
// still be recognised.
func TestScannerHandlesSplitChunks(t *testing.T) {
	scanner := NewScanner(FamilyNative, LaneText)
	for _, chunk := range []string{"0.00 I srv  llama_ser", "ver: model loa", "ded\n"} {
		scanner.Write([]byte(chunk))
	}
	if scanner.Result().Verdict != Ready {
		t.Fatalf("verdict = %v, want Ready across split chunks", scanner.Result().Verdict)
	}
}

func TestScannerKeepsFirstVerdict(t *testing.T) {
	scanner := NewScanner(FamilyNative, LaneText)
	scanner.Write([]byte("srv  llama_server: exiting due to model loading error\n"))
	scanner.Write([]byte("srv  llama_server: model loaded\n"))
	if scanner.Result().Verdict != Failed {
		t.Fatalf("verdict = %v, want the first (Failed) verdict retained", scanner.Result().Verdict)
	}
}

// The cause must win over its consequence: "exiting due to model loading error" is true
// but explains nothing, while the allocation failure above it is the actual reason.
func TestDecisiveFailureLinePrefersTheCause(t *testing.T) {
	line := DecisiveFailureLine(nativeOOMTranscript)
	if !strings.Contains(line, "cudaMalloc failed: out of memory") {
		t.Fatalf("decisive line = %q, want the allocation failure", line)
	}
}

func TestDecisiveFailureLineEmptyWhenNothingRecognisable(t *testing.T) {
	if line := DecisiveFailureLine("starting up\nall good\n"); line != "" {
		t.Fatalf("decisive line = %q, want empty", line)
	}
}

// An unterminated line must not grow without bound.
func TestScannerBoundsPendingLine(t *testing.T) {
	scanner := NewScanner(FamilyNative, LaneText)
	scanner.Write([]byte(strings.Repeat("x", maxLineBytes*3)))
	if len(scanner.pending) > maxLineBytes {
		t.Fatalf("pending = %d bytes, want <= %d", len(scanner.pending), maxLineBytes)
	}
}
