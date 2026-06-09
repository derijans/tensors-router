package recipes

import "testing"

func TestStoreSavesVoiceAndMusicComponents(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	recipe := Recipe{
		ID:       "audio",
		PublicID: "audio",
		Voice: &Component{
			Kind:           KindVoice,
			NodeID:         "node-a",
			ModelID:        "voice",
			ConfigFilename: "voice.kcpps",
		},
		Music: &Component{
			Kind:           KindMusic,
			NodeID:         "node-a",
			ModelID:        "music",
			ConfigFilename: "music.kcpps",
		},
	}
	if err := store.Save(recipe, false); err != nil {
		t.Fatal(err)
	}
	if _, component, ok := store.Voice("audio"); !ok || component.ModelID != "voice" {
		t.Fatalf("missing voice component %#v ok=%v", component, ok)
	}
	if _, component, ok := store.Music("audio"); !ok || component.ModelID != "music" {
		t.Fatalf("missing music component %#v ok=%v", component, ok)
	}
}
