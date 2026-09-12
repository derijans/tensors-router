package proxy

import (
	"context"
	"sync"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
	"tensors-router/internal/schedulingcost"
)

// schedulingCostSource is the registry's view of what every node costs. It is
// replaced wholesale on each refresh rather than mutated, so a selection always
// reads one coherent snapshot.
type schedulingCostSource struct {
	mu       sync.RWMutex
	table    *schedulingcost.Table
	backlogs map[string]map[string]offloadGroupStats
}

func newSchedulingCostSource() *schedulingCostSource {
	return &schedulingCostSource{backlogs: map[string]map[string]offloadGroupStats{}}
}

func (source *schedulingCostSource) Replace(table *schedulingcost.Table, backlogs map[string]map[string]offloadGroupStats) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.table = table
	source.backlogs = backlogs
}

func (source *schedulingCostSource) Table() *schedulingcost.Table {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table
}

func laneSection(lane string) string {
	switch lane {
	case cluster.RouteLaneText:
		return routeranalytics.SectionLLM
	default:
		return routeranalytics.SectionImage
	}
}

func (source *schedulingCostSource) PredictMS(nodeID string, modelID string, lane string, work schedulingcost.Work) (float64, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.PredictMS(schedulingcost.ModelKey{NodeID: nodeID, ModelID: modelID, Section: laneSection(lane)}, work)
}

func (source *schedulingCostSource) PredictQueueMS(nodeID string, modelID string, lane string, count int64, work schedulingcost.Work) (float64, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.PredictQueueMS(schedulingcost.ModelKey{NodeID: nodeID, ModelID: modelID, Section: laneSection(lane)}, count, work)
}

func (source *schedulingCostSource) SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.LoadMS(schedulingcost.LoadKey{NodeID: nodeID, ConfigFilename: configFilename})
}

func (source *schedulingCostSource) NodeBacklog(nodeID string, groupID string, lane string) (int64, schedulingcost.Work) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	stats, ok := source.backlogs[nodeID][backlogKey(lane, groupID)]
	if !ok {
		return 0, schedulingcost.Work{}
	}
	return stats.BacklogCount, stats.BacklogWork
}

func (source *schedulingCostSource) TokenProfile(nodeID string, modelID string) (schedulingcost.TokenProfile, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.TokenProfile(schedulingcost.ProfileKey{NodeID: nodeID, ModelID: modelID})
}

// fitLocalCosts refits this node from its own analytics database. Raw request
// rows never leave the node that recorded them, so each node fits itself and
// publishes only the coefficients.
func (service *Service) fitLocalCosts(ctx context.Context) schedulingcost.NodeCosts {
	if service.analyticsStore == nil {
		return schedulingcost.NodeCosts{}
	}
	minSamples := int64(service.schedulingMinSamples)

	imageSamples, loadSamples, err := service.analyticsStore.CostSamples(ctx, routeranalytics.SectionImage, service.schedulingSampleWindow, time.Now())
	if err != nil {
		service.logger.Printf("scheduling cost sampling failed: %v", err)
		return schedulingcost.NodeCosts{}
	}
	textSamples, err := service.analyticsStore.TextCostSamples(ctx, service.schedulingSampleWindow, time.Now())
	if err != nil {
		service.logger.Printf("scheduling text cost sampling failed: %v", err)
		textSamples = nil
	}
	profileSamples, err := service.analyticsStore.TokenProfileSamples(ctx, service.schedulingSampleWindow, time.Now())
	if err != nil {
		service.logger.Printf("scheduling token profile sampling failed: %v", err)
		profileSamples = nil
	}

	fitSamples := make([]schedulingcost.Sample, 0, len(imageSamples)+len(textSamples))
	for _, sample := range append(append([]routeranalytics.CostSample{}, imageSamples...), textSamples...) {
		fitSamples = append(fitSamples, schedulingcost.Sample{
			NodeID:          sample.NodeID,
			ModelID:         sample.ModelID,
			Section:         sample.Section,
			Arity:           sample.Arity,
			Count:           sample.Count,
			SumDuration:     sample.SumDuration,
			SumWork:         sample.SumWork,
			SumWorkDuration: sample.SumWorkDuration,
			SumWorkProduct:  sample.SumWorkProduct,
		})
	}
	fitLoads := make([]schedulingcost.LoadSample, 0, len(loadSamples))
	for _, sample := range loadSamples {
		fitLoads = append(fitLoads, schedulingcost.LoadSample{
			NodeID:         sample.NodeID,
			ConfigFilename: sample.ConfigFilename,
			Count:          sample.Count,
			SumDuration:    sample.SumDuration,
		})
	}
	fitProfiles := make([]schedulingcost.TokenProfileSample, 0, len(profileSamples))
	for _, sample := range profileSamples {
		fitProfiles = append(fitProfiles, schedulingcost.TokenProfileSample{
			NodeID:           sample.NodeID,
			ModelID:          sample.ModelID,
			Count:            sample.Count,
			SumRatio:         sample.SumRatio,
			SumRatioSquared:  sample.SumRatioSquared,
			SumOutput:        sample.SumOutput,
			SumOutputSquared: sample.SumOutputSquared,
		})
	}
	return schedulingcost.Build(fitSamples, fitLoads, fitProfiles, minSamples).NodeCosts()
}
