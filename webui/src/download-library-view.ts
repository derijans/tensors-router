import { SafeHTML, emptyHTML, html } from "./safe-html";
import { formatBytes } from "./utils";
import type { ArtifactRecord, DownloadLibraryResponse, UnhashedFile } from "./types";

export function renderDownloadLibrary(library: DownloadLibraryResponse | null): SafeHTML {
  const artifacts = library?.artifacts ?? [];
  const unhashed = library?.unhashed ?? [];
  if (artifacts.length === 0 && unhashed.length === 0) {
    return html`<p class="muted">No indexed artifacts on this node.</p>`;
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
    <p class="muted">Not hashed — Rescan to hash</p>
    ${files.map(file => html`<div class="download-entry"><strong>${file.path}</strong><span>${formatBytes(file.size)} · not hashed</span></div>`)}
  `;
}
