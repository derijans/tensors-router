import type { SessionResponse } from "./types";

export interface BootstrapApplication {
  getSession: () => Promise<SessionResponse>;
  applySession: (session: SessionResponse) => void;
  showApp: () => void;
  showLogin: () => void;
  loadInitialData: () => Promise<void>;
}

export async function bootstrapApplication(application: BootstrapApplication): Promise<void> {
  let session: SessionResponse;
  try {
    session = await application.getSession();
  } catch {
    application.showLogin();
    return;
  }

  application.applySession(session);
  application.showApp();
  try {
    await application.loadInitialData();
  } catch {
    return;
  }
}
