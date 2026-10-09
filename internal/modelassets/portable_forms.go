package modelassets

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

func Export(content []byte, hashFile HashFile, findOrigin FindOrigin) ([]byte, error) {
	if hashFile == nil {
		return nil, fmt.Errorf("model hash function is required")
	}
	config, err := decodeConfig(content)
	if err != nil {
		return nil, err
	}
	if err := validateForms(config); err != nil {
		return nil, err
	}
	for key, value := range config {
		if !isModelField(key) || value == nil {
			continue
		}
		if err := exportModelField(config, key, value, hashFile, findOrigin); err != nil {
			return nil, err
		}
	}
	return json.MarshalIndent(config, "", "  ")
}

func exportModelField(config map[string]any, key string, value any, hashFile HashFile, findOrigin FindOrigin) error {
	paths, array, ok := pathValues(value)
	if !ok {
		return fmt.Errorf("model field %q must be a string or string array", key)
	}
	if len(paths) == 0 || (!array && strings.TrimSpace(paths[0]) == "") {
		return nil
	}
	hashes := make([]string, len(paths))
	filenames := make([]string, len(paths))
	hfs := make([]any, len(paths))
	for index, path := range paths {
		hash, filename, err := portablePathIdentity(key, path, hashFile)
		if err != nil {
			return err
		}
		hashes[index] = hash
		filenames[index] = filename
		hfs[index] = knownOriginURI(findOrigin, hash)
	}
	delete(config, key)
	config[key+"_hash"] = scalarOrArray(hashes, array)
	config[key+"_filename"] = scalarOrArray(filenames, array)
	if anyNonNil(hfs) {
		config[key+"_hf"] = scalarOrArray(hfs, array)
	}
	return nil
}

func portablePathIdentity(key string, path string, hashFile HashFile) (string, string, error) {
	if strings.TrimSpace(path) == "" {
		return "", "", fmt.Errorf("model field %q contains an empty path", key)
	}
	hash, err := hashFile(path)
	if err != nil || !validHash(hash) {
		return "", "", fmt.Errorf("model field %q could not be hashed", key)
	}
	filename := filepath.Base(path)
	if !safeFilename(filename) {
		return "", "", fmt.Errorf("model field %q has an unsafe filename", key)
	}
	return hash, filename, nil
}

func knownOriginURI(findOrigin FindOrigin, hash string) any {
	if findOrigin == nil {
		return nil
	}
	origin, found := findOrigin(hash)
	if !found {
		return nil
	}
	if uri := origin.URI(); uri != "" {
		return uri
	}
	return nil
}

func ResolveDetailed(content []byte, findPath ResolveReferenceDetailed) (ResolveResult, error) {
	if findPath == nil {
		return ResolveResult{}, fmt.Errorf("asset lookup function is required")
	}
	config, err := decodeConfig(content)
	if err != nil {
		return ResolveResult{}, err
	}
	if err := validateForms(config); err != nil {
		return ResolveResult{}, err
	}
	result := ResolveResult{}
	hashKeys := make([]string, 0)
	for key := range config {
		if strings.HasSuffix(key, "_hash") {
			hashKeys = append(hashKeys, key)
		}
	}
	for _, key := range hashKeys {
		fields, err := resolvePortableField(config, strings.TrimSuffix(key, "_hash"), findPath)
		if err != nil {
			return ResolveResult{}, err
		}
		result.Fields = append(result.Fields, fields...)
	}
	result.Content, err = json.MarshalIndent(config, "", "  ")
	return result, err
}

func resolvePortableField(config map[string]any, base string, findPath ResolveReferenceDetailed) ([]FieldResult, error) {
	hashes, array, _ := pathValues(config[base+"_hash"])
	filenames, filenameArray, _ := pathValues(config[base+"_filename"])
	hfValues := make([]string, len(hashes))
	if value, exists := config[base+"_hf"]; exists {
		hfValues, _, _ = nullableStringValues(value)
	}
	if array != filenameArray {
		return nil, fmt.Errorf("model field %q has mismatched portable forms", base)
	}
	fields := make([]FieldResult, 0, len(hashes))
	resolved := make([]string, len(hashes))
	allResolved := true
	for index, hash := range hashes {
		fieldName := base
		if array {
			fieldName = fmt.Sprintf("%s[%d]", base, index)
		}
		resolution, found := findPath(Reference{Hash: hash, Filename: filenames[index], HF: hfValues[index]})
		if !found {
			fields = append(fields, FieldResult{Field: fieldName, Hash: hash, Failure: "asset unavailable"})
			allResolved = false
			continue
		}
		resolved[index] = resolution.Path
		fields = append(fields, FieldResult{Field: fieldName, Hash: hash, Resolved: true, Source: resolution.Source, Verification: resolution.Verification, Commit: resolution.Commit})
	}
	if allResolved {
		config[base] = scalarOrArray(resolved, array)
		delete(config, base+"_hash")
		delete(config, base+"_filename")
		delete(config, base+"_hf")
	}
	return fields, nil
}

func validateForms(config map[string]any) error {
	for key := range config {
		if err := validatePortableKey(config, key); err != nil {
			return err
		}
	}
	return nil
}

func validatePortableKey(config map[string]any, key string) error {
	base, suffix, portable := portableKey(key)
	if !portable {
		return nil
	}
	if !isModelField(base) {
		return fmt.Errorf("unknown portable model field %q", base)
	}
	if _, exists := config[base]; exists {
		return fmt.Errorf("model field %q contains both path and hash forms", base)
	}
	if suffix == "_hash" {
		return validateHashForm(config, key, base)
	}
	if _, exists := config[base+"_hash"]; !exists {
		return fmt.Errorf("model field %q has portable metadata without a hash", base)
	}
	return nil
}

func validateHashForm(config map[string]any, key string, base string) error {
	hashes, array, ok := pathValues(config[key])
	if !ok || len(hashes) == 0 {
		return fmt.Errorf("model field %q has an invalid hash form", base)
	}
	filenames, filenameArray, ok := pathValues(config[base+"_filename"])
	if !ok || array != filenameArray || len(filenames) != len(hashes) {
		return fmt.Errorf("model field %q has mismatched hash and filename forms", base)
	}
	for index := range hashes {
		if !validHash(hashes[index]) || !safeFilename(filenames[index]) {
			return fmt.Errorf("model field %q has invalid portable metadata", base)
		}
	}
	hf, exists := config[base+"_hf"]
	if !exists {
		return nil
	}
	return validateHFForm(hf, base, array, len(hashes))
}

func validateHFForm(hf any, base string, array bool, count int) error {
	values, hfArray, ok := nullableStringValues(hf)
	if !ok || hfArray != array || len(values) != count {
		return fmt.Errorf("model field %q has mismatched Hugging Face form", base)
	}
	for _, value := range values {
		if value != "" && !safeHFURI(value) {
			return fmt.Errorf("model field %q has an unsafe Hugging Face URI", base)
		}
	}
	return nil
}
