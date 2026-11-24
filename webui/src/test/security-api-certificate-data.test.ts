import { describe, expect, it } from "vitest";
import { apiCertificateNotice, apiCertificateWarning, certificateSourceLabel } from "../features/security/security-api-certificate-data";

const now = new Date("2026-06-01T00:00:00Z");

describe("API certificate state", () => {
  it("does not guess when a node did not report its API certificate", () => {
    expect(apiCertificateWarning(undefined, now)).toBeNull();
  });

  it("warns when a node has no API certificate", () => {
    expect(apiCertificateWarning({present: false}, now)).toEqual({label: "No API certificate", tone: "warning"});
  });

  it("warns within 30 days of expiry and escalates once expired", () => {
    expect(apiCertificateWarning({present: true, not_after: "2026-07-01T00:00:00Z"}, now)).toEqual({label: "API certificate expires in 30 d", tone: "warning"});
    expect(apiCertificateWarning({present: true, not_after: "2026-07-02T00:00:00Z"}, now)).toBeNull();
    expect(apiCertificateWarning({present: true, not_after: "2026-05-31T00:00:00Z"}, now)).toEqual({label: "API certificate expired", tone: "danger"});
  });

  it("labels where the certificate came from", () => {
    expect(certificateSourceLabel("migrated")).toBe("imported from config");
    expect(certificateSourceLabel("master")).toBe("issued by master");
    expect(certificateSourceLabel("supplied")).toBe("imported");
    expect(certificateSourceLabel(undefined)).toBe("source unknown");
  });

  it("describes a slave's API certificate with its source and expiry", () => {
    expect(apiCertificateNotice({present: false}, now).tone).toBe("warning");
    expect(apiCertificateNotice({present: true, name: "api", source: "master", not_after: "2026-06-06T00:00:00Z"}, now))
      .toEqual({message: "API certificate api: issued by master, expires in 5 d.", tone: "warning"});
    const healthy = apiCertificateNotice({present: true, name: "api", source: "migrated", not_after: "2027-06-01T00:00:00Z"}, now);
    expect(healthy.tone).toBe("success");
    expect(healthy.message).toContain("api: imported from config, valid until");
  });
});
