import { SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import type { NodeHeldRequest, NodeState, NodeStateBackend, NodeStateModelRow } from "../../types";
import { badge } from "../../ui/badge";
import { renderNodeRequests } from "./node-requests-view";
import { formatElapsedSeconds } from "../../lib/format-time";
import { laneAccent } from "../../domain/lanes";
import { nodeBackendActionKey, nodeUnloadKey } from "./node-action-keys-data";
import { mutedText } from "../../ui/text";
import { lifecycleTone } from "./nodes-data";
import { renderBackendLifecycle, renderLaunchOptions, renderRuntimeIdentity } from "./node-backend-lifecycle-view";
import { button } from "../../ui/buttons";

const backendOrder = ["koboldcpp", "llama-server", "vllm", "sd-server", "whisper-server"];

export function renderNodeStateSnapshot(nodeID: string, snapshot: NodeState, pendingUnload: string, pendingBackendAction = ""): SafeHTML {
  const backends = [...(snapshot.backends || [])].sort((left, right) => backendRank(left.id) - backendRank(right.id));
  return html`
    <div class="node-state-backends">
      ${backends.length > 0 ? html`${backends.map(backend => renderBackend(nodeID, backend, pendingUnload, pendingBackendAction))}` : mutedText("No backend binaries detected.", "p", "node-state-empty")}
    </div>
    ${renderRequestSections(snapshot)}
    ${renderFFmpegAvailability(snapshot)}
  `;
}

function renderRequestSections(snapshot: NodeState): SafeHTML {
  if (snapshot.requests != null) {
    return renderNodeRequests(snapshot.requests);
  }
  return html`
    <section class="node-active-requests" aria-label="Active requests">
      <h4>Active requests</h4>
      ${renderActiveRequests(snapshot.active_requests ?? [])}
    </section>
    ${renderHeldRequests(snapshot.held_requests)}
  `;
}

function renderActiveRequests(activeRequests: string[]): SafeHTML {
  if (activeRequests.length === 0) {
    return mutedText("No active requests.", "p", "node-state-empty");
  }
  return html`<ul>${activeRequests.map(listItem)}</ul>`;
}

function listItem(value: string): SafeHTML {
  return html`<li>${value}</li>`;
}

function renderHeldRequests(heldRequests: NodeHeldRequest[] | null | undefined): SafeHTML {
  if (heldRequests == null) {
    return emptyHTML;
  }
  return html`
    <section class="node-held-requests" aria-label="Held requests">
      <h4>Held requests</h4>
      ${heldRequests.length > 0 ? html`<ul>${heldRequests.map(renderHeldRequest)}</ul>` : mutedText("No held requests.", "p", "node-state-empty")}
    </section>
  `;
}

function renderHeldRequest(request: NodeHeldRequest): SafeHTML {
  return html`
    <li class="node-held-request">
      ${badge(request.lane, laneAccent(request.lane))}
      <span>${request.model_id}</span>
      ${mutedText(formatElapsedSeconds(request.waiting_ms))}
      ${request.state === "lent" ? html`${badge("lent", "success")}${mutedText(html`→ ${request.helper_node_id || "?"}/${request.helper_model_id || "?"}`)}` : badge(request.state, request.state === "held" ? "warning" : "neutral")}
    </li>
  `;
}

function renderFFmpegAvailability(snapshot: NodeState): SafeHTML {
  if (snapshot.ffmpeg_available === undefined) {
    return emptyHTML;
  }
  if (!snapshot.ffmpeg_available) {
    return html`
    <section class="node-ffmpeg" aria-label="ffmpeg">
      <h4>ffmpeg</h4>
      ${mutedText("Not available. Video generation and non-WAV transcription will fail on this node.", "p", "node-state-empty")}
    </section>
  `;
  }
  const path = snapshot.ffmpeg_path ? html`<code>${snapshot.ffmpeg_path}</code>` : "Available";
  return html`
    <section class="node-ffmpeg" aria-label="ffmpeg">
      <h4>ffmpeg</h4>
      ${mutedText(path, "p")}
    </section>
  `;
}

function renderBackend(nodeID: string, backend: NodeStateBackend, pendingUnload: string, pendingBackendAction: string): SafeHTML {
  const initializationPending = pendingBackendAction === nodeBackendActionKey("init", backend.id);
  const cancellationPending = pendingBackendAction === nodeBackendActionKey("cancel", backend.id);
  const lifecycleState = initializationPending ? "initializing" : backend.lifecycle_state || "ready";
  return html`
    <article class="node-state-backend">
      <div class="node-backend-heading">
        <h4>${backend.display_name}</h4>
        <div class="node-backend-chips">
          ${badge(backend.mode, "info")}
          ${badge(lifecycleState, lifecycleTone(lifecycleState))}
        </div>
      </div>
      ${lifecycleState === "ready" ? renderReadyBackend(nodeID, backend, pendingUnload) : renderBackendLifecycle(nodeID, backend, lifecycleState, cancellationPending)}
    </article>
  `;
}

function renderReadyBackend(nodeID: string, backend: NodeStateBackend, pendingUnload: string): SafeHTML {
  return html`
    ${renderRuntimeIdentity(backend)}
    ${backend.loaded_models.length > 0 ? html`<div class="node-loaded-models">${backend.loaded_models.map(model => renderLoadedModel(nodeID, backend.id, model, pendingUnload))}</div>` : mutedText("No loaded models.", "p", "node-state-empty")}
    ${renderLaunchOptions(nodeID, backend)}
  `;
}

function renderLoadedModel(nodeID: string, backendID: string, model: NodeStateModelRow, pendingUnload: string): SafeHTML {
  const pending = pendingUnload === nodeUnloadKey(backendID, model.runtime_id);
  return html`
    <div class="node-loaded-model">
      <div>
        <strong>${model.model_id}</strong>
        ${mutedText(html`${model.lane} / ${model.runtime_id}`, "div")}
      </div>
      ${button(pending ? "Unloading..." : "Unload", {"data-node-unload": true, "data-node-id": nodeID, "data-backend-id": backendID, "data-runtime-id": model.runtime_id, "data-generation": model.generation, disabled: pending})}
    </div>
  `;
}

function backendRank(backendID: string): number {
  const rank = backendOrder.indexOf(backendID);
  return rank === -1 ? backendOrder.length : rank;
}
