package recipes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tensors-router/internal/atomicfile"
)

const (
	KindText       = "text"
	KindImage      = "image"
	KindEmbeddings = "embeddings"
	KindVoice      = "voice"
	KindMusic      = "music"
)

type Component struct {
	Kind           string `json:"kind"`
	NodeID         string `json:"node_id"`
	NodeURL        string `json:"node_url,omitempty"`
	ModelID        string `json:"model_id,omitempty"`
	ImageID        string `json:"image_id,omitempty"`
	ConfigFilename string `json:"config_filename"`
}

type Recipe struct {
	ID            string     `json:"id"`
	PublicID      string     `json:"public_id"`
	PublicImageID string     `json:"public_image_id,omitempty"`
	Created       int64      `json:"created"`
	Text          *Component `json:"text,omitempty"`
	Image         *Component `json:"image,omitempty"`
	Embeddings    *Component `json:"embeddings,omitempty"`
	Voice         *Component `json:"voice,omitempty"`
	Music         *Component `json:"music,omitempty"`
}

type Store struct {
	mu       sync.Mutex
	path     string
	snapshot atomic.Pointer[recipeSnapshot]
	persist  func([]Recipe) error
}

type storeFile struct {
	Recipes []Recipe `json:"recipes"`
}

type recipeSnapshot struct {
	recipes    []Recipe
	components map[string]map[string]recipeComponentEntry
	images     map[string]recipeComponentEntry
}

type recipeComponentEntry struct {
	recipe    Recipe
	component Component
}

