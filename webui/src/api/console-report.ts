import { jsonRecord } from "../lib/json";
import { errorMessage } from "../lib/errors";

interface ConsoleReportDetail {
  [key: string]: unknown;
}

export function reportErrorToConsole(scope: string, error: unknown, detail?: ConsoleReportDetail): void {
  const message = errorMessage(error);
  const diagnostic = backendDiagnosticOf(error);
  const label = `tensors-router · ${scope}`;
  const group = typeof console.groupCollapsed === "function" ? console.groupCollapsed.bind(console) : console.error.bind(console);
  group(`${label}: ${message}`);
  if (error instanceof Error && error.stack) {
    console.error(error.stack);
  }
  if (detail && Object.keys(detail).length > 0) {
    console.error("context", detail);
  }
  if (diagnostic) {
    console.error("backend diagnostic", diagnostic);
    const output = diagnostic.output;
    if (typeof output === "string" && output !== "") {
      console.error(output);
    }
  }
  if (typeof console.groupEnd === "function") {
    console.groupEnd();
  }
}

function backendDiagnosticOf(error: unknown): Record<string, unknown> | undefined {
  if (!(error instanceof Error) || !("data" in error)) {
    return undefined;
  }
  const record = jsonRecord((error as { data: unknown }).data);
  return jsonRecord(record?.backend_diagnostic) ?? undefined;
}
