package catalog

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestGPULayersAcceptsANumberWrittenAsAString(t *testing.T) {
	for raw, want := range map[string]int{
		`{"gpulayers":999,"draftgpulayers":20}`:      999,
		`{"gpulayers":"999","draftgpulayers":"20"}`:  999,
		`{"gpulayers":" -1 ","draftgpulayers":"20"}`: -1,
	} {
		var config RuntimeConfig
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if int(config.GPULayers) != want || int(config.DraftGPULayers) != 20 {
			t.Fatalf("%s parsed to gpulayers=%d draftgpulayers=%d", raw, config.GPULayers, config.DraftGPULayers)
		}
	}
}

func TestGPULayersStillRejectsTextAndNamesTheField(t *testing.T) {
	var config RuntimeConfig
	err := json.Unmarshal([]byte(`{"gpulayers":"all"}`), &config)
	var typeError *json.UnmarshalTypeError
	if !errors.As(err, &typeError) || typeError.Field != "gpulayers" {
		t.Fatalf("error = %v, want a type error naming gpulayers", err)
	}
}

func TestEmptyGPULayersStringMeansUnset(t *testing.T) {
	var config RuntimeConfig
	if err := json.Unmarshal([]byte(`{"gpulayers":""}`), &config); err != nil {
		t.Fatal(err)
	}
	if config.GPULayers != 0 {
		t.Fatalf("gpulayers = %d, want unset", config.GPULayers)
	}
}
