export const jinjaKwargsKey = "jinja_kwargs";

export const jinjaKwargsPrecedenceKey = "router_jinja_kwargs_precedence";

export const jinjaKwargsPrecedenceLabels: Record<"config" | "client", string> = {
  config: "Config wins",
  client: "Client wins"
};

export function compareOptionKeys(left: string, right: string): number {
  return optionSortKey(left).localeCompare(optionSortKey(right));
}

function optionSortKey(key: string): string {
  if (key === jinjaKwargsPrecedenceKey) {
    return `${jinjaKwargsKey}~`;
  }
  return key;
}
