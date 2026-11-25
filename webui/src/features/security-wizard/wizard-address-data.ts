export interface HostPort {
  host: string;
  port: string;
}

const wildcardHosts = new Set(["", "0.0.0.0", "::", "[::]"]);

export function splitHostPort(address: string): HostPort {
  const trimmed = address.trim();
  const bracketed = /^\[([^\]]*)\]:(\d*)$/.exec(trimmed);
  if (bracketed) {
    return {host: bracketed[1] ?? "", port: bracketed[2] ?? ""};
  }
  const separator = trimmed.lastIndexOf(":");
  if (separator < 0 || trimmed.indexOf(":") !== separator) {
    return {host: trimmed, port: ""};
  }
  return {host: trimmed.slice(0, separator), port: trimmed.slice(separator + 1)};
}

export function joinHostPort(host: string, port: string): string {
  return host.includes(":") ? `[${host}]:${port}` : `${host}:${port}`;
}

export function isWildcardHost(host: string): boolean {
  return wildcardHosts.has(host);
}

export function urlHost(value: string): string {
  try {
    return new URL(value).hostname.replace(/^\[|\]$/g, "");
  } catch {
    return "";
  }
}

export function validPort(port: string): boolean {
  const value = Number(port);
  return /^\d+$/.test(port) && value > 0 && value < 65536;
}

export function shiftedPort(port: string, offset: number): string {
  return validPort(port) && Number(port) + offset < 65536 ? String(Number(port) + offset) : "";
}
