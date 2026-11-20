import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import type { ArtifactRecord, DownloadLibraryResponse, UnhashedFile } from "../../types";
import { formatBytes } from "../../lib/format-bytes";
import { mutedText } from "../../ui/text";

export function renderDownloadLibrary(library: DownloadLibraryResponse | null): SafeHTML {
  const artifacts = library?.artifacts ?? [];
  const unhashed = library?.unhashed ?? [];
  if (artifacts.length === 0 && unhashed.length === 0) {
    return mutedText("No indexed artifacts on this node.", "p");
  }
  return html`${artifacts.map(renderArtifact)}${renderUnhashedFiles(unhashed)}`;
}

function renderArtifact(artifact: ArtifactRecord): SafeHTML {
  return html`<div class="download-entry"><strong>${artifact.path}</strong><span>${formatBytes(artifact.size)} · ${artifact.verification_source} · ${artifact.sha256}</span></div>`;
}

function renderUnhashedFiles(files: readonly UnhashedFile[]): SafeHTML {
  if (files.length === 0) {
    return emptyHTML;
  }
  return html`
    ${mutedText("Not hashed — Rescan to hash", "p")}
    ${files.map(file => html`<div class="download-entry"><strong>${file.path}</strong><span>${formatBytes(file.size)} · not hashed</span></div>`)}
  `;
}
