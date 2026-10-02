import { describe, expect, it } from "vitest";
import { renderVersionRows } from "../analytics-versions-view";
import { SafeHTML } from "../safe-html";

describe("analytics router versions", () => {
  it("lists each node's builds and flags events recorded before stamping", () => {
    const markup = SafeHTML.render(renderVersionRows([
      {node_id: "slave <1>", router_version: "", first_seen: 0, last_seen: 1, event_count: 8},
      {node_id: "slave <1>", router_version: "v0.7.3", first_seen: 2, last_seen: 3, event_count: 4}
    ]));

    expect(markup).toContain("slave &lt;1&gt;");
    expect(markup).toContain('class="badge tone-warning">unknown (recorded before version stamping)</span>');
    expect(markup).toContain("<td>v0.7.3</td>");
    expect(markup.match(/<tr>/g)).toHaveLength(2);
  });
});
