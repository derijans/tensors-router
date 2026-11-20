export interface PlotSize {
  width: number;
  height: number;
}

export interface StackedLayer<Key extends string> {
  key: Key;
  top: number[];
  bottom: number[];
}

export function stackLayers<Key extends string>(keys: readonly Key[], valuesByKey: Record<Key, number[]>, length: number): StackedLayer<Key>[] {
  const floor = new Array<number>(length).fill(0);
  return keys.map(key => {
    const bottom = [...floor];
    const values = valuesByKey[key];
    for (let index = 0; index < length; index++) {
      floor[index] = (floor[index] ?? 0) + Math.max(0, values[index] ?? 0);
    }
    return {key, bottom, top: [...floor]};
  });
}

export function plotX(index: number, count: number, width: number): number {
  return count <= 1 ? width / 2 : (index / (count - 1)) * width;
}

export function plotY(value: number, peak: number, height: number): number {
  return peak <= 0 ? height : height - (Math.max(0, value) / peak) * height;
}

export function paddedPeak(values: readonly number[]): number {
  const peak = Math.max(0, ...values);
  return peak > 0 ? peak * 1.1 : 1;
}

export function linePath(values: readonly number[], peak: number, size: PlotSize): string {
  return values
    .map((value, index) => `${index === 0 ? "M" : "L"}${plotX(index, values.length, size.width).toFixed(2)} ${plotY(value, peak, size.height).toFixed(2)}`)
    .join(" ");
}

export function bandPath(top: readonly number[], bottom: readonly number[], peak: number, size: PlotSize): string {
  if (top.length === 0) {
    return "";
  }
  const reversedBottom = bottom
    .map((value, index) => `L${plotX(index, bottom.length, size.width).toFixed(2)} ${plotY(value, peak, size.height).toFixed(2)}`)
    .reverse()
    .join(" ");
  return `${linePath(top, peak, size)} ${reversedBottom} Z`;
}

export function areaUnderPath(values: readonly number[], peak: number, size: PlotSize): string {
  return bandPath(values, values.map(() => 0), peak, size);
}

export function tickIndexes(count: number, desiredTicks = 5): number[] {
  if (count <= 0) {
    return [];
  }
  if (count <= desiredTicks) {
    return Array.from({length: count}, (_, index) => index);
  }
  const lastIndex = count - 1;
  const indexes = Array.from({length: desiredTicks}, (_, tick) => Math.round((tick * lastIndex) / (desiredTicks - 1)));
  return Array.from(new Set(indexes));
}

export function segmentOffsets(widths: readonly number[]): number[] {
  const offsets: number[] = [];
  let running = 0;
  for (const width of widths) {
    offsets.push(running);
    running += width;
  }
  return offsets;
}
