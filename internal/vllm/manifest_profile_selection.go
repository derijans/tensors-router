package vllm

import (
	"fmt"
	"sort"
	"strings"
)

func SelectProfile(manifest Manifest, requested string, detection Detection) (Profile, error) {
	requested = strings.TrimSpace(requested)
	if requested != "" && requested != "auto" {
		return selectRequestedProfile(manifest, requested, detection)
	}
	accelerators := detectedAccelerators(detection.Devices)
	candidates, missingPrerequisites := autoProfileCandidates(manifest, detection, accelerators)
	if len(candidates) == 0 {
		return Profile{}, noProfileCandidateError(detection, accelerators, missingPrerequisites)
	}
	sort.SliceStable(candidates, func(left, right int) bool {
		if candidates[left].Priority == candidates[right].Priority {
			return candidates[left].ID < candidates[right].ID
		}
		return candidates[left].Priority > candidates[right].Priority
	})
	return candidates[0], nil
}

func selectRequestedProfile(manifest Manifest, requested string, detection Detection) (Profile, error) {
	for _, profile := range manifest.Profiles {
		if profile.ID != requested {
			continue
		}
		if reason := incompatibleReason(profile, detection); reason != "" {
			return Profile{}, fmt.Errorf("vLLM profile %q is not compatible: %s", requested, reason)
		}
		return profile, nil
	}
	return Profile{}, fmt.Errorf("vLLM profile %q is not present in authorized manifest", requested)
}

func autoProfileCandidates(manifest Manifest, detection Detection, accelerators []string) ([]Profile, []string) {
	candidates := make([]Profile, 0, len(manifest.Profiles))
	missingPrerequisites := make([]string, 0)
	for _, profile := range manifest.Profiles {
		if !profileMatchesHost(profile, detection, accelerators) {
			continue
		}
		if missing := missingProfilePrerequisites(profile, detection); len(missing) > 0 {
			missingPrerequisites = append(missingPrerequisites, profile.ID+": "+strings.Join(missing, ", "))
			continue
		}
		candidates = append(candidates, profile)
	}
	return candidates, missingPrerequisites
}

func profileMatchesHost(profile Profile, detection Detection, accelerators []string) bool {
	if !containsFold(profile.OperatingSystems, detection.OS) || !containsFold(profile.Architectures, detection.Architecture) {
		return false
	}
	if len(accelerators) > 0 {
		return intersectsFold(profile.Devices, accelerators)
	}
	return containsFold(profile.Devices, "cpu")
}

func noProfileCandidateError(detection Detection, accelerators []string, missingPrerequisites []string) error {
	if len(accelerators) > 0 && len(missingPrerequisites) > 0 {
		sort.Strings(missingPrerequisites)
		return fmt.Errorf("detected accelerator prerequisites are missing (%s); CPU fallback is disabled", strings.Join(missingPrerequisites, "; "))
	}
	return fmt.Errorf("no authorized vLLM profile supports %s/%s devices %s", detection.OS, detection.Architecture, strings.Join(detection.Devices, ","))
}
