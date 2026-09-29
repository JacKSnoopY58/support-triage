# Support Triage

ระบบช่วยคัดแยก ticket ของทีม support จากบทสนทนาหลายข้อความ ค้น FAQ ที่เกี่ยวข้อง แล้วให้ OpenAI `gpt-4o-mini` เสนอ urgency, ประเภทปัญหา, action และข้อความตอบ Go policy ตรวจข้อเสนอก่อนตัดสินผลสุดท้ายและควบคุมการเปิด incident แบบจำลอง ทุก turn มีประวัติและ audit ให้ตรวจย้อนหลัง

Repository นี้รวมทุกส่วนสำหรับส่งมอบในที่เดียว:

```text
support-triage/
├── backend/       Go + Fiber API, GORM/PostgreSQL, FAQ, OpenAI และ policy
├── frontend/      HTML/CSS/JavaScript และ Nginx
├── compose.yaml   เปิด frontend, API และ PostgreSQL พร้อมกัน
└── .env.example   ตัวอย่างค่าตั้งต้น (ไม่มี API key จริง)
```

รายละเอียดชั้น `controller → service → repository → PostgreSQL` และกฎใน `businesslogic` อยู่ใน [backend/README.md](backend/README.md)

## เริ่มใช้งาน

ต้องมี Docker Desktop ที่รองรับ Compose และ OpenAI API key ที่ใช้งานได้ คัดลอก `.env.example` เป็น `.env` ที่ **ราก repository** แล้วใส่ `OPENAI_API_KEY` ไฟล์ `.env` ถูก Git ignore และไม่ควรใส่คีย์ใน frontend

เปิดระบบจากราก repository ด้วย Docker Compose:

```sh
docker compose up -d --build
```

