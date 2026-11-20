import type { CookComponent } from "../types";

export function hasKind(components: CookComponent[], kind: string): boolean {
  return components.some(component => component.kind === kind);
}
