package proxy

import (
	"context"
	"path/filepath"
	"strings"

	routeranalytics "tensors-router/internal/analytics"
	"tensors-router/internal/catalog"
	"tensors-router/internal/hardware"
)

func (analytics *requestAnalytics) vramActive() bool {
	return analytics.store != nil && analytics.vramEnabled && analytics.vramSource != nil
}

func (analytics *requestAnalytics) activeVRAMSource() hardware.VRAMSource {
	if !analytics.vramActive() {
		return nil
	}
	return analytics.vramSource
}

// beginLoad times every model load whenever analytics is on, because the
// load duration is what a scheduler needs to weigh a model switch. VRAM sampling
// is a separate, optional enrichment: a node without a VRAM source still records
// how long its loads take.
func (analytics *requestAnalytics) beginLoad(ctx context.Context) *routeranalytics.VRAMLoadMeasurement {
	if analytics.store == nil {
		return nil
	}
	return routeranalytics.StartVRAMLoad(ctx, analytics.activeVRAMSource())
}

func (analytics *requestAnalytics) finishLoad(ctx context.Context, measurement *routeranalytics.VRAMLoadMeasurement) {
	if measurement == nil {
		return
	}
	measurement.Finish(ctx, analytics.activeVRAMSource())
}

func (analytics *requestAnalytics) recordLoad(modelID string, configFilename string, readiness backendReadiness, backendMode string, measurement *routeranalytics.VRAMLoadMeasurement) {
	if measurement == nil || analytics.store == nil {
		return
	}
	event := routeranalytics.Event{
		NodeID:         analytics.nodeID,
		ModelID:        modelID,
		Section:        analytics.loadSection(configFilename, readiness),
		BackendMode:    backendMode,
		EventType:      routeranalytics.EventTypeModelLoad,
		Route:          "model_load",
		ConfigFilename: configFilename,
		StatusCode:     200,
		Success:        true,
	}
	measurement.ApplyTo(&event)
	analytics.store.Record(event)
}

func applyVRAMLoadStateLocked(state *activeConfigState, measurement *routeranalytics.VRAMLoadMeasurement) {
	state.vramBaselineValid = false
	state.vramBaselineMB = 0
	state.vramTotalMB = 0
	if measurement == nil {
		return
	}
	baseline, measured := measurement.Baseline()
	if !measured {
		return
	}
	state.vramBaselineValid = true
	state.vramBaselineMB = baseline.UsedMB
	state.vramTotalMB = baseline.TotalMB
}

func clearVRAMLoadStateLocked(state *activeConfigState) {
	state.vramBaselineValid = false
	state.vramBaselineMB = 0
	state.vramTotalMB = 0
}

func (analytics *requestAnalytics) beginWork(runtime *backendRuntime) routeranalytics.EventFinalizer {
	if !analytics.vramActive() || runtime == nil {
		return nil
	}
	baselineMB, hasBaseline := runtimeVRAMBaseline(runtime)
	return routeranalytics.StartVRAMWorkSampler(analytics.vramSource, analytics.vramInterval, baselineMB, hasBaseline).Finish
}

func (analytics *requestAnalytics) close(ctx context.Context) error {
	if analytics.vramSampler == nil {
		return nil
	}
	return analytics.vramSampler.Close(ctx)
}

func runtimeVRAMBaseline(runtime *backendRuntime) (int64, bool) {
	runtime.state.mu.Lock()
	defer runtime.state.mu.Unlock()
	return runtime.state.vramBaselineMB, runtime.state.vramBaselineValid
}

func (service *Service) loadAnalyticsSection(configFilename string, readiness backendReadiness) string {
	if readiness == readinessText && service.textRuntimeHoldsOnlyEmbeddings(configFilename) {
		return routeranalytics.SectionEmbed
	}
	return readinessAnalyticsSection(readiness)
}

func (service *Service) textRuntimeHoldsOnlyEmbeddings(filename string) bool {
	path := service.configPathForFilename(filename)
	if path == "" {
		return false
	}
	metadata, err := catalog.LoadRuntimeConfig(path)
	if err != nil {
		return false
	}
	embeddingsModel := strings.TrimSpace(metadata.EmbeddingsModel)
	return embeddingsModel != "" && metadata.TextModelPath() == embeddingsModel
}

func (service *Service) configPathForFilename(filename string) string {
	if filename == "" || filename != filepath.Base(filename) {
		return ""
	}
	if service.catalog != nil {
		models, err := service.catalog.List()
		if err != nil {
			return ""
		}
		for _, model := range models {
			if model.Filename == filename {
				return model.Path
			}
		}
	}
	if service.configDir == "" {
		return ""
	}
	return filepath.Join(service.configDir, filename)
}

func readinessAnalyticsSection(readiness backendReadiness) string {
	switch readiness {
	case readinessImage:
		return routeranalytics.SectionImage
	case readinessEmbeddings:
		return routeranalytics.SectionEmbed
	case readinessSpeech, readinessTranscription:
		return routeranalytics.SectionVoice
	case readinessMusic:
		return routeranalytics.SectionMusic
	default:
		return routeranalytics.SectionLLM
	}
}
