import { describe, expect, it } from "vitest";
import { cookResultHTML } from "../cook-result";

describe("cook result summary", () => {
  it("renders plan facts and keeps raw data behind diagnostic disclosure", () => {
    const html = cookResultHTML({
      plan: {
        id: "mix",
        public_id: "public-mix",
        requires_master_recipe: true,
        configs: [{node_id: "node-a", model_id: "local", filename: "mix.kcpps", kinds: ["text"], reused: false}]
      },
      validation: []
    });
    expect(html).toContain("Cook plan");
    expect(html).toContain("public-mix");
    expect(html).toContain("node-a / mix.kcpps / text");
    expect(html).toContain("Raw diagnostic");
    expect(html).not.toContain("<script>");
  });
});
