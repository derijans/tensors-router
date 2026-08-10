import type { FileRecord, Model, NodeInventory } from "./types";
import { fileRoles } from "./utils";

export interface ModelInventoryFilters {
  query: string;
  nodeIDs: string[];
  enabled: string;
  backend: string;
  capability: string;
}

export interface FileInventoryFilters {
  query: string;
  nodeIDs: string[];
  role: string;
  extension: string;
  hash: string;
}

export function filterInventoryModels(models: Model[], filters: ModelInventoryFilters): Model[] {
  return models.filter(model => selectedNode(model.node_id, filters.nodeIDs))
    .filter(model => filters.enabled === "all" || (filters.enabled === "disabled") === Boolean(model.disabled))
    .filter(model => !filters.backend || model.backend_mode === filters.backend)
    .filter(model => !filters.capability || modelHasCapability(model, filters.capability))
    .filter(model => matchesQuery(model, filters.query));
}

export function filterInventoryFiles(files: FileRecord[], filters: FileInventoryFilters): FileRecord[] {
  return files.filter(file => selectedNode(file.node_id, filters.nodeIDs))
    .filter(file => !filters.role || fileRoles(file).includes(filters.role))
    .filter(file => !filters.extension || normalizedExtension(file) === filters.extension)
    .filter(file => filters.hash === "all" || (filters.hash === "hashed") === Boolean(file.sha256))
    .filter(file => matchesQuery(file, filters.query));
}

export function inventoryModels(models: Model[], nodes: NodeInventory[]): Model[] {
  return models.length > 0 ? models : nodes.flatMap(node => node.models ?? []);
}

export function inventoryFiles(nodes: NodeInventory[]): FileRecord[] {
  return nodes.flatMap(node => node.files ?? []);
}

export function modelBackends(models: Model[]): string[] {
  return uniqueSorted(models.map(model => model.backend_mode));
}

export function modelCapabilities(models: Model[]): string[] {
  return ["llm", "image", "embeddings", "multimodal", "voice", "music"].filter(capability => models.some(model => modelHasCapability(model, capability)));
}

export function fileRoleOptions(files: FileRecord[]): string[] {
  return uniqueSorted(files.flatMap(fileRoles));
}

export function fileExtensionOptions(files: FileRecord[]): string[] {
  return uniqueSorted(files.map(normalizedExtension));
}

function selectedNode(nodeID: string, nodeIDs: string[]): boolean {
  return nodeIDs.length === 0 || nodeIDs.includes("*") || nodeIDs.includes(nodeID || "");
}

function matchesQuery(value: unknown, query: string): boolean {
  const normalized = query.trim().toLowerCase();
  return !normalized || JSON.stringify(value).toLowerCase().includes(normalized);
}

function modelHasCapability(model: Model, capability: string): boolean {
  switch (capability) {
    case "llm": return model.has_llm;
    case "image": return model.has_image;
    case "embeddings": return model.has_embeddings;
    case "multimodal": return model.has_multimodal;
    case "voice": return model.has_voice;
    case "music": return model.has_music;
    default: return false;
  }
}

function normalizedExtension(file: FileRecord): string {
  const extension = (file.extension || file.basename.split(".").pop() || "").trim().toLowerCase();
  return extension && !extension.startsWith(".") ? `.${extension}` : extension;
}

function uniqueSorted(values: string[]): string[] {
  return [...new Set(values.map(value => value.trim()).filter(Boolean))].sort((left, right) => left.localeCompare(right));
}
