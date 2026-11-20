import { SafeHTML, emptyHTML, html } from "../lib/safe-html";

export type AttributeValue = string | number | boolean | undefined;

export type Attributes = Readonly<Record<string, AttributeValue>>;

const attributeNamePattern = /^[a-z][a-z0-9-]*$/;

export function attributes(values: Attributes): SafeHTML {
  return html`${Object.entries(values).map(([name, value]) => attribute(name, value))}`;
}

export function attributeFlag(name: string, enabled: boolean): SafeHTML {
  return attribute(name, enabled);
}

function attribute(name: string, value: AttributeValue): SafeHTML {
  if (!attributeNamePattern.test(name)) {
    throw new Error(`Invalid attribute name: ${name}`);
  }
  if (value === false || value === undefined) {
    return emptyHTML;
  }
  if (value === true) {
    return html` ${name}`;
  }
  return html` ${name}="${value}"`;
}

export function classNames(...names: readonly (string | false | undefined)[]): string {
  return names.filter(Boolean).join(" ");
}
