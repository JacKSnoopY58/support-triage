"use strict";

import { byId, notice, setBusy } from "../../core/ui.js";
import { apiRequest, idempotencyKey } from "../../core/api.js";
import { renderConversation } from "./view.js";

const followupForm = byId("followup-form");
const lookupForm = byId("lookup-form");
let currentConversationID = "";
let pendingFollowup = null;

async function submitFollowup(event) {
  event.preventDefault();
  if (!currentConversationID || !followupForm.reportValidity()) return;
  const message = byId("followup-body").value.trim();
  if (!message) return;
  if (!pendingFollowup || pendingFollowup.message !== message || pendingFollowup.conversationID !== currentConversationID) {
    pendingFollowup = {
      message, conversationID: currentConversationID,
      body: JSON.stringify({ body: message, occurred_at: new Date().toISOString() }),
      key: idempotencyKey()
    };
  }
  const button = byId("submit-followup");
  setBusy(button, true, "กำลังส่ง...");
  notice("กำลังวิเคราะห์ข้อความร่วมกับประวัติเดิม", "info");
  try {
    await apiRequest(`/conversations/${encodeURIComponent(currentConversationID)}/messages`, {
      method: "POST", headers: { "Content-Type": "application/json", "Idempotency-Key": pendingFollowup.key },
      body: pendingFollowup.body
    });
    pendingFollowup = null;
    byId("followup-body").value = "";
    await openConversation(currentConversationID);
    notice("บันทึกข้อความและอัปเดตผลคัดแยกแล้ว", "success");
  } catch (error) {
    notice(error.message, "error");
  } finally {
    setBusy(button, false);
  }
}

export async function openConversation(id) {
  const normalized = id.trim();
  if (!normalized) throw new Error("กรุณากรอก Conversation ID");
  const data = await apiRequest(`/conversations/${encodeURIComponent(normalized)}`);
  currentConversationID = data.id;
  byId("conversation-id").value = data.id;
  pendingFollowup = null;
  renderConversation(data);
  return data;
}

export function initConversations() {
  followupForm.addEventListener("submit", submitFollowup);
  lookupForm.addEventListener("submit", async (event) => {
    event.preventDefault();
    try {
      await openConversation(byId("conversation-id").value);
      notice("โหลดประวัติสำเร็จ", "success");
    } catch (error) {
      notice(error.message, "error");
    }
  });
}
