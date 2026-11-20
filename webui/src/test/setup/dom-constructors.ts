const elementConstructorNames = [
  "HTMLElement",
  "HTMLButtonElement",
  "HTMLDialogElement",
  "HTMLFormElement",
  "HTMLInputElement",
  "HTMLSelectElement",
  "HTMLTableCellElement",
  "HTMLTableSectionElement",
  "HTMLTextAreaElement",
  "HTMLUListElement"
] as const;

const globals = globalThis as Record<string, unknown>;
for (const name of elementConstructorNames) {
  globals[name] ??= class {};
}
