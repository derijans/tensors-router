import { type HTMLValue, SafeHTML, html } from "../lib/safe-html";
import { classNames } from "./attributes";

export type TextTag = "span" | "div" | "p" | "small";

const textElements: Record<TextTag, (className: string, content: HTMLValue) => SafeHTML> = {
  span: (className, content) => html`<span class="${className}">${content}</span>`,
  div: (className, content) => html`<div class="${className}">${content}</div>`,
  p: (className, content) => html`<p class="${className}">${content}</p>`,
  small: (className, content) => html`<small class="${className}">${content}</small>`
};

export function mutedText(content: HTMLValue, tag: TextTag = "span", extraClass = ""): SafeHTML {
  return textElements[tag](classNames("muted", extraClass), content);
}

export function errorText(content: HTMLValue, tag: TextTag = "p", extraClass = ""): SafeHTML {
  return textElements[tag](classNames("error-text", extraClass), content);
}

export function errorAlert(content: HTMLValue, extraClass = ""): SafeHTML {
  return html`<div class="${classNames("error-text", extraClass)}" role="alert">${content}</div>`;
}

export function actionStatus(content: HTMLValue): SafeHTML {
  return textElements.p("action-status", content);
}

export interface NodeErrorSource {
  node_id?: string;
  node_url?: string;
  error: string;
}

export function nodeErrorLine(failure: NodeErrorSource): string {
  return `${failure.node_id || failure.node_url || "node"}: ${failure.error}`;
}

export function nodeErrorText(failure: NodeErrorSource, tag: TextTag = "p"): SafeHTML {
  return errorText(nodeErrorLine(failure), tag);
}
