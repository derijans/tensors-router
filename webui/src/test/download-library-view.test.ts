import { describe, expect, it } from "vitest";
import { renderDownloadLibrary } from "../download-library-view";
import { SafeHTML } from "../safe-html";

describe("download library", () => {
  it("lists files the start scan found but did not hash", () => {
    const markup = SafeHTML.render(renderDownloadLibrary({
      artifacts: [{path: "D:/models/known.gguf", sha256: "abc", size: 2048, verification_source: "sidecar"}],
      unhashed: [{path: "D:/models/loose.gguf", size: 1024}],
      jobs: []
    }));

    expect(markup).toContain("D:/models/known.gguf");
    expect(markup).toContain("Not hashed — Rescan to hash");
    expect(markup).toContain("D:/models/loose.gguf");
  });

  it("does not call a library empty while unhashed files exist", () => {
    const markup = SafeHTML.render(renderDownloadLibrary({artifacts: [], unhashed: [{path: "D:/models/loose.gguf", size: 1024}], jobs: []}));

    expect(markup).not.toContain("No indexed artifacts on this node.");
  });
});
