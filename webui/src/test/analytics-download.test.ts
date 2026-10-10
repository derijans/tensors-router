import { readFileSync } from "node:fs";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { databaseDownloadNodeIDs, databaseDownloadPath, downloadAnalyticsDatabases } from "../analytics-download";
import { state } from "../state";
import type { InventoryResponse } from "../types";

const page = readFileSync(new URL("../../index.html", import.meta.url), "utf8");

describe("analytics database download", () => {
  it("is offered as a button beside Flush to disk", () => {
    expect(page).toMatch(/<button id="analyticsDownloadButton"[^>]*data-operation-group="analytics"[^>]*>Download DB<\/button>/);
  });

  it("downloads only the node chosen in the node filter", () => {
    expect(databaseDownloadNodeIDs("slave-a", ["master", "slave-a", "slave-b"])).toEqual(["slave-a"]);
  });

  it("downloads every live node when All nodes is selected", () => {
    expect(databaseDownloadNodeIDs("", ["master", "slave-a"])).toEqual(["master", "slave-a"]);
  });

  it("asks the router for a node by id and encodes it", () => {
    expect(databaseDownloadPath("slave a/1")).toBe("/api/store/download?node_id=slave+a%2F1");
  });
});

describe("starting the browser downloads", () => {
  const clicked: string[] = [];

  beforeEach(() => {
    clicked.length = 0;
    vi.useFakeTimers();
    vi.stubGlobal("document", {
      body: {append: (link: {href: string}): void => { clicked.push(link.href); }},
      createElement: (): {href: string; download: string; click: () => void; remove: () => void} => ({href: "", download: "", click: () => undefined, remove: () => undefined})
    });
    state.inventory = {nodes: [{node_id: "master"}, {node_id: "slave-a"}]} as unknown as InventoryResponse;
  });

  it("starts one download per live node when no node is selected", async () => {
    delete state.analytics.query.node_id;

    const finished = downloadAnalyticsDatabases();
    await vi.runAllTimersAsync();
    await finished;

    expect(clicked).toEqual(["/api/store/download?node_id=master", "/api/store/download?node_id=slave-a"]);
  });

  it("starts a single download for the selected node", async () => {
    state.analytics.query.node_id = "slave-a";

    await downloadAnalyticsDatabases();

    expect(clicked).toEqual(["/api/store/download?node_id=slave-a"]);
  });
});
