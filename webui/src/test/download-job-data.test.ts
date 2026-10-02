import { describe, expect, it } from "vitest";
import { downloadJobActions, formatDuration, remainingSeconds, trackDownloadProgress } from "../download-job-data";
import type { DownloadJob } from "../types";

function job(completed: number, state = "running"): DownloadJob {
  return {id: "job", repository: "owner/model", revision: "main", commit: "c", state, total_bytes: 1000, completed_bytes: completed, created_at: "", updated_at: "", files: []};
}

describe("download job actions", () => {
  it("lets queued jobs be paused or cancelled, not only running ones", () => {
    expect(downloadJobActions("queued")).toEqual(["pause", "cancel"]);
    expect(downloadJobActions("running")).toEqual(["pause", "cancel"]);
    expect(downloadJobActions("failed")).toEqual(["resume", "cancel"]);
    expect(downloadJobActions("completed")).toEqual([]);
  });
});

describe("download progress tracking", () => {
  it("derives throughput from successive polls", () => {
    const first = trackDownloadProgress({samples: new Map(), bytesPerSecond: new Map()}, [job(100)], 0);
    const second = trackDownloadProgress(first, [job(300)], 2000);
    expect(second.bytesPerSecond.get("job")).toBe(100);
  });

  it("keeps the last measured throughput while the server has not reported new bytes yet", () => {
    const first = trackDownloadProgress({samples: new Map(), bytesPerSecond: new Map()}, [job(100)], 0);
    const measured = trackDownloadProgress(first, [job(300)], 2000);
    const unchanged = trackDownloadProgress(measured, [job(300)], 3500);
    expect(unchanged.bytesPerSecond.get("job")).toBe(100);
  });

  it("forgets jobs that stopped running", () => {
    const first = trackDownloadProgress({samples: new Map(), bytesPerSecond: new Map()}, [job(100)], 0);
    const paused = trackDownloadProgress(first, [job(100, "paused")], 1000);
    expect(paused.samples.size).toBe(0);
  });

  it("estimates the remaining time from throughput", () => {
    expect(remainingSeconds(job(400), 100)).toBe(6);
    expect(remainingSeconds(job(400), undefined)).toBeUndefined();
    expect(formatDuration(65)).toBe("1:05");
    expect(formatDuration(3725)).toBe("1:02:05");
  });
});
