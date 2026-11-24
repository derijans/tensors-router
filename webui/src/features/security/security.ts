import { errorMessage } from "../../lib/errors";
import { getSecuritySummary } from "../../api/security-target-api";
import { switchSecurityProfileToSecure } from "../../api/security-api";
import { state } from "../../state/app-state";
import type { SecuritySummary } from "../../types";
import { confirmDestructive } from "../shell/dialogs";
import { securityNodeOptions, selectableSecurityNodeID, syncReportedAPICertificate } from "./security-node-options-data";
import { renderSecurity } from "./security-render";
import { currentSecurityTarget } from "./security-selection";
import { localSecurityTarget } from "./security-target-data";

let latestLoad = 0;

export async function loadSecurity(): Promise<void> {
  const load = ++latestLoad;
  state.security.nodeID = selectableSecurityNodeID(securityNodeOptions(state.inventory, new Date()), state.security.nodeID);
  state.security.loading = true;
  state.security.error = "";
  renderSecurity();
  const target = currentSecurityTarget();
  const [local, remote] = await Promise.allSettled([
    getSecuritySummary(localSecurityTarget),
    target.scope === "node" ? getSecuritySummary(target) : Promise.resolve(null)
  ]);
  if (load !== latestLoad) {
    return;
  }
  state.security.local = settledSummary(local, state.security.local);
  state.security.remote = settledSummary(remote, state.security.remote);
  if (target.scope === "node") {
    syncReportedAPICertificate(state.inventory, target.nodeID, state.security.remote);
  }
  state.security.error = [local, remote].flatMap(result => result.status === "rejected" ? [errorMessage(result.reason)] : []).join(" ");
  state.security.loading = false;
  renderSecurity();
}

function settledSummary(result: PromiseSettledResult<SecuritySummary | null>, previous: SecuritySummary | null): SecuritySummary | null {
  return result.status === "fulfilled" ? result.value : previous;
}

export async function selectSecurityNode(nodeID: string): Promise<void> {
  state.security.nodeID = nodeID;
  state.security.remote = null;
  state.security.action = "";
  await loadSecurity();
}

export async function switchToSecureProfile(): Promise<void> {
  const consequences = "The router rewrites security.profile in its config.yaml. After a restart, model APIs need an inference token and admin APIs need an admin token, so set router.token in webui.yaml to an admin token.";
  if (!await confirmDestructive("Switch to secure profile?", consequences, "Switch to secure")) {
    return;
  }
  await runSecurityChange("Updated config.yaml.", switchSecurityProfileToSecure);
}

export async function runSecurityChange<T>(action: string | ((result: T) => string), change: () => Promise<T>): Promise<T> {
  state.security.error = "";
  try {
    const result = await change();
    state.security.action = typeof action === "string" ? action : action(result);
    return result;
  } catch (error) {
    state.security.error = errorMessage(error);
    throw error;
  } finally {
    await loadSecurity();
  }
}
