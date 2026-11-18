# AGENTS.md — Pocket Sentinel (go-app)

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
- **Deploy Script:** เมื่อ deploy จาก macOS ให้ใช้ `./scripts/deploy.sh` ซึ่งจะ cross-compile ARM64 Android (`CGO_ENABLED=0 GOOS=android GOARCH=arm64`), sync แบบ atomic, และแจ้งเตือนสถานะผ่าน Telegram

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
- **Auth (PIN Protection):** ต้องส่ง PIN ผ่าน Header `X-Relay-PIN: 1804`, Query `?pin=1804`, หรือ `Authorization: Bearer 1804` (Default PIN คือ `1804` ตั้งค่าผ่าน `RELAY_PIN` ใน `.env`)
- **Auto Detection:**
  - รูปภาพ (`.png`, `.jpg`, `.jpeg`, `.gif`, `.webp`) ส่งเป็น Telegram Photo พร้อมพรีวิวในแชท
  - ไฟล์เอกสารทั่วไป (PDF, zip, text, code) ส่งเป็น Telegram Document พร้อมชื่อไฟล์เดิม
  - ข้อความส่งเป็น HTML/text
- **Max Upload Limit:** 20 MB
- **Web UI & App Hub:** เข้าถึงได้ผ่านหน้า Dashboard (`/`) ในโซน App Hub & Gateways หรือเปิดตรงผ่าน `/?app=relay` หรือ `/relay` รองรับการกด `⌘+V` / `Ctrl+V` เพื่อวางภาพจากคลิปบอร์ดส่งเข้า Telegram ได้ทันที


