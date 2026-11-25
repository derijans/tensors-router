import { defaultAuthorityName } from "./wizard-authority-data";
import type { WizardNodeReport } from "./wizard-nodes-data";

export type WizardStep = "token" | "authority" | "identity" | "apiCertificate" | "trust" | "profile";

export interface WizardRunResult {
  ok: boolean;
  message: string;
}

export type WizardStepResults = Record<string, WizardRunResult>;

interface WizardState {
  open: boolean;
  loading: boolean;
  error: string;
  reports: WizardNodeReport[];
  authorityID: string;
  newAuthorityName: string;
  apiSelection: Record<string, boolean>;
  clusterPorts: Record<string, string>;
  results: Partial<Record<WizardStep, WizardStepResults>>;
}

export const wizardState: WizardState = {
  open: false,
  loading: false,
  error: "",
  reports: [],
  authorityID: "",
  newAuthorityName: defaultAuthorityName,
  apiSelection: {},
  clusterPorts: {},
  results: {}
};
