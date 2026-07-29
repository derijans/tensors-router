export function defaultNodeSelection(localNodeID: string, nodeIDs: string[]): string[] {
  if (localNodeID && nodeIDs.includes(localNodeID)) {
    return [localNodeID];
  }
  return nodeIDs.length > 0 ? [nodeIDs[0]!] : ["*"];
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
