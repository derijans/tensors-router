import { describe, expect, it } from "vitest";
import { pendingSettingChanges, renderLendingSettingRows } from "../lending-settings-view";
import { SafeHTML } from "../safe-html";
import type { LendingSettingEntry } from "../types";

const entries: LendingSettingEntry[] = [
  {key: "offload_probe_idle", kind: "duration", description: "probe <idle>", default: "5s", config: "10s", override: "2s", effective: "2s", source: "db"},
  {key: "scheduling_min_samples", kind: "integer", description: "samples", default: "20", effective: "20", source: "default"}
];

const holdSetting: LendingSettingEntry = {key: "offload_hold_for_faster_helper", kind: "boolean", description: "hold", default: "true", config: "false", effective: "false", source: "config"};

describe("lending settings view", () => {
  it("shows every layer and offers Default only where a database value exists", () => {
    const markup = SafeHTML.render(renderLendingSettingRows(entries, true));

    expect(markup).toContain("probe &lt;idle&gt;");
    expect(markup).toContain('value="2s" placeholder="10s"');
    expect(markup).toContain('value="" placeholder="20"');
    expect(markup.match(/data-lending-setting-reset=/g)).toHaveLength(1);
    expect(markup).toContain('data-lending-setting-reset="offload_probe_idle"');
    expect(markup).toContain('class="badge tone-warning">2s</span><span class="muted">database</span>');
  });

  it("offers a boolean setting as true, false, or falling back to the lower layer", () => {
    const markup = SafeHTML.render(renderLendingSettingRows([holdSetting], true));

    expect(markup).toContain('<select class="lending-setting-input" data-lending-setting-input="offload_hold_for_faster_helper"');
    expect(markup).toContain('<option value="" selected>not set (false)</option>');
    expect(markup).toContain('<option value="true">true</option>');
    expect(markup).toContain('<option value="false">false</option>');
  });

  it("marks the stored boolean override as selected", () => {
    const markup = SafeHTML.render(renderLendingSettingRows([{...holdSetting, override: "true", effective: "true", source: "db"}], true));

    expect(markup).toContain('<option value="true" selected>true</option>');
    expect(markup).toContain('<option value="">not set (false)</option>');
  });

  it("disables editing where the router has no settings store", () => {
    const markup = SafeHTML.render(renderLendingSettingRows(entries, false));

    expect(markup.match(/ disabled>/g)).toHaveLength(2);
    expect(markup).not.toContain("data-lending-setting-reset");
  });

  it("saves changed values, drops cleared overrides, and ignores untouched inputs", () => {
    const changes = pendingSettingChanges(entries, [
      {key: "offload_probe_idle", value: " "},
      {key: "scheduling_min_samples", value: " 6 "},
      {key: "unknown_setting", value: "1"}
    ]);

    expect(changes).toEqual({set: {scheduling_min_samples: "6"}, reset: ["offload_probe_idle"]});
    expect(pendingSettingChanges(entries, [{key: "offload_probe_idle", value: "2s"}, {key: "scheduling_min_samples", value: ""}])).toEqual({set: {}, reset: []});
  });
});
