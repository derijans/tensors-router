package schedulingcost

type ModelKey struct {
	NodeID  string
	ModelID string
	Section string
}

type LoadKey struct {
	NodeID         string
	ConfigFilename string
}

// Table is an immutable snapshot of one refresh cycle's fits. Every lookup on a
// nil table reports "unqualified", so an absent cost source degrades to the
// existing round-robin rather than to a guess.
type Table struct {
	estimates map[ModelKey]Estimate
	loads     map[LoadKey]float64
}

func Build(samples []Sample, loadSamples []LoadSample, minSamples int64) *Table {
	estimates := make(map[ModelKey]Estimate, len(samples))
	for _, sample := range samples {
		estimate, ok := Fit(sample, minSamples)
		if !ok {
			continue
		}
		estimates[ModelKey{NodeID: sample.NodeID, ModelID: sample.ModelID, Section: sample.Section}] = estimate
	}
	loads := make(map[LoadKey]float64, len(loadSamples))
	for _, sample := range loadSamples {
		mean, ok := FitLoad(sample, 1)
		if !ok {
			continue
		}
		loads[LoadKey{NodeID: sample.NodeID, ConfigFilename: sample.ConfigFilename}] = mean
	}
	return &Table{estimates: estimates, loads: loads}
}

func (table *Table) Estimate(key ModelKey) (Estimate, bool) {
	if table == nil {
		return Estimate{}, false
	}
	estimate, ok := table.estimates[key]
	return estimate, ok
}

func (table *Table) PredictMS(key ModelKey, work float64) (float64, bool) {
	estimate, ok := table.Estimate(key)
	if !ok {
		return 0, false
	}
	return estimate.PredictMS(work), true
}

// PredictQueueMS prices a whole pending queue rather than one request: each
// entry pays the fixed per-request cost once, and the variable cost scales with
// the summed work.
func (table *Table) PredictQueueMS(key ModelKey, count int64, totalWork float64) (float64, bool) {
	estimate, ok := table.Estimate(key)
	if !ok {
		return 0, false
	}
	if count <= 0 {
		return 0, true
	}
	if totalWork < 0 {
		totalWork = 0
	}
	return float64(count)*estimate.BaseMS + estimate.SlopeMS*totalWork, true
}

func (table *Table) LoadMS(key LoadKey) (float64, bool) {
	if table == nil {
		return 0, false
	}
	mean, ok := table.loads[key]
	return mean, ok
}

func (table *Table) ModelCosts() []ModelCost {
	if table == nil {
		return nil
	}
	costs := make([]ModelCost, 0, len(table.estimates))
	for key, estimate := range table.estimates {
		costs = append(costs, ModelCost{
			ModelID: key.ModelID,
			Section: key.Section,
			BaseMS:  estimate.BaseMS,
			SlopeMS: estimate.SlopeMS,
			Samples: estimate.Samples,
		})
	}
	return costs
}

func (table *Table) LoadCosts() []LoadCost {
	if table == nil {
		return nil
	}
	costs := make([]LoadCost, 0, len(table.loads))
	for key, mean := range table.loads {
		costs = append(costs, LoadCost{ConfigFilename: key.ConfigFilename, LoadMS: mean})
	}
	return costs
}

// ModelCost and LoadCost are the wire form a node publishes in its runtime
// status. NodeID is implied by the status envelope and is filled in by the
// master when it merges tables from several nodes.
type ModelCost struct {
	ModelID string  `json:"model_id"`
	Section string  `json:"section"`
	BaseMS  float64 `json:"base_ms"`
	SlopeMS float64 `json:"slope_ms"`
	Samples int64   `json:"samples"`
}

type LoadCost struct {
	ConfigFilename string  `json:"config_filename"`
	LoadMS         float64 `json:"load_ms"`
}

type NodeCosts struct {
	Models []ModelCost `json:"models,omitempty"`
	Loads  []LoadCost  `json:"loads,omitempty"`
}

func (table *Table) NodeCosts() NodeCosts {
	return NodeCosts{Models: table.ModelCosts(), Loads: table.LoadCosts()}
}

func Merge(costsByNode map[string]NodeCosts) *Table {
	estimates := map[ModelKey]Estimate{}
	loads := map[LoadKey]float64{}
	for nodeID, costs := range costsByNode {
		for _, cost := range costs.Models {
			estimates[ModelKey{NodeID: nodeID, ModelID: cost.ModelID, Section: cost.Section}] = Estimate{
				BaseMS:  cost.BaseMS,
				SlopeMS: cost.SlopeMS,
				Samples: cost.Samples,
			}
		}
		for _, cost := range costs.Loads {
			loads[LoadKey{NodeID: nodeID, ConfigFilename: cost.ConfigFilename}] = cost.LoadMS
		}
	}
	return &Table{estimates: estimates, loads: loads}
}
