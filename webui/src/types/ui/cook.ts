import type { CookComponent, FileRecord, InventoryResponse, LaneKind, Model, NodeInventory } from "../api";
import type { Options } from "../json";
import type { Accent } from "./common";

export type CookMode = "quick" | "constructor";

export type PaletteName = "configs" | "files" | "options";

export type SimpleCookMode = "edit" | "new" | "copy";

export type SidebarValueType = "field" | "model";

export interface PaletteOptionPayload {
  type: "option";
  key: string;
}

export interface PaletteComponentPayload {
  type: "component";
  lane: LaneKind;
  label: string;
  subtitle: string;
  meta: string[];
  component: CookComponent;
  model?: Model;
  file?: FileRecord;
}

export type PalettePayload = PaletteOptionPayload | PaletteComponentPayload;

export interface PaletteEntry {
  title: string;
  subtitle: string;
  badge: string;
  accent: Accent;
  meta: string[];
  payload: PalettePayload;
}

export interface SimpleCookSidebar {
  key: string;
  type: SidebarValueType;
}

export interface FieldPreset {
  name: string;
  backendMode: string;
  section: string;
  values: Options;
}

export interface ConstructorFieldEditor {
  lane: LaneKind;
  draft: Options;
  pendingPayload?: PaletteComponentPayload;
}

export interface FieldRenderContext {
  node: NodeInventory | null;
  nodeFiles: FileRecord[];
  nodeModels: Model[];
  otherNodeModels: Model[];
  comparableBySection: Map<string, Model[]>;
  currentFields: Options;
}

export interface FieldGroup {
  section: string;
  keys: string[];
}

export interface SidebarValueRow {
  value: string;
  config: string;
}

export interface SimpleCookDraft {
  nodeID: string;
  configID: string;
  fields: Options;
  cleanFields: Options;
  cleanID: string;
  mode: SimpleCookMode;
  fieldFilter: string;
  openSections: string[];
  sidebar: SimpleCookSidebar | null;
}

export interface ConstructorDraft {
  lanes: Record<LaneKind, PaletteComponentPayload | null>;
  targetNodes: Record<LaneKind, string>;
  laneOptions: Record<LaneKind, Options>;
  backendMode: string;
  backendTouched: boolean;
  options: Options;
  fieldEditor: ConstructorFieldEditor | null;
  fieldPresets: FieldPreset[];
  showUsedAll: boolean;
  showOptionsAll: boolean;
  cleanSnapshot: string;
}

export interface ConstructorContext {
  draft: ConstructorDraft;
  inventory: InventoryResponse | null;
}
