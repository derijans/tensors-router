import type { DownloadJob } from "./types";

export type DownloadJobAction = "pause" | "resume" | "cancel";

export interface DownloadProgressSample {
  bytes: number;
  atMilliseconds: number;
}

export interface DownloadProgressTracking {
  samples: Map<string, DownloadProgressSample>;
  bytesPerSecond: Map<string, number>;
}

export function downloadJobActions(jobState: string): DownloadJobAction[] {
  switch (jobState) {
    case "queued":
    case "running":
      return ["pause", "cancel"];
    case "paused":
    case "failed":
      return ["resume", "cancel"];
    default:
      return [];
  }
}

export function trackDownloadProgress(previous: DownloadProgressTracking, jobs: DownloadJob[], nowMilliseconds: number): DownloadProgressTracking {
  const samples = new Map<string, DownloadProgressSample>();
  const bytesPerSecond = new Map<string, number>();
  for (const job of jobs.filter(candidate => candidate.state === "running")) {
    const earlier = previous.samples.get(job.id);
    if (earlier && earlier.bytes === job.completed_bytes) {
      samples.set(job.id, earlier);
      const unchangedRate = previous.bytesPerSecond.get(job.id);
      if (unchangedRate !== undefined) {
        bytesPerSecond.set(job.id, unchangedRate);
      }
      continue;
    }
    samples.set(job.id, {bytes: job.completed_bytes, atMilliseconds: nowMilliseconds});
    const elapsedSeconds = earlier ? (nowMilliseconds - earlier.atMilliseconds) / 1000 : 0;
    if (earlier && elapsedSeconds > 0 && job.completed_bytes > earlier.bytes) {
      bytesPerSecond.set(job.id, (job.completed_bytes - earlier.bytes) / elapsedSeconds);
    }
  }
  return {samples, bytesPerSecond};
}

export function remainingSeconds(job: DownloadJob, bytesPerSecond: number | undefined): number | undefined {
  if (!bytesPerSecond || bytesPerSecond <= 0) {
    return undefined;
  }
  return Math.max(job.total_bytes - job.completed_bytes, 0) / bytesPerSecond;
}

export function formatDuration(totalSeconds: number): string {
  const seconds = Math.ceil(totalSeconds);
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = String(seconds % 60).padStart(2, "0");
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, "0")}:${rest}` : `${minutes}:${rest}`;
}
