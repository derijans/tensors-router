// Package backendreadiness decides whether a backend finished loading a model by
// reading the backend's own stdout/stderr, rather than inferring it from an HTTP probe.
//
// An HTTP probe cannot tell "still loading" apart from "the process died and came back
// with no model": both surface as KoboldCpp reporting a model id of "inactive". The
// backends announce both states unambiguously on their output, so watching the output
// yields an immediate and specific verdict where polling yields a timeout.
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
	family  Family
	lane    Lane
	pending []byte
	result  Result
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
		return classifyKobold(trimmed, scanner.lane)
	case FamilyNative:
		return classifyNative(trimmed)
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

func classifyKobold(line string, lane Lane) (Result, bool) {
	label, module := koboldLoadLabel(lane)

	// "Load Text Model OK: True" / "... : False" is the most direct signal.
	if prefix := "Load " + label + " Model OK: "; strings.HasPrefix(line, prefix) {
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if strings.EqualFold(value, "True") {
			return Result{Verdict: Ready}, true
		}
		return Result{Verdict: Failed, Reason: line}, true
	}

	// The module banner is printed once the server is up. If our capability is listed as
	// inactive here, the process is serving without the model we asked for — which is
	// what an aborted load leaves behind, and is never going to resolve by waiting.
	if rest, ok := strings.CutPrefix(line, "Inactive Modules: "); ok {
		if moduleListed(rest, module) {
			return Result{Verdict: Failed, Reason: line}, true
		}
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

// nativeFailureMarkers are substrings that appear on the decisive line when a
// llama.cpp / stable-diffusion.cpp / whisper.cpp server cannot bring a model up.
var nativeFailureMarkers = []string{
	"exiting due to model loading error",
	"failed to create_context with model",
	"failed to load model",
	"cudaMalloc failed",
	"failed to allocate compute buffers",
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
		for index, marker := range nativeFailureMarkers {
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
