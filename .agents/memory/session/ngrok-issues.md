# ปัญหาและอุปสรรคของการใช้งาน ngrok บน vivo V11 (Android 10 / Termux)

**วันที่บันทึก:** 14 พฤศจิกายน 2026  
**อุปกรณ์เป้าหมาย:** vivo 1804 (V11), Snapdragon 660, Android 10 (Non-root), Linux Kernel 4.4, Termux ARM64  
**วัตถุประสงค์:** สรุป Root Cause และปัญหาทั้งหมดที่พบจากการใช้ `ngrok` เป็น Public Tunnel เพื่อใช้เป็นบทเรียนสำหรับทีมและประเมินทางเลือกอื่นในอนาคต

---

## 1. บทสรุปปัญหา (Executive Summary)

การนำ `ngrok` มาใช้งานเป็น Public Tunnel บน Android Non-root ผ่าน Termux ก่อให้เกิดปัญหาและความล่าช้าในการพัฒนาสูงมาก (High Maintenance Overhead) เนื่องจาก `ngrok` เป็นซอฟต์แวร์แบบ Closed-source Binary ที่ถูกออกแบบมาสำหรับ Linux Distribution ทั่วไป ไม่ได้ออกแบบมาสำหรับ Android Bionic libc และติดข้อจำกัดด้าน Linux Kernel 4.4 รวมถึงระบบจัดการ Process ของ Termux + PM2

---

## 2. รายละเอียดปัญหาและสาเหตุเชิงลึก (Detailed Root Causes)

### 2.1 Syscall Crash บน Linux Kernel 4.4 (`faccessat2` / `bad system call`)
* **อาการ:** รันคำสั่ง `ngrok http 8080` ตรงๆ บน Termux แล้ว Process หยุดทำงานทันที (Crash / Aborted / Bad system call)
* **สาเหตุ:**
  * vivo V11 ทำงานบน **Linux Kernel 4.4**
  * Binary ของ ngrok ARM64 ถูกคอมไพล์ด้วย glibc/Go รุ่นใหม่ที่เรียกใช้ Syscall ยุคหลัง เช่น `faccessat2` (syscall 439) ซึ่งไม่มีอยู่ใน Kernel 4.4 ของ Android 10
* **วิธีแก้ชั่วคราว:** ต้องรันผ่าน `termux-chroot` (PRoot environment) เพื่อจำลอง System Calls และ Filesystem Hierarchy Standard (FHS) ซึ่งเพิ่มเลเยอร์จำลองและ overhead

---

### 2.2 ปัญหา TLS / Root CA Certificate หาย (`x509: certificate signed by unknown authority`)
* **อาการ:** เมื่อรันผ่าน `termux-chroot` แล้ว ngrok ไม่สามารถเชื่อมต่อกับ Cloud Relay ได้ โดยแจ้งเตือนข้อผิดพลาดเรื่อง SSL/TLS Handshake
* **สาเหตุ:**
  * ภายใน `termux-chroot` ngrok พยายามค้นหา CA Certificate ตามพาทมาตรฐานของ Linux เช่น `/etc/ssl/certs/ca-certificates.crt`
  * บน Termux ไฟล์ Root CA ตั้งอยู่ที่ `/data/data/com.termux/files/usr/etc/tls/cert.pem` ทำให้ ngrok มองไม่เห็นใบรับรอง
* **วิธีแก้ชั่วคราว:** ต้องระบุตัวแปรสภาพแวดล้อมบังคับ:
  ```bash
  export SSL_CERT_FILE=/etc/tls/cert.pem
  ```

---

### 2.3 ปัญหา Process ค้างเมื่อรัน Background ใต้ PM2 (Non-TTY Hang & Output Buffering)
* **อาการ:**
  * เมื่อให้ PM2 จัดการ Process `ngrok` ผ่าน `ecosystem.config.js` คำสั่งตรวจสอบสถานะผ่าน SSH จะค้าง (Hang)
  * เมื่อเรียกดู Log ผ่าน `pm2 logs` บน SSH จะค้างจน timeout
* **สาเหตุ:**
  * ngrok มีหน้าจอ Terminal UI (TUI) แบบ interactive และมีระบบ Log Buffering
  * เมื่อไม่มี Pseudo-TTY (TTY-less environment ใน PM2) ngrok จะพยายาม flush ข้อมูลหรือรอ TTY event ทำให้ Process หรือ I/O stream ค้าง
