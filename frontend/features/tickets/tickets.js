"use strict";

import { byId, element, localDateTime, notice, setBusy } from "../../core/ui.js";
import { apiRequest, idempotencyKey } from "../../core/api.js";

const ticketForm = byId("ticket-form");
let pendingTicket = null;
let onCreated;

const samples = {
  billing: {
    customer: { plan: "Free", region: "United States", seats: 1, tenure_months: 4, prior_tickets: 0 },
    messages: [
      "My payment failed when I tried to upgrade to Pro. Can you check what's wrong?",
      "I tried again with a different card. Now I see TWO pending charges but my account still shows Free plan.",
      "My bank app shows THREE charges of $29.99. I still don't have Pro access. I need export features for a presentation in two hours."
    ]
  },
  outage: {
    customer: { plan: "Enterprise", region: "Thailand", seats: 45, tenure_months: 8, prior_tickets: 0 },
    messages: [
      "ระบบเข้าไม่ได้ครับ ขึ้น error 500",
      "ลองหลายเครื่องและหลายเบราว์เซอร์แล้ว เพื่อนร่วมงานก็เข้าไม่ได้",
      "เรามี demo กับลูกค้ารายใหญ่บ่ายนี้ แต่ status page บอกว่าระบบปกติ ทั้งที่ 45 คนใช้งานไม่ได้"
    ]
  },
  darkmode: {
    customer: { plan: "Pro", region: "United States", seats: 1, tenure_months: 5, prior_tickets: 0 },
    messages: [
      "Hey, do you support dark mode? No rush.",
      "I found Settings > Appearance, but only see Light and System Default. My Mac is in dark mode but your app still shows light theme.",
      "Can I schedule dark mode to switch on automatically at 6pm?"
    ]
  }
};

function addMessage(role = "customer", body = "", occurredAt = new Date()) {
  const list = byId("message-list");
  const card = element("div", "message-editor");
  const head = element("div", "message-editor-head");
  const title = element("strong", "", `ข้อความ ${list.children.length + 1}`);
  const remove = element("button", "remove-message", "ลบข้อความ");
  remove.type = "button";
  remove.addEventListener("click", () => {
    if (list.children.length <= 1) return;
    card.remove();
    [...list.children].forEach((item, index) => { item.querySelector("strong").textContent = `ข้อความ ${index + 1}`; });
    pendingTicket = null;
  });
  head.append(title, remove);

  const fields = element("div", "message-editor-fields");
  const roleLabel = element("label", "field");
  roleLabel.append(element("span", "", "ผู้ส่ง"));
  const roleInput = document.createElement("select");
  roleInput.innerHTML = '<option value="customer">ลูกค้า</option><option value="operator">Operator</option>';
  roleInput.value = role;
  roleLabel.append(roleInput);
  const timeLabel = element("label", "field");
  timeLabel.append(element("span", "", "เวลา"));
  const timeInput = document.createElement("input");
  timeInput.type = "datetime-local";
  timeInput.required = true;
  timeInput.value = localDateTime(occurredAt);
  timeLabel.append(timeInput);
  fields.append(roleLabel, timeLabel);

  const bodyLabel = element("label", "field");
  bodyLabel.append(element("span", "", "รายละเอียด"));
  const bodyInput = document.createElement("textarea");
  bodyInput.required = true;
  bodyInput.maxLength = 5000;
  bodyInput.placeholder = "ลูกค้าพบปัญหาอะไร และส่งข้อมูลเพิ่มเติมว่าอย่างไร";
  bodyInput.value = body;
  bodyLabel.append(bodyInput);
  card.append(head, fields, bodyLabel);
  list.append(card);
}

function loadSample(name) {
  const sample = samples[name];
  if (!sample) return;
  byId("plan").value = sample.customer.plan;
  byId("region").value = sample.customer.region;
  byId("seats").value = sample.customer.seats;
  byId("tenure").value = sample.customer.tenure_months;
  byId("prior").value = sample.customer.prior_tickets;
  byId("message-list").replaceChildren();
  sample.messages.forEach((body, index) => {
    addMessage("customer", body, new Date(Date.now() - (sample.messages.length - index) * 3600000));
  });
  pendingTicket = null;
  notice("โหลด ticket ตัวอย่างแล้ว กด “วิเคราะห์ด้วย LLM” เพื่อส่งไปยัง OpenAI", "info");
}

function ticketPayload() {
  const messages = [...byId("message-list").children].map((card) => ({
    role: card.querySelector("select").value,
    body: card.querySelector("textarea").value.trim(),
    occurred_at: new Date(card.querySelector('input[type="datetime-local"]').value).toISOString()
  }));
  return {
    customer: {
      plan: byId("plan").value,
      region: byId("region").value.trim(),
      seats: Number(byId("seats").value),
      tenure_months: Number(byId("tenure").value),
      prior_tickets: Number(byId("prior").value)
    },
    messages
  };
}

async function submitTicket(event) {
  event.preventDefault();
  if (!ticketForm.reportValidity()) return;
  let body;
  try {
    body = JSON.stringify(ticketPayload());
  } catch {
    notice("กรุณาตรวจวันและเวลาของข้อความ", "error");
    return;
  }
  if (!pendingTicket || pendingTicket.body !== body) pendingTicket = { body, key: idempotencyKey() };
  const button = byId("submit-ticket");
  setBusy(button, true, "กำลังวิเคราะห์...");
  notice("กำลังค้น FAQ และรอผลจาก LLM", "info");
  try {
    const result = await apiRequest("/tickets", {
      method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": pendingTicket.key }, body
    });
    pendingTicket = null;
    await onCreated(result.conversation_id);
    notice("วิเคราะห์ ticket สำเร็จ สามารถดูผลและสนทนาต่อด้านล่าง", "success");
  } catch (error) {
    notice(error.message, "error");
  } finally {
    setBusy(button, false);
  }
}

export function initTickets(openConversation) {
  onCreated = openConversation;
  byId("add-message").addEventListener("click", () => {
    if (byId("message-list").children.length >= 100) return;
    addMessage();
    pendingTicket = null;
  });
  document.querySelectorAll("[data-sample]").forEach((button) => {
    button.addEventListener("click", () => loadSample(button.dataset.sample));
  });
  ticketForm.addEventListener("submit", submitTicket);
  addMessage();
}