หากมี `.env` อยู่แล้ว ไม่ต้องคัดลอกทับ เปิดหน้าเว็บที่ [http://localhost:3000/](http://localhost:3000/) ตรวจ API ที่ [http://localhost:8081/healthz](http://localhost:8081/healthz) และดูรายการ API ที่ [http://localhost:8081/api](http://localhost:8081/api) หน้าเว็บเรียก API ผ่าน Nginx proxy ภายใน Compose จึงไม่ต้องตั้ง CORS หรือส่ง API key ให้ browser

หยุดด้วย `docker compose down` ข้อมูล PostgreSQL ยังคงอยู่ใน Docker volume; อย่าใช้ `docker compose down -v` หากต้องการรักษาข้อมูล พอร์ตค่าเริ่มต้นคือ frontend `3000`, API `8081`, PostgreSQL `5433` และเปลี่ยนพอร์ตบนเครื่องได้ใน `.env`

สำหรับผู้ที่ไม่มี OpenAI API key: `/healthz`, หน้าเว็บ, การค้นประวัติ และการทดสอบแบบออฟไลน์ยังใช้ได้ แต่การวิเคราะห์ ticket จริงจะตอบ `503 model_not_configured` หากคีย์มีปัญหาด้านเครดิตหรือสิทธิ์ อาจตอบ `502 model_error`; ตรวจรายละเอียดใน `docker compose logs --tail 30 api`

## API

Base URL สำหรับเรียก API ตรงคือ `http://localhost:8081` หากเรียกจาก JavaScript ในหน้าเว็บ ระบบใช้ prefix `/backend` ผ่าน Nginx proxy ทุก POST ต้องส่ง `Content-Type: application/json` และ `Idempotency-Key` ความยาว 8–128 อักขระ

| Method | Path | หน้าที่ | สำเร็จ |
| --- | --- | --- | --- |
| GET | `/` หรือ `/api` | ชื่อ service และรายการ route | 200 |
| GET | `/healthz` | ตรวจว่า API ทำงาน | 200 |
| POST | `/tickets` | สร้าง conversation และวิเคราะห์ ticket ทั้ง thread | 201 |
| POST | `/conversations/:id/messages` | เพิ่มข้อความจาก operator แล้ววิเคราะห์ใหม่โดยใช้ประวัติเดิม | 200 |
| GET | `/conversations/:id` | อ่าน customer, messages, decisions, tool calls และ side-effect attempts | 200 |

### สร้าง ticket — `POST /tickets`

Body มี `customer` และ `messages` ตามลำดับเวลา `role` เป็น `customer` หรือ `operator`; `occurred_at` ใช้ RFC3339:

```json
{
  "customer": {"plan":"Enterprise","region":"Thailand","seats":45,"tenure_months":8,"prior_tickets":0},
  "messages": [
    {"role":"customer","body":"ระบบเข้าไม่ได้ครับ ขึ้น error 500","occurred_at":"2026-09-28T10:00:00Z"},
    {"role":"customer","body":"เพื่อนร่วมงานก็เข้าไม่ได้","occurred_at":"2026-09-28T10:30:00Z"}
  ]
}
```

ทดลองจากหน้าเว็บ `http://localhost:3000/` โดยเลือก ticket ตัวอย่างหรือกรอกข้อมูลเอง แล้วกด **วิเคราะห์ด้วย LLM**

Response มี `conversation_id`, `request_id`, `reply` และ `decision` ภายใน `decision` มี urgency, issue types, proposed/final action, rationale, FAQ hits, model และ incident status ตามกรณี บันทึก `conversation_id` เพื่อนำไปอ่านประวัติหรือส่งข้อความต่อ

### ส่งข้อความต่อ — `POST /conversations/:id/messages`

ใช้ ID จากผลสร้าง ticket; body เป็นข้อความ operator หนึ่งรายการ:

```json
{"body":"Please check the Asia region first.","occurred_at":"2026-09-28T13:00:00Z"}
```

บนหน้าเว็บ ใส่ข้อความในช่อง **ข้อความจาก operator** แล้วกด **ส่งข้อความต่อ**

Response มีรูปแบบเดียวกับ `POST /tickets` และสะท้อน decision ล่าสุด

### อ่านประวัติ — `GET /conversations/:id`

บนหน้าเว็บ หลังสร้าง ticket ระบบจะเปิดประวัติให้อัตโนมัติ หรือวาง `conversation_id` ในช่อง **Conversation ID** แล้วกด **เปิดประวัติ**

Response รวม `customer`, `messages`, `decisions`, `tool_calls` และ `side_effect_attempts` เพื่อดูว่า FAQ, OpenAI และ incident gateway ทำงานอย่างไร ถ้าไม่พบ ID จะได้ `404 not_found`

### การส่งซ้ำและข้อผิดพลาด

POST ที่ใช้ `Idempotency-Key` เดิมกับ payload เดิมจะคืนผลเดิมโดยไม่วิเคราะห์ซ้ำ; ถ้าใช้คีย์เดิมกับข้อมูลต่างกันจะได้ `409 idempotency_conflict` และถ้าคำขอเดิมยังทำงานจะได้ `409 request_in_progress` พร้อม `Retry-After` ข้อผิดพลาดมีรูปแบบ `{"error":{"code":"...","message":"...","request_id":"..."}}` โดย JSON ผิดรูปแบบได้ `400`, ข้อมูลไม่ผ่าน validation ได้ `422`, โมเดลล้มเหลวได้ `502` และ dependency หรือ API key ไม่พร้อมได้ `503`

Ticket รับได้สูงสุด 100 ข้อความ ข้อความละไม่เกิน 5000 ไบต์ และ HTTP body สูงสุด 1 MiB การเปิด incident เป็น mock ที่เก็บผลไว้ใน PostgreSQL เพื่อพิสูจน์การป้องกันการเปิดซ้ำ; ยังไม่ได้เชื่อม incident provider ภายนอก

## โครงสร้างฐานข้อมูล

PostgreSQL มี 3 ตารางตาม [backend/migrations/001_init.sql](backend/migrations/001_init.sql) โดยหนึ่ง conversation มีได้หลาย request เมื่อมีข้อความติดตาม แต่ละ request บันทึกผลการวิเคราะห์หนึ่งรอบ:

โค้ดแยก [DB entity](backend/repository/postgres/entity.go) ที่ตรงกับคอลัมน์ตาราง ออกจาก [model ของ service](backend/model/types.go) โดย [mapper](backend/repository/postgres/mapper.go) แปลงข้อมูลระหว่างสองฝั่งก่อนส่งให้ service

| ตาราง | คีย์และข้อมูลสำคัญ | หน้าที่ |
| --- | --- | --- |
| `conversations` | `id` (primary key), `customer_json`, `messages_json`, `created_at` | เก็บข้อมูลลูกค้าและข้อความทั้งหมดใน thread; `messages_json` เป็น JSONB array รวมข้อความลูกค้า, operator และคำตอบจากระบบ |
| `requests` | `idempotency_key` (primary key), `conversation_id` (foreign key), `payload_hash`, `status`, `lease_until`, `response_json`, `proposal_json`, `decision_json`, `tool_calls_json`, `created_at`, `updated_at` | เก็บสถานะและผลของการวิเคราะห์แต่ละรอบ รวมข้อเสนอจาก LLM, ผลที่ผ่าน policy และประวัติการเรียก FAQ/LLM/incident; `response_json` ใช้คืนผลเดิมเมื่อส่งคำขอซ้ำ |
| `side_effect_attempts` | `operation_key` (primary key), `conversation_id` และ `request_key` (foreign keys), `status`, `external_id`, `error_code`, `created_at`, `updated_at` | บันทึกความพยายามเปิด incident ก่อนเรียก provider และเก็บ ID ที่ได้ เพื่อ retry หรือตรวจผลโดยไม่เปิด incident ซ้ำ |

ความสัมพันธ์คือ `conversations.id → requests.conversation_id` และ `conversations.id → side_effect_attempts.conversation_id`; `requests.idempotency_key → side_effect_attempts.request_key` สถานะ request เป็น `processing`, `completed` หรือ `failed` ส่วนสถานะ incident attempt เป็น `pending`, `completed`, `unknown` หรือ `failed` มี index บน `(conversation_id, created_at)` สำหรับอ่านประวัติ และคีย์ของแต่ละตารางป้องกันรายการซ้ำ

ข้อมูลอยู่ใน Docker volume `support-triage-service-git_pgdata` เพื่อรักษาข้อมูลเดิมหลังรวม Compose การหยุดด้วย `docker compose down` ไม่ลบข้อมูล แต่ `docker compose down -v` จะลบ volume นี้

`backend/migrations/001_init.sql` เป็นไฟล์ schema เดียวและ Compose รันเฉพาะตอนสร้าง volume ว่าง ฐานเดิมที่เป็น schema 3 ตารางใช้ต่อได้ ส่วนฐานเก่าแบบ 4 หรือ 7 ตารางต้องสำรองและแปลงข้อมูลแยกก่อนใช้เวอร์ชันนี้

## การทดสอบ

จากราก repository รัน unit tests และ vet ใน Docker ได้โดยไม่ใช้ API key:

```powershell
docker run --rm -v "${PWD}/backend:/src" -w /src golang:1.27-alpine sh -c "go test ./... && go vet ./..."
```
