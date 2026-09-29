# Support Triage Service

บริการคัดแยกคำร้องขอความช่วยเหลือผ่าน HTTP API พัฒนาด้วย Go, Fiber และ PostgreSQL ระบบค้น FAQ ในเครื่องและให้โมเดล OpenAI GPT เสนอข้อมูลที่มีโครงสร้าง จากนั้นใช้กฎในโค้ดตัดสินการดำเนินการสุดท้ายและควบคุมการเปิด incident ทุกข้อความ การตัดสินใจ และการเรียกเครื่องมือมีบันทึกให้ตรวจสอบย้อนหลัง เอกสารเริ่มใช้งานและ API ทุกเส้นทางอยู่ใน [README ชั้นนอก](../README.md)

## เริ่มต้นใช้งาน

จากราก `support-triage` ให้คัดลอก `.env.example` เป็น `.env` ใส่ `OPENAI_API_KEY` แล้วรัน `docker compose up -d --build` คำสั่งนี้เปิด backend, frontend และ PostgreSQL พร้อมกัน

หน้าเว็บอยู่ใน `../frontend` ภายใน Git repository เดียวกัน เปิด `http://localhost:3000/` หลัง Compose พร้อม หน้าเว็บส่งคำขอผ่าน Nginx proxy ไปยัง API และ **ไม่ขอ API key ในเบราว์เซอร์**

โฟลเดอร์นี้เก็บ Go backend, migration และเอกสาร ส่วน ticket ตัวอย่างสำหรับทดลองผ่านหน้าเว็บอยู่ใน `../frontend/features/tickets/tickets.js`

บริการใช้ OpenAI `gpt-4o-mini` ทางเดียว ต้องมี `OPENAI_API_KEY` ที่ใช้งานได้จึงจะวิเคราะห์ ticket ได้

เปิด `http://localhost:8081/api` เพื่อดูรายการ API; `GET /healthz` จะได้ `{"status":"ok"}` Docker Compose จะสร้างฐานข้อมูลและตารางเมื่อเริ่มครั้งแรก ต้องมีพอร์ต 3000, 5433 และ 8081 ว่าง เปลี่ยนพอร์ตภายนอกได้ด้วย `WEB_PORT`, `DB_PORT` และ `API_PORT` โดยไม่เปลี่ยนพอร์ตภายใน container

ระบบอ่านคีย์จาก `.env` ชั้นนอก ซึ่งถูก Git ignore หากเคยส่งคีย์ในแชตหรือเผยแพร่ที่อื่น ให้เพิกถอนคีย์เดิมและสร้างใหม่ หยุดระบบด้วย `docker compose down` จากราก repository ข้อมูลสนทนายังคงอยู่ใน volume `pgdata` คำสั่ง `docker compose down -v` จะลบฐานข้อมูลในเครื่อง ให้ใช้เฉพาะเมื่อต้องการเริ่มข้อมูลใหม่

## โครงสร้าง backend

`cmd/server/main.go` เป็นจุดเริ่มต้นของโปรแกรม: สร้าง PostgreSQL repository, FAQ searcher, OpenAI client และ incident provider แล้วส่ง dependencies ให้ service และ controller ผ่าน constructor

| โฟลเดอร์ | หน้าที่ |
| --- | --- |
| `controller/` | รับ HTTP request ผ่าน Fiber, ตรวจรูปแบบ input, แปลงผลลัพธ์และข้อผิดพลาดเป็น HTTP response แล้วเรียก service |
| `service/` | ประสาน workflow ของ ticket และ conversation; ใช้ repository interface สำหรับข้อมูล และเรียก FAQ/LLM/incident ผ่าน interfaces |
| `businesslogic/` | กฎธุรกิจที่ไม่ขึ้นกับ HTTP หรือฐานข้อมูล: ตรวจ proposal, กรอง FAQ ID และใช้ policy ตัดสิน action สุดท้าย |
| `model/` | entity ที่ service และ business logic ใช้ เช่น Customer, Proposal, Decision และ Conversation; ไม่มี GORM tags |
| `repository/` | interface และข้อมูลที่ service ใช้ติดต่อชั้นจัดเก็บ โดยไม่มี GORM หรือ SQL |
| `repository/postgres/` | `entity.go` เก็บ struct ของตาราง DB, `mapper.go` แปลงเป็น model ของ service และ `store.go` อ่านเขียนผ่าน GORM/PostgreSQL |
| `providers/` | implementation ของ FAQ, OpenAI และ incident mock ที่ service เรียกผ่าน interfaces |
| `triageprompt/` | คำสั่งและ JSON schema ที่ส่งให้ LLM |