* **วิธีแก้ชั่วคราว:**
  * ต้องสร้าง Wrapper Script [`run-ngrok.sh`](file:///Users/most-too/Desktop/v11_server/run-ngrok.sh)
  * บังคับพารามิเตอร์ส่ง log ลงไฟล์ตรงๆ: `--log=/home/go-app/ngrok.log`
  * บังคับใช้ flag `--nostream` ทุกครั้งที่เรียก `pm2 logs` ผ่าน SSH

---

### 2.4 ปัญหา Zombie / Orphaned PRoot Processes และ Port Conflict (Port 4040)
* **อาการ:**
  * เมื่อสั่ง `pm2 restart ngrok-tunnel` หรือ deploy โค้ดใหม่ ngrok ตัวใหม่ไม่สามารถเปิดทำงานได้ และฟ้อง `address already in use` ที่พอร์ต `4040` (Inspection Web UI)
  * เกิด Error ซ้ำซ้อน: `ERR_NGROK_108: You may only run one agent session at a time on your account`
* **สาเหตุ:**
  * `run-ngrok.sh` รันผ่าน `termux-chroot` (ซึ่งใช้ `proot` และ `ptrace` ในการ hook syscalls)
  * เมื่อ PM2 สั่ง Stop/Restart มันจะส่ง `SIGTERM` ให้แก่ Script แม่ แต่ Process ลูก (`proot` และ `ngrok`) ไม่ยอมตายตาม กลายเป็น Zombie/Orphan process ค้างอยู่ใน Background แย่งพอร์ตและครอง Tunnel Session ไว้
* **วิธีแก้ชั่วคราว:**
  * ต้องสั่ง kill แบบถอนรากถอนโคนก่อน deploy เสมอ:
    ```bash
    pkill -9 -f ngrok || true
    pkill -9 -f proot || true
    ```

---

### 2.5 ข้อจำกัดโควตา Free Plan (1 Active Session)
* **อาการ:** เมื่อมี Session ค้าง (จากปัญหาข้อ 2.4) แล้วรัน Process ใหม่ ngrok จะถูกบล็อกทันที
* **ผลกระทบ:** ทำให้ขั้นตอน Automated Deploy บน CI/CD หรือผ่านสคริปต์ `deploy.sh` ล้มเหลวแบบไม่แน่นอน (Flaky Deployments)

---

### 2.6 Android Power Management / Doze Mode ตัดการเชื่อมต่อ Tunnel
* **อาการ:** เมื่อวางโทรศัพท์ทิ้งไว้จนหน้าจอดับ (Screen Off / Sleep) Tunnel ของ ngrok จะหลุดบ่อยครั้ง แม้ว่าจะเปิด `termux-wake-lock` แล้วก็ตาม
* **สาเหตุ:** การเชื่อมต่อ Long-lived TCP Socket ของ ngrok Relay ถูก Doze Mode ของ Android 10 บีบ Bandwidth หรือพัก Network Radio ต่างจากระบบ VPN/WireGuard ของ Tailscale ที่เกาะผ่าน Kernel/VpnService ได้เสถียรกว่า

---

## 3. สรุปความซับซ้อนที่ต้องเพิ่มเข้ามาเพื่อรองรับ ngrok

เพื่อให้ ngrok ทำงานได้บนโปรเจกต์นี้ ต้องสร้าง Boilerplate และ Workaround เพิ่มขึ้นมาจำนวนมาก:
1. ไฟล์ [`run-ngrok.sh`](file:///Users/most-too/Desktop/v11_server/run-ngrok.sh) ห่อหุ้ม `termux-chroot` + `SSL_CERT_FILE` + `--log`
2. การปรับ [`ecosystem.config.js`](file:///Users/most-too/Desktop/v11_server/ecosystem.config.js) ให้รองรับการอ่าน Token, Static Domain และแยก Process จาก Go Server
3. การปรับ [`scripts/status.sh`](file:///Users/most-too/Desktop/v11_server/scripts/status.sh) ให้ curl เช็ก localhost:4040 ด้วย regex แทนการอ่าน CLI ปกติ
4. คำสั่งพิเศษในการจัดการ Zombie Process ก่อนการ Deploy

---

## 4. ข้อเสนอแนะทางเลือกที่ดีกว่าในระยะยาว (Alternative Recommendations)

หากต้องการลดความซับซ้อน เพิ่มความเร็วในการพัฒนา และหลีกเลี่ยงปัญหาของ ngrok แนะนำพิจารณาทางเลือกเหล่านี้:

1. **Tailscale Funnel / Serve (แนะนำสูงสุด):**
   * **ข้อดี:** ทำงานผ่าน Tailscale Native บน Android ซึ่งรองรับการจัดการพลังงานของ Android ได้ดีมาก, ไม่มีปัญหาเรื่อง Syscall หรือ Root CA, ไม่ต้องใช้ PRoot
   * **ความเสถียร:** สูงมาก ไม่เกิด Zombie process
2. **Cloudflare Tunnel (`cloudflared` pure-go or reverse proxy):**
   * **ข้อดี:** ฟรี ไม่จำกัด Session, รองรับ Custom Domain ของตัวเองได้ฟรี, รองรับ zero-trust access
   * **ข้อสังเกต:** ต้องใช้ binary ที่คอมไพล์ด้วย pure Go สำหรับ Android ARM64 เพื่อหลีกเลี่ยง libc issues
3. **SSH Reverse Tunnel (เช่น `bore.pub` หรือ VPS ส่วนตัว):**
   * **ข้อดี:** รันคำสั่ง `ssh -R ...` สั้นๆ คำสั่งเดียว ใช้ OpenSSH ใน Termux ได้ทันที ไม่ต้องมี Third-party agent
