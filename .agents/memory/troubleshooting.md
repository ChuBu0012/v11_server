# Pocket Sentinel — Incident & Troubleshooting Knowledge Base

บันทึกประวัติปัญหาทางเทคนิค สาเหตุที่แท้จริง (Root Cause) และแนวทางแก้ไขเพื่อป้องกันไม่ให้เกิดขึ้นซ้ำในอนาคต

---

## 1. Gatus SPA Subpath Reverse Proxy (ภาพแตก 404 + หน้าจอว่างเปล่า)

- **วันที่เกิดเหตุ:** 2026-11-21
- **อาการที่พบ (Symptoms):**
  - เปิด `https://spindle-said-hemstitch.ngrok-free.dev/status/` แล้วโหลดหัวเว็บ "vivo V11 Status Page" ขึ้นมา แต่ตัวเนื้อหาแดชบอร์ดด้านล่างเป็นจอสีดำว่างเปล่า ไม่แสดงรายการ endpoints
  - ไอคอนโลโก้ Gatus มุมบนซ้ายแตก (Broken Image)
  - Console เบราว์เซอร์แจ้ง Error: `GET /img/logo.svg 404 (Not Found)`
  - Unit Test ของ Go ที่ทดสอบ Reverse Proxy เกิด `panic: interface conversion: *httptest.ResponseRecorder is not http.CloseNotifier`

- **สาเหตุที่แท้จริง (Root Cause):**
  1. **Root Asset 404:** Webpack/Vite คอมไพล์ไฟล์ SPA โดยอ้างอิง Root Path (`publicPath: "/"`) เสมอ เมื่อเปิดหน้าเว็บผ่าน Subpath `/status/` เบราว์เซอร์จะยังคงร้องขอ `/img/logo.svg`, `/favicon.ico` ที่ Host Root (`/`) ซึ่งเดิมใน `main.go` ยังไม่ได้เขียน Route ดักส่งต่อไปยัง Gatus
  2. **The Silent Router Trap:** Vue Router ในไฟล์ `app.js` ของ Gatus ถูกคอมไพล์ด้วย `createWebHistory("/")` แบบ Hardcode เมื่อเข้าผ่าน `/status/` ตัว Router จะนำ `/status/` ไปเทียบกับ Routes ที่มี (`/`, `/endpoints/:key`) ซึ่งไม่ตรงกับอันใดเลย ทำให้ `<router-view>` ไม่แสดงเนื้อหา
  3. **PWA Service Worker Hijack:** Service Worker (`sw.js`) ของ Pocket Sentinel ดักจับทุก Request ใต้โดเมน เมื่อ asset เกิด 404 ตัว Service Worker หรือ HTTP Cache จะจำผลลัพธ์นั้นไว้ ทำให้ถึงแม้จะแก้ที่เซิร์ฟเวอร์แล้ว ผู้ใช้ก็ยังคงเห็นภาพแตกจากแคชเดิม
  4. **Gin ResponseWriter กับ CloseNotifier:** `httputil.ReverseProxy` ของ Go จะ type-assert `ResponseWriter` ว่า implement `http.CloseNotifier` หรือไม่ แต่ `httptest.NewRecorder()` ในชุดทดสอบไม่ได้ implement เมธอดนี้ ทำให้เทสต์ panic

- **วิธีแก้ไข (Solution):**
  1. **เพิ่ม Route Proxy ครบถ้วนใน `main.go`:**
     - Proxy ทั้ง `/status/*path`, `/api/v1/*path`, `/js/*path`, `/css/*path`, `/img/*path` และ Favicons ทั้งหมดไปยัง `http://127.0.0.1:8081`
     - ส่ง Header `Cache-Control: no-cache, no-store, must-revalidate`
  2. **แพตช์ Dynamic Router Base ใน Gatus:**
     - แก้ไข JavaScript ในตัวไบนารี Gatus ให้ตรวจจับ Subpath แบบไดนามิก:
       ```javascript
       history: createWebHistory(location.pathname.startsWith("/status") ? "/status/" : "/")
       ```
     - ทำให้ใช้งานได้ทั้งการเข้าผ่าน Subpath Proxy (`/status/`) และการเข้าผ่าน Private Tailscale Root (`http://100.92.194.87:8081/`)
  3. **Bypass PWA Service Worker (`sw.js`):**
     - ใส่เงื่อนไขข้ามการแคชสำหรับ `/status`, `/img/`, `/js/`, `/css/`, `/api/` ใน Event `fetch` ให้ส่งตรงเข้า Network ทันที
     - ปรับเวอร์ชัน `CACHE_NAME` ใน `sw.js` เป็น `sentinel-shell-v42` เพื่อล้างแคชเก่าทิ้ง
  4. **ใช้ `testCloseNotifierRecorder` ใน Unit Test:**
     - ครอบ `httptest.ResponseRecorder` ด้วย struct ที่รองรับ `CloseNotify() <-chan bool`

---

## 2. SQLite Connection Pool Lock Contention (`SQLITE_BUSY: database is locked (5)`)

- **วันที่เกิดเหตุ:** 2026-11-21
- **อาการที่พบ (Symptoms):**
  - OwnTracks Location Report ล้มเหลว มี Error ใน `go-server-error.log`:
    `store location: insert history: database is locked (5) (SQLITE_BUSY)`
    `query latest locations: context canceled`
  - พิกัดของ `device=me` หยุดอัปเดตตั้งแต่เวลาประมาณ 18:24
  - มี Request ยิงเข้ามายัง API `/api/location/report` ถี่มากระดับทุก ๆ 100–200ms จากอุปกรณ์ `friend`

