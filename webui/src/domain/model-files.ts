import type { FileRecord } from "../types";

export function fileRoles(file: FileRecord): string[] {
  return file.roles ?? [file.role || "unknown"];
}

export function normalizedExtension(file: FileRecord): string {
  const extension = (file.extension || file.basename.split(".").pop() || "").trim().toLowerCase();
  return extension && !extension.startsWith(".") ? `.${extension}` : extension;
}
