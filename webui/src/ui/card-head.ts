import { type HTMLValue, SafeHTML, html } from "../lib/safe-html";

export function cardHead(heading: HTMLValue, aside: HTMLValue): SafeHTML {
  return html`<header class="card-head">${heading}${aside}</header>`;
}
