import { SafeHTML, html } from "../../lib/safe-html";
import { jinjaKwargsPrecedenceLabels } from "../../domain/option-keys";
import { optionElement } from "../../ui/select";

export function jinjaKwargsPrecedenceOptions(selectedValue: string): SafeHTML {
  return html`${Object.entries(jinjaKwargsPrecedenceLabels).map(([precedence, label]) => optionElement(precedence, label, precedence === selectedValue))}`;
}
