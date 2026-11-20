import { type SafeHTML, html } from "../../lib/safe-html";
import type { NodeRuntimeSlice } from "../../types";
import { button } from "../../ui/buttons";
import { errorAlert, mutedText } from "../../ui/text";
import { nodeStatePanelID } from "./node-card-view";
import { renderNodeStateSnapshot } from "./node-state-view";

export function nodeStatePanelMarkup(nodeID: string, slice: NodeRuntimeSlice): SafeHTML {
  return html`
    <section id="${nodeStatePanelID(nodeID)}" class="node-state-panel" aria-label="Runtime state for ${nodeID}">
      <div class="node-state-header">
        <h3>Runtime state - ${nodeID}</h3>
        ${button("Close", {"data-node-close": true, "data-node-id": nodeID})}
      </div>
      ${slice.loading && !slice.snapshot ? mutedText("Loading runtime state...", "p", "node-state-message") : ""}
      ${slice.error ? errorAlert(slice.error, "node-state-message") : ""}
      ${slice.snapshot ? renderNodeStateSnapshot(nodeID, slice.snapshot, slice.pendingUnload, slice.pendingBackendAction) : ""}
    </section>
  `;
}
