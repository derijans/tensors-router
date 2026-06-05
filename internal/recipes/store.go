package recipes

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	KindText       = "text"
	KindImage      = "image"
	KindEmbeddings = "embeddings"
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
}

type Store struct {
	mu   sync.Mutex
	path string
}

type storeFile struct {
	Recipes []Recipe `json:"recipes"`
}

func NewStore(dir string) (*Store, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("recipe store dir is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{path: filepath.Join(dir, "split-recipes.json")}, nil
}

func (store *Store) List() ([]Recipe, error) {
	if store == nil {
		return []Recipe{}, nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	return store.loadLocked()
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

	recipes, err := store.loadLocked()
	if err != nil {
		return err
	}
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
	return store.saveLocked(recipes)
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

	recipes, err := store.loadLocked()
	if err != nil {
		return err
	}
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
	return store.saveLocked(filtered)
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
	recipes, err := store.List()
	if err != nil {
		return Recipe{}, Component{}, false
	}
	for _, recipe := range recipes {
		if recipe.PublicImageID == publicImageID && recipe.Image != nil {
			return recipe, *recipe.Image, true
		}
	}
	return Recipe{}, Component{}, false
}

func (store *Store) component(id string, kind string) (Recipe, Component, bool) {
	if store == nil || strings.TrimSpace(id) == "" {
		return Recipe{}, Component{}, false
	}
	recipes, err := store.List()
	if err != nil {
		return Recipe{}, Component{}, false
	}
	for _, recipe := range recipes {
		if recipe.PublicID != id {
			continue
		}
		component := recipeComponent(recipe, kind)
		if component != nil {
			return recipe, *component, true
		}
	}
	return Recipe{}, Component{}, false
}

func recipeComponent(recipe Recipe, kind string) *Component {
	switch kind {
	case KindText:
		return recipe.Text
	case KindImage:
		return recipe.Image
	case KindEmbeddings:
		return recipe.Embeddings
	default:
		return nil
	}
}

func (store *Store) loadLocked() ([]Recipe, error) {
	content, err := os.ReadFile(store.path)
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

func (store *Store) saveLocked(recipes []Recipe) error {
	body, err := json.MarshalIndent(storeFile{Recipes: recipes}, "", "  ")
	if err != nil {
		return err
	}
	tmpPath := store.path + ".tmp"
	if err := os.WriteFile(tmpPath, body, 0o600); err != nil {
		return err
	}
	return os.Rename(tmpPath, store.path)
}

func validateRecipe(recipe Recipe) error {
	if strings.TrimSpace(recipe.ID) == "" || recipe.ID != filepath.Base(recipe.ID) {
		return fmt.Errorf("recipe id is invalid")
	}
	if recipe.Text == nil && recipe.Image == nil && recipe.Embeddings == nil {
		return fmt.Errorf("recipe has no components")
	}
	for _, component := range []*Component{recipe.Text, recipe.Image, recipe.Embeddings} {
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
