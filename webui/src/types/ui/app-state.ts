import type { NodeMemoryFit, AnalyticsQuery, AnalyticsResponse, LoadCaptureAttempt, LoadCaptureDetailResponse, LoadCaptureListResponse, LoadCaptureOutputChunk, LoadCaptureQuery, LoadErrorRecord, BenchmarkRecord, BenchmarkSection, BenchmarkType, DownloadCapabilitiesResponse, DownloadLibraryResponse, DownloadPlan, InventoryResponse, NodeState, RouterProcessStatus, RoutingLane, RoutingLinksResponse, WebUICatalogResponse } from "../api";
import type { ModelColumnChoices } from "../../features/models/model-columns-data";
import type { ConversionWarning, OperationStatus, OverviewPeriod } from "./common";
import type { ConstructorDraft, CookMode, PaletteName, PalettePayload, SimpleCookDraft } from "./cook";

export type ModelInventorySubtab = "models" | "files";

export interface NodeRuntimeSlice {
  snapshot: NodeState | null;
  loading: boolean;
  error: string;
  pollGeneration: number;
  pollTimer: number | null;
  pendingUnload: string;
  pendingBackendAction: string;
}

export interface AppState {
  inventory: InventoryResponse | null;
  routingLinks: Record<RoutingLane, RoutingLinksResponse | null>;
  router: RouterProcessStatus | null;
  nodeSnapshots: Record<string, NodeState>;
  overview: {
    period: OverviewPeriod;
    analytics: AnalyticsResponse | null;
    error: string;
  };
  search: {
    query: string;
    activeIndex: number;
  };
  nodes: {
    expanded: string[];
    byNode: Record<string, NodeRuntimeSlice>;
  };
  models: {
    activeSubtab: ModelInventorySubtab;
    configNodeIDs: string[];
    fileNodeIDs: string[];
    modelSearch: string;
    enabledFilter: string;
    backendFilter: string;
    capabilityFilter: string;
    fileSearch: string;
    fileRoleFilter: string;
    fileExtensionFilter: string;
    fileHashFilter: string;
    initialized: boolean;
    columnChoices: ModelColumnChoices;
    roomForEveryColumn: boolean;
    memoryFit: Record<string, NodeMemoryFit>;
  };
  benchmark: {
    modelKey: string;
    type: BenchmarkType;
    sections: BenchmarkSection[];
    record: BenchmarkRecord | null;
    running: boolean;
    error: string;
  };
  analytics: {
    query: AnalyticsQuery;
    data: AnalyticsResponse | null;
    loading: boolean;
    error: string;
    showDetails: boolean;
  };
  loadCaptures: {
    query: LoadCaptureQuery;
    data: LoadCaptureListResponse | null;
    attempts: LoadCaptureAttempt[];
    nextCursor: string;
    detail: LoadCaptureDetailResponse | null;
    output: LoadCaptureOutputChunk[];
    outputCursor: number;
    outputMore: boolean;
    loading: boolean;
    error: string;
  };
  loadErrors: {
    records: LoadErrorRecord[];
    enabled: boolean;
    selectedID: string;
    nodeErrors: {node_id: string; error: string}[];
    loading: boolean;
    error: string;
  };
  webuis: {
    data: WebUICatalogResponse | null;
    filter: string;
    loading: boolean;
    error: string;
    action: string;
  };
  downloads: {
    available: boolean;
    capabilities: DownloadCapabilitiesResponse | null;
    nodeID: string;
    plan: DownloadPlan | null;
    selectedPlanFiles: string[];
    library: DownloadLibraryResponse | null;
    search: {id: string; downloads: number; likes: number; gated?: string; tags?: string[]; author?: string; updated_at?: string}[];
    searchStatus: "idle" | "searching" | "ok" | "empty" | "error";
    searchError: string;
    searchQuery: string;
    selectedRepository: string;
    nextCursor: string;
    filterTab: string;
    filters: string[];
    expandedFilterGroups: string[];
    observedFilters: string[];
    candidates: {repository: string; repository_path: string; commit: string; sha256?: string; state: "exact" | "mismatched" | "unverifiable"}[];
    finderMessage: string;
    modelHandoff: {nodeID: string; publicID: string; configID: string; configFilename: string; field: string; position?: number; filename: string; hash: string} | null;
    error: string;
    pollError: string;
  };
  activeTab: string;
  activeCookMode: CookMode;
  activePalette: PaletteName;
  simpleCook: SimpleCookDraft;
  constructor: ConstructorDraft;
  operations: Record<string, OperationStatus>;
  conversionWarnings: Record<string, ConversionWarning>;
  acceptedConversionSignature: string;
  palettePayloads: Record<string, PalettePayload>;
}