ลำดับการเรียกข้อมูลคือ `HTTP → controller → service → repository interface → PostgreSQL implementation → DB` ส่วนการตัดสินใจใช้ `service → businesslogic` และการเรียก FAQ/LLM/incident ใช้ `service → provider interface → providers` ดังนั้น service ไม่ import GORM หรือ PostgreSQL และกฎธุรกิจทดสอบได้โดยไม่ต้องเปิด HTTP server หรือฐานข้อมูล

DB entity ใน `postgres/entity.go` ไม่ถูกส่งออกไปยัง service: repository แปลงแถวและ JSONB ให้เป็น `model.Conversation` หรือ `model.SideEffectAttempt` ก่อนคืนค่า การเปลี่ยนคอลัมน์จึงอยู่ในชั้น PostgreSQL ส่วน service ใช้ชนิดข้อมูลที่ตรงกับงานคัดแยก ticket

## โครงสร้างฐานข้อมูล

PostgreSQL มี 3 ตาราง โดยเก็บข้อมูลที่ต้องใช้ต่อและข้อมูลตรวจสอบย้อนหลังไว้ด้วยกันตามหน้าที่:

| ตาราง | ข้อมูลที่เก็บ |
| --- | --- |
| `conversations` | ข้อมูลลูกค้าและข้อความทั้ง thread ใน `messages_json` |
| `requests` | idempotency key, สถานะการประมวลผล, ผลลัพธ์เดิมสำหรับคำขอที่ส่งซ้ำ, proposal/decision และประวัติการเรียก FAQ/LLM/incident ใน `tool_calls_json` |
| `side_effect_attempts` | สถานะความพยายามเปิด incident และ `external_id` ที่ mock ใช้เป็นรหัสถาวร เพื่อ retry หรือสืบหาผลที่ยังไม่แน่ชัด |

ข้อความและ audit ยังอ่านได้ผ่าน `GET /conversations/:id` เหมือนเดิม การเก็บรายการข้อความและ tool calls เป็น JSONB ช่วยลดจำนวนตารางสำหรับงานขนาดเล็กนี้ แต่ถ้าต้องค้นหรือวิเคราะห์แต่ละข้อความในปริมาณมาก ควรแยกตารางอีกครั้ง

`migrations/001_init.sql` เป็นไฟล์ schema เดียวสำหรับฐานข้อมูลใหม่ Compose รันไฟล์นี้เมื่อสร้าง volume ว่างครั้งแรกเท่านั้น ฐานข้อมูลเดิมที่เป็น schema 3 ตารางใช้งานต่อได้ ส่วนฐานเก่าแบบ 4 หรือ 7 ตารางต้องสำรองและแปลงข้อมูลแยกก่อนใช้โค้ดปัจจุบัน เพราะชุดส่งมอบนี้ไม่มีสคริปต์อัปเกรดฐานเก่า

หาก `POST /tickets` ตอบ `502 model_error` ให้ดู `docker compose -f ../compose.yaml logs --tail 30 api` ถ้า log ระบุ `OpenAI returned HTTP 429 (code=...)` แปลว่าคำขอไปถึง OpenAI แล้ว แต่ OpenAI ปฏิเสธ ให้ดูค่า `code` จริง: rate limit ชั่วคราวอาจต้องรอหรือลดอัตราการเรียก ส่วนเครดิตหมดหรือถึงขีดจำกัดการใช้จ่ายต้องแก้ที่บัญชี API คีย์ควรเป็นของโปรเจกต์ที่มีสิทธิ์และเครดิตเพียงพอ

## การเรียก API

ทุกคำขอแบบ POST ต้องส่ง header `Idempotency-Key` ความยาว 8–128 ตัวอักษร หากส่งคีย์เดิมกับ JSON เดิม ระบบจะคืนผลที่บันทึกไว้ หากเปลี่ยนข้อมูลแต่ใช้คีย์เดิมจะได้ `409` หากคำขอแรกยังทำงานอยู่ การส่งซ้ำจะได้ `409 request_in_progress` พร้อม `Retry-After: 2`

