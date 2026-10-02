import { bindLendingTab, setLendingTabActive } from "../lending-tab";
import { onTabActivation } from "../shell/navigation";
import { runTask } from "../tasks";

export function bindLending(): void {
  onTabActivation(tab => setLendingTabActive(tab === "lending"));
  bindLendingTab((task, key, label) => runTask(task, key, "lending", label));
}
