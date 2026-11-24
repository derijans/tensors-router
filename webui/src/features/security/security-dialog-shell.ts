import { type SafeHTML, emptyHTML, setHTML } from "../../lib/safe-html";
import { downloadBlob } from "../../lib/download";
import type { SecurityTarget } from "../../types";
import { showStatus } from "../../ui/status-line";
import { pemBlob, pemFilename } from "./security-data";
import { securityElements } from "./security-elements";
import { currentSecurityTarget } from "./security-selection";
import { localSecurityTarget } from "./security-target-data";

export interface DialogDownload {
  content: string;
  name: string;
  suffix: string;
}

let dialogTarget: SecurityTarget = localSecurityTarget;
let dialogDownloads: Record<string, DialogDownload> = {};

export function openSecurityDialog(content: SafeHTML, downloads: Record<string, DialogDownload> = {}): void {
  if (!securityElements.dialog.open) {
    dialogTarget = currentSecurityTarget();
  }
  dialogDownloads = downloads;
  setHTML(securityElements.dialogBody, content);
  showStatus(securityElements.dialogStatus, "", false);
  if (!securityElements.dialog.open) {
    securityElements.dialog.showModal();
  }
}

export function closeSecurityDialog(): void {
  dialogDownloads = {};
  setHTML(securityElements.dialogBody, emptyHTML);
  securityElements.dialog.close();
}

export function securityDialogTarget(): SecurityTarget {
  return dialogTarget;
}

export function showSecurityDialogError(message: string): void {
  showStatus(securityElements.dialogStatus, message, true);
}

export function downloadDialogPart(part: string): void {
  const download = dialogDownloads[part];
  if (download) {
    downloadBlob(pemBlob(download.content), pemFilename(download.name, download.suffix));
  }
}

export async function copyDisplayedSecret(): Promise<void> {
  const field = securityElements.dialogBody.querySelector<HTMLInputElement>("[data-security-secret]");
  if (field) {
    await navigator.clipboard.writeText(field.value);
    showStatus(securityElements.dialogStatus, "Copied.", false);
  }
}

export function formText(values: FormData, field: string): string {
  const value = values.get(field);
  return typeof value === "string" ? value.trim() : "";
}

export function formChecked(values: FormData, field: string): boolean {
  return values.get(field) !== null;
}
