import { SafeHTML, html } from "../../lib/safe-html";
import { badge } from "../../ui/badge";
import { downloadJobActions, remainingSeconds } from "./download-job-data";
import type { DownloadJob, JobFile, Tone } from "../../types";
import { formatClockDuration } from "../../lib/format-time";
import { formatBytes } from "../../lib/format-bytes";
import { errorText, mutedText } from "../../ui/text";
import { button, buttonStrip } from "../../ui/buttons";
import { progressBar } from "../../ui/progress";
import { lookupOr } from "../../lib/collections";

export function renderDownloadJob(job: DownloadJob, bytesPerSecond: number | undefined): SafeHTML {
  const remaining = remainingSeconds(job, bytesPerSecond);
  const speed = bytesPerSecond ? ` · ${formatBytes(bytesPerSecond)}/s` : "";
  const eta = remaining !== undefined ? ` · ${formatClockDuration(remaining)} left` : "";
  return html`
    <div class="download-entry download-job">
      <div class="download-job-head"><strong>${job.repository}</strong>${badge(job.state, downloadJobTone(job.state))}</div>
      ${progressBar(job.completed_bytes, job.total_bytes, {"aria-label": `Download progress for ${job.repository}`})}
      ${mutedText(html`${formatBytes(job.completed_bytes)} / ${formatBytes(job.total_bytes)}${speed}${eta}`)}
      ${job.files.length > 1 ? html`<ul class="download-job-files">${job.files.map(renderJobFile)}</ul>` : ""}
      ${job.error ? errorText(job.error) : ""}
      ${buttonStrip(downloadJobActions(job.state).map(action => button(action, {"data-download-job": job.id, "data-download-action": action})))}
    </div>
  `;
}

function renderJobFile(file: JobFile): SafeHTML {
  return html`<li><code>${file.path}</code>${mutedText(html`${formatBytes(file.completed_bytes)} / ${formatBytes(file.size)} · ${file.state}`)}</li>`;
}

const downloadJobTones: Record<string, Tone> = {
  running: "info",
  completed: "success",
  failed: "danger",
  paused: "warning"
};

function downloadJobTone(jobState: string): Tone {
  return lookupOr(downloadJobTones, jobState, "neutral");
}
