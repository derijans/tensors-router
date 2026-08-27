import { describe, expect, it } from "vitest";
import { modelAssetHandoff } from "../model-asset-handoff";
import { testModel } from "./factories";

const hash = "0a2fac16f388b4839f075dedb681357aec3e73a96bd66b413e462b6853550c99";

function unresolvedModel(): ReturnType<typeof testModel> {
  const model = testModel("Krea2Turbo", {sdmodel_hash: hash, sdmodel_filename: "diffusion.gguf"});
  model.asset_state = "unresolved";
  return model;
}

describe("model asset handoff", () => {
  it("offers the missing asset of an unresolved config", () => {
    expect(modelAssetHandoff(unresolvedModel())).toEqual({
      nodeID: "node-a",
      publicID: "Krea2Turbo",
      configID: "Krea2Turbo",
      configFilename: "Krea2Turbo.kcpps",
      field: "sdmodel",
      filename: "diffusion.gguf",
      hash
    });
  });

  it("carries the array position of a multi-value field", () => {
    const model = testModel("loras", {sdlora_hash: [hash], sdlora_filename: ["style.safetensors"]});
    model.asset_state = "failed";
    expect(modelAssetHandoff(model)?.position).toBe(0);
  });

  // A resolved config that fails to load failed in the backend, not in asset
  // resolution: sending the user to the downloader offers a file already on disk.
  it("stays quiet when the assets are ready", () => {
    const model = unresolvedModel();
    model.asset_state = "ready";
    expect(modelAssetHandoff(model)).toBeNull();
  });

  it("stays quiet while resolution is still running", () => {
    const model = unresolvedModel();
    model.asset_state = "resolving";
    expect(modelAssetHandoff(model)).toBeNull();
  });

  it("stays quiet for a config that carries no portable field", () => {
    const model = testModel("plain", {model_param: "/models/plain.gguf"});
    model.asset_state = "unresolved";
    expect(modelAssetHandoff(model)).toBeNull();
  });

  it("stays quiet when the model is unknown", () => {
    expect(modelAssetHandoff(undefined)).toBeNull();
  });
});
