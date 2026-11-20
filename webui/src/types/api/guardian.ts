export type GuardianScope = "cluster" | "node" | "model";

export interface GuardianEndpoint {
  node_id: string;
  model_id: string;
}

export interface GuardianAssignment {
  scope: GuardianScope;
  node_id?: string;
  model_id?: string;
  guardian: GuardianEndpoint;
}

export interface GuardianCandidate {
  node_id: string;
  model_id: string;
  filename: string;
}

export interface GuardianModelsResponse {
  assignments: GuardianAssignment[];
  candidates: GuardianCandidate[];
}
