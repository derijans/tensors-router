import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const page = readFileSync(new URL("../../index.html", import.meta.url), "utf8");

function tagNameForID(id: string): string | undefined {
  const element = page.match(new RegExp(`<([a-z][a-z0-9-]*)\\b[^>]*\\bid=["']${id}["'][^>]*>`, "i"));
  return element?.[1]?.toLowerCase();
}

describe("models dashboard DOM contract", () => {
  it.each(["modelsNodeFilter", "filesNodeFilter"])("renders #%s as a select", id => {
    expect(tagNameForID(id)).toBe("select");
  });
});
