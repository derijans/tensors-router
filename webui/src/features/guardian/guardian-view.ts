import { SafeHTML, html } from "../../lib/safe-html";
import { endpointLabel, endpointValue, type GuardianRow } from "./guardian-data";
import type { GuardianCandidate } from "../../types";
import { optionElement } from "../../ui/select";
import { mutedText } from "../../ui/text";
import { emptyTableRow } from "../../ui/empty-state";
import { attributeFlag } from "../../ui/attributes";

export function renderGuardianRows(rows: readonly GuardianRow[], candidates: readonly GuardianCandidate[], editable: boolean): SafeHTML {
  if (candidates.length === 0) {
    return emptyTableRow("No multimodal text model is available to act as a guardian.", 3, "muted");
  }
  return html`${rows.map(row => renderGuardianRow(row, candidates, editable))}`;
}

function renderGuardianRow(row: GuardianRow, candidates: readonly GuardianCandidate[], editable: boolean): SafeHTML {
  const {target, assigned, effective} = row;
  const selected = assigned ? endpointValue(assigned) : "";
  return html`
    <tr data-guardian-scope="${target.scope}" data-guardian-node="${target.nodeID}" data-guardian-model="${target.modelID}">
      <td>${target.label}</td>
      <td>
        <select data-guardian-select aria-label="Guardian for ${target.label}"${attributeFlag("disabled", !editable)}>
          ${optionElement("", "None", selected === "")}
          ${candidates.map(candidate => renderCandidateOption(candidate, selected))}
        </select>
      </td>
      <td>${mutedText(describeEffect(assigned, effective))}</td>
    </tr>
  `;
}

function renderCandidateOption(candidate: GuardianCandidate, selected: string): SafeHTML {
  const value = endpointValue({node_id: candidate.node_id, model_id: candidate.model_id});
  return optionElement(value, endpointLabel(candidate), value === selected);
}

function describeEffect(assigned: GuardianRow["assigned"], effective: GuardianRow["effective"]): string {
  if (assigned !== null) {
    return "set here";
  }
  return effective === null ? "none" : `inherits ${endpointLabel(effective)}`;
}
