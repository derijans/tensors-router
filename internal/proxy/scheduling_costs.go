package proxy

import (
	"context"
	"sync"
	"time"

	routeranalytics "tensors-router/internal/analytics"
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

func (source *schedulingCostSource) PredictMS(nodeID string, modelID string, lane string, work float64) (float64, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.PredictMS(schedulingcost.ModelKey{NodeID: nodeID, ModelID: modelID, Section: routeranalytics.SectionImage}, work)
}

func (source *schedulingCostSource) PredictQueueMS(nodeID string, modelID string, lane string, count int64, work float64) (float64, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.PredictQueueMS(schedulingcost.ModelKey{NodeID: nodeID, ModelID: modelID, Section: routeranalytics.SectionImage}, count, work)
}

func (source *schedulingCostSource) SwitchPenaltyMS(nodeID string, configFilename string) (float64, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.LoadMS(schedulingcost.LoadKey{NodeID: nodeID, ConfigFilename: configFilename})
}

func (source *schedulingCostSource) NodeBacklog(nodeID string, groupID string) (int64, float64) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	stats, ok := source.backlogs[nodeID][groupID]
	if !ok {
		return 0, 0
	}
	return stats.BacklogCount, stats.BacklogWork
}

// fitLocalCosts refits this node from its own analytics database. Raw request
// rows never leave the node that recorded them, so each node fits itself and
// publishes only the coefficients.
func (service *Service) fitLocalCosts(ctx context.Context) schedulingcost.NodeCosts {
	if service.analyticsStore == nil {
		return schedulingcost.NodeCosts{}
	}
	samples, loadSamples, err := service.analyticsStore.CostSamples(ctx, routeranalytics.SectionImage, service.schedulingSampleWindow, time.Now())
	if err != nil {
		service.logger.Printf("scheduling cost sampling failed: %v", err)
		return schedulingcost.NodeCosts{}
	}
	fitSamples := make([]schedulingcost.Sample, 0, len(samples))
	for _, sample := range samples {
		fitSamples = append(fitSamples, schedulingcost.Sample{
			NodeID:          sample.NodeID,
			ModelID:         sample.ModelID,
			Section:         sample.Section,
			Count:           sample.Count,
			SumWork:         sample.SumWork,
			SumDuration:     sample.SumDuration,
			SumWorkDuration: sample.SumWorkDuration,
			SumWorkSquared:  sample.SumWorkSquared,
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
	return schedulingcost.Build(fitSamples, fitLoads, int64(service.schedulingMinSamples)).NodeCosts()
}
