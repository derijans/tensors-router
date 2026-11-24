import { SafeHTML, html } from "../../lib/safe-html";
import { attributes } from "../../ui/attributes";
import type { ApiCertificateNotice } from "./security-api-certificate-data";
import type { SecurityNodeOption } from "./security-node-options-data";

export function securityNodeOptionsMarkup(options: readonly SecurityNodeOption[], selectedNodeID: string): SafeHTML {
  return html`${options.map(option => html`<option${attributes({
    value: option.nodeID,
    selected: option.nodeID === selectedNodeID,
    "data-attention": option.needsAttention
  })}>${option.label}</option>`)}`;
}

export function apiCertificateNoticeMarkup(notice: ApiCertificateNotice): SafeHTML {
  return html`<p class="security-notice tone-${notice.tone}">${notice.message}</p>`;
}
