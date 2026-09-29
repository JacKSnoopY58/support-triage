"use strict";

export const byId = (id) => document.getElementById(id);

export function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = String(text);
  return node;
}

export function localDateTime(value = new Date()) {
  const adjusted = new Date(value.getTime() - value.getTimezoneOffset() * 60000);
  return adjusted.toISOString().slice(0, 16);
}


export function notice(message, kind = "info") {
  const box = byId("notice");
  box.className = `notice notice-${kind}`;
  box.textContent = message;
  box.hidden = false;
}

export function setBusy(button, busy, label) {
  button.disabled = busy;
  if (busy) {
    button.dataset.label = button.textContent;
    button.textContent = label;
  } else if (button.dataset.label) {
    button.textContent = button.dataset.label;
    delete button.dataset.label;
  }
}


export function formatTime(value) {
  if (!value) return "";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString("th-TH", { dateStyle: "short", timeStyle: "short" });
}
