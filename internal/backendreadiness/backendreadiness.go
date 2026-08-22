// Package backendreadiness reads a backend's own stdout/stderr to tell apart the load
// states an HTTP probe cannot distinguish.
//
// A readiness probe reports "no model" identically whether the backend is midway through
// a load, sitting idle because a reload was lost, or about to exit. Waiting is right for
// the first, re-issuing the reload is right for the second, and failing is right for the
// third, so the probe alone cannot drive the decision.
//
// The markers below were derived by launching each backend directly against deliberately
// broken configs (missing file, empty file, truncated file, non-model file, bad magic,
// unsatisfiable context) and recording what each printed and did:
//
//   - Every broken config makes BOTH backends exit. Process exit is the reliable failure
//     signal; llama.cpp names the cause first, KoboldCpp exits silently.
//   - KoboldCpp prints "Loading <lane> Model:" when a load starts and
//     "Load <lane> Model OK: True/False" when it finishes. Between those it is working,
//     however long it takes — a cold multi-gigabyte load exceeded two minutes.
//   - With no model, KoboldCpp prints neither line and serves "inactive" indefinitely.
//     That is what a lost reload looks like, and only another reload clears it.
//   - The module banner is NOT usable: KoboldCpp restarts to apply a reload and every
//     start prints "Inactive Modules: TextGeneration ...", so it appears on the healthy
//     path too.
package backendreadiness

import (
	"strings"
)

// Lane names the capability being loaded. It selects which of a backend's per-capability
// markers count, so that an image load is not declared ready by a text-model banner.
type Lane string

const (
	LaneText          Lane = "text"
	LaneEmbeddings    Lane = "embeddings"
	LaneImage         Lane = "image"
	LaneSpeech        Lane = "speech"
	LaneTranscription Lane = "transcription"
	LaneMusic         Lane = "music"
)

// Family names the backend process whose output is being read.
type Family string

const (
	FamilyKobold Family = "kobold"
	FamilyNative Family = "native"
)

// Verdict is the conclusion drawn from the output so far.
type Verdict int

const (
	// Undecided means the output so far carries no definitive marker.
	Undecided Verdict = iota
	// Ready means the backend reported the model loaded and serving.
	Ready
	// Failed means the backend reported the load could not complete.
	Failed
)

// maxLineBytes caps a single retained line so a backend emitting an unterminated
// progress bar cannot grow the scanner's buffer without bound.
const maxLineBytes = 64 * 1024

// Result is the outcome of scanning backend output.
type Result struct {
	Verdict Verdict
	// Reason is the decisive output line, verbatim, when Verdict is Failed.
	Reason string
}

// Scanner consumes backend output incrementally and reports the first definitive
// verdict. It is safe for a single producer; callers that share it across goroutines
// must serialize access themselves.
type Scanner struct {
	family Family
	lane   Lane
	// loading records that the backend announced a load in progress. A backend that is
	// loading is making progress no matter how long it takes; one that never announced a
	// load and reports no model is idle, which is the lost-reload case.
	loading bool
	pending []byte
	result  Result
}

// Loading reports whether the backend announced that a load is under way.
func (scanner *Scanner) Loading() bool {
	return scanner.loading
}

func NewScanner(family Family, lane Lane) *Scanner {
	return &Scanner{family: family, lane: lane}
}

// Write feeds a chunk of backend output. It returns the verdict known so far; once a
// verdict other than Undecided is reached it is retained and further output ignored.
func (scanner *Scanner) Write(chunk []byte) Result {
	if scanner.result.Verdict != Undecided {
		return scanner.result
	}
	scanner.pending = append(scanner.pending, chunk...)
	for {
		index := indexNewline(scanner.pending)
		if index < 0 {
			if len(scanner.pending) > maxLineBytes {
				scanner.pending = scanner.pending[len(scanner.pending)-maxLineBytes:]
			}
			return scanner.result
		}
		line := string(scanner.pending[:index])
		scanner.pending = scanner.pending[index+1:]
		if result, ok := scanner.classify(line); ok {
			scanner.result = result
			return scanner.result
		}
	}
}

// Result reports the verdict reached so far.
func (scanner *Scanner) Result() Result {
	return scanner.result
}

func indexNewline(value []byte) int {
	for index, current := range value {
		if current == '\n' {
			return index
		}
	}
	return -1
}

func (scanner *Scanner) classify(line string) (Result, bool) {
	line = strings.TrimRight(line, "\r")
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return Result{}, false
	}
	switch scanner.family {
	case FamilyKobold:
		return scanner.classifyKobold(trimmed)
	case FamilyNative:
		return scanner.classifyNative(trimmed)
	default:
		return Result{}, false
	}
}

// koboldLoadLabel is the capability word KoboldCpp prints in its
// "Load <label> Model OK: <bool>" line, and the module name it lists under
// "Active Modules:" / "Inactive Modules:".
func koboldLoadLabel(lane Lane) (string, string) {
	switch lane {
	case LaneImage:
		return "Image", "ImageGeneration"
	case LaneTranscription:
		return "Whisper", "VoiceRecognition"
	case LaneSpeech:
		return "TTS", "TextToSpeech"
	case LaneEmbeddings:
		return "Embeddings", "VectorEmbeddings"
	case LaneMusic:
		return "Music", "MusicGen"
	default:
		return "Text", "TextGeneration"
	}
}

