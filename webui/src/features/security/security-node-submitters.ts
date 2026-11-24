import { issueFromMasterAuthority, trustMasterAuthority } from "../../api/security-node-api";
import type { SecurityTarget } from "../../types";
import { isRequestKind, parseHosts } from "./security-data";
import { closeSecurityDialog, formChecked, formText } from "./security-dialog-shell";
import type { SecurityFormSubmitter } from "./security-form-submitter";
import { runSecurityChange } from "./security";

function targetNodeID(target: SecurityTarget): string {
  if (target.scope !== "node") {
    throw new Error("Select a slave node first.");
  }
  return target.nodeID;
}

async function submitMasterIssue(values: FormData, target: SecurityTarget): Promise<void> {
  const nodeID = targetNodeID(target);
  const kind = formText(values, "kind");
  if (!isRequestKind(kind)) {
    throw new Error("Choose what the certificate is for.");
  }
  const request = {
    authority_id: formText(values, "authority_id"),
    kind,
    name: formText(values, "name"),
    hosts: parseHosts(formText(values, "hosts")),
    activate: formChecked(values, "activate")
  };
  await runSecurityChange(`Issued ${request.name} to ${nodeID}`, () => issueFromMasterAuthority(nodeID, request));
  closeSecurityDialog();
}

async function submitTrust(values: FormData, target: SecurityTarget): Promise<void> {
  const nodeID = targetNodeID(target);
  await runSecurityChange(`${nodeID} now trusts the master CA`, () => trustMasterAuthority(nodeID, formText(values, "certificate_id")));
  closeSecurityDialog();
}

export const nodeFormSubmitters = {
  "master-issue": submitMasterIssue,
  trust: submitTrust
} satisfies Record<string, SecurityFormSubmitter>;
