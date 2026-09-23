export type HTMLValue = SafeHTML | readonly HTMLValue[] | string | number | boolean | null | undefined;

export class SafeHTML {
  readonly #markup: string;

  private constructor(markup: string) {
    this.#markup = markup;
  }

  static fromTemplate(strings: TemplateStringsArray, values: readonly HTMLValue[]): SafeHTML {
    let markup = strings[0] ?? "";
    values.forEach((value, index) => {
      markup += SafeHTML.render(value) + (strings[index + 1] ?? "");
    });
    return new SafeHTML(markup);
  }

  static render(value: HTMLValue): string {
    if (value instanceof SafeHTML) {
      return value.#markup;
    }
    if (Array.isArray(value)) {
      return (value as readonly HTMLValue[]).map(item => SafeHTML.render(item)).join("");
    }
    return escapeMarkup(value);
  }

  isEmpty(): boolean {
    return this.#markup.trim() === "";
  }
}

export function html(strings: TemplateStringsArray, ...values: readonly HTMLValue[]): SafeHTML {
  return SafeHTML.fromTemplate(strings, values);
}

export const emptyHTML = html``;

export function listOrFallback(items: readonly SafeHTML[], fallback: SafeHTML): SafeHTML {
  return items.length > 0 ? html`${items}` : fallback;
}

export function setHTML(element: Element, content: SafeHTML): void {
  element.innerHTML = SafeHTML.render(content);
}

export function setOuterHTML(element: Element, content: SafeHTML): void {
  element.outerHTML = SafeHTML.render(content);
}

const markupEntities: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  "\"": "&quot;",
  "'": "&#39;",
  "`": "&#96;"
};

function escapeMarkup(value: unknown): string {
  return displayText(value).replace(/[&<>"'`]/g, character => markupEntities[character] ?? character);
}

export function displayText(value: unknown): string {
  if (value === null || value === undefined) {
    return "";
  }
  if (typeof value === "string") {
    return value;
  }
  if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") {
    return value.toString();
  }
  const json = JSON.stringify(value);
  return json ?? "";
}
