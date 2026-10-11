package guardianmodels

import (
	"fmt"
	"strings"
)

type Scope string

const (
	ClusterScope Scope = "cluster"
	NodeScope    Scope = "node"
	ModelScope   Scope = "model"
)

type Endpoint struct {
	NodeID  string `json:"node_id"`
	ModelID string `json:"model_id"`
}

func (endpoint Endpoint) blank() bool {
	return endpoint.NodeID == "" || endpoint.ModelID == ""
}

func (endpoint Endpoint) String() string {
	return endpoint.NodeID + "/" + endpoint.ModelID
}

type Assignment struct {
	Scope    Scope    `json:"scope"`
	NodeID   string   `json:"node_id,omitempty"`
	ModelID  string   `json:"model_id,omitempty"`
	Guardian Endpoint `json:"guardian"`
}

func (assignment Assignment) normalized() (Assignment, error) {
	assignment.NodeID = strings.TrimSpace(assignment.NodeID)
	assignment.ModelID = strings.TrimSpace(assignment.ModelID)
	assignment.Guardian = Endpoint{NodeID: strings.TrimSpace(assignment.Guardian.NodeID), ModelID: strings.TrimSpace(assignment.Guardian.ModelID)}
	if assignment.Guardian.blank() {
		return Assignment{}, fmt.Errorf("guardian node_id and model_id are required")
	}
	switch assignment.Scope {
	case ClusterScope:
		assignment.NodeID, assignment.ModelID = "", ""
	case NodeScope:
		if assignment.NodeID == "" {
			return Assignment{}, fmt.Errorf("node_id is required for a node guardian")
		}
		assignment.ModelID = ""
	case ModelScope:
		if assignment.NodeID == "" || assignment.ModelID == "" {
			return Assignment{}, fmt.Errorf("node_id and model_id are required for a model guardian")
		}
	default:
		return Assignment{}, fmt.Errorf("unknown guardian scope %q", assignment.Scope)
	}
	return assignment, nil
}
