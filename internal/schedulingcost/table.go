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
	profiles  map[ProfileKey]TokenProfile
}

func Build(samples []Sample, loadSamples []LoadSample, profileSamples []TokenProfileSample, minSamples int64) *Table {
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
	profiles := make(map[ProfileKey]TokenProfile, len(profileSamples))
	for _, sample := range profileSamples {
		profile, ok := FitTokenProfile(sample, minSamples)
		if !ok {
			continue
		}
		profiles[ProfileKey{NodeID: sample.NodeID, ModelID: sample.ModelID}] = profile
	}
	return &Table{estimates: estimates, loads: loads, profiles: profiles}
}

func (table *Table) Estimate(key ModelKey) (Estimate, bool) {
	if table == nil {
		return Estimate{}, false
	}
	estimate, ok := table.estimates[key]
	return estimate, ok
}

func (table *Table) PredictMS(key ModelKey, work Work) (float64, bool) {
	estimate, ok := table.Estimate(key)
	if !ok {
		return 0, false
	}
	return estimate.PredictMS(work)
}

// PredictQueueMS prices a whole pending queue rather than one request: each
// entry pays the fixed per-request cost once, and the variable cost scales with
// the summed work.
func (table *Table) PredictQueueMS(key ModelKey, count int64, totalWork Work) (float64, bool) {
	estimate, ok := table.Estimate(key)
	if !ok {
		return 0, false
	}
	return estimate.PredictQueueMS(count, totalWork)
}

func (table *Table) LoadMS(key LoadKey) (float64, bool) {
	if table == nil {
		return 0, false
	}
	mean, ok := table.loads[key]
	return mean, ok
}

func (table *Table) TokenProfile(key ProfileKey) (TokenProfile, bool) {
	if table == nil {
		return TokenProfile{}, false
	}
	profile, ok := table.profiles[key]
	return profile, ok
}

func (table *Table) ModelCosts() []ModelCost {
	if table == nil {
		return nil
	}
	costs := make([]ModelCost, 0, len(table.estimates))
	for key, estimate := range table.estimates {
		costs = append(costs, ModelCost{
			ModelID:  key.ModelID,
			Section:  key.Section,
			BaseMS:   estimate.BaseMS,
			SlopesMS: append([]float64{}, estimate.SlopeMS[:estimate.Arity]...),
			Samples:  estimate.Samples,
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

func (table *Table) TokenCosts() []TokenCost {
	if table == nil {
		return nil
	}
	costs := make([]TokenCost, 0, len(table.profiles))
	for key, profile := range table.profiles {
		costs = append(costs, TokenCost{
			ModelID:                   key.ModelID,
			ConservativeBytesPerToken: profile.ConservativeBytesPerToken,
			ReservedOutputTokens:      profile.ReservedOutputTokens,
			Samples:                   profile.Samples,
		})
	}
	return costs
}

type ModelCost struct {
	ModelID  string    `json:"model_id"`
	Section  string    `json:"section"`
	BaseMS   float64   `json:"base_ms"`
	SlopesMS []float64 `json:"slopes_ms"`
	Samples  int64     `json:"samples"`
}

type LoadCost struct {
	ConfigFilename string  `json:"config_filename"`
	LoadMS         float64 `json:"load_ms"`
}

type TokenCost struct {
	ModelID                   string  `json:"model_id"`
	ConservativeBytesPerToken float64 `json:"conservative_bytes_per_token"`
	ReservedOutputTokens      float64 `json:"reserved_output_tokens"`
	Samples                   int64   `json:"samples"`
}

type NodeCosts struct {
	Models []ModelCost `json:"models,omitempty"`
	Loads  []LoadCost  `json:"loads,omitempty"`
	Tokens []TokenCost `json:"tokens,omitempty"`
}

func (table *Table) NodeCosts() NodeCosts {
	return NodeCosts{Models: table.ModelCosts(), Loads: table.LoadCosts(), Tokens: table.TokenCosts()}
}

func Merge(costsByNode map[string]NodeCosts) *Table {
	estimates := map[ModelKey]Estimate{}
	loads := map[LoadKey]float64{}
	profiles := map[ProfileKey]TokenProfile{}
	for nodeID, costs := range costsByNode {
		for _, cost := range costs.Models {
			arity := len(cost.SlopesMS)
			if arity < 1 || arity > MaxWorkTerms {
				continue
			}
			var slopes [MaxWorkTerms]float64
			copy(slopes[:], cost.SlopesMS)
			estimates[ModelKey{NodeID: nodeID, ModelID: cost.ModelID, Section: cost.Section}] = Estimate{
				BaseMS:  cost.BaseMS,
				SlopeMS: slopes,
				Arity:   arity,
				Samples: cost.Samples,
			}
		}
		for _, cost := range costs.Loads {
			loads[LoadKey{NodeID: nodeID, ConfigFilename: cost.ConfigFilename}] = cost.LoadMS
		}
		for _, cost := range costs.Tokens {
			profiles[ProfileKey{NodeID: nodeID, ModelID: cost.ModelID}] = TokenProfile{
				ConservativeBytesPerToken: cost.ConservativeBytesPerToken,
				ReservedOutputTokens:      cost.ReservedOutputTokens,
				Samples:                   cost.Samples,
			}
		}
	}
	return &Table{estimates: estimates, loads: loads, profiles: profiles}
}
