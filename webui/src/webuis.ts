import { getWebUIs, loadWebUI, setWebUISession } from "./api";
import { elements } from "./elements";
import { state } from "./state";
import { chip, escapeAttribute, escapeHTML, kindColor } from "./utils";
import {
  filteredWebUIEntries,
  groupWebUIs,
  webUIDialogData,
  webUIOpenStatus,
  type WebUIGroup
} from "./webui-data";
import type { WebUICompatibleModel, WebUIEntry } from "./types";

export {
  filteredWebUIEntries,
  groupWebUIs,
  webUIDialogData,
  webUIModelChoices,
  webUIOpenStatus
} from "./webui-data";

export async function loadWebUIs(): Promise<void> {
  state.webuis.loading = true;
  state.webuis.error = "";
  renderWebUIs();
  try {
    state.webuis.data = await getWebUIs();
  } catch (error) {
    state.webuis.error = error instanceof Error ? error.message : String(error);
  } finally {
    state.webuis.loading = false;
    renderWebUIs();
  }
}

export function updateWebUIFilter(value: string): void {
  state.webuis.filter = value;
  renderWebUIs();
}

export async function setWebUIEnabled(id: string, enabled: boolean): Promise<void> {
  state.webuis.action = enabled ? "Enabled" : "Disabled";
  state.webuis.error = "";
  renderWebUIs();
  try {
    state.webuis.data = await setWebUISession({id, enabled});
  } catch (error) {
    state.webuis.error = error instanceof Error ? error.message : String(error);
    throw error;
  } finally {
    renderWebUIs();
  }
}

export function openSelectedWebUI(id: string): void {
  const entry = findWebUIEntry(id);
  if (!entry) {
    return;
  }
  const status = webUIOpenStatus(entry);
  if (!status.openable) {
    showWebUIDialog(entry);
    return;
  }
  openExternalURL(entry.url);
}

export function showSelectedWebUIDialog(id: string): void {
  const entry = findWebUIEntry(id);
  if (entry) {
    showWebUIDialog(entry);
  }
}

export function closeWebUIDialog(): void {
  elements.webuiDialog.close();
}

export async function loadSelectedWebUIModel(id: string, modelID: string, imageID: string): Promise<void> {
  const entry = findWebUIEntry(id);
  if (!entry) {
    return;
  }
  state.webuis.action = `Loading ${modelID || imageID || entry.name}...`;
  state.webuis.error = "";
  renderWebUIs();
  try {
    const response = await loadWebUI({id, model_id: modelID, image_id: imageID});
    await loadWebUIs();
    const updated = findWebUIEntry(id);
    if (updated?.enabled && response.url) {
      closeWebUIDialog();
      openExternalURL(response.url);
      return;
    }
    state.webuis.action = `Loaded ${response.model_id || response.image_id || entry.name}`;
  } catch (error) {
    state.webuis.error = error instanceof Error ? error.message : String(error);
    throw error;
  } finally {
    renderWebUIs();
  }
}

export function renderWebUIs(): void {
  const entries = filteredWebUIEntries(state.webuis.data?.data ?? [], state.webuis.filter);
  elements.webuiStatus.textContent = webUIStatusText(entries.length);
  elements.webuiStatus.classList.toggle("error-text", state.webuis.error !== "");
  elements.webuiGrid.innerHTML = entries.length ? groupWebUIs(entries).map(renderWebUIGroup).join("") : `<div class="detail-empty">No WebUIs</div>`;
}

function renderWebUIGroup(group: WebUIGroup): string {
  return `
    <section class="webui-node-group">
      <div class="webui-node-head">
        <h3>${escapeHTML(group.nodeID)}</h3>
        <span class="pill">${group.entries.length} WebUIs</span>
      </div>
      <div class="webui-cards">
        ${group.entries.map(renderWebUICard).join("")}
      </div>
    </section>
  `;
}

function renderWebUICard(entry: WebUIEntry): string {
  const status = webUIOpenStatus(entry);
  return `
    <article class="webui-card">
      <div class="webui-card-head">
        <div>
          <strong>${escapeHTML(entry.name)}</strong>
          <div class="webui-url">${escapeHTML(entry.url)}</div>
        </div>
        <label class="toggle-row">
          <input type="checkbox" data-operation-group="webui" data-webui-toggle="${escapeAttribute(entry.id)}" ${entry.enabled ? "checked" : ""}>
          <span>Enable</span>
        </label>
      </div>
      <div class="node-meta">
        ${chip(entry.backend, "cyan")}
        ${chip(entry.backend_mode, "violet")}
        ${chip(entry.lane, kindColor(entry.lane))}
        ${chip(entry.active ? "active" : "idle", entry.active ? "lime" : "amber")}
      </div>
      <div class="webui-model-summary">${escapeHTML(webUIModelSummary(entry))}</div>
      <div class="webui-actions">
        <button type="button" data-webui-open="${escapeAttribute(entry.id)}">Open</button>
        <button type="button" data-webui-details="${escapeAttribute(entry.id)}">${status.openable ? "Models" : "Resolve"}</button>
      </div>
    </article>
  `;
}

function showWebUIDialog(entry: WebUIEntry): void {
  const data = webUIDialogData(entry);
  elements.webuiDialogBody.innerHTML = `
    <div class="field-dialog-head">
      <div>
        <h2>${escapeHTML(data.title)}</h2>
        <p class="muted">${escapeHTML(data.message)}</p>
      </div>
      <button type="button" data-webui-dialog-close>Close</button>
    </div>
    <div class="webui-url">${escapeHTML(entry.url)}</div>
    <div class="webui-dialog-actions">
      ${data.canEnable ? `<button type="button" data-operation-group="webui" data-webui-enable="${escapeAttribute(entry.id)}">Enable</button>` : ""}
    </div>
    <div class="webui-model-list">
      ${data.canLoad ? data.models.map(model => renderWebUIModelRow(entry, model)).join("") : `<div class="detail-empty">No compatible models</div>`}
    </div>
  `;
  elements.webuiDialog.showModal();
}

function renderWebUIModelRow(entry: WebUIEntry, model: WebUICompatibleModel): string {
  return `
    <div class="webui-model-row">
      <div>
        <strong>${escapeHTML(model.id)}</strong>
        <div class="muted">${escapeHTML(model.filename)}</div>
      </div>
      <div class="node-meta">
        ${chip(model.node_id, "cyan")}
        ${chip(model.active ? "active" : "available", model.active ? "lime" : "amber")}
      </div>
      <button type="button" data-operation-group="webui" data-webui-load="${escapeAttribute(entry.id)}" data-webui-load-model="${escapeAttribute(model.model_id)}" data-webui-load-image="${escapeAttribute(model.image_id || "")}">Load</button>
    </div>
  `;
}

function webUIStatusText(count: number): string {
  if (state.webuis.error) {
    return state.webuis.error;
  }
  if (state.webuis.loading) {
    return "Loading...";
  }
  return state.webuis.action || `${count} WebUIs`;
}

function webUIModelSummary(entry: WebUIEntry): string {
  const active = entry.active_image_id || entry.active_model_id;
  if (active) {
    return `Active: ${active}`;
  }
  return `${entry.compatible_models.length} compatible`;
}

function findWebUIEntry(id: string): WebUIEntry | undefined {
  return (state.webuis.data?.data ?? []).find(entry => entry.id === id);
}

function openExternalURL(url: string): void {
  const opened = window.open(url, "_blank", "noopener,noreferrer");
  if (opened) {
    opened.opener = null;
  }
}
