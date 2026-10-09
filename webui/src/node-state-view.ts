import { SafeHTML, emptyHTML, html } from "./safe-html";
import type { BackendLaunchOptions, NodeHeldRequest, NodeState, NodeStateBackend, NodeStateModelRow, Tone } from "./types";
import { formatBytes } from "./utils";
import { badge, laneAccent } from "./markup-primitives";

const backendOrder = ["koboldcpp", "llama-server", "vllm", "sd-server", "whisper-server"];

export function renderNodeStateSnapshot(nodeID: string, snapshot: NodeState, pendingUnload: string, pendingBackendAction = ""): SafeHTML {
  const backends = [...(snapshot.backends || [])].sort((left, right) => backendRank(left.id) - backendRank(right.id));
  return html`
    <div class="node-state-backends">
      ${backends.length > 0 ? html`${backends.map(backend => renderBackend(nodeID, backend, pendingUnload, pendingBackendAction))}` : html`<p class="muted node-state-empty">No backend binaries detected.</p>`}
    </div>
    <section class="node-active-requests" aria-label="Active requests">
      <h4>Active requests</h4>
      ${renderActiveRequests(snapshot.active_requests ?? [])}
    </section>
    ${renderHeldRequests(snapshot.held_requests)}
    ${renderFFmpegAvailability(snapshot)}
  `;
}

function renderActiveRequests(activeRequests: string[]): SafeHTML {
  if (activeRequests.length === 0) {
    return html`<p class="muted node-state-empty">No active requests.</p>`;
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
      ${heldRequests.length > 0 ? html`<ul>${heldRequests.map(renderHeldRequest)}</ul>` : html`<p class="muted node-state-empty">No held requests.</p>`}
    </section>
  `;
}

function renderHeldRequest(request: NodeHeldRequest): SafeHTML {
  return html`
    <li class="node-held-request">
      ${badge(request.lane, laneAccent(request.lane))}
      <span>${request.model_id}</span>
      <span class="muted">${formatWaiting(request.waiting_ms)}</span>
      ${request.state === "lent" ? html`${badge("lent", "success")}<span class="muted">→ ${request.helper_node_id || "?"}/${request.helper_model_id || "?"}</span>` : badge("held", "warning")}
    </li>
  `;
}

function formatWaiting(waitingMilliseconds: number): string {
  return `${(Math.max(0, waitingMilliseconds) / 1000).toFixed(1)}s`;
}

function renderFFmpegAvailability(snapshot: NodeState): SafeHTML {
  if (snapshot.ffmpeg_available === undefined) {
    return emptyHTML;
  }
  if (!snapshot.ffmpeg_available) {
    return html`
    <section class="node-ffmpeg" aria-label="ffmpeg">
      <h4>ffmpeg</h4>
      <p class="muted node-state-empty">Not available. Video generation and non-WAV transcription will fail on this node.</p>
    </section>
  `;
  }
  const path = snapshot.ffmpeg_path ? html`<code>${snapshot.ffmpeg_path}</code>` : "Available";
  return html`
    <section class="node-ffmpeg" aria-label="ffmpeg">
      <h4>ffmpeg</h4>
      <p class="muted">${path}</p>
    </section>
  `;
}

function renderBackend(nodeID: string, backend: NodeStateBackend, pendingUnload: string, pendingBackendAction: string): SafeHTML {
  const initializationPending = pendingBackendAction === backendActionKey("init", backend.id);
  const cancellationPending = pendingBackendAction === backendActionKey("cancel", backend.id);
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
    ${backend.loaded_models.length > 0 ? html`<div class="node-loaded-models">${backend.loaded_models.map(model => renderLoadedModel(nodeID, backend.id, model, pendingUnload))}</div>` : html`<p class="muted node-state-empty">No loaded models.</p>`}
    ${renderLaunchOptions(nodeID, backend)}
  `;
}

function renderBackendLifecycle(nodeID: string, backend: NodeStateBackend, lifecycleState: string, cancellationPending: boolean): SafeHTML {
  if (lifecycleState === "initializing") {
    return html`
      ${renderRuntimeIdentity(backend)}
      <div class="node-backend-lifecycle">
        <strong>${backend.initialization_phase || "Initializing"}</strong>
        ${renderInitializationProgress(backend)}
        <div class="node-backend-actions">
          <button class="badge tone-warning node-backend-init-action" type="button" disabled>backend needs init</button>
          <button type="button" data-node-backend-init-cancel data-node-id="${nodeID}" data-backend-id="${backend.id}"${cancellationPending ? " disabled" : ""}>${cancellationPending ? "Cancelling..." : "Cancel"}</button>
        </div>
      </div>
    `;
  }
  const reason = backend.error || lifecycleReason(lifecycleState);
  const initializationAction = lifecycleState === "needs_init" || (lifecycleState === "failed" && backend.retryable);
  return html`
    ${renderRuntimeIdentity(backend)}
    <div class="node-backend-lifecycle">
      ${lifecycleMessage(reason, lifecycleState)}
      ${initializationAction ? initializationButton(nodeID, backend) : emptyHTML}
    </div>
    ${renderLaunchOptions(nodeID, backend)}
  `;
}

function lifecycleMessage(reason: string, lifecycleState: string): SafeHTML {
  if (!reason) {
    return emptyHTML;
  }
  const tone = lifecycleState === "failed" ? "error-text" : "muted";
  return html`<p class="${tone} node-state-message">${reason}</p>`;
}

function initializationButton(nodeID: string, backend: NodeStateBackend): SafeHTML {
  return html`<button class="badge tone-warning node-backend-init-action" type="button" data-node-backend-init data-node-id="${nodeID}" data-backend-id="${backend.id}"${profileAttribute(backend.selected_profile)}>backend needs init</button>`;
}

function profileAttribute(profile: string | undefined): SafeHTML {
  return profile ? html` data-profile="${profile}"` : emptyHTML;
}

const launchOptionFields: {key: keyof BackendLaunchOptions; label: string}[] = [
  {key: "hf_hub_offline", label: "HF_HUB_OFFLINE"},
  {key: "transformers_offline", label: "TRANSFORMERS_OFFLINE"},
  {key: "hf_datasets_offline", label: "HF_DATASETS_OFFLINE"}
];

// Launch options only exist for the vLLM companion, and only matter once it can
// actually start a runtime. Applying them unloads any loaded runtime, so the control is
// explicit rather than auto-saving on every toggle.
function renderLaunchOptions(nodeID: string, backend: NodeStateBackend): SafeHTML {
  if (backend.mode !== "vllm" || !backend.launch_options) {
    return emptyHTML;
  }
  const options = backend.launch_options;
  const checkboxes = html`${launchOptionFields.map(field => {
    const checked = options[field.key] ? " checked" : "";
    return html`<label class="node-backend-launch-option"><input type="checkbox" data-node-backend-launch-option="${field.key}" data-node-id="${nodeID}" data-backend-id="${backend.id}"${checked}> ${field.label}</label>`;
  })}`;
  return html`
    <div class="node-backend-launch-options">
      <p class="muted">Launch environment</p>
      ${checkboxes}
      <button type="button" class="badge tone-accent" data-node-backend-launch-apply data-node-id="${nodeID}" data-backend-id="${backend.id}">Apply and reload</button>
    </div>
  `;
}

function renderRuntimeIdentity(backend: NodeStateBackend): SafeHTML {
  const rows = [
    backend.runtime_version ? `Version: ${backend.runtime_version}` : "",
    backend.selected_profile ? `Profile: ${backend.selected_profile}` : "",
    backend.detected_profile && backend.detected_profile !== backend.selected_profile ? `Detected: ${backend.detected_profile}` : "",
    backend.manifest_trust && backend.manifest_trust !== "tuf" && backend.manifest_trust !== "unverified" ? `Manifest trust: ${backend.manifest_trust}` : ""
  ].filter(Boolean);
  const identity = rows.length > 0 ? html`<div class="muted node-backend-runtime">${rows.map(identitySpan)}</div>` : "";
  // Unlike every other trust tier, "unverified" pins nothing at all - it is called
  // out on its own line, not folded into the muted identity row, so it cannot be
  // mistaken for routine metadata.
  const unverifiedWarning = backend.manifest_trust === "unverified" ? html`<p class="error-text node-backend-unverified">Unverified install: no manifest, no digest pinning - installed straight from PyPI</p>` : "";
  return html`${identity}${unverifiedWarning}`;
}

function identitySpan(value: string): SafeHTML {
  return html`<span>${value}</span>`;
}

function renderInitializationProgress(
backend: NodeStateBackend): SafeHTML {
  const completedBytes = positiveBytes(backend.initialization_bytes);
  const totalBytes = positiveBytes(backend.initialization_total_bytes);
  if (totalBytes === 0) {
    const label = completedBytes > 0 ? `${formatBytes(completedBytes)} completed` : "Waiting for progress";
    return html`<progress class="node-backend-progress" aria-label="Initialization progress"></progress><span class="muted">${label}</span>`;
  }
  const boundedCompleted = Math.min(completedBytes, totalBytes);
  const percent = Math.floor((boundedCompleted / totalBytes) * 100);
  const label = `${formatBytes(completedBytes)} / ${formatBytes(totalBytes)} (${percent}%)`;
  return html`<progress class="node-backend-progress" aria-label="Initialization progress" value="${boundedCompleted}" max="${totalBytes}"></progress><span class="muted">${label}</span>`;
}

function positiveBytes(value: number | undefined): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}

