import { describe, expect, it } from "vitest";
import {
  candidateKey,
  draftsFromCandidates,
  groupCandidatesByNode,
  linkCountsForModel,
  linksRequest,
  newlyLinkedDifferentWeights,
  requestHasNoLinks,
  routingButtonLabel,
  updateLinkDraft
} from "../routing-links-data";
import { routingDialogMarkup } from "../routing-links-markup";
import { SafeHTML } from "../safe-html";
import type { RoutingCandidate, RoutingLinksResponse } from "../types";

const unlinked = {selected: false, load_if_unloaded: false, restore_after_borrow: false};

function candidate(overrides: Partial<RoutingCandidate> = {}): RoutingCandidate {
  return {
    node_id: "master",
    model_id: "cc-ff",
    filename: "cc.kcpps",
    weights_match: true,
    eligible: true,
    lends_to: unlinked,
    borrows_from: unlinked,
    ...overrides
  };
}

const slaveAnchor = {node_id: "slave", model_id: "cc11-ff"};
const masterKey = candidateKey({node_id: "master", model_id: "cc-ff"});

describe("draftsFromCandidates", () => {
  it("seeds each direction from its own saved link", () => {
    const drafts = draftsFromCandidates([
      candidate({borrows_from: {selected: true, load_if_unloaded: false, restore_after_borrow: true}})
    ]);

    expect(drafts.lends_to.get(masterKey)?.selected).toBe(false);
    expect(drafts.borrows_from.get(masterKey)).toEqual({selected: true, loadIfUnloaded: false, restoreAfterBorrow: true});
  });

  it("defaults an unsaved link to allow loading, matching how lending behaved before the flag existed", () => {
    const drafts = draftsFromCandidates([candidate()]);

    expect(drafts.lends_to.get(masterKey)?.loadIfUnloaded).toBe(true);
    expect(drafts.borrows_from.get(masterKey)?.loadIfUnloaded).toBe(true);
  });
});

describe("linksRequest", () => {
  it("keeps a link one-way: ticking borrows_from does not lend the anchor's work", () => {
    const candidates = [candidate()];
    const drafts = draftsFromCandidates(candidates);
    updateLinkDraft(drafts, "borrows_from", masterKey, "selected", true);

    const request = linksRequest(slaveAnchor, candidates, drafts);

    expect(request.lends_to).toEqual([]);
    expect(request.borrows_from).toEqual([
      {node_id: "master", model_id: "cc-ff", load_if_unloaded: true, restore_after_borrow: false}
    ]);
  });

  it("carries the per-link flags the operator set", () => {
    const candidates = [candidate()];
    const drafts = draftsFromCandidates(candidates);
    updateLinkDraft(drafts, "lends_to", masterKey, "selected", true);
    updateLinkDraft(drafts, "lends_to", masterKey, "loadIfUnloaded", false);
    updateLinkDraft(drafts, "lends_to", masterKey, "restoreAfterBorrow", true);

    expect(linksRequest(slaveAnchor, candidates, drafts).lends_to).toEqual([
      {node_id: "master", model_id: "cc-ff", load_if_unloaded: false, restore_after_borrow: true}
    ]);
  });

  it("never sends an ineligible candidate, even if a stale draft has it ticked", () => {
    const candidates = [candidate({eligible: false, ineligible_reason: "serves requests concurrently"})];
    const drafts = draftsFromCandidates(candidates);
    updateLinkDraft(drafts, "lends_to", masterKey, "selected", true);

    expect(requestHasNoLinks(linksRequest(slaveAnchor, candidates, drafts))).toBe(true);
  });
});

