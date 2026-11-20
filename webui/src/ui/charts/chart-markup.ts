import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import { areaUnderPath, bandPath, linePath, paddedPeak, segmentOffsets, stackLayers, tickIndexes, type PlotSize } from "./chart-geometry";
import type { ChartLayer, MemorySegment, StackedSeries } from "../../types";

const areaPlot: PlotSize = {width: 1000, height: 260};
const sparkPlot: PlotSize = {width: 120, height: 36};

export function chartLegend(series: StackedSeries): SafeHTML {
  return html`${series.layers.map(chartLegendItem)}`;
}

function chartLegendItem(layer: ChartLayer): SafeHTML {
  return html`<li class="${layer.accent}"><i></i>${layer.label}</li>`;
}

export function stackedAreaChart(series: StackedSeries, formatTick: (bucketStart: number) => string, label: string): SafeHTML {
  const count = series.bucketStarts.length;
  if (count === 0 || series.layers.length === 0) {
    return html`<div class="empty-state">No requests in this period.</div>`;
  }
  const accentByKey = new Map(series.layers.map(layer => [layer.key, layer.accent]));
  const valuesByKey = Object.fromEntries(series.layers.map(layer => [layer.key, layer.values]));
  const layers = stackLayers(series.layers.map(layer => layer.key), valuesByKey, count);
  const peak = paddedPeak(layers.at(-1)?.top ?? []);
  const gridlines = [1, 2, 3].map(step => (areaPlot.height * step) / 4);
  return html`
    <svg class="area-chart" viewBox="0 0 ${areaPlot.width} ${areaPlot.height}" preserveAspectRatio="none" role="img" aria-label="${label}">
      ${gridlines.map(y => html`<line class="chart-gridline" x1="0" x2="${areaPlot.width}" y1="${y}" y2="${y}"></line>`)}
      ${[...layers].reverse().map(layer => html`
        <path class="chart-area ${accentByKey.get(layer.key)}" d="${bandPath(layer.top, layer.bottom, peak, areaPlot)}"></path>
        <path class="chart-edge ${accentByKey.get(layer.key)}" d="${linePath(layer.top, peak, areaPlot)}"></path>
      `)}
    </svg>
    <div class="chart-axis">
      ${tickIndexes(count).map(index => html`<span>${formatTick(series.bucketStarts[index] ?? 0)}</span>`)}
    </div>
  `;
}

export function sparkline(values: readonly number[], label: string): SafeHTML {
  if (values.length < 2) {
    return emptyHTML;
  }
  const peak = paddedPeak(values);
  return html`
    <svg class="sparkline" viewBox="0 0 ${sparkPlot.width} ${sparkPlot.height}" preserveAspectRatio="none" role="img" aria-label="${label}">
      <path class="sparkline-fill" d="${areaUnderPath(values, peak, sparkPlot)}"></path>
      <path class="sparkline-line" d="${linePath(values, peak, sparkPlot)}"></path>
    </svg>
  `;
}

export function memoryBar(segments: readonly MemorySegment[], label: string): SafeHTML {
  const offsets = segmentOffsets(segments.map(segment => segment.percent));
  return html`
    <svg class="memory-bar" viewBox="0 0 100 10" preserveAspectRatio="none" role="img" aria-label="${label}">
      <rect class="memory-free" x="0" y="0" width="100" height="10"></rect>
      ${segments.map((segment, index) => html`
        <rect class="memory-segment ${segment.accent}" x="${(offsets[index] ?? 0).toFixed(2)}" y="0" width="${Math.max(0.4, segment.percent - 0.5).toFixed(2)}" height="10"><title>${segment.label}</title></rect>
      `)}
    </svg>
  `;
}
