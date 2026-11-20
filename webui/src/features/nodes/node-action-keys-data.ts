export function nodeUnloadKey(backendID: string, runtimeID: string): string {
  return `${backendID} ${runtimeID}`;
}

export function nodeBackendActionKey(action: "init" | "cancel" | "launch-options", backendID: string): string {
  return `${action} ${backendID}`;
}
