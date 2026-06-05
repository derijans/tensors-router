package siteapi

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/inventory"
	"tensors-router/internal/recipes"
)

type NodeInventory struct {
	NodeID      string                 `json:"node_id"`
	NodeURL     string                 `json:"node_url,omitempty"`
	Source      string                 `json:"source"`
	Role        string                 `json:"role"`
	BackendMode string                 `json:"backend_mode"`
	Available   bool                   `json:"available"`
	Models      []cluster.Model        `json:"models"`
	Files       []inventory.FileRecord `json:"files"`
	Error       string                 `json:"error,omitempty"`
}

type InventoryResponse struct {
	Role    string           `json:"role"`
	NodeID  string           `json:"node_id"`
	NodeURL string           `json:"node_url,omitempty"`
	Nodes   []NodeInventory  `json:"nodes"`
	Models  []cluster.Model  `json:"models"`
	Recipes []recipes.Recipe `json:"recipes"`
}

type CookRequest struct {
	ID         string           `json:"id"`
	Overwrite  bool             `json:"overwrite"`
	Components []cook.Component `json:"components"`
}

type CookResponse struct {
	Plan   cook.Plan       `json:"plan"`
	Recipe *recipes.Recipe `json:"recipe,omitempty"`
}

type RouterProcessStatus struct {
	Managed bool   `json:"managed"`
	Running bool   `json:"running"`
	URL     string `json:"url"`
	PID     int    `json:"pid,omitempty"`
	Error   string `json:"error,omitempty"`
}
