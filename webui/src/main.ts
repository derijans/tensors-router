import { getSession } from "./api/session-api";
import { setCsrfToken } from "./api/client";
import { refreshAll, refreshInventory } from "./features/shell/refresh";
import { bootstrapApplication } from "./bootstrap";
import { bindConstructor } from "./features/cook/constructor/constructor-bindings";
import { bindCookModes, bindRecipes, bindSimpleCook } from "./features/cook/cook-bindings";
import { bindDownloads } from "./features/downloads/download-bindings";
import { bindAnalytics, bindBenchmarks, bindLoadDiagnostics } from "./features/insights/insight-bindings";
import { bindLending } from "./features/lending/lending-bindings";
import { bindModels } from "./features/models/model-bindings";
import { bindNodes } from "./features/nodes/node-bindings";
import { bindOverview } from "./features/overview/overview-bindings";
import { bindRouterControls, bindSession } from "./features/shell/session-bindings";
import { bindWebUIs } from "./features/webuis/webui-bindings";
import { registerSafetyDialog } from "./features/shell/dialogs";
import { markConstructorClean, registerDirtyStateGuard } from "./features/shell/dirty-state";
import { registerOperationRetry, runOperation } from "./features/shell/operations";
import { showApp, showLogin } from "./features/shell/render-dashboard";
import { registerRoutingLinksDialog } from "./features/models/routing-links/routing-links-dialog";
import { bindGlobalSearch } from "./features/search/search";
import { registerSeparateRuntimeDialog } from "./features/models/separate-runtime/separate-runtime-dialog";
import { activateTab, bindNavigation } from "./features/shell/navigation";
import { state } from "./state/app-state";

bindNavigation();
bindSession();
bindRouterControls();
bindGlobalSearch();
bindOverview();
bindNodes();
bindWebUIs();
bindDownloads();
bindModels();
bindBenchmarks();
bindAnalytics();
bindLoadDiagnostics();
bindLending();
bindCookModes();
bindSimpleCook();
bindConstructor();
bindRecipes();
registerSafetyDialog();
registerRoutingLinksDialog(refreshInventory);
registerSeparateRuntimeDialog(refreshInventory);
registerOperationRetry();
registerDirtyStateGuard();
markConstructorClean();

void bootstrapApplication({
  getSession,
  applySession: session => { setCsrfToken(session.csrf); },
  showApp,
  showLogin,
  loadInitialData: async () => {
    await runOperation({key: "initial-load", group: "refresh", label: "Loading data…", task: refreshAll});
    activateTab(state.activeTab);
  }
});
