import { formatBytes } from "../../lib/format-bytes";
import { formatCalendarDate } from "../../lib/format-time";
import { type SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import type { AppState, DownloadPlan } from "../../types";
import { badge } from "../../ui/badge";
import { button, buttonStrip } from "../../ui/buttons";
import { checkbox, toggleRow } from "../../ui/form";
import { actionStatus, errorText, mutedText } from "../../ui/text";
import { selectedDownloadBytes } from "./download-plan-data";

type DownloadsState = AppState["downloads"];
type DownloadCandidate = DownloadsState["candidates"][number];
type DownloadSearchResult = DownloadsState["search"][number];
type PlannedFile = DownloadPlan["files"][number];

const shownResultTagLimit = 6;

export function searchResultsMarkup(downloads: DownloadsState): SafeHTML {
  const finder = downloads.finderMessage ? actionStatus(downloads.finderMessage) : emptyHTML;
  const rows = html`${downloads.search.map(result => searchResultEntry(result, downloads.selectedRepository === result.id))}`;
  const candidates = html`${downloads.candidates.map((candidate, index) => candidateEntry(candidate, index, downloads.modelHandoff !== null))}`;
  return html`${finder}${searchStatusNotice(downloads)}${rows}${candidates}`;
}

export function planMarkup(plan: DownloadPlan, selectedFiles: readonly string[]): SafeHTML {
  const selected = new Set(selectedFiles);
  const selectedBytes = selectedDownloadBytes(plan, [...selectedFiles]);
  return html`
    <div class="download-entry">
      <strong>${plan.commit}</strong>
      <span>${plan.destination} · ${formatBytes(selectedBytes)} selected of ${formatBytes(plan.total_bytes)}</span>
      ${plan.unsafe_warning ? errorText("Hugging Face reports unsafe or pending security status. Starting requires confirmation.") : ""}
      ${plan.gated ? actionStatus("Gated repository: approve access on Hugging Face and use an authorized token.") : ""}
      ${skippedFilesNotice(plan)}
      ${buttonStrip([
        button("Select all", {"data-download-plan-select": "all"}),
        button("Select none", {"data-download-plan-select": "none"}),
        button("Required only", {"data-download-plan-select": "required"})
      ])}
      <ul class="plan-files">${plan.files.map(file => plannedFileItem(file, selected.has(file.path)))}</ul>
    </div>
  `;
}

function candidateEntry(candidate: DownloadCandidate, index: number, hasModelHandoff: boolean): SafeHTML {
  return html`
    <div class="download-entry candidate-${candidate.state}">
      <strong>${candidate.repository} / ${candidate.repository_path}</strong>
      <span>${candidate.state} · ${candidate.sha256 || "no verifiable LFS SHA-256"}</span>
      ${candidateActions(candidate, index, hasModelHandoff)}
    </div>
  `;
}

function candidateActions(candidate: DownloadCandidate, index: number, hasModelHandoff: boolean): SafeHTML {
  if (candidate.state === "exact") {
    return button("Bind verified origin", {"data-download-candidate-bind": index});
  }
  if (candidate.state === "mismatched" && hasModelHandoff && candidate.sha256) {
    return button("Replace expected model", {class: "danger", "data-download-candidate-replace": index});
  }
  return emptyHTML;
}

function searchStatusNotice(downloads: DownloadsState): SafeHTML {
  switch (downloads.searchStatus) {
    case "searching":
      return actionStatus("Searching Hugging Face…");
    case "error":
      return errorText(html`Search failed: ${downloads.searchError || "unknown error"}`);
    case "empty":
      return mutedText(html`No models match ${downloads.searchQuery ? `"${downloads.searchQuery}"` : "the current filters"}.`, "p");
    default:
      return emptyHTML;
  }
}

function searchResultEntry(result: DownloadSearchResult, selected: boolean): SafeHTML {
  return html`
    <button class="download-entry${selected ? " selected" : ""}" type="button" data-download-repository="${result.id}">
      <strong>${result.id}</strong>
      <span>${searchResultMeta(result).join(" · ")}</span>
      ${searchResultTags(result)}
    </button>`;
}

function searchResultMeta(result: DownloadSearchResult): string[] {
  const meta = [`${result.downloads.toLocaleString()} downloads`, `${result.likes.toLocaleString()} likes`];
  if (result.updated_at) {
    meta.push(`updated ${formatCalendarDate(result.updated_at)}`);
  }
  if (result.gated && result.gated !== "false") {
    meta.push("gated");
  }
  return meta;
}

function searchResultTags(result: DownloadSearchResult): SafeHTML {
  const tags = (result.tags || []).filter(tag => !tag.includes(":") || /^(license|pipeline_tag|library):/.test(tag)).slice(0, shownResultTagLimit);
  if (tags.length === 0) {
    return emptyHTML;
  }
  return html`<span class="download-tags">${tags.map(tag => badge(tag, "neutral"))}</span>`;
}

function skippedFilesNotice(plan: DownloadPlan): SafeHTML {
  const skipped = plan.skipped || [];
  if (skipped.length === 0) {
    return emptyHTML;
  }
  return mutedText(html`Skipped: ${skipped.map(file => `${file.path} (${file.reason})`).join(", ")}`, "p");
}

function plannedFileItem(file: PlannedFile, checked: boolean): SafeHTML {
  return html`<li>${toggleRow(checkbox({"data-download-plan-file": file.path}, checked), html`<code>${file.path}</code>${mutedText(html`${formatBytes(file.size)} · ${file.reason}`)}${file.required ? badge("required", "accent") : ""}`)}</li>`;
}
