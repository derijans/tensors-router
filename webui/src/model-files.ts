import { hashModelFile } from "./api";
import { setModelActionStatus } from "./model-actions";
import { renderTables } from "./render-dashboard";
import { state } from "./state";

export async function calculateModelFileHash(nodeID: string, path: string): Promise<void> {
  try {
    const response = await hashModelFile({node_id: nodeID, path});
    const file = (state.inventory?.nodes ?? [])
      .find(node => node.node_id === response.node_id)?.files
      .find(item => item.path === response.path);
    if (!file) {
      throw new Error("Hashed model file is no longer in the inventory");
    }
    file.sha256 = response.sha256;
    setModelActionStatus(`Hashed ${file.basename}`, false);
    renderTables();
  } catch (error) {
    setModelActionStatus(error instanceof Error ? error.message : String(error), true);
    throw error;
  }
}

export async function copyModelFileHash(hash: string): Promise<void> {
  try {
    if (!navigator.clipboard) {
      throw new Error("Clipboard access is unavailable");
    }
    await navigator.clipboard.writeText(hash);
    setModelActionStatus("SHA-256 copied", false);
  } catch (error) {
    setModelActionStatus(error instanceof Error ? error.message : String(error), true);
    throw error;
  }
}
