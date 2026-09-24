package comfyvideo

import (
	"fmt"
	"path"
	"slices"
	"strings"
)

type LoRA struct {
	Name     string
	Strength float64
}

const defaultLoRAStrength = 1.0

func findLoRAs(graph Graph) ([]LoRA, error) {
	var loras []LoRA
	for _, nodeID := range sortedNodeIDs(graph) {
		node := graph[nodeID]
		if !classTypeContains(node, "loraloader") {
			continue
		}
		requestedName, ok := literalName(node.Inputs["lora_name"])
		if !ok {
			continue
		}
		name, err := loRAFileName(requestedName)
		if err != nil {
			return nil, fmt.Errorf("node %s: %w", nodeID, err)
		}
		loras = append(loras, LoRA{Name: name, Strength: loRAStrength(node)})
	}
	return loras, nil
}

func loRAStrength(node Node) float64 {
	if strength, ok := numberValue(node.Inputs["strength_model"]); ok {
		return strength
	}
	return defaultLoRAStrength
}

func loRAFileName(requestedName string) (string, error) {
	slashed := strings.ReplaceAll(requestedName, `\`, "/")
	if path.IsAbs(slashed) || hasWindowsVolume(slashed) {
		return "", fmt.Errorf("LoRA %q must be a name, not an absolute path", requestedName)
	}
	if strings.ContainsRune(slashed, 0) || slices.Contains(strings.Split(slashed, "/"), "..") {
		return "", fmt.Errorf("LoRA %q must not traverse directories", requestedName)
	}
	name := path.Base(slashed)
	if name == "." || name == "/" {
		return "", fmt.Errorf("LoRA %q does not name a file", requestedName)
	}
	return name, nil
}

func hasWindowsVolume(name string) bool {
	return len(name) >= 2 && name[1] == ':'
}
