import { state } from "./state.js";

export async function api(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (state.csrf && options.method && options.method !== "GET") {
    headers.set("X-CSRF-Token", state.csrf);
  }
  const response = await fetch(path, {...options, headers});
  const text = await response.text();
  const data = parseResponse(text);
  if (!response.ok) {
    const error = new Error(errorMessage(data, text, response.statusText));
    error.data = data;
    throw error;
  }
  return data;
}

function parseResponse(text) {
  if (!text) {
    return null;
  }
  try {
    return JSON.parse(text);
  } catch {
    return {raw: text};
  }
}

function errorMessage(data, text, fallback) {
  if (typeof data?.error === "string") {
    return data.error;
  }
  if (typeof data?.error?.message === "string") {
    return data.error.message;
  }
  return text || fallback;
}
