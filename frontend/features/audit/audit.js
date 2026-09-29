"use strict";

import { element } from "../../core/ui.js";

export function parseAuditJSON(raw) {
  if (!raw) return null;
  try { return JSON.parse(raw); } catch { return raw; }
}

export function renderAuditGroup(parent, title, items, describe) {
  const group = element("div", "audit-group");
  group.append(element("h4", "", `${title} (${items.length})`));
  for (const item of items) {
    const entry = describe(item);
    const card = element("div", "audit-item");
    card.append(element("strong", "", entry.title), element("small", "", entry.meta));
    card.append(element("pre", "", JSON.stringify(entry.body, null, 2)));
    group.append(card);
  }
  parent.append(group);
}