- **สาเหตุที่แท้จริง (Root Cause):**
  1. **PRAGMA busy_timeout ติดแค่ Connection เดียวใน Pool:** ใน Go `database/sql` หากสั่ง `db.Exec("PRAGMA busy_timeout = 5000")` หลังเปิด DB จะมีผลแค่กับ Connection แรกที่หยิบมาประมวลผลเท่านั้น ส่วน Connection อื่น ๆ ใน Pool (`SetMaxOpenConns(3)`) ที่ถูกเปิดขึ้นมาใหม่ในภายหลังจะมี `busy_timeout = 0` (Fail Fast ทันทีที่มีการแย่ง Write Lock)
  2. **Context Timeout สั้นเกินไป:** `storeOperationTimeout` ใน `modules/location/store.go` ตั้งไว้เพียง `3 * time.Second` เมื่อ Connection พยายามรอ Lock แต่ Context ถูกตัดจบก่อน จึงเกิดข้อผิดพลาด `context canceled`
  3. **Stationary GPS Jitter สแปมฐานข้อมูล:** เมื่อโทรศัพท์หยุดนิ่งหรือชาร์จไฟ OwnTracks ในโหมดความแม่นยำสูงยังคงส่งพิกัดซ้ำ ๆ ทุก 100ms–1s โดยที่พิกัดแทบไม่เปลี่ยน (< 5 เมตร) ส่งผลให้เซิร์ฟเวอร์ต้องรัน SQL 6–8 คำสั่งต่อรอบแย่ง Write Lock ตลอดเวลา

- **วิธีแก้ไข (Solution):**
  1. **กำหนด PRAGMAs ใน DSN String (`shared/db/db.go`):**
     - กำหนด PRAGMAs ลงใน DSN Query Parameters โดยตรง:
       `_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)...`
     - เพื่อรับประกันว่า **ทุก Connection ใน Connection Pool** จะได้รับ `busy_timeout = 10000ms` (10 วินาที) โดยอัตโนมัติตั้งแต่ตอนถูกสร้าง
  2. **ขยาย `storeOperationTimeout`:** เพิ่มเวลาจาก 3 วินาทีเป็น 10 วินาที เพื่อให้สอดคล้องกับ `busy_timeout`
  3. **Smart Stationary Debounce (`modules/location/service.go`):**
     - ตรวจสอบพิกัดใหม่ก่อนบันทึกลงฐานข้อมูล หากพิกัดเดิมกับพิกัดใหม่อยู่ห่างกันไม่เกิน 5.0 เมตร, ห่างจากครั้งก่อนไม่เกิน 10 วินาที, แบตเตอรี่เท่าเดิม และความแม่นยำไม่ดีขึ้น ให้ข้ามการบันทึกฐานข้อมูลทันที (`return nil, nil`)
     - ลดภาระการเขียน SQL ได้มากกว่า 95% เมื่ออุปกรณ์อยู่นิ่ง

---

## 3. ngrok Bandwidth Quota Exhaustion (`ERR_NGROK_725`)

- **วันที่เกิดเหตุ:** 2026-11-21
- **อาการที่พบ (Symptoms):**
  - การเข้าถึง Pocket Sentinel หรือ Gatus ผ่านโดเมนสาธารณะ `https://spindle-said-hemstitch.ngrok-free.dev/` ล้มเหลวทุกเส้นทาง
  - เซิร์ฟเวอร์ตอบกลับเป็น `HTTP 403 Forbidden` พร้อม Error Code: `ERR_NGROK_725`
  - ข้อความแสดง: `This ngrok account has reached its network bandwidth limit for the month.`

- **สาเหตุที่แท้จริง (Root Cause):**
  - บัญชีฟรีของ ngrok มีโควต้า Data Transfer จำกัดที่ 1 GB ต่อเดือน
  - โทรศัพท์มือถือเปิดแอป OwnTracks ในโหมด "Move" (ส่งพิกัดต่อเนื่องตามความถี่สูง) ส่งพิกัดเข้ามาสะสมมากกว่า 33,900 คำขอในเวลาเพียงไม่กี่ชั่วโมง ทำให้สูบปริมาณข้อมูลจนเกิน 1 GB

- **วิธีแก้ไขและแนวทางป้องกัน (Solution & Prevention):**
  1. **สลับหรือรีเซ็ต Authtoken ของ ngrok:** สร้างหรือใช้บัญชีใหม่เพื่อนำ Token ใหม่มารัน Tunnel ต่อ
  2. **ปรับแต่งการตั้งค่าแอป OwnTracks บนมือถือ:**
     - ในแอป OwnTracks ให้เปลี่ยนโหมดจาก **"Move"** เป็น **"Significant Changes"**
     - ตั้งค่า `locatorDisplacement` เป็นอย่างน้อย 15–20 เมตร และ `locatorInterval` เป็น 60–120 วินาที
  3. **ใช้ Tailscale เป็นช่องทางสำรองที่ไม่มีขีดจำกัดแบนด์วิดท์:**
     - เข้าถึงระบบผ่าน Private Tailscale Mesh IP: `http://100.92.194.87:8080/`
     - สำหรับอุปกรณ์ส่วนตัว เช่น OwnTracks ของตนเอง สามารถตั้ง Endpoint วิ่งผ่าน Tailscale IP ได้โดยตรง ไม่เสียโควต้าของ ngrok
