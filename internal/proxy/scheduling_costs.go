package proxy

import (
	"context"
	"sync"
	"time"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/cluster"
	"tensors-router/internal/schedulingcost"
)

// schedulingCostSource is every node's fitted cost table. It is replaced wholesale
// on each refresh rather than mutated, so a reader always sees one coherent snapshot.
type schedulingCostSource struct {
	mu    sync.RWMutex
	table *schedulingcost.Table
}

func newSchedulingCostSource() *schedulingCostSource {
	return &schedulingCostSource{}
}

func (source *schedulingCostSource) Replace(table *schedulingcost.Table) {
	source.mu.Lock()
	defer source.mu.Unlock()
	source.table = table
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

func (source *schedulingCostSource) TokenProfile(nodeID string, modelID string) (schedulingcost.TokenProfile, bool) {
	source.mu.RLock()
	defer source.mu.RUnlock()
	return source.table.TokenProfile(schedulingcost.ProfileKey{NodeID: nodeID, ModelID: modelID})
}

// fitLocalCosts refits this node from its own analytics database. Raw request
// rows never leave the node that recorded them, so each node fits itself and
// publishes only the coefficients.
func (scheduler *scheduler) fitLocalCosts(ctx context.Context) schedulingcost.NodeCosts {
	if scheduler.analytics.store == nil {
		return schedulingcost.NodeCosts{}
	}
	minSamples := int64(scheduler.minSamples)

	imageSamples, loadSamples, err := scheduler.analytics.store.CostSamples(ctx, routeranalytics.SectionImage, scheduler.sampleWindow, time.Now())
	if err != nil {
		scheduler.logger.Printf("scheduling cost sampling failed: %v", err)
		return schedulingcost.NodeCosts{}
	}
	textSamples, err := scheduler.analytics.store.TextCostSamples(ctx, scheduler.sampleWindow, time.Now())
	if err != nil {
		scheduler.logger.Printf("scheduling text cost sampling failed: %v", err)
		textSamples = nil
	}
	profileSamples, err := scheduler.analytics.store.TokenProfileSamples(ctx, scheduler.sampleWindow, time.Now())
	if err != nil {
		scheduler.logger.Printf("scheduling token profile sampling failed: %v", err)
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