function lifecycleReason(lifecycleState: string): string {
  if (lifecycleState === "companion_missing") {
    return "vLLM companion is missing.";
  }
  if (lifecycleState === "unsupported") {
    return "vLLM is unsupported on this platform.";
  }
  return "";
}

function lifecycleTone(lifecycleState: string): Tone {
  if (lifecycleState === "ready") {
    return "success";
  }
  if (lifecycleState === "failed" || lifecycleState === "unsupported" || lifecycleState === "companion_missing") {
    return "danger";
  }
  return lifecycleState === "initializing" ? "info" : "warning";
}

function backendActionKey(action: string, backendID: string): string {
  return `${action} ${backendID}`;
}

function renderLoadedModel(nodeID: string, backendID: string, model: NodeStateModelRow, pendingUnload: string): SafeHTML {
  const pending = pendingUnload === `${backendID} ${model.runtime_id}`;
  return html`
    <div class="node-loaded-model">
      <div>
        <strong>${model.model_id}</strong>
        <div class="muted">${model.lane} / ${model.runtime_id}</div>
      </div>
      <button type="button" data-node-unload data-node-id="${nodeID}" data-backend-id="${backendID}" data-runtime-id="${model.runtime_id}" data-generation="${model.generation}"${pending ? " disabled" : ""}>${pending ? "Unloading..." : "Unload"}</button>
    </div>
  `;
}

function backendRank(backendID: string): number {
  const rank = backendOrder.indexOf(backendID);
  return rank === -1 ? backendOrder.length : rank;
}
