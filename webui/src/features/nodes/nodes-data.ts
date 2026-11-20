import type { BackendInitializationJob, NodeStateBackend, Tone } from "../../types";
import { lookupOr } from "../../lib/collections";

type OptionalBackendField = "initialization_phase" | "error" | "selected_profile" | "detected_profile";

export function applyInitializationJob(backend: NodeStateBackend, job: BackendInitializationJob): void {
  backend.lifecycle_state = lifecycleStateForJob(backend, job);
  backend.initialization_job_id = job.job_id;
  backend.initialization_bytes = job.completed_bytes;
  backend.initialization_total_bytes = job.total_bytes;
  backend.retryable = Boolean(job.retryable);
  assignOptionalBackendValue(backend, "initialization_phase", job.phase);
  assignOptionalBackendValue(backend, "error", job.error);
  assignOptionalBackendValue(backend, "selected_profile", job.selected_profile);
  assignOptionalBackendValue(backend, "detected_profile", job.detected_profile);
}

export function lifecycleStateForJob(backend: NodeStateBackend, job: BackendInitializationJob): NodeStateBackend["lifecycle_state"] {
  if (job.state === "completed") {
    return "ready";
  }
  if (job.state === "failed") {
    return "failed";
  }
  if (job.state === "cancelled") {
    return backend.runtime_version ? "ready" : "needs_init";
  }
  return "initializing";
}

function assignOptionalBackendValue(backend: NodeStateBackend, key: OptionalBackendField, value: string | undefined): void {
  if (value) {
    backend[key] = value;
    return;
  }
  delete backend[key];
}

export function lifecycleReason(lifecycleState: string): string {
  if (lifecycleState === "companion_missing") {
    return "vLLM companion is missing.";
  }
  if (lifecycleState === "unsupported") {
    return "vLLM is unsupported on this platform.";
  }
  return "";
}

const lifecycleTones: Record<string, Tone> = {
  ready: "success",
  failed: "danger",
  unsupported: "danger",
  companion_missing: "danger",
  initializing: "info"
};

export function lifecycleTone(lifecycleState: string): Tone {
  return lookupOr(lifecycleTones, lifecycleState, "warning");
}
