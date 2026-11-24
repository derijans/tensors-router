import { pushClusterToken } from "../../api/security-node-api";
import { currentSecurityTarget } from "./security-selection";
import { clusterTokenPushMessage } from "./security-target-data";
import { runSecurityChange } from "./security";

export async function pushClusterTokenToNode(): Promise<void> {
  const target = currentSecurityTarget();
  if (target.scope === "node") {
    await runSecurityChange(response => clusterTokenPushMessage(target.nodeID, response), () => pushClusterToken(target.nodeID));
  }
}
