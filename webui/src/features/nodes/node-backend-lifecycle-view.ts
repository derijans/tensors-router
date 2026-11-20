import { type SafeHTML, emptyHTML, html } from "../../lib/safe-html";
import type { BackendLaunchOptions, NodeStateBackend } from "../../types";
import { badgeButton } from "../../ui/badge";
import { formatBytes } from "../../lib/format-bytes";
import { errorText, mutedText } from "../../ui/text";
import { indeterminateProgress, progressBar } from "../../ui/progress";
import { lifecycleReason } from "./nodes-data";
import { button } from "../../ui/buttons";
import { checkbox } from "../../ui/form";

export function renderBackendLifecycle(nodeID: string, backend: NodeStateBackend, lifecycleState: string, cancellationPending: boolean): SafeHTML {
  if (lifecycleState === "initializing") {
    return html`
      ${renderRuntimeIdentity(backend)}
      <div class="node-backend-lifecycle">
        <strong>${backend.initialization_phase || "Initializing"}</strong>
        ${renderInitializationProgress(backend)}
        <div class="node-backend-actions">
          ${badgeButton("backend needs init", "warning", {disabled: true}, "node-backend-init-action")}
          ${button(cancellationPending ? "Cancelling..." : "Cancel", {"data-node-backend-init-cancel": true, "data-node-id": nodeID, "data-backend-id": backend.id, disabled: cancellationPending})}
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
  return badgeButton("backend needs init", "warning", {
    "data-node-backend-init": true,
    "data-node-id": nodeID,
    "data-backend-id": backend.id,
    "data-profile": backend.selected_profile || undefined
  }, "node-backend-init-action");
}

const launchOptionFields: {key: keyof BackendLaunchOptions; label: string}[] = [
  {key: "hf_hub_offline", label: "HF_HUB_OFFLINE"},
  {key: "transformers_offline", label: "TRANSFORMERS_OFFLINE"},
  {key: "hf_datasets_offline", label: "HF_DATASETS_OFFLINE"}
];

// Launch options only exist for the vLLM companion, and only matter once it can
// actually start a runtime. Applying them unloads any loaded runtime, so the control is
// explicit rather than auto-saving on every toggle.
export function renderLaunchOptions(nodeID: string, backend: NodeStateBackend): SafeHTML {
  if (backend.mode !== "vllm" || !backend.launch_options) {
    return emptyHTML;
  }
  const options = backend.launch_options;
  const checkboxes = html`${launchOptionFields.map(field => {
    const launchOption = checkbox({"data-node-backend-launch-option": field.key, "data-node-id": nodeID, "data-backend-id": backend.id}, Boolean(options[field.key]));
    return html`<label class="node-backend-launch-option">${launchOption} ${field.label}</label>`;
  })}`;
  return html`
    <div class="node-backend-launch-options">
      ${mutedText("Launch environment", "p")}
      ${checkboxes}
      ${badgeButton("Apply and reload", "accent", {"data-node-backend-launch-apply": true, "data-node-id": nodeID, "data-backend-id": backend.id})}
    </div>
  `;
}

export function renderRuntimeIdentity(backend: NodeStateBackend): SafeHTML {
  const rows = [
    backend.runtime_version ? `Version: ${backend.runtime_version}` : "",
    backend.selected_profile ? `Profile: ${backend.selected_profile}` : "",
    backend.detected_profile && backend.detected_profile !== backend.selected_profile ? `Detected: ${backend.detected_profile}` : "",
    backend.manifest_trust && backend.manifest_trust !== "tuf" && backend.manifest_trust !== "unverified" ? `Manifest trust: ${backend.manifest_trust}` : ""
  ].filter(Boolean);
  const identity = rows.length > 0 ? mutedText(rows.map(identitySpan), "div", "node-backend-runtime") : "";
  // Unlike every other trust tier, "unverified" pins nothing at all - it is called
  // out on its own line, not folded into the muted identity row, so it cannot be
  // mistaken for routine metadata.
  const unverifiedWarning = backend.manifest_trust === "unverified" ? errorText("Unverified install: no manifest, no digest pinning - installed straight from PyPI", "p", "node-backend-unverified") : "";
  return html`${identity}${unverifiedWarning}`;
}

function identitySpan(value: string): SafeHTML {
  return html`<span>${value}</span>`;
}

const initializationProgressAttributes = {class: "node-backend-progress", "aria-label": "Initialization progress"};

function renderInitializationProgress(
backend: NodeStateBackend): SafeHTML {
  const completedBytes = positiveBytes(backend.initialization_bytes);
  const totalBytes = positiveBytes(backend.initialization_total_bytes);
  if (totalBytes === 0) {
    const label = completedBytes > 0 ? `${formatBytes(completedBytes)} completed` : "Waiting for progress";
    return html`${indeterminateProgress(initializationProgressAttributes)}${mutedText(label)}`;
  }
  const percent = Math.floor((Math.min(completedBytes, totalBytes) / totalBytes) * 100);
  const label = `${formatBytes(completedBytes)} / ${formatBytes(totalBytes)} (${percent}%)`;
  return html`${progressBar(completedBytes, totalBytes, initializationProgressAttributes)}${mutedText(label)}`;
}

function positiveBytes(value: number | undefined): number {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? Math.floor(value) : 0;
}
