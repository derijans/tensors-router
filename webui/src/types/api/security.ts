export type SecurityTokenRole = "inference" | "admin" | "metrics" | "cluster";

export type SecurityCertificateKind = "identity" | "authority" | "trusted_authority" | "pinned_peer" | "api_certificate";

export interface SecurityToken {
  id: string;
  name: string;
  role: SecurityTokenRole;
  source: string;
  created_at: string;
}

export interface SecurityCertificate {
  id: string;
  name: string;
  kind: SecurityCertificateKind;
  subject: string;
  issuer: string;
  dns_names: string[];
  ip_addresses: string[];
  fingerprint: string;
  not_before: string;
  not_after: string;
  active: boolean;
  source: SecurityCertificateSource;
  created_at: string;
}

export type SecurityProfile = "secure" | "trusted_lan";

export type SecurityListenerName = "api" | "cluster" | "shared";

export interface SecurityListener {
  name: SecurityListenerName;
  bind: string;
  tls: boolean;
  restart_required: boolean;
}

export interface SecurityStatus {
  identity_active: boolean;
  api_certificate_active: boolean;
  listeners: SecurityListener[];
  profile: SecurityProfile;
  profile_restart_required: boolean;
  secure_profile_blocker?: string;
}

export interface SecurityTokenCreateRequest {
  name: string;
  role: SecurityTokenRole;
  value: string;
}

export interface SecurityTokenCreatedResponse {
  token: SecurityToken;
  secret: string;
}

export interface SecurityTokenSecretResponse {
  secret: string;
}

export interface SecurityCertificateImportRequest {
  name: string;
  kind: SecurityCertificateKind;
  certificate_pem: string;
  private_key_pem: string;
  activate: boolean;
}

export interface SecurityIdentityGenerateRequest {
  name: string;
  hosts: string[];
  activate: boolean;
}

export interface SecurityAuthorityCreateRequest {
  name: string;
}

export interface SecurityIssueRequest {
  name: string;
  hosts: string[];
  install_locally: boolean;
  activate: boolean;
}

export interface SecurityIssuedIdentity {
  certificate_pem: string;
  private_key_pem: string;
}

export type SecurityCertificateSource = "created" | "supplied" | "migrated" | "issued" | "master";

export type SecurityRequestKind = "identity" | "api_certificate";

export type SecurityTarget = {scope: "local"} | {scope: "node"; nodeID: string};

export interface SecurityApiCertificateState {
  present: boolean;
  name?: string;
  source?: SecurityCertificateSource;
  not_after?: string;
  fingerprint?: string;
}

export interface SecurityPendingKey {
  id: string;
  name: string;
  kind: SecurityRequestKind;
  dns_names: string[];
  ip_addresses: string[];
  request_pem: string;
  created_at: string;
}

export interface SecuritySummary {
  node_id: string;
  status: SecurityStatus;
  api_certificate: SecurityApiCertificateState;
  tokens: SecurityToken[];
  certificates: SecurityCertificate[];
  pending_keys: SecurityPendingKey[];
  default_hosts: string[];
}

export interface SecurityCertificateRequestCreate {
  name: string;
  kind: SecurityRequestKind;
  hosts: string[];
}

export interface SecuritySignedCertificateInstall {
  certificate_pem: string;
  activate: boolean;
}

export interface SecurityNodeIssueRequest {
  authority_id: string;
  kind: SecurityRequestKind;
  name: string;
  hosts: string[];
  activate: boolean;
}

export interface SecurityClusterTokenPushResponse {
  token: SecurityToken;
  created: boolean;
}
