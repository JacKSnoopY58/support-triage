"use strict";

import { byId } from "./core/ui.js";
import { apiRequest } from "./core/api.js";
import { initTickets } from "./features/tickets/tickets.js";
import { initConversations, openConversation } from "./features/conversations/conversations.js";

async function checkHealth() {
  const target = byId("health");
  try {
    await apiRequest("/healthz");
    target.className = "health health-ok";
    target.textContent = "● API พร้อมใช้งาน";
  } catch {
    target.className = "health health-error";
    target.textContent = "● API ไม่พร้อม";
  }
}

initConversations();
initTickets(openConversation);
checkHealth();
