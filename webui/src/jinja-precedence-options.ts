import { jinjaKwargsPrecedenceLabels } from "./constants";
import { optionElement } from "./markup-primitives";
import { SafeHTML, html } from "./safe-html";

export function jinjaKwargsPrecedenceOptions(selectedValue: string): SafeHTML {
  return html`${Object.entries(jinjaKwargsPrecedenceLabels).map(([precedence, label]) => optionElement(precedence, label, precedence === selectedValue))}`;
}
