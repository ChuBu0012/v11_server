# AGENTS.md — Pocket Sentinel (go-app)

## 1. Project Overview & Stack
- **Role:** Pocket Sentinel — autonomous health and telemetry monitoring service
- **Stack:**
  - Go (Gin HTTP Framework)
  - SQLite: Pure-Go driver (`modernc.org/sqlite`) ห้ามใช้ CGO เพื่อป้องกันปัญหา build บน Android Kernel 4.4
  - Process Manager: PM2 (`ecosystem.config.js` คุม `go-server` และ `ngrok-tunnel`)
  - Public Ingress: `ngrok` (รันผ่าน `run-ngrok.sh` ด้วย termux-chroot)
  - Port: `8080` (`http://localhost:8080`)

## 2. Development & Build Rules (On-Device Independent)
- **Standalone Binary:** คุณสามารถพัฒนา แก้ไขโค้ด และคอมไพล์บน v11 ได้โดยตรงด้วย:
  `go build -o server .`
- **ข้อห้ามเด็ดขาด:** ห้ามสั่ง `pm2 start "go run ..."` เด็ดขาด เพราะจะทำให้เกิด Orphan process กิน RAM มหาศาลและถูก LMK ฆ่าทิ้ง
- **Restart Cycle:** เมื่อคอมไพล์ binary ใหม่เสร็จแล้ว ให้ reload process ด้วย:
  `pm2 restart go-server`
  `pm2 logs go-server --lines 20 --nostream`