func (scanner *Scanner) classifyKobold(line string) (Result, bool) {
	label, module := koboldLoadLabel(scanner.lane)

	// "Loading <lane> Model: <ref>" means a load is under way for this lane.
	if strings.HasPrefix(line, "Loading "+label+" Model:") {
		scanner.loading = true
		return Result{}, false
	}

	// The per-load verdict, when KoboldCpp prints one, is decisive on its own.
	if prefix := "Load " + label + " Model OK: "; strings.HasPrefix(line, prefix) {
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if strings.EqualFold(value, "True") {
			return Result{Verdict: Ready}, true
		}
		return Result{Verdict: Failed, Reason: line}, true
	}

	// The module banner is never a failure signal for KoboldCpp, in any sequence.
	//
	// KoboldCpp restarts itself to apply an admin reload, and every start prints the
	// no-model banner "Inactive Modules: TextGeneration ..." before loading. So the banner
	// appears on the healthy path; and because a restart also follows a "Loading ..." line
	// from the config being replaced, even "a load began and then the module was inactive"
	// describes a normal switch. There is no ordering that separates it from an
	// interrupted load, so using it aborts switches that were about to succeed.
	//
	// A load that truly stops making progress is caught by the caller's idle watchdog
	// instead, which needs no guess about what the text means.
	if strings.HasPrefix(line, "Inactive Modules: ") {
		return Result{}, false
	}
	if rest, ok := strings.CutPrefix(line, "Active Modules: "); ok {
		if moduleListed(rest, module) {
			return Result{Verdict: Ready}, true
		}
		return Result{}, false
	}

	return Result{}, false
}

func moduleListed(list string, module string) bool {
	for _, field := range strings.Fields(list) {
		if field == module {
			return true
		}
	}
	return false
}

// nativeFailureMarkers are lines that mean the load is over and will not recover. Each
// one is printed by llama.cpp / stable-diffusion.cpp / whisper.cpp at the point it gives
// up on the model.
//
// Symptoms are deliberately excluded. An allocation failure such as "cudaMalloc failed"
// or a bare "out of memory" can be printed while the server retries a smaller allocation
// or falls back to another device, and can also appear in ordinary memory reporting.
// Treating a symptom as a verdict aborts loads that would have succeeded — the same
// mistake the KoboldCpp module banner caused. Symptoms are still used to explain a
// failure after the fact; see nativeDiagnosticMarkers.
var nativeFailureMarkers = []string{
	"exiting due to model loading error",
	"failed to create_context with model",
	"failed to load model",
}

// nativeDiagnosticMarkers name the cause of a failure that has already happened. They are
// only consulted once the process has exited, so a symptom line here cannot abort a live
// load — it just makes the resulting error say why. Ordered least to most specific.
var nativeDiagnosticMarkers = []string{
	"exiting due to model loading error",
	"failed to create_context with model",
	"failed to load model",
	"failed to allocate compute buffers",
	"cudaMalloc failed",
	"out of memory",
}

// nativeReadyMarkers appear once the server has the model resident and is accepting
// connections.
var nativeReadyMarkers = []string{
	"llama_server: model loaded",
	"server is listening on",
	"llama_server: listening on",
}

// DecisiveFailureLine finds the line in captured backend output that best explains why a
// load failed, so an error can name the cause ("cudaMalloc failed: out of memory")
// instead of only the exit status. It returns "" when nothing recognisable is present.
//
// The most specific marker wins over the most recent one: a server's closing "exiting due
// to model loading error" is true but says nothing, while the allocation failure above it
// is the actual reason.
func DecisiveFailureLine(output string) string {
	best := ""
	bestRank := -1
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" {
			continue
		}
		rank := -1
		for index, marker := range nativeDiagnosticMarkers {
			if strings.Contains(line, marker) && index > rank {
				rank = index
			}
		}
		// Ties keep the earliest line, which is the cause rather than its consequence.
		if rank > bestRank {
			best = line
			bestRank = rank
		}
	}
	return best
}

// nativeLoadingMarkers are printed when a load begins.
var nativeLoadingMarkers = []string{
	"load_model: loading model",
	"loading model from",
}

func (scanner *Scanner) classifyNative(line string) (Result, bool) {
	for _, marker := range nativeLoadingMarkers {
		if strings.Contains(line, marker) {
			scanner.loading = true
			return Result{}, false
		}
	}
	return classifyNative(line)
}

func classifyNative(line string) (Result, bool) {
	for _, marker := range nativeFailureMarkers {
		if strings.Contains(line, marker) {
			return Result{Verdict: Failed, Reason: line}, true
		}
	}
	for _, marker := range nativeReadyMarkers {
		if strings.Contains(line, marker) {
			return Result{Verdict: Ready}, true
		}
	}
	return Result{}, false
}
