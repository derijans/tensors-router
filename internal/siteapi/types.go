package siteapi

import (
	"tensors-router/internal/cluster"
	"tensors-router/internal/cook"
	"tensors-router/internal/hardware"
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
	Hardware    hardware.Info          `json:"hardware"`
	Models      []cluster.Model        `json:"models"`
	Files       []inventory.FileRecord `json:"files"`
	Error       string                 `json:"error,omitempty"`
}

type InventoryResponse struct {
	Role            string                  `json:"role"`
	NodeID          string                  `json:"node_id"`
	NodeURL         string                  `json:"node_url,omitempty"`
	Nodes           []NodeInventory         `json:"nodes"`
	Models          []cluster.Model         `json:"models"`
	Recipes         []recipes.Recipe        `json:"recipes"`
	OptionCatalog   []cook.OptionDefinition `json:"option_catalog"`
	ObservedOptions []cook.OptionDefinition `json:"observed_options"`
}

type CookRequest struct {
	ID         string           `json:"id"`
	Overwrite  bool             `json:"overwrite"`
	Components []cook.Component `json:"components"`
	Options    cook.Options     `json:"options,omitempty"`
}

type CookResponse struct {
	Plan       cook.Plan              `json:"plan"`
	Recipe     *recipes.Recipe        `json:"recipe,omitempty"`
	Validation []cook.ValidationIssue `json:"validation,omitempty"`
}

type ConfigFileRequest struct {
	NodeID    string       `json:"node_id,omitempty"`
	NodeURL   string       `json:"node_url,omitempty"`
	ID        string       `json:"id,omitempty"`
	Filename  string       `json:"filename,omitempty"`
	Overwrite bool         `json:"overwrite"`
	Options   cook.Options `json:"options"`
}

type ConfigFileResponse struct {
	NodeID         string       `json:"node_id"`
	NodeURL        string       `json:"node_url,omitempty"`
	ID             string       `json:"id"`
	Filename       string       `json:"filename"`
	WouldOverwrite bool         `json:"would_overwrite,omitempty"`
	Deleted        bool         `json:"deleted,omitempty"`
	Options        cook.Options `json:"options,omitempty"`
}

type RouterProcessStatus struct {
	Managed      bool   `json:"managed"`
	Running      bool   `json:"running"`
	URL          string `json:"url"`
	PID          int    `json:"pid,omitempty"`
	CanShutdown  bool   `json:"can_shutdown"`
	CanForceKill bool   `json:"can_force_kill"`
	Error        string `json:"error,omitempty"`
}
