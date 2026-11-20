import { laneAccent, laneMetadata } from "../../../domain/lanes";
import { jinjaKwargsPrecedenceKey } from "../../../domain/option-keys";
import { optionValueText } from "../../../domain/option-values";
import { type SafeHTML, html } from "../../../lib/safe-html";
import type { JsonValue, LaneKind, NodeInventory, PaletteComponentPayload, PaletteEntry } from "../../../types";
import { badge } from "../../../ui/badge";
import { button, linkButton } from "../../../ui/buttons";
import { optionElement } from "../../../ui/select";
import { mutedText } from "../../../ui/text";
import { jinjaKwargsPrecedenceOptions } from "../jinja-precedence-options-view";

const collapsedRowLimit = 9;

export interface SelectedLaneView {
  lane: LaneKind;
  selected: PaletteComponentPayload;
  nodes: readonly NodeInventory[];
  targetNodeID: string;
  overrideCount: number;
}

export function paletteItemMarkup(entry: PaletteEntry, payloadID: string): SafeHTML {
  const addButton = entry.payload.type === "option"
    ? button("Add", {"data-add-option": entry.payload.key})
    : button("Use", {"data-select-payload": payloadID});
  return html`
    <article class="palette-item" draggable="true" data-drag-payload="${payloadID}">
      <div class="palette-title">
        <strong>${entry.title}</strong>
        ${badge(entry.badge, entry.accent)}
      </div>
      ${mutedText(entry.subtitle, "div")}
      <div class="palette-meta">${entry.meta.map(item => badge(item, "neutral"))}</div>
      ${addButton}
    </article>
  `;
}

export function laneShell(lane: LaneKind): SafeHTML {
  const metadata = laneMetadata[lane];
  return html`
    <section class="lane ${laneAccent(lane)}" data-lane="${lane}">
      <div class="lane-head">
        <div>
          <h3>${metadata.label}</h3>
          <span>${metadata.section}</span>
        </div>
        ${button("Clear", {"data-clear-lane": lane})}
      </div>
      <div class="lane-drop" data-drop-lane="${lane}"></div>
    </section>
  `;
}

export function emptyLaneMarkup(lane: LaneKind): SafeHTML {
  return html`<div class="lane-empty">${laneMetadata[lane].dropLabel}</div>`;
}

export function selectedLaneMarkup(view: SelectedLaneView): SafeHTML {
  const {lane, selected} = view;
  return html`
    <article class="selected-card">
      <strong>${selected.label}</strong>
      ${mutedText(selected.subtitle, "div")}
      <div class="palette-meta">${selected.meta.map(item => badge(item, "neutral"))}</div>
      ${selected.component.option_key ? mutedText(html`Assigned to ${selected.component.option_key}`, "div") : ""}
      <label>
        Target node
        <select data-lane-target="${lane}">${view.nodes.map(node => optionElement(node.node_id, node.node_id || "node", node.node_id === view.targetNodeID))}</select>
      </label>
      <div class="lane-card-actions">
        ${button("Edit fields", {"data-edit-lane-fields": lane})}
        ${view.overrideCount ? badge(`${view.overrideCount} overrides`, laneAccent(lane)) : ""}
      </div>
    </article>
  `;
}

export function usedModelRows(lane: LaneKind, selected: PaletteComponentPayload, paths: readonly string[]): SafeHTML[] {
  return [
    html`
      <div class="used-row">
        ${badge(laneMetadata[lane].shortLabel, laneAccent(lane))}
        <span>${selected.label}</span>
      </div>
    `,
    ...paths.map(path => mutedText(path, "div"))
  ];
}

export function inheritedOptionRow(key: string, value: JsonValue | undefined, overrideLane: LaneKind | null): SafeHTML {
  return html`
    <div class="option-row">
      ${badge(key, "neutral")}
      ${overrideLane ? badge(`${laneMetadata[overrideLane].shortLabel} override`, laneAccent(overrideLane)) : ""}
      ${mutedText(optionValueText(value))}
    </div>
  `;
}

export function optionEditorRow(key: string, value: JsonValue | undefined): SafeHTML {
  const control = key === jinjaKwargsPrecedenceKey
    ? html`<select data-option-input="${key}">${jinjaKwargsPrecedenceOptions(value === "client" ? "client" : "config")}</select>`
    : html`<input data-option-input="${key}" value="${optionValueText(value)}">`;
  return html`
    <div class="option-editor">
      <span>${key}</span>
      ${control}
      ${button("Remove", {"data-remove-option": key})}
    </div>
  `;
}

export function limitedRows(rows: readonly SafeHTML[], showAll: boolean, target: string): SafeHTML[] {
  if (rows.length <= collapsedRowLimit) {
    return [...rows];
  }
  if (showAll) {
    return [...rows, linkButton("Show less", {"data-toggle-list": target})];
  }
  return [...rows.slice(0, collapsedRowLimit), linkButton(`Show all ${rows.length}`, {"data-toggle-list": target})];
}
