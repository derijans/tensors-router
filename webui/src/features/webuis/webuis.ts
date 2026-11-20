import { SafeHTML, html, setHTML } from "../../lib/safe-html";
import { getWebUIs, loadWebUI, setWebUISession } from "../../api/webuis-api";
import { state } from "../../state/app-state";
import { badge, badgeRow, statusDot } from "../../ui/badge";
import {
  filteredWebUIEntries,
  groupWebUIs,
  webUIDialogData,
  webUIOpenStatus,
  type WebUIGroup
} from "./webui-data";
import type { WebUICompatibleModel, WebUIEntry } from "../../types";
import { errorMessage } from "../../lib/errors";
import { pluralSuffix } from "../../lib/text";
import { laneAccent } from "../../domain/lanes";
import { emptyCard, emptyState } from "../../ui/empty-state";
import { mutedText } from "../../ui/text";
import { button, buttonStrip, closeButton, primaryButton } from "../../ui/buttons";
import { labelledToggle } from "../../ui/form";
import { webuisElements } from "./webuis-elements";
import { showStatus } from "../../ui/status-line";

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
    state.webuis.error = errorMessage(error);
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
    state.webuis.error = errorMessage(error);
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
  webuisElements.webuiDialog.close();
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
    state.webuis.error = errorMessage(error);
    throw error;
  } finally {
    renderWebUIs();
  }
}

export function renderWebUIs(): void {
  const entries = filteredWebUIEntries(state.webuis.data?.data ?? [], state.webuis.filter);
  showStatus(webuisElements.webuiStatus, webUIStatusText(entries.length), state.webuis.error !== "");
  setHTML(webuisElements.webuiGrid, entries.length ? html`${groupWebUIs(entries).map(renderWebUIGroup)}` : emptyCard("No WebUIs match."));
}

function renderWebUIGroup(group: WebUIGroup): SafeHTML {
  return html`
    <section class="webui-node-group">
      <header class="section-head">
        <h3>${group.nodeID}</h3>
        ${badge(`${group.entries.length} WebUI${pluralSuffix(group.entries.length)}`, "neutral")}
      </header>
      <div class="webui-cards">
        ${group.entries.map(renderWebUICard)}
      </div>
    </section>
  `;
}

function renderWebUICard(entry: WebUIEntry): SafeHTML {
  const status = webUIOpenStatus(entry);
  return html`
    <article class="card webui-card">
      <header class="webui-card-head">
        <span class="node-icon ${laneAccent(entry.lane)}"><svg class="icon"><use href="#icon-webuis"/></svg></span>
        <div class="webui-card-title">
          <h3>${entry.name}</h3>
          <p class="webui-url">${entry.url}</p>
        </div>
        ${labelledToggle({"aria-label": `Enable ${entry.name}`, "data-operation-group": "webui", "data-webui-toggle": entry.id}, entry.enabled, "Enabled")}
      </header>
      ${badgeRow([
        badge(entry.lane, laneAccent(entry.lane)),
        badge(entry.backend, "info"),
        badge(entry.backend_mode, "neutral"),
        entry.active ? statusDot("Active", "success") : statusDot("Idle", "neutral")
      ])}
      ${mutedText(webUIModelSummary(entry), "p", "webui-model-summary")}
      ${buttonStrip([
        primaryButton("Open", {"data-webui-open": entry.id}),
        button(status.openable ? "Models" : "Resolve", {"data-webui-details": entry.id})
      ])}
    </article>
  `;
}

function showWebUIDialog(entry: WebUIEntry): void {
  const data = webUIDialogData(entry);
  setHTML(webuisElements.webuiDialogBody, html`
    <header class="dialog-head">
      <div>
        <h2>${data.title}</h2>
        ${mutedText(data.message, "p")}
      </div>
      ${closeButton("Close", {"data-webui-dialog-close": true})}
    </header>
    <p class="webui-url">${entry.url}</p>
    ${data.canEnable ? buttonStrip([primaryButton("Enable", {"data-operation-group": "webui", "data-webui-enable": entry.id})]) : ""}
    <div class="webui-model-list">
      ${data.canLoad ? html`${data.models.map(model => renderWebUIModelRow(entry, model))}` : emptyState("No compatible models")}
    </div>
  `);
  webuisElements.webuiDialog.showModal();
}

function renderWebUIModelRow(entry: WebUIEntry, model: WebUICompatibleModel): SafeHTML {
  return html`
    <div class="webui-model-row">
      <div>
        <strong>${model.id}</strong>
        ${mutedText(model.filename, "div")}
      </div>
      ${badgeRow([
        badge(model.node_id, "info"),
        badge(model.active ? "active" : "available", model.active ? "success" : "neutral")
      ])}
      <button type="button" data-operation-group="webui" data-webui-load="${entry.id}" data-webui-load-model="${model.model_id}" data-webui-load-image="${model.image_id || ""}">Load</button>
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
