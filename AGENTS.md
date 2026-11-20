# AGENTS.md — Pocket Sentinel (go-app)

## 0. Git Synchronization Protocol (CRITICAL)
- **Pull First (ก่อนเริ่มงาน):** ทุกครั้งที่เอเจนต์เริ่มเซสชันหรือรับงานใหม่ ต้องรัน `git pull origin main` ทันทีเสมอเพื่อป้องกัน merge conflict และซิงค์โค้ดล่าสุดจากรีโมท
- **Push Last (เมื่อจบงาน):** เมื่อเอเจนต์ทำงาน ตรวจสอบ ทดสอบ หรือ deploy เสร็จสมบูรณ์แล้ว ต้อง commit การเปลี่ยนแปลง และรัน `git push origin main` ก่อนจบงานเสมอ ห้ามทิ้งงานค้างไว้บน local เด็ดขาด

## 1. Project Overview & Stack
- **Role:** Pocket Sentinel — autonomous health and telemetry monitoring service
- **Stack:**
  - Go (Gin HTTP Framework in ReleaseMode)
  - SQLite: Pure-Go driver (`modernc.org/sqlite`) ห้ามใช้ CGO เพื่อป้องกันปัญหา build บน Android Kernel 4.4
  - Process Manager: PM2 (`ecosystem.config.js` คุม `go-server` และ `ngrok-tunnel`)
  - Public Ingress: `ngrok` (รันผ่าน `run-ngrok.sh` ด้วย termux-chroot)
  - Active Public Endpoint: `https://spindle-said-hemstitch.ngrok-free.dev`
  - Port: `8080` (`http://localhost:8080`)
  - Memory Budget: < 300MB RAM (ปกติรันอยู่ที่ ~12–20MB)

## 2. Development, Build & Deploy Rules (On-Device Independent)
- **Standalone Binary:** คุณสามารถพัฒนา แก้ไขโค้ด และคอมไพล์บน v11 ได้โดยตรงด้วย:
  `go build -o server .`
- **ข้อห้ามเด็ดขาด:** ห้ามสั่ง `pm2 start "go run ..."` เด็ดขาด เพราะจะทำให้เกิด Orphan process กิน RAM มหาศาลและถูก LMK ฆ่าทิ้ง
- **Restart Cycle:** เมื่อคอมไพล์ binary ใหม่เสร็จแล้ว ให้ reload process ด้วย:
  `pm2 restart go-server`
  `pm2 logs go-server --lines 20 --nostream` (CRITICAL: ต้องใส่ `--nostream` ทุกครั้งเพื่อไม่ให้ SSH ค้าง)
- **Deploy Script:** เมื่อ deploy จาก macOS ให้ใช้ `./scripts/deploy.sh -m "คำอธิบายสิ่งที่อัปเดต"` ซึ่งจะ cross-compile ARM64 Android (`CGO_ENABLED=0 GOOS=android GOARCH=arm64`), sync แบบ atomic, และแจ้งเตือนสถานะพร้อมรายละเอียดการอัปเดต (`📦 สิ่งที่อัปเดต:`) ผ่าน Telegram ทันที (หากไม่ระบุ `-m` จะดึงข้อความจาก Git commit ล่าสุดโดยอัตโนมัติ)

## 3. Frontend Architecture & Alpine.js Proxy Guardrails
- **ข้อห้ามเด็ดขาด (Proxy Violation):** ห้ามนำ Object จากภายนอกที่มี prototype ซับซ้อน เช่น Three.js (Scene, Mesh, Renderer, Camera, OrbitControls) หรือ Chart.js (`new Chart()`) ไปเก็บใน Object ของ `Alpine.data(...)` เด็ดขาด
  - **เหตุผล:** Alpine.js จะ wrap ทุก property ด้วย `Proxy` ทำให้เกิด `Uncaught TypeError: 'get' on proxy: property 'modelViewMatrix' is a read-only and non-configurable data property` และทำให้ Chart.js ไม่ render
  - **วิธีปฏิบัติที่ถูกต้อง:** ประกาศตัวแปรเก็บ instance (`const tc = { ... }`, `const chartInstances = { ... }`) ไว้นอก closure ของ `Alpine.data` ใน scope ของไฟล์ แล้วให้ Alpine เก็บเฉพาะ primitive state, text, และ boolean flags เท่านั้น
- **Canvas Sizing & Lifecycle:** Canvas ที่ถูกซ่อนตอนเริ่มต้น (`:hidden` หรือ `x-show`) จะมีขนาด 0x0 ตอน init เมื่อ unhide จะต้องเรียก `chart.resize()` ใน `this.$nextTick()` เสมอ
- **Event Fallback:** ในคอมโพเนนต์ Alpine ห้ามพึ่งพาเฉพาะ DOM CustomEvent (เช่น `sentinel:history-loaded`) อย่างเดียว เพราะอาจเกิด race condition ให้มี fallback `loadHistory()` ใน `init()` เสมอ
- **Service Worker Invalidation:** เมื่อมีการแก้ไขไฟล์ frontend ใน `modules/dashboard/` ต้องอัปเดตเวอร์ชัน `CACHE_NAME` ใน `sw.js` (เช่น `sentinel-shell-v2` -> `sentinel-shell-v3`) เสมอ เพื่อไม่ให้เบราว์เซอร์หรือ PWA ดึงแคชเก่า