ทดลองผ่านหน้าเว็บ `http://localhost:3000/`: เลือกตัวอย่าง outage, billing หรือ dark mode แล้วกด **วิเคราะห์ด้วย LLM** หน้าเว็บจะแสดงผลและประวัติ conversation ให้อัตโนมัติ หากต้องการสนทนาต่อ ให้พิมพ์ในช่อง **ข้อความจาก operator** แล้วกด **ส่งข้อความต่อ** หรือวาง `conversation_id` เดิมเพื่อเปิดประวัติ


| เส้นทาง | หน้าที่ | สถานะเมื่อสำเร็จ |
| --- | --- | --- |
| `GET /` | รายการเส้นทาง API | `200` |
| `GET /api` | รายการเส้นทาง API | `200` |
| `POST /tickets` | รับ ticket หนึ่งชุดข้อความและคัดแยก | `201` |
| `POST /conversations/:id/messages` | เพิ่มข้อความจาก operator และตัดสินใจอีกรอบ | `200` |
| `GET /conversations/:id` | อ่านข้อความ การตัดสินใจ การเรียกเครื่องมือ และความพยายามทำ side effect | `200` |
| `GET /healthz` | ตรวจว่า API ทำงานอยู่ | `200` |

ข้อผิดพลาดอยู่ในรูป `{"error":{"code":"...","message":"...","request_id":"..."}}` โดย JSON ไม่ถูกต้องได้ `400`, ข้อมูลไม่ผ่านการตรวจสอบได้ `422`, ไม่พบ conversation ได้ `404`, คีย์ซ้ำที่ขัดแย้งหรือคำขอเดิมยังประมวลผลได้ `409`, โมเดลล้มเหลวได้ `502`, และ dependency ไม่พร้อมหรือไม่ได้ตั้ง API key ได้ `503` ขนาด body สูงสุด 1 MiB; ticket มีได้สูงสุด 100 ข้อความ ข้อความละไม่เกิน 5000 ไบต์; `occurred_at` ต้องเป็นรูปแบบ RFC3339

## การทดสอบ

การทดสอบที่ไม่เรียก OpenAI รันด้วย Docker จากโฟลเดอร์ `backend`:

```powershell
docker run --rm -v "${PWD}:/src" -w /src golang:1.27-alpine sh -c "go test ./... && go vet ./..."
```

ผลตรวจในเครื่องผ่าน unit tests, integration test ของ incident mock กับ PostgreSQL, Docker build และ health check ชุดทดสอบ OpenAI client ใช้ HTTP server จำลอง จึงไม่ต้องใช้ API key จริงและไม่ได้วัดความแม่นยำของ GPT

integration tests ต้องมีฐานข้อมูลจาก Compose และฐานทดสอบแยกต่างหาก คำสั่งต่อไปนี้รันจากโฟลเดอร์ `backend`:

```powershell
docker compose -f ../compose.yaml up -d db
docker compose -f ../compose.yaml exec -T db psql -U triage -d postgres -c 'CREATE DATABASE triage_three_test;'
Get-Content migrations/001_init.sql -Raw | docker compose -f ../compose.yaml exec -T db psql -U triage -d triage_three_test
docker run --rm --network support-triage_default -e TEST_DATABASE_URL='postgres://triage:triage@db:5432/triage_three_test?sslmode=disable' -v "${PWD}:/src" -w /src golang:1.27-alpine go test -tags=integration ./...
```

คำสั่งสร้างฐาน `triage_three_test` ต้องทำเพียงครั้งแรกและควรเป็นฐานว่าง integration test ที่มีอยู่ตรวจการเปิด incident mock พร้อมกันและการเก็บ ID ใน PostgreSQL จริง ส่วนการทดสอบ OpenAI client ใช้ HTTP endpoint จำลอง จึงไม่ต้องใช้ API key จริง

จำลองความล้มเหลวได้ด้วย `INCIDENT_FAIL_MODE=before`, `after` หรือ `misleading`; `FAQ_FAIL=1`; หรือกำหนด `FAQ_DELAY_MS` และ `INCIDENT_DELAY_MS` เป็นจำนวนบวก แล้วเริ่ม API ใหม่ ตรวจผลผ่าน HTTP response และประวัติ audit ของ conversation
