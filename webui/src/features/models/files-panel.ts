import { inventoryScanNotices } from "../nodes/inventory-scan-notices-view";
import { SafeHTML, html, setHTML } from "../../lib/safe-html";
import { fileExtensionOptions, fileRoleOptions, filterInventoryFiles } from "./model-inventory-data";
import { state } from "../../state/app-state";
import type { FileRecord, NodeInventory } from "../../types";
import { formatBytes } from "../../lib/format-bytes";
import { fillSelectWithAll } from "../../ui/select";
import { fileRoles, normalizedExtension } from "../../domain/model-files";
import { emptyTableRow } from "../../ui/empty-state";
import { modelsElements } from "./models-elements";

export function renderFilesPanel(files: FileRecord[], nodes: NodeInventory[]): void {
  fillSelectWithAll(modelsElements.fileRoleFilter, "All roles", fileRoleOptions(files), state.models.fileRoleFilter);
  fillSelectWithAll(modelsElements.fileExtensionFilter, "All extensions", fileExtensionOptions(files), state.models.fileExtensionFilter);
  const filtered = filterInventoryFiles(files, {
    query: state.models.fileSearch,
    nodeIDs: state.models.fileNodeIDs,
    role: state.models.fileRoleFilter,
    extension: state.models.fileExtensionFilter,
    hash: state.models.fileHashFilter
  });
  modelsElements.filesRowCount.textContent = `${filtered.length} of ${files.length} files`;
  setHTML(modelsElements.filesScanNotices, inventoryScanNotices(nodes));
  setHTML(modelsElements.filesTable, filtered.length > 0 ? html`${filtered.map(fileRow)}` : emptyTableRow("No files match the current filters.", 6));
}

function fileRow(file: FileRecord): SafeHTML {
  return html`
    <tr>
      <td><div class="model-cell"><strong>${file.basename}</strong><small>${file.path}</small></div></td>
      <td>${file.node_id || ""}</td>
      <td>${fileRoles(file).join(", ")}</td>
      <td>${normalizedExtension(file)}</td>
      <td>${formatBytes(file.size || 0)}</td>
      <td>${fileHashCell(file.node_id || "", file.path, file.sha256 || "")}</td>
    </tr>`;
}

function fileHashCell(nodeID: string, path: string, hash: string): SafeHTML {
  if (!hash) {
    return html`<button type="button" data-operation-group="models" data-hash-file-node="${nodeID}" data-hash-file-path="${path}">Hash</button>`;
  }
  return html`<span title="${hash}"><code>${hash.slice(0, 8)}</code> <button type="button" data-copy-file-hash="${hash}">Copy</button></span>`;
}
