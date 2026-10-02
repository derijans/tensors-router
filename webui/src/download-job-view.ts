import { SafeHTML, html } from "./safe-html";
import { badge } from "./markup-primitives";
import { downloadJobActions, formatDuration, remainingSeconds } from "./download-job-data";
import { formatBytes } from "./utils";
import type { DownloadJob, JobFile, Tone } from "./types";

export function renderDownloadJob(job: DownloadJob, bytesPerSecond: number | undefined): SafeHTML {
  const remaining = remainingSeconds(job, bytesPerSecond);
  const speed = bytesPerSecond ? ` · ${formatBytes(bytesPerSecond)}/s` : "";
  const eta = remaining !== undefined ? ` · ${formatDuration(remaining)} left` : "";
  return html`
    <div class="download-entry download-job">
      <div class="download-job-head"><strong>${job.repository}</strong>${badge(job.state, downloadJobTone(job.state))}</div>
      <progress value="${Math.min(job.completed_bytes, job.total_bytes)}" max="${Math.max(job.total_bytes, 1)}" aria-label="Download progress for ${job.repository}"></progress>
      <span class="muted">${formatBytes(job.completed_bytes)} / ${formatBytes(job.total_bytes)}${speed}${eta}</span>
      ${job.files.length > 1 ? html`<ul class="download-job-files">${job.files.map(renderJobFile)}</ul>` : ""}
      ${job.error ? html`<p class="error-text">${job.error}</p>` : ""}
      <div class="button-strip">${downloadJobActions(job.state).map(action => html`<button type="button" data-download-job="${job.id}" data-download-action="${action}">${action}</button>`)}</div>
    </div>
  `;
}

function renderJobFile(file: JobFile): SafeHTML {
  return html`<li><code>${file.path}</code><span class="muted">${formatBytes(file.completed_bytes)} / ${formatBytes(file.size)} · ${file.state}</span></li>`;
}

function downloadJobTone(jobState: string): Tone {
  switch (jobState) {
    case "running":
      return "info";
    case "completed":
      return "success";
    case "failed":
      return "danger";
    case "paused":
      return "warning";
    default:
      return "neutral";
  }
}