## 4. Deterministic Guardrails & Lifecycle Hooks
- โปรเจกต์มี Deterministic Hook อยู่ที่ `.agents/hooks.json` และ `./scripts/hook-validate.sh` ซึ่งรันเป็น `PostToolUse` ดักจับทุกครั้งที่มีการเขียนหรือแก้ไขไฟล์ `.go`
- Hook จะรัน `gofmt -w` และ `go vet ./...` โดยอัตโนมัติเพื่อคืน compiler feedback ให้เอเจนต์ทันที
- Hook script ต้องรองรับ non-blocking input (`[ ! -t 0 ]`) เพื่อไม่ให้ค้างเมื่อรันในเทอร์มินัลโดยไม่มี stdin

## 5. Agent Runner & Telegram Guardrails
- **Prompt Injection Defense:** สกัดกั้นคำสั่งอันตราย (`rm -rf`, DAN mode, system hijack) ด้วย multi-layer validation ใน `modules/bot/guard.go`
- **Dynamic Reasoning Effort:** รองรับการกำหนด effort ผ่าน syntax `/task [low|medium|high|max] <คำสั่ง>` หรือ `--effort=<level>`
- **Progress Heartbeats:** ขณะ agy ประมวลผล ระบบจะอัปเดตข้อความ ACK เดิมด้วย `editMessageText` ทุก 15 วินาที พร้อมแจ้งเวลาและ RAM ป้องกันการสแปมแชท

## 6. Telegram Relay & Gateway API
- **Active Endpoint:** `https://spindle-said-hemstitch.ngrok-free.dev`
- **Unified Relay Endpoint:** `POST /api/relay/send`
- **Auth:** ไม่มี PIN แล้ว (เจ้าของเลือกให้เป็น public ตั้งแต่ 2026-11-20) — Relay, Schedule และ Location management API เรียกได้โดยไม่ต้องส่ง PIN; header `X-Relay-PIN` เดิมถูกละเลย ส่วน OwnTracks secret token (`/api/location/report`) และ viewer token (`/live/<token>`) ยังคงใช้อยู่
- **Auto Detection:**
  - รูปภาพ (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`) ส่งเป็น Telegram Photo พร้อมพรีวิวในแชท
  - ไฟล์เอกสารทั่วไป (PDF, zip, text, code) ส่งเป็น Telegram Document พร้อมชื่อไฟล์เดิม
  - ข้อความส่งเป็น HTML/text
- **Max Upload Limit:** 20 MB
- **Web UI & App Hub:** เข้าถึงได้ผ่านหน้า Dashboard (`/`) ในโซน App Hub & Gateways หรือเปิดตรงผ่าน `/?app=relay` หรือ `/relay` รองรับการกด `⌘+V` / `Ctrl+V` เพื่อวางภาพจากคลิปบอร์ดส่งเข้า Telegram ได้ทันที

## 7. Telegram & Discord Message Templates
- เมื่อสร้างหรือแก้ไขข้อความ Telegram/Discord, alert, command menu หรือ Relay template ให้อ่าน `.agents/skills/telegram-discord-message-templates/SKILL.md` ก่อน
- ใช้ Telegram HTML และ Discord Markdown ให้ตรงกับ sender, escape dynamic content ตาม platform และให้ปุ่ม task คัดลอก draft เพื่อให้เจ้าของตรวจทานก่อนส่ง



## 8. Feature Completion: Reachable, Not Just Written
Feature จะนับว่า **เสร็จ** เมื่อเจ้าของ *reachable* ได้จริงจากทุกช่องทางที่ feature นั้นสัญญาไว้ หลัง deploy:
- **คำสั่ง Telegram:** มี `case` ใน `modules/bot/router.go` + บรรทัดใน `/help` + เทสต์ใน `modules/bot/*_test.go`; ถ้าใช้บ่อยให้เพิ่มปุ่มใน `MakeHelpKeyboard()` ด้วย
- **หน้าเว็บ / UI:** มีทางเข้าจากหน้าที่เจ้าของเปิดอยู่จริง (task nav, การ์ดในหน้าภาพรวม หรือปุ่มบนหน้านั้น) และไฟล์ static ใหม่อยู่ใน `SHELL_ASSETS` ของ `sw.js`
- **งานตามเวลา:** worker ถูก start ใน `main.go` และส่งผลไปยังช่องทางที่สัญญาไว้ (Telegram / dashboard)
- **โค้ดฝั่ง server** (เช่น AI helper ใน `shared/ai/`) จะนับเป็น feature ก็ต่อเมื่อถูกเรียกจาก route, worker หรือคำสั่งแล้ว — ตัวอย่าง: `shared/ai/monitor_insight.go` มีโค้ดแต่ยังไม่ถูกต่อสาย จึงยังไม่ใช่ feature

ในข้อความสรุปงานถึงเจ้าของ ให้แยกสองหัวข้อชัดเจน: **ใช้ได้แล้ว** (พร้อมคำสั่งหรือ URL ที่ลองได้ทันทีหลัง deploy) และ **ยังไม่ได้ทำ / อยู่ในแผน** — งานที่ยังคุยออกแบบอยู่ห้ามเขียนปนกับงานที่ส่งมอบแล้ว
