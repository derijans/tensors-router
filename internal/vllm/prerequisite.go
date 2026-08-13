package vllm

var supportedPrerequisites = map[string]struct{}{
	"compiler":         {},
	"container_engine": {},
	"intel_gpu":        {},
	"metal":            {},
	"nvidia_driver":    {},
	"rocm_driver":      {},
}

func supportedPrerequisite(id string) bool {
	_, found := supportedPrerequisites[id]
	return found
}
