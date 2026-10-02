import { describe, expect, it } from "vitest";
import { hasRoomForEveryColumn, modelColumns, parsedModelColumnChoices, visibleModelColumns } from "../model-columns";

describe("model table columns", () => {
  it("shows the core columns when the table is narrow", () => {
    expect(visibleModelColumns({}, false)).toEqual(["id", "node", "backend", "capabilities", "available", "routing", "actions"]);
  });

  it("shows every column once the table has room for them", () => {
    expect(visibleModelColumns({}, true)).toEqual(modelColumns.map(column => column.key));
    expect(hasRoomForEveryColumn(1500)).toBe(true);
    expect(hasRoomForEveryColumn(1499)).toBe(false);
  });

  it("lets an explicit choice override the width rule both ways", () => {
    expect(visibleModelColumns({routing: false}, true)).not.toContain("routing");
    expect(visibleModelColumns({separate: true}, false)).toContain("separate");
  });

  it("keeps Separate right after Routing", () => {
    const keys = modelColumns.map(column => column.key);
    expect(keys.indexOf("separate")).toBe(keys.indexOf("routing") + 1);
  });

  it("drops stored choices it does not recognise", () => {
    expect(parsedModelColumnChoices({routing: false, ghost: true, node: "yes"})).toEqual({routing: false});
    expect(parsedModelColumnChoices(["routing"])).toEqual({});
    expect(parsedModelColumnChoices(null)).toEqual({});
  });
});
