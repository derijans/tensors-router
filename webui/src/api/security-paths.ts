import type { SecurityTarget } from "../types";

const securityBase = "/api/security";

export function securityTargetPath(target: SecurityTarget): string {
  return target.scope === "local" ? securityBase : `${securityBase}/nodes/${encodeURIComponent(target.nodeID)}`;
}

export function securityResourcePath(collection: string, id: string, action = ""): string {
  const resource = `/${collection}/${encodeURIComponent(id)}`;
  return action ? `${resource}/${action}` : resource;
}
