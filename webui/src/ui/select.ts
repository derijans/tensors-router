import { SafeHTML, displayText, html, setHTML } from "../lib/safe-html";
import type { SelectChoice } from "../types";
import { attributes } from "./attributes";

export function optionElement(value: string, label: unknown, selected = false): SafeHTML {
  return html`<option${attributes({value, selected})}>${displayText(label)}</option>`;
}

export function selectChoice(value: string, label: string): SelectChoice {
  return {value, label};
}

export function optionList(choices: readonly SelectChoice[], selected = ""): SafeHTML {
  return html`${choices.map(choice => optionElement(choice.value, choice.label, choice.value === selected))}`;
}

export function valueOptions(values: readonly string[], selected: string): SafeHTML {
  return html`${values.map(value => optionElement(value, value, value === selected))}`;
}

export function fillSelectWithAll(select: HTMLSelectElement, allLabel: string, values: readonly string[], selected: string): void {
  setHTML(select, html`${optionElement("", allLabel)}${valueOptions(values, selected)}`);
}

export function replaceOptionsKeepingSelection(select: HTMLSelectElement, choices: readonly SelectChoice[]): void {
  const selected = select.value;
  setHTML(select, optionList(choices));
  if (Array.from(select.options).some(option => option.value === selected)) {
    select.value = selected;
  }
}
