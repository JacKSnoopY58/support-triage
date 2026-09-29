"use strict";

import { byId, element, formatTime } from "../../core/ui.js";
import { parseAuditJSON, renderAuditGroup } from "../audit/audit.js";

function fact(text) { return element("span", "fact", text); }

function renderSummary(decision) {
  const target = byId("decision-summary");
  target.replaceChildren();
  if (!decision) return;
  const box = element("div", "decision-hero");
  const top = element("div", "decision-top");
  top.append(element("h3", "", "ผลคัดแยกล่าสุด"), element("span", `pill pill-${decision.urgency}`, decision.urgency));
  box.append(top, element("p", "decision-reply", decision.reply));
  const facts = element("div", "facts");
  facts.append(
    fact(`Action: ${decision.action}`),
    fact(`Issue: ${decision.primary_issue_type}`),
    fact(`Area: ${decision.product_area}`),
    fact(`ภาษา: ${decision.language}`),
    fact(`มนุษย์ตรวจ: ${decision.requires_human_approval ? "ใช่" : "ไม่"}`),
    fact(`Model: ${decision.model}`)
  );
  if (decision.proposed_action !== decision.action) facts.append(fact(`LLM เสนอ: ${decision.proposed_action}`));
  if (decision.incident_status) facts.append(fact(`Incident: ${decision.incident_status}${decision.incident_id ? ` (${decision.incident_id})` : ""}`));
  if (decision.faq_ids?.length) facts.append(fact(`FAQ: ${decision.faq_ids.join(", ")}`));
  box.append(facts, element("p", "decision-rationale", decision.rationale));
  target.append(box);
}

export function renderConversation(data) {
  byId("conversation-empty").hidden = true;
  byId("conversation-content").hidden = false;
  const messages = data.messages || [];
  const decisions = data.decisions || [];
  renderSummary(decisions.at(-1));
  byId("message-count").textContent = `(${messages.length})`;
  byId("decision-count").textContent = `(${decisions.length})`;

  const messageList = byId("history-messages");
  messageList.replaceChildren();
  for (const message of messages) {
    const card = element("div", "timeline-item");
    const meta = element("div", "item-meta");
    meta.append(element("strong", "", message.role), element("span", "", formatTime(message.occurred_at)));
    card.append(meta, element("p", "", message.body));
    messageList.append(card);
  }

  const decisionList = byId("history-decisions");
  decisionList.replaceChildren();
  for (const decision of decisions) {
    const card = element("div", "history-decision");
    const meta = element("div", "item-meta");
    meta.append(element("strong", "", `${decision.urgency} · ${decision.action}`), element("span", "", formatTime(decision.created_at)));
    card.append(meta, element("p", "", decision.rationale));
    decisionList.append(card);
  }

  const audit = byId("audit-content");
  audit.replaceChildren();
  renderAuditGroup(audit, "Tool calls", data.tool_calls || [], (call) => ({
    title: `${call.name} · ${call.status}`,
    meta: formatTime(call.created_at),
    body: { input: parseAuditJSON(call.input_json), output: parseAuditJSON(call.output_json), error_code: call.error_code || undefined }
  }));
  renderAuditGroup(audit, "Side-effect attempts", data.side_effect_attempts || [], (attempt) => ({
    title: `${attempt.operation_key} · ${attempt.status}`,
    meta: formatTime(attempt.updated_at),
    body: { external_id: attempt.external_id || null, error_code: attempt.error_code || null }
  }));
}
