// Package comfyvideo recognizes ComfyUI API-format video workflows and
// extracts the parameters a backend generation call needs from them. It has
// no dependency on the proxy package: it only reads a decoded graph and
// returns plain data, the same way KoboldCpp's own ComfyUI emulation walks a
// graph for a KSampler node without understanding the rest of it.
package comfyvideo

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Node is one entry of a ComfyUI API-format prompt graph.
type Node struct {
	ClassType string         `json:"class_type"`
	Inputs    map[string]any `json:"inputs"`
}

// Graph is a full ComfyUI API-format prompt, keyed by node id.
type Graph map[string]Node

// Params is what a video generation request needs, extracted from a Graph.
type Params struct {
	Prompt         string
	NegativePrompt string
	Width          int
	Height         int
	Frames         int
	FPS            int
	Seed           int64
	Steps          int
	CFG            float64
	SamplerName    string
	Scheduler      string
	// ReferenceImage is the name a LoadImage-shaped node referenced, as
	// previously returned by /upload/image. Empty for text-to-video.
	ReferenceImage string
}

var videoClassTypeHints = []string{
	"savewebm", "saveanimatedwebp", "savevideo", "createvideo", "vhs_videocombine",
	"wanimagetovideo", "wantextvideo", "wanvideo", "emptyhunyuanlatentvideo", "hunyuanvideo",
	"minimaxh3", "minimax_h3", "ltxvideo", "svd_img2vid", "videolinearcfgguidance",
}

var videoLengthInputKeys = []string{"length", "num_frames", "video_frames", "frames"}

// DecodeWorkflow parses a ComfyUI /prompt request body. The real API shape
// is {"prompt": <graph>, ...optional fields}; a bare graph is also accepted
// so callers that already unwrapped the envelope keep working.
func DecodeWorkflow(body []byte) (Graph, error) {
	var envelope struct {
		Prompt json.RawMessage `json:"prompt"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && len(envelope.Prompt) > 0 {
		return decodeGraph(envelope.Prompt)
	}
	return decodeGraph(body)
}

func decodeGraph(raw json.RawMessage) (Graph, error) {
	var graph Graph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return nil, fmt.Errorf("invalid ComfyUI workflow JSON: %w", err)
	}
	if len(graph) == 0 {
		return nil, fmt.Errorf("empty ComfyUI workflow")
	}
	return graph, nil
}

// IsVideoWorkflow reports whether graph looks like it produces video rather
// than a still image: a node class name that names a known video sink or
// video-specific latent, or a frame-count input greater than one anywhere in
// the graph. Video model families (Wan, MiniMax-H3, HunyuanVideo, LTX, SVD)
// use enough different node names that this stays a heuristic, matching how
// KoboldCpp's own emulation is a heuristic over KSampler shape rather than a
// fixed schema.
func IsVideoWorkflow(graph Graph) bool {
	for _, node := range graph {
		classType := strings.ToLower(node.ClassType)
		for _, hint := range videoClassTypeHints {
			if strings.Contains(classType, hint) {
				return true
			}
		}
		for _, key := range videoLengthInputKeys {
			if count, ok := numberValue(node.Inputs[key]); ok && count > 1 {
				return true
			}
		}
	}
	return false
}

// ParseWorkflow extracts generation parameters from graph. A sampler node is
// identified structurally, by carrying both a "positive" and a "negative"
// input, rather than by class name, so it works across the differently
// named samplers each video model family ships. Prompts are resolved by
// following that node's link references to the CLIPTextEncode-shaped node
// that produced them.
func ParseWorkflow(graph Graph) Params {
	params := Params{Width: 512, Height: 512, Frames: 1, FPS: 16}

	if samplerID, sampler, ok := findSamplerNode(graph); ok {
		params.Prompt = resolveLinkedText(graph, sampler.Inputs["positive"])
		params.NegativePrompt = resolveLinkedText(graph, sampler.Inputs["negative"])
		if seed, ok := numberValue(sampler.Inputs["seed"]); ok {
			params.Seed = int64(seed)
		} else if seed, ok := numberValue(sampler.Inputs["noise_seed"]); ok {
			params.Seed = int64(seed)
		}
		if steps, ok := numberValue(sampler.Inputs["steps"]); ok {
			params.Steps = int(steps)
		}
		if cfg, ok := numberValue(sampler.Inputs["cfg"]); ok {
			params.CFG = cfg
		}
		if name, ok := sampler.Inputs["sampler_name"].(string); ok {
			params.SamplerName = name
		}
		if scheduler, ok := sampler.Inputs["scheduler"].(string); ok {
			params.Scheduler = scheduler
		}
		_ = samplerID
	}

	for _, nodeID := range sortedNodeIDs(graph) {
		inputs := graph[nodeID].Inputs
		if width, ok := numberValue(inputs["width"]); ok && width > 0 {
			params.Width = int(width)
		}
		if height, ok := numberValue(inputs["height"]); ok && height > 0 {
			params.Height = int(height)
		}
		for _, key := range videoLengthInputKeys {
			if frames, ok := numberValue(inputs[key]); ok && frames > 0 {
				params.Frames = int(frames)
			}
		}
		if fps, ok := numberValue(inputs["fps"]); ok && fps > 0 {
			params.FPS = int(fps)
		} else if fps, ok := numberValue(inputs["frame_rate"]); ok && fps > 0 {
			params.FPS = int(fps)
		}
	}
	params.ReferenceImage = findReferenceImage(graph)

	return params
}

// findReferenceImage returns the uploaded image name an image-to-video graph
// loads. Only a literal string counts: a link reference points at another
// node's output, which is a generated image rather than an upload.
func findReferenceImage(graph Graph) string {
	for _, nodeID := range sortedNodeIDs(graph) {
		node := graph[nodeID]
		if !strings.Contains(strings.ToLower(node.ClassType), "loadimage") {
			continue
		}
		if name, ok := node.Inputs["image"].(string); ok && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name)
		}
	}
	return ""
}

func findSamplerNode(graph Graph) (string, Node, bool) {
	for _, nodeID := range sortedNodeIDs(graph) {
		node := graph[nodeID]
		if _, hasPositive := node.Inputs["positive"]; !hasPositive {
			continue
		}
		if _, hasNegative := node.Inputs["negative"]; !hasNegative {
			continue
		}
		return nodeID, node, true
	}
	return "", Node{}, false
}

// resolveLinkedText follows a ComfyUI link reference ["nodeID", slotIndex]
// to the text a CLIPTextEncode-shaped node produced. A bare string is
// returned as-is so a workflow that inlines a literal prompt still works.
func resolveLinkedText(graph Graph, value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	link, ok := value.([]any)
	if !ok || len(link) == 0 {
		return ""
	}
	nodeID, ok := link[0].(string)
	if !ok {
		return ""
	}
	node, ok := graph[nodeID]
	if !ok {
		return ""
	}
	if text, ok := node.Inputs["text"].(string); ok {
		return text
	}
	return ""
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func sortedNodeIDs(graph Graph) []string {
	ids := make([]string, 0, len(graph))
	for id := range graph {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
