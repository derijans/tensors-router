package recipes

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

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

func TestStorePublishesOnlyAfterPersistence(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	original := Recipe{
		ID:       "audio",
		PublicID: "audio",
		Voice: &Component{
			Kind:           KindVoice,
			NodeID:         "node-a",
			ModelID:        "voice-a",
			ConfigFilename: "voice.kcpps",
		},
	}
	if err := store.Save(original, false); err != nil {
		t.Fatal(err)
	}
	store.persist = func([]Recipe) error { return errors.New("write failed") }
	updated := cloneRecipe(original)
	updated.Voice.ModelID = "voice-b"
	if err := store.Save(updated, true); err == nil {
		t.Fatal("expected persistence failure")
	}
	_, component, ok := store.Voice("audio")
	if !ok || component.ModelID != "voice-a" {
		t.Fatalf("failed write changed published snapshot %#v ok=%v", component, ok)
	}
}

func TestStoreLoadsOnceAndClonesResults(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	recipe := Recipe{
		ID:       "audio",
		PublicID: "audio",
		Voice: &Component{
			Kind:           KindVoice,
			NodeID:         "node-a",
			ModelID:        "voice-a",
			ConfigFilename: "voice.kcpps",
		},
	}
	if err := store.Save(recipe, false); err != nil {
		t.Fatal(err)
	}
	listed, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	listed[0].Voice.ModelID = "mutated"
	if err := os.WriteFile(filepath.Join(dir, "split-recipes.json"), []byte(`{"recipes":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	listed, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Voice.ModelID != "voice-a" {
		t.Fatalf("snapshot was reread or exposed to mutation %#v", listed)
	}
}

func TestStoreConcurrentReadersObservePublishedSnapshots(t *testing.T) {
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
			ModelID:        "voice-0",
			ConfigFilename: "voice.kcpps",
		},
	}
	if err := store.Save(recipe, false); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	errorsFound := make(chan error, 8)
	var readers sync.WaitGroup
	for range 8 {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-done:
					return
				default:
					_, component, ok := store.Voice("audio")
					if !ok || component.ModelID == "" {
						errorsFound <- errors.New("reader observed incomplete snapshot")
						return
					}
				}
			}
		}()
	}
	for index := 1; index <= 20; index++ {
		updated := cloneRecipe(recipe)
		updated.Voice.ModelID = fmt.Sprintf("voice-%d", index)
		if err := store.Save(updated, true); err != nil {
			t.Fatal(err)
		}
	}
	close(done)
	readers.Wait()
	close(errorsFound)
	for err := range errorsFound {
		t.Fatal(err)
	}
}
