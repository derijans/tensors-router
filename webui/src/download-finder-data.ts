export function splitSearchFilters(values: string[]): {filters: string[]; apps: string[]; providers: string[]; datasets: string[]; inference: boolean} {
  const unique = [...new Set(values)];
  return {
    filters: unique.filter(value => !/^(?:app|provider|dataset):/.test(value) && value !== "inference:true"),
    apps: unique.filter(value => value.startsWith("app:")).map(value => value.slice(4)),
    providers: unique.filter(value => value.startsWith("provider:")).map(value => value.slice(9)),
    datasets: unique.filter(value => value.startsWith("dataset:")).map(value => value.slice(8)),
    inference: unique.includes("inference:true")
  };
}

export function normalizeModelHash(value: string): string {
  const normalized = value.toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(normalized)) {
    throw new Error("Expected SHA-256 must contain exactly 64 hexadecimal characters");
  }
  return normalized;
}

export function normalizeParameterRange(minimum: string, maximum: string): string | undefined {
  if (!minimum && !maximum) {
    return undefined;
  }
  if ((minimum && !/^\d+(?:\.\d+)?$/.test(minimum)) || (maximum && !/^\d+(?:\.\d+)?$/.test(maximum))) {
    throw new Error("Parameter bounds must be non-negative numbers");
  }
  const minValue = minimum ? `${minimum}B` : "0B";
  const maxValue = maximum ? `${maximum}B` : "";
  return maxValue ? `min:${minValue},max:${maxValue}` : `min:${minValue}`;
}

export function parseOfficialHFURL(value: string): {repository: string; revision: string; file: string} {
  if (value.startsWith("hf://")) {
    const match = /^hf:\/\/([\w.-]+\/[\w.-]+)(?:@([\w.-]+)\/(.+))?$/.exec(value);
    if (!match || match[3]?.split("/").some(segment => !safeHFSegment(segment))) {
      throw new Error("Invalid hf:// model URL");
    }
    return {repository: match[1]!, revision: match[2] || "", file: match[3] || ""};
  }
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new Error("Enter an official Hugging Face model URL");
  }
  if (parsed.protocol !== "https:" || parsed.hostname !== "huggingface.co" || parsed.port || parsed.username || parsed.password) {
    throw new Error("Only https://huggingface.co model URLs are accepted");
  }
  let segments: string[];
  try {
    segments = parsed.pathname.split("/").filter(Boolean).map(segment => decodeURIComponent(segment));
  } catch {
    throw new Error("Invalid Hugging Face model URL encoding");
  }
  if (segments.length < 2 || !segments.every(safeHFSegment)) {
    throw new Error("Invalid Hugging Face model URL");
  }
  const repository = `${segments[0]}/${segments[1]}`;
  if (segments.length === 2) {
    return {repository, revision: "", file: ""};
  }
  if ((segments[2] !== "blob" && segments[2] !== "resolve") || segments.length < 5) {
    throw new Error("URL must identify a model repository or model file");
  }
  return {repository, revision: segments[3]!, file: segments.slice(4).join("/")};
}

function safeHFSegment(value: string): boolean {
  return value.length > 0 && value.length <= 255 && value !== "." && value !== ".." && /^[\w.+-]+$/u.test(value);
}
