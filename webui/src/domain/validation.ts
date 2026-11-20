import type { ValidationIssue } from "../types";

export function issue(severity: "warning" | "error", code: string, message: string, field: string): ValidationIssue {
  return {severity, code, message, field};
}