describe("newlyLinkedDifferentWeights", () => {
  it("asks only when a mismatched candidate gains its first link", () => {
    const candidates = [
      candidate({model_id: "flux", weights_match: false}),
      candidate({model_id: "same", weights_match: true})
    ];
    const drafts = draftsFromCandidates(candidates);
    updateLinkDraft(drafts, "borrows_from", candidateKey({node_id: "master", model_id: "flux"}), "selected", true);
    updateLinkDraft(drafts, "lends_to", candidateKey({node_id: "master", model_id: "same"}), "selected", true);

    expect(newlyLinkedDifferentWeights(candidates, drafts).map(item => item.model_id)).toEqual(["flux"]);
  });

  it("does not re-ask about a mismatch already linked in some direction", () => {
    const candidates = [candidate({model_id: "flux", weights_match: false, lends_to: {selected: true, load_if_unloaded: true, restore_after_borrow: false}})];
    const drafts = draftsFromCandidates(candidates);
    updateLinkDraft(drafts, "borrows_from", candidateKey({node_id: "master", model_id: "flux"}), "selected", true);

    expect(newlyLinkedDifferentWeights(candidates, drafts)).toEqual([]);
  });
});

describe("linkCountsForModel and routingButtonLabel", () => {
  const response: RoutingLinksResponse = {
    links: [
      {owner: {node_id: "master", model_id: "cc-ff"}, helper: {node_id: "slave", model_id: "cc11-ff"}, load_if_unloaded: true, restore_after_borrow: false}
    ]
  };

  it("counts each direction separately", () => {
    expect(linkCountsForModel(response, {node_id: "master", model_id: "cc-ff"})).toEqual({lendsTo: 1, borrowsFrom: 0});
    expect(linkCountsForModel(response, slaveAnchor)).toEqual({lendsTo: 0, borrowsFrom: 1});
    expect(linkCountsForModel(null, slaveAnchor)).toEqual({lendsTo: 0, borrowsFrom: 0});
  });

  it("names the lane and shows both directions once any link exists", () => {
    expect(routingButtonLabel("image", {lendsTo: 0, borrowsFrom: 0})).toBe("Image routing");
    expect(routingButtonLabel("text", {lendsTo: 0, borrowsFrom: 2})).toBe("LLM routing · lends 0 · borrows 2");
  });
});

describe("groupCandidatesByNode", () => {
  it("groups by node and orders nodes predictably", () => {
    const groups = groupCandidatesByNode([
      candidate({node_id: "slave-b", model_id: "b1"}),
      candidate({node_id: "slave-a", model_id: "a1"}),
      candidate({node_id: "slave-a", model_id: "a2"})
    ]);

    expect(groups.map(group => group.nodeId)).toEqual(["slave-a", "slave-b"]);
    expect(groups[0]?.candidates.map(item => item.model_id)).toEqual(["a1", "a2"]);
  });
});

describe("routingDialogMarkup", () => {
  function markupFor(candidates: RoutingCandidate[]): string {
    return SafeHTML.render(routingDialogMarkup({
      lane: "text",
      anchor: slaveAnchor,
      candidates,
      drafts: draftsFromCandidates(candidates),
      acknowledged: false
    }));
  }

  function checkboxTag(markup: string, direction: string, field: string): string {
    const tag = markup.match(new RegExp(`<input[^>]*data-routing-direction="${direction}"[^>]*data-routing-field="${field}"[^>]*>`))?.[0];
    if (!tag) {
      throw new Error(`no ${direction} ${field} checkbox rendered`);
    }
    return tag;
  }

  it("renders both directions with flags disabled until the link is ticked", () => {
    const markup = markupFor([candidate({borrows_from: {selected: true, load_if_unloaded: false, restore_after_borrow: false}})]);

    expect(checkboxTag(markup, "lends_to", "selected")).not.toContain(" checked");
    expect(checkboxTag(markup, "lends_to", "loadIfUnloaded")).toContain(" disabled");
    expect(checkboxTag(markup, "borrows_from", "selected")).toContain(" checked");
    expect(checkboxTag(markup, "borrows_from", "loadIfUnloaded")).not.toContain(" disabled");
    expect(checkboxTag(markup, "borrows_from", "loadIfUnloaded")).not.toContain(" checked");
  });

  it("makes an ineligible text candidate unselectable and says why", () => {
    const markup = markupFor([candidate({eligible: false, ineligible_reason: "serves requests concurrently"})]);

    expect(checkboxTag(markup, "lends_to", "selected")).toContain(" disabled");
    expect(checkboxTag(markup, "borrows_from", "selected")).toContain(" disabled");
    expect(markup).toContain("serves requests concurrently");
  });
});
