package comfyvideo

import "slices"

const maxImagePassThroughHops = 8

var (
	startFrameInputKeys        = []string{"start_image", "start_frame", "first_frame", "first_image", "init_image"}
	endFrameInputKeys          = []string{"end_image", "end_frame", "last_frame", "last_image"}
	imageToVideoClassHints     = []string{"imgtovideo", "imagetovideo", "img2vid"}
	imageToVideoStartInputKeys = []string{"image"}
	imagePassThroughInputKeys  = []string{"image", "images", "pixels"}
	audioNameInputKeys         = []string{"audio", "audio_file"}
)

type frameRole int

const (
	notAFrame frameRole = iota
	startFrame
	endFrame
)

func applyMediaReferences(params *Params, graph Graph) {
	uploadedImages := uploadedImageNodes(graph)
	wiredFrameNodes := map[string]bool{}
	for _, nodeID := range sortedNodeIDs(graph) {
		node := graph[nodeID]
		for _, key := range sortedInputKeys(node.Inputs) {
			role := frameRoleOf(node, key)
			if role == notAFrame {
				continue
			}
			sourceID, ok := uploadedImageSource(graph, uploadedImages, node.Inputs[key], maxImagePassThroughHops)
			if !ok {
				continue
			}
			wiredFrameNodes[sourceID] = true
			assignFrame(params, role, uploadedImages[sourceID])
		}
	}
	for _, nodeID := range sortedNodeIDs(graph) {
		if name, ok := uploadedImages[nodeID]; ok && !wiredFrameNodes[nodeID] {
			params.ReferenceImages = append(params.ReferenceImages, name)
		}
	}
	if params.StartFrame == "" && len(params.ReferenceImages) > 0 {
		params.StartFrame = params.ReferenceImages[0]
	}
	params.ReferenceAudios = uploadedAudioNames(graph)
}

func frameRoleOf(node Node, inputKey string) frameRole {
	switch {
	case slices.Contains(startFrameInputKeys, inputKey):
		return startFrame
	case slices.Contains(endFrameInputKeys, inputKey):
		return endFrame
	case isImageToVideoNode(node) && slices.Contains(imageToVideoStartInputKeys, inputKey):
		return startFrame
	default:
		return notAFrame
	}
}

func isImageToVideoNode(node Node) bool {
	return slices.ContainsFunc(imageToVideoClassHints, func(hint string) bool { return classTypeContains(node, hint) })
}

func assignFrame(params *Params, role frameRole, name string) {
	switch {
	case role == startFrame && params.StartFrame == "":
		params.StartFrame = name
	case role == endFrame && params.EndFrame == "":
		params.EndFrame = name
	}
}

func uploadedImageSource(graph Graph, uploadedImages map[string]string, value any, remainingHops int) (string, bool) {
	nodeID, ok := linkedNodeID(value)
	if !ok {
		return "", false
	}
	if _, isUpload := uploadedImages[nodeID]; isUpload {
		return nodeID, true
	}
	if remainingHops == 0 {
		return "", false
	}
	upstream := graph[nodeID]
	for _, key := range imagePassThroughInputKeys {
		if sourceID, found := uploadedImageSource(graph, uploadedImages, upstream.Inputs[key], remainingHops-1); found {
			return sourceID, true
		}
	}
	return "", false
}

func uploadedImageNodes(graph Graph) map[string]string {
	names := map[string]string{}
	for nodeID, node := range graph {
		if !classTypeContains(node, "loadimage") {
			continue
		}
		if name, ok := literalName(node.Inputs["image"]); ok {
			names[nodeID] = name
		}
	}
	return names
}

func uploadedAudioNames(graph Graph) []string {
	var names []string
	for _, nodeID := range sortedNodeIDs(graph) {
		node := graph[nodeID]
		if !classTypeContains(node, "loadaudio") {
			continue
		}
		for _, key := range audioNameInputKeys {
			if name, ok := literalName(node.Inputs[key]); ok {
				names = append(names, name)
				break
			}
		}
	}
	return names
}
