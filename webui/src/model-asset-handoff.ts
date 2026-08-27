import type { Model } from "./types";

export interface ModelAssetHandoff {
  nodeID: string;
  publicID: string;
  configID: string;
  configFilename: string;
  field: string;
  position?: number;
  filename: string;
  hash: string;
}

const handoffStates = new Set(["unresolved", "failed"]);

const hashPattern = /^[0-9a-f]{64}$/;

export function modelAssetHandoff(model: Model | undefined): ModelAssetHandoff | null {
  if (!model || !handoffStates.has(model.asset_state ?? "")) {
    return null;
  }
  const options = model.options || {};
  for (const [key, value] of Object.entries(options)) {
    if (!key.endsWith("_hash")) {
      continue;
    }
    const field = key.slice(0, -5);
    const filenameValue = options[`${field}_filename`];
    const hashes = Array.isArray(value) ? value : [value];
    const filenames = Array.isArray(filenameValue) ? filenameValue : [filenameValue];
    const position = hashes.findIndex(hash => typeof hash === "string" && hashPattern.test(hash));
    const hash = hashes[position];
    const filename = filenames[position];
    if (position < 0 || typeof hash !== "string" || typeof filename !== "string" || filename.length === 0) {
      continue;
    }
    return {
      nodeID: model.node_id || "",
      publicID: model.public_id || model.local_id,
      configID: model.local_id,
      configFilename: model.filename,
      field,
      ...(Array.isArray(value) ? {position} : {}),
      filename,
      hash
    };
  }
  return null;
}
