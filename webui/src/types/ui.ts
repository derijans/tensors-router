import type { CookComponent, FileRecord, InventoryResponse, LaneKind, Model, NodeInventory, RouterProcessStatus } from "./api";
import type { Options } from "./json";

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
  color: string;
  meta: string[];
  payload: PalettePayload;
}

export interface SelectChoice {
  value: string;
  label: string;
}

export interface SimpleCookSidebar {
  key: string;
  type: SidebarValueType;
}

export interface FieldRenderContext {
  node: NodeInventory | null;
  nodeFiles: FileRecord[];
  nodeModels: Model[];
  otherNodeModels: Model[];
  comparableBySection: Map<string, Model[]>;
}

export interface FieldGroup {
  section: string;
  keys: string[];
}

export interface SidebarValueRow {
  value: string;
  config: string;
}

export interface AppState {
  csrf: string;
  inventory: InventoryResponse | null;
  router: RouterProcessStatus | null;
  activeTab: string;
  activeCookMode: CookMode;
  activePalette: PaletteName;
  simpleCook: {
    nodeID: string;
    configID: string;
    fields: Options;
    cleanFields: Options;
    mode: SimpleCookMode;
    fieldFilter: string;
    sidebar: SimpleCookSidebar | null;
  };
  constructor: {
    lanes: Record<LaneKind, PaletteComponentPayload | null>;
    targetNodes: Record<LaneKind, string>;
    options: Options;
    showUsedAll: boolean;
    showOptionsAll: boolean;
  };
  palettePayloads: Record<string, PalettePayload>;
}

export type RefreshInventory = () => Promise<void>;
