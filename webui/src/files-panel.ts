import { inventoryScanNotices } from "./inventory-scan-notices";
import { optionElement } from "./markup-primitives";
import { SafeHTML, html, setHTML } from "./safe-html";
import { elements } from "./elements";
import { fileExtensionOptions, fileRoleOptions, filterInventoryFiles } from "./model-inventory-data";
import { state } from "./state";
import type { FileRecord, NodeInventory } from "./types";
import { fileRoles, formatBytes } from "./utils";

export function renderFilesPanel(files: FileRecord[], nodes: NodeInventory[]): void {
  renderSelect(elements.fileRoleFilter, "All roles", fileRoleOptions(files), state.models.fileRoleFilter);
  renderSelect(elements.fileExtensionFilter, "All extensions", fileExtensionOptions(files), state.models.fileExtensionFilter);
  const filtered = filterInventoryFiles(files, {
    query: state.models.fileSearch,
    nodeIDs: state.models.fileNodeIDs,
    role: state.models.fileRoleFilter,
    extension: state.models.fileExtensionFilter,
    hash: state.models.fileHashFilter
  });
  elements.filesRowCount.textContent = `${filtered.length} of ${files.length} files`;
  setHTML(elements.filesScanNotices, inventoryScanNotices(nodes));
  setHTML(elements.filesTable, filtered.length > 0 ? html`${filtered.map(fileRow)}` : html`<tr><td class="empty-state" colspan="6">No files match the current filters.</td></tr>`);
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

function normalizedExtension(file: FileRecord): string {
  const extension = (file.extension || file.basename.split(".").pop() || "").trim().toLowerCase();
  return extension && !extension.startsWith(".") ? `.${extension}` : extension;
}

function fileHashCell(nodeID: string, path: string, hash: string): SafeHTML {
  if (!hash) {
    return html`<button type="button" data-operation-group="models" data-hash-file-node="${nodeID}" data-hash-file-path="${path}">Hash</button>`;
  }
  return html`<span title="${hash}"><code>${hash.slice(0, 8)}</code> <button type="button" data-copy-file-hash="${hash}">Copy</button></span>`;
}

function renderSelect(select: HTMLSelectElement, allLabel: string, values: string[], selected: string): void {
  setHTML(select, html`<option value="">${allLabel}</option>${values.map(value => optionElement(value, value, value === selected))}`);
}
