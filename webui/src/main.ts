import { getSession } from "./api";
import { refreshAll, refreshInventory } from "./app-data";
import { bootstrapApplication } from "./bootstrap";
import { bindConstructor } from "./bindings/constructor-bindings";
import { bindCookModes, bindRecipes, bindSimpleCook } from "./bindings/cook-bindings";
import { bindDownloads } from "./bindings/download-bindings";
import { bindAnalytics, bindBenchmarks, bindLoadDiagnostics } from "./bindings/insight-bindings";
import { bindLending } from "./bindings/lending-bindings";
import { bindModels } from "./bindings/model-bindings";
import { bindNodes } from "./bindings/node-bindings";
import { bindOverview } from "./bindings/overview-bindings";
import { bindRouterControls, bindSession } from "./bindings/session-bindings";
import { bindWebUIs } from "./bindings/webui-bindings";
import { registerSafetyDialog } from "./dialogs";
import { markConstructorClean, registerDirtyStateGuard } from "./dirty-state";
import { registerOperationRetry, runOperation } from "./operations";
import { showApp, showLogin } from "./render-dashboard";
import { registerRoutingLinksDialog } from "./routing-links-dialog";
import { bindGlobalSearch } from "./search/search";
import { registerSeparateRuntimeDialog } from "./separate-runtime-dialog";
import { activateTab, bindNavigation } from "./shell/navigation";
import { state } from "./state";

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
  applySession: session => { state.csrf = session.csrf; },
  showApp,
  showLogin,
  loadInitialData: async () => {
    await runOperation({key: "initial-load", group: "refresh", label: "Loading data…", task: refreshAll});
    activateTab(state.activeTab);
  }
});
