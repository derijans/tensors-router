export type BackendMode = "kobold" | "llama_sdcpp" | "vllm";

export const backendModeKey = "backend_mode";

export const backendModes: BackendMode[] = ["kobold", "llama_sdcpp", "vllm"];

export const backendModeLabels: Record<BackendMode, string> = {
  kobold: "Kobold",
  llama_sdcpp: "llama/sd.cpp",
  vllm: "vLLM"
};

export const defaultBackendMode: BackendMode = "kobold";

export function isBackendMode(value: unknown): value is BackendMode {
  return typeof value === "string" && backendModes.includes(value as BackendMode);
}

export function backendModeOrDefault(value: unknown): BackendMode {
  return isBackendMode(value) ? value : defaultBackendMode;
}
