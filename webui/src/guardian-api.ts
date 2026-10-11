import { api } from "./api";
import type { GuardianAssignment, GuardianModelsResponse, GuardianScope } from "./types";

const guardianPath = "/api/guardian-models";

export function getGuardianModels(): Promise<GuardianModelsResponse> {
  return api<GuardianModelsResponse>(guardianPath);
}

export function saveGuardianModel(assignment: GuardianAssignment): Promise<GuardianModelsResponse> {
  return api<GuardianModelsResponse>(guardianPath, {method: "POST", body: JSON.stringify(assignment)});
}

export function clearGuardianModel(scope: GuardianScope, nodeID: string, modelID: string): Promise<GuardianModelsResponse> {
  const query = new URLSearchParams({scope, node_id: nodeID, model_id: modelID});
  return api<GuardianModelsResponse>(`${guardianPath}?${query.toString()}`, {method: "DELETE"});
}
