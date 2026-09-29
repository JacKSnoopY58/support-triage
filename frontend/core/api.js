"use strict";

export function idempotencyKey() {
  return `web-${crypto.randomUUID()}`;
}

export async function apiRequest(path, options = {}) {
  let response;
  try {
    response = await fetch(`/backend${path}`, options);
  } catch {
    throw new Error("เชื่อมต่อ API ไม่ได้ ตรวจว่า Docker service กำลังทำงาน");
  }
  let payload;
  try {
    payload = await response.json();
  } catch {
    throw new Error(`เซิร์ฟเวอร์ตอบกลับไม่ถูกต้อง (HTTP ${response.status})`);
  }
  if (!response.ok) {
    const error = payload.error || {};
    const descriptions = {
      model_not_configured: "ยังไม่ได้ตั้ง OPENAI_API_KEY ในไฟล์ .env",
      model_error: "LLM วิเคราะห์ไม่สำเร็จ ตรวจว่าโมเดลพร้อมใช้งานและดู docker compose logs api",
      request_in_progress: "คำขอเดิมยังประมวลผลอยู่ รอสักครู่แล้วลองอีกครั้ง"
    };
    const message = descriptions[error.code] || error.message || `HTTP ${response.status}`;
    throw new Error(`${message}${error.request_id ? ` · request_id: ${error.request_id}` : ""}`);
  }
  return payload;
}
