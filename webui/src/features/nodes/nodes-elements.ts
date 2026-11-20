import { getRequiredElement } from "../../lib/dom";

export const nodesElements = {
  nodeCount: getRequiredElement("nodeCount", HTMLElement),
  nodesGrid: getRequiredElement("nodesGrid", HTMLElement),
  nodesDetail: getRequiredElement("nodesDetail", HTMLElement),
  nodesRefreshButton: getRequiredElement("nodesRefreshButton", HTMLButtonElement),
  nodesScanNotices: getRequiredElement("nodesScanNotices", HTMLElement)
};
