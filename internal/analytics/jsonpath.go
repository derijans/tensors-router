package analytics

import "tensors-router/internal/jsonpath"

func firstNumber(root map[string]any, paths ...[]string) float64 {
	return jsonpath.FirstNumber(root, paths...)
}

func firstString(root map[string]any, paths ...[]string) string {
	return jsonpath.FirstString(root, paths...)
}

func nestedArray(root map[string]any, path []string) ([]any, bool) {
	return jsonpath.Array(root, path)
}

func resolvePath(root map[string]any, path []string) (any, bool) {
	return jsonpath.Resolve(root, path)
}

func decodeObject(body []byte) (map[string]any, bool) {
	return jsonpath.DecodeObject(body)
}
