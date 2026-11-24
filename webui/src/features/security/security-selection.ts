import { state } from "../../state/app-state";
import type { SecurityCertificate, SecuritySummary, SecurityTarget } from "../../types";
import { securityTargetFor } from "./security-target-data";

export function currentSecurityTarget(): SecurityTarget {
  return securityTargetFor(state.security.nodeID);
}

export function displayedSecuritySummary(): SecuritySummary | null {
  return state.security.nodeID === "" ? state.security.local : state.security.remote;
}

export function displayedDefaultHosts(): string[] {
  return displayedSecuritySummary()?.default_hosts ?? [];
}

export function localCertificates(): SecurityCertificate[] {
  return state.security.local?.certificates ?? [];
}
