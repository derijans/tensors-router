export type SecurityTokenRole = "inference" | "admin" | "metrics" | "cluster";

export type SecurityCertificateKind = "identity" | "authority" | "trusted_authority" | "pinned_peer";

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
  created_at: string;
}

export type SecurityProfile = "secure" | "trusted_lan";

export interface SecurityStatus {
  listener_tls: boolean;
  identity_active: boolean;
  restart_required: boolean;
  profile: SecurityProfile;
  profile_restart_required: boolean;
  secure_profile_blocker?: string;
}

export interface SecurityTokensResponse {
  tokens: SecurityToken[];
}

export interface SecurityCertificatesResponse {
  certificates: SecurityCertificate[];
  default_hosts: string[];
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
