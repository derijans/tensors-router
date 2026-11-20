import { describe, expect, it } from "vitest";
import { groupBy, lookupOr, toggledMember, uniqueSortedNonBlank } from "../lib/collections";
import { formatBytes } from "../lib/format-bytes";
import { clampedPercent, formatIntegerOrFixed } from "../lib/format-number";
import { formatCalendarDate, formatElapsedSeconds, formatEpochTime, formatTimestamp, timestampMillis } from "../lib/format-time";
import { matchesQuery } from "../lib/text";

describe("time formatting", () => {
  it("shows unparseable timestamps as they were received", () => {
    expect(formatTimestamp("not a date")).toBe("not a date");
    expect(formatCalendarDate("not a date")).toBe("not a date");
  });

  it("reduces a valid timestamp to its calendar date", () => {
    expect(formatCalendarDate("2026-03-04T05:06:07Z")).toBe("2026-03-04");
  });

  it("reports missing epoch times as never", () => {
    expect(formatEpochTime(0)).toBe("never");
  });

  it("treats blank and invalid timestamps as absent", () => {
    expect(timestampMillis("")).toBeUndefined();
    expect(timestampMillis("garbage")).toBeUndefined();
    expect(timestampMillis("1970-01-01T00:00:01Z")).toBe(1000);
  });

  it("never shows negative elapsed time", () => {
    expect(formatElapsedSeconds(-250)).toBe("0.0s");
    expect(formatElapsedSeconds(1250)).toBe("1.3s");
  });
});

describe("number formatting", () => {
  it("switches byte units at each power of 1024", () => {
    expect(formatBytes(1023)).toBe("1023 B");
    expect(formatBytes(1024)).toBe("1.0 KB");
    expect(formatBytes(1024 * 1024)).toBe("1.0 MB");
    expect(formatBytes(1024 * 1024 * 1024)).toBe("1.0 GB");
  });

  it("keeps whole numbers exact and rounds fractions to the requested digits", () => {
    expect(formatIntegerOrFixed(12, 2)).toBe("12");
    expect(formatIntegerOrFixed(1.256, 2)).toBe("1.26");
  });

  it("clamps a share into the 0 to 100 percent range", () => {
    expect(clampedPercent(150, 100)).toBe(100);
    expect(clampedPercent(-1, 100)).toBe(0);
    expect(clampedPercent(25, 100)).toBe(25);
  });
});

describe("text helpers", () => {
  it("matches a trimmed query without regard to case", () => {
    expect(matchesQuery("Qwen3 Coder", "  coder ")).toBe(true);
    expect(matchesQuery("Qwen3 Coder", "llama")).toBe(false);
    expect(matchesQuery("anything", "   ")).toBe(true);
  });
});

describe("collections", () => {
  it("adds a missing member and removes a present one", () => {
    expect(toggledMember(["a", "b"], "c")).toEqual(["a", "b", "c"]);
    expect(toggledMember(["a", "b"], "a")).toEqual(["b"]);
  });

  it("groups items by key in first-seen order", () => {
    const groups = groupBy([{node: "b", id: 1}, {node: "a", id: 2}, {node: "b", id: 3}], item => item.node);
    expect([...groups.keys()]).toEqual(["b", "a"]);
    expect(groups.get("b")?.map(item => item.id)).toEqual([1, 3]);
  });

  it("falls back for keys that only exist on the object prototype", () => {
    const tones: Record<string, string> = {ready: "success"};
    expect(lookupOr(tones, "ready", "neutral")).toBe("success");
    expect(lookupOr(tones, "toString", "neutral")).toBe("neutral");
    expect(lookupOr(tones, "constructor", "neutral")).toBe("neutral");
  });

  it("drops blank values before deduplicating and sorting", () => {
    expect(uniqueSortedNonBlank([" b", "a", "", "b ", "  "])).toEqual(["a", "b"]);
  });
});
