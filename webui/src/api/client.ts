import { jsonRecord } from "../lib/json";
import { reportErrorToConsole } from "./console-report";
import { errorMessage } from "../lib/errors";
import type { ErrorResponse, ValidationIssue } from "../types";

export type WebError = Error & { data: unknown };

let csrfToken = "";

export function setCsrfToken(token: string): void {
  csrfToken = token;
}

export async function api<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await send(path, options);
  return parseResponse(await response.text()) as T;
}

export async function apiBlob(path: string, options: RequestInit = {}): Promise<Blob> {
  const response = await send(path, options);
  return response.blob();
}

async function send(path: string, options: RequestInit): Promise<Response> {
  const headers = new Headers(options.headers);
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (csrfToken && options.method && options.method !== "GET") {
    headers.set("X-CSRF-Token", csrfToken);
  }
  const response = await fetch(path, {...options, headers});
  if (!response.ok) {
    const text = await response.text();
    const data = parseResponse(text);
    const failure = webError(responseErrorMessage(data, text, response.statusText), data);
    reportErrorToConsole(`${options.method ?? "GET"} ${path}`, failure, {status: response.status});
    throw failure;
  }
  return response;
}

export function errorBody(error: unknown): ErrorResponse {
  if (isWebError(error)) {
    const record = jsonRecord(error.data);
    const validation = validationIssues(record?.validation);
    return validation ? {error: error.message, validation} : {error: error.message};
  }
  return {error: errorMessage(error)};
}

export function isWebError(error: unknown): error is WebError {
  return error instanceof Error && "data" in error;
}

function parseResponse(text: string): unknown {
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text) as unknown;
  } catch {
    return {raw: text};
  }
}

function responseErrorMessage(data: unknown, text: string, fallback: string): string {
  const body = jsonRecord(data);
  if (typeof body?.error === "string") {
    return body.error;
  }
  const nested = jsonRecord(body?.error);
  if (typeof nested?.message === "string") {
    return nested.message;
  }
  return text || fallback;
}

function webError(message: string, data: unknown): WebError {
  const error = new Error(message) as WebError;
  error.data = data;
  return error;
}

function validationIssues(value: unknown): ValidationIssue[] | null {
  if (!Array.isArray(value)) {
    return null;
  }
  const issues = value.filter(isValidationIssue);
  return issues.length > 0 ? issues : null;
}

function isValidationIssue(value: unknown): value is ValidationIssue {
  const record = jsonRecord(value);
  return typeof record?.severity === "string" &&
    typeof record.code === "string" &&
    typeof record.message === "string";
}
