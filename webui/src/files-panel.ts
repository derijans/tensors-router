import { elements } from "./elements";
import { fileExtensionOptions, fileRoleOptions, filterInventoryFiles } from "./model-inventory-data";
import { state } from "./state";
import type { FileRecord, NodeInventory } from "./types";
import { escapeAttribute, escapeHTML, fileRoles, formatBytes } from "./utils";

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
  elements.filesScanNotices.innerHTML = scanNotices(nodes);
  elements.filesTable.innerHTML = filtered.length > 0 ? filtered.map(fileRow).join("") : `<tr><td class="inventory-empty" colspan="6">No files match the current filters.</td></tr>`;
}

function fileRow(file: FileRecord): string {
  return `
    <tr>
      <td title="${escapeAttribute(file.path)}">${escapeHTML(file.basename)}</td>
      <td>${escapeHTML(file.node_id || "")}</td>
      <td>${escapeHTML(fileRoles(file).join(", "))}</td>
      <td>${escapeHTML(normalizedExtension(file))}</td>
      <td>${formatBytes(file.size || 0)}</td>
      <td>${fileHashCell(file.node_id || "", file.path, file.sha256 || "")}</td>
    </tr>`;
}

function normalizedExtension(file: FileRecord): string {
  const extension = (file.extension || file.basename.split(".").pop() || "").trim().toLowerCase();
  return extension && !extension.startsWith(".") ? `.${extension}` : extension;
}

function fileHashCell(nodeID: string, path: string, hash: string): string {
  if (!hash) {
    return `<button type="button" data-operation-group="models" data-hash-file-node="${escapeAttribute(nodeID)}" data-hash-file-path="${escapeAttribute(path)}">Hash</button>`;
  }
  return `<span title="${escapeAttribute(hash)}"><code>${escapeHTML(hash.slice(0, 8))}</code> <button type="button" data-copy-file-hash="${escapeAttribute(hash)}">Copy</button></span>`;
}

function renderSelect(select: HTMLSelectElement, allLabel: string, values: string[], selected: string): void {
  select.innerHTML = `<option value="">${escapeHTML(allLabel)}</option>${values.map(value => `<option value="${escapeAttribute(value)}"${value === selected ? " selected" : ""}>${escapeHTML(value)}</option>`).join("")}`;
}

function scanNotices(nodes: NodeInventory[]): string {
  return nodes.filter(node => node.error).map(node => `<div class="inventory-notice error-text">${escapeHTML(node.node_id || node.node_url || "unknown node")}: ${escapeHTML(node.error || "scan failed")}</div>`).join("");
}
