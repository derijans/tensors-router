export function defaultNodeSelection(localNodeID: string, nodeIDs: string[]): string[] {
  void localNodeID;
  void nodeIDs;
  return ["*"];
}

export function retainedNodeSelection(selected: string[], localNodeID: string, nodeIDs: string[]): string[] {
  const valid = selected.filter(value => value === "*" || nodeIDs.includes(value));
  return valid.length > 0 ? valid : defaultNodeSelection(localNodeID, nodeIDs);
}

export function changedNodeSelection(values: string[], previous: string[]): string[] {
  const unique = [...new Set(values)];
  if (!unique.includes("*") || unique.length === 1) {
    return unique;
  }
  return previous.includes("*") ? unique.filter(value => value !== "*") : ["*"];
}
