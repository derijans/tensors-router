import { bindModelAssetCandidate, substituteModelAsset } from "../../api/model-assets-api";
import { loadModelConfig } from "../../api/models-api";
import { state } from "../../state/app-state";
import { normalizeModelHash } from "./download-finder-data";
import { downloadToken, normalizedExpectedHash } from "./download-inputs";
import { renderDownloads } from "./downloads";
import { downloadsElements } from "./downloads-elements";
import { updateDownloadSearchMode } from "./download-finder";

export function prefillDownloadContext(context: {nodeID: string; publicID: string; configID: string; configFilename: string; field: string; position?: number; filename: string; hash: string}): void {
  if (state.downloads.capabilities?.nodes.some(node => node.node_id === context.nodeID)) {
    state.downloads.nodeID = context.nodeID;
  }
  state.downloads.modelHandoff = context;
  downloadsElements.downloadSearchMode.value = "filename";
  downloadsElements.downloadSearchInput.value = context.filename;
  downloadsElements.downloadExpectedHashInput.value = context.hash;
  updateDownloadSearchMode();
  state.downloads.finderMessage = "Prefilled from unresolved Models load. Search to verify repository candidates.";
  renderDownloads();
}

export async function replaceDownloadCandidate(index: number): Promise<void> {
  const candidate = state.downloads.candidates[index];
  const context = state.downloads.modelHandoff;
  if (candidate?.state !== "mismatched" || !candidate.sha256 || !context) {
    throw new Error("A verified mismatching Models candidate is required");
  }
  const token = downloadToken();
  await substituteModelAsset({
    node_id: context.nodeID,
    id: context.configID,
    filename: context.configFilename,
    field: context.field,
    ...(context.position === undefined ? {} : {position: context.position}),
    expected_sha256: context.hash,
    sha256: normalizeModelHash(candidate.sha256),
    repository: candidate.repository,
    repository_path: candidate.repository_path,
    commit: candidate.commit,
    ...(token ? {token} : {}),
    confirm: true
  });
  state.downloads.modelHandoff = null;
  state.downloads.finderMessage = `Config intentionally updated to ${candidate.repository_path}; loading model`;
  renderDownloads();
  await loadModelConfig({model: context.publicID});
  state.downloads.finderMessage = `Config intentionally updated to ${candidate.repository_path} and loaded`;
  renderDownloads();
}

export async function bindDownloadCandidate(index: number): Promise<void> {
  const candidate = state.downloads.candidates[index];
  const hash = normalizedExpectedHash();
  if (candidate?.state !== "exact" || !state.downloads.nodeID) {
    throw new Error("Only an exact verified candidate can be bound");
  }
  const token = downloadToken();
  await bindModelAssetCandidate({
    node_id: state.downloads.nodeID,
    sha256: hash,
    repository: candidate.repository,
    repository_path: candidate.repository_path,
    commit: candidate.commit,
    ...(token ? {token} : {})
  });
  state.downloads.finderMessage = `Verified origin bound for ${candidate.repository_path}`;
  renderDownloads();
}