func NewStore(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("recipe store dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	store := &Store{path: filepath.Join(dir, "split-recipes.json")}
	recipes, err := loadRecipes(store.path)
	if err != nil {
		return nil, err
	}
	store.persist = store.persistRecipes
	store.snapshot.Store(newRecipeSnapshot(recipes))
	return store, nil
}

func (store *Store) List() ([]Recipe, error) {
	if store == nil {
		return []Recipe{}, nil
	}
	return cloneRecipes(store.snapshot.Load().recipes), nil
}

func (store *Store) Save(recipe Recipe, overwrite bool) error {
	if store == nil {
		return fmt.Errorf("recipe store is not configured")
	}
	if err := validateRecipe(recipe); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	recipes := cloneRecipes(store.snapshot.Load().recipes)
	if recipe.PublicID == "" {
		recipe.PublicID = recipe.ID
	}
	if recipe.Created == 0 {
		recipe.Created = time.Now().Unix()
	}
	replaced := false
	for index := range recipes {
		if recipes[index].ID == recipe.ID {
			if !overwrite {
				return fmt.Errorf("recipe %q already exists", recipe.ID)
			}
			recipes[index] = recipe
			replaced = true
			break
		}
	}
	if !replaced {
		recipes = append(recipes, recipe)
	}
	if err := store.persist(recipes); err != nil {
		return err
	}
	store.snapshot.Store(newRecipeSnapshot(recipes))
	return nil
}

func (store *Store) Delete(id string) error {
	if store == nil {
		return fmt.Errorf("recipe store is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" || id != filepath.Base(id) {
		return fmt.Errorf("recipe id is invalid")
	}
	store.mu.Lock()
	defer store.mu.Unlock()

	recipes := cloneRecipes(store.snapshot.Load().recipes)
	filtered := recipes[:0]
	found := false
	for _, recipe := range recipes {
		if recipe.ID == id {
			found = true
			continue
		}
		filtered = append(filtered, recipe)
	}
	if !found {
		return fmt.Errorf("recipe %q was not found", id)
	}
	if err := store.persist(filtered); err != nil {
		return err
	}
	store.snapshot.Store(newRecipeSnapshot(filtered))
	return nil
}

func (store *Store) Text(id string) (Recipe, Component, bool) {
	return store.component(id, KindText)
}

func (store *Store) Embeddings(id string) (Recipe, Component, bool) {
	return store.component(id, KindEmbeddings)
}

func (store *Store) Image(publicImageID string) (Recipe, Component, bool) {
	if store == nil || strings.TrimSpace(publicImageID) == "" {
		return Recipe{}, Component{}, false
	}
	entry, ok := store.snapshot.Load().images[publicImageID]
	if !ok {
		return Recipe{}, Component{}, false
	}
	return cloneRecipe(entry.recipe), entry.component, true
}

func (store *Store) Voice(id string) (Recipe, Component, bool) {
	return store.component(id, KindVoice)
}

func (store *Store) Music(id string) (Recipe, Component, bool) {
	return store.component(id, KindMusic)
}

func (store *Store) component(id string, kind string) (Recipe, Component, bool) {
	if store == nil || strings.TrimSpace(id) == "" {
		return Recipe{}, Component{}, false
	}
	entry, ok := store.snapshot.Load().components[kind][id]
	if !ok {
		return Recipe{}, Component{}, false
	}
	return cloneRecipe(entry.recipe), entry.component, true
}

func recipeComponent(recipe Recipe, kind string) *Component {
	switch kind {
	case KindText:
		return recipe.Text
	case KindImage:
		return recipe.Image
	case KindEmbeddings:
		return recipe.Embeddings
	case KindVoice:
		return recipe.Voice
	case KindMusic:
		return recipe.Music
	default:
		return nil
	}
}

func loadRecipes(path string) ([]Recipe, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Recipe{}, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return []Recipe{}, nil
	}
	var body storeFile
	if err := json.Unmarshal(content, &body); err != nil {
		return nil, err
	}
	if body.Recipes == nil {
		return []Recipe{}, nil
	}
	return body.Recipes, nil
}

func (store *Store) persistRecipes(recipes []Recipe) error {
	body, err := json.MarshalIndent(storeFile{Recipes: recipes}, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(store.path, body, 0o600)
}

func newRecipeSnapshot(recipes []Recipe) *recipeSnapshot {
	snapshot := &recipeSnapshot{
		recipes: cloneRecipes(recipes),
		components: map[string]map[string]recipeComponentEntry{
			KindText:       {},
			KindEmbeddings: {},
			KindVoice:      {},
			KindMusic:      {},
		},
		images: map[string]recipeComponentEntry{},
	}
	for _, recipe := range snapshot.recipes {
		for _, kind := range []string{KindText, KindEmbeddings, KindVoice, KindMusic} {
			component := recipeComponent(recipe, kind)
			if component == nil {
				continue
			}
			if _, exists := snapshot.components[kind][recipe.PublicID]; !exists {
				snapshot.components[kind][recipe.PublicID] = recipeComponentEntry{recipe: recipe, component: *component}
			}
		}
		if recipe.Image != nil && recipe.PublicImageID != "" {
			if _, exists := snapshot.images[recipe.PublicImageID]; !exists {
				snapshot.images[recipe.PublicImageID] = recipeComponentEntry{recipe: recipe, component: *recipe.Image}
			}
		}
	}
	return snapshot
}

func cloneRecipes(recipes []Recipe) []Recipe {
	cloned := make([]Recipe, len(recipes))
	for index := range recipes {
		cloned[index] = cloneRecipe(recipes[index])
	}
	return cloned
}

func cloneRecipe(recipe Recipe) Recipe {
	recipe.Text = cloneComponent(recipe.Text)
	recipe.Image = cloneComponent(recipe.Image)
	recipe.Embeddings = cloneComponent(recipe.Embeddings)
	recipe.Voice = cloneComponent(recipe.Voice)
	recipe.Music = cloneComponent(recipe.Music)
	return recipe
}

func cloneComponent(component *Component) *Component {
	if component == nil {
		return nil
	}
	cloned := *component
	return &cloned
}

func validateRecipe(recipe Recipe) error {
	if strings.TrimSpace(recipe.ID) == "" || recipe.ID != filepath.Base(recipe.ID) {
		return fmt.Errorf("recipe id is invalid")
	}
	if recipe.Text == nil && recipe.Image == nil && recipe.Embeddings == nil && recipe.Voice == nil && recipe.Music == nil {
		return fmt.Errorf("recipe has no components")
	}
	for _, component := range []*Component{recipe.Text, recipe.Image, recipe.Embeddings, recipe.Voice, recipe.Music} {
		if component == nil {
			continue
		}
		if strings.TrimSpace(component.NodeID) == "" {
			return fmt.Errorf("recipe component node_id is required")
		}
		if strings.TrimSpace(component.ConfigFilename) == "" || component.ConfigFilename != filepath.Base(component.ConfigFilename) {
			return fmt.Errorf("recipe component config filename is invalid")
		}
	}
	return nil
}
