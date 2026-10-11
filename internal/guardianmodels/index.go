package guardianmodels

type Index struct {
	cluster *Endpoint
	byNode  map[string]Endpoint
	byModel map[Endpoint]Endpoint
}

func NewIndex(assignments []Assignment) *Index {
	index := &Index{byNode: map[string]Endpoint{}, byModel: map[Endpoint]Endpoint{}}
	for _, assignment := range assignments {
		guardian := assignment.Guardian
		switch assignment.Scope {
		case ClusterScope:
			index.cluster = &guardian
		case NodeScope:
			index.byNode[assignment.NodeID] = guardian
		case ModelScope:
			index.byModel[Endpoint{NodeID: assignment.NodeID, ModelID: assignment.ModelID}] = guardian
		}
	}
	return index
}

func (index *Index) Resolve(nodeID string, modelID string) (Endpoint, bool) {
	if index == nil {
		return Endpoint{}, false
	}
	model := Endpoint{NodeID: nodeID, ModelID: modelID}
	guardian, found := index.mostSpecific(model)
	if !found || guardian == model {
		return Endpoint{}, false
	}
	return guardian, true
}

func (index *Index) mostSpecific(model Endpoint) (Endpoint, bool) {
	if guardian, found := index.byModel[model]; found {
		return guardian, true
	}
	if guardian, found := index.byNode[model.NodeID]; found {
		return guardian, true
	}
	if index.cluster != nil {
		return *index.cluster, true
	}
	return Endpoint{}, false
}

func (index *Index) Assignments() []Assignment {
	if index == nil {
		return nil
	}
	var assignments []Assignment
	if index.cluster != nil {
		assignments = append(assignments, Assignment{Scope: ClusterScope, Guardian: *index.cluster})
	}
	for nodeID, guardian := range index.byNode {
		assignments = append(assignments, Assignment{Scope: NodeScope, NodeID: nodeID, Guardian: guardian})
	}
	for model, guardian := range index.byModel {
		assignments = append(assignments, Assignment{Scope: ModelScope, NodeID: model.NodeID, ModelID: model.ModelID, Guardian: guardian})
	}
	return assignments
}
