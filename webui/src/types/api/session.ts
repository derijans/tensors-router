export interface SessionResponse {
  authenticated: boolean;
  csrf: string;
}

export interface RouterProcessStatus {
  managed: boolean;
  running: boolean;
  url: string;
  pid?: number;
  can_shutdown: boolean;
  can_force_kill: boolean;
  error?: string;
}
