import { getRequiredElement } from "../../lib/dom";

export const overviewElements = {
  overviewKpis: getRequiredElement("overviewKpis", HTMLElement),
  overviewLegend: getRequiredElement("overviewLegend", HTMLUListElement),
  overviewChart: getRequiredElement("overviewChart", HTMLElement),
  overviewAttention: getRequiredElement("overviewAttention", HTMLUListElement),
  overviewAttentionCount: getRequiredElement("overviewAttentionCount", HTMLElement),
  overviewNodes: getRequiredElement("overviewNodes", HTMLElement),
  overviewRecent: getRequiredElement("overviewRecent", HTMLTableSectionElement)
};
