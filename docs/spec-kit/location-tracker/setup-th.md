# คู่มือตั้งค่า Location & Rendezvous Tracker

คู่มือนี้ตั้งค่าติดตามตำแหน่งระหว่างโทรศัพท์ Android สองเครื่อง โดยเครื่องเจ้าของใช้รหัส `me` และเครื่องเพื่อนใช้ `friend` ข้อมูลจะส่งผ่าน HTTPS มายัง Pocket Sentinel, บันทึกใน SQLite และส่งเหตุการณ์สำคัญไปยัง Discord

## 1. เตรียม Pocket Sentinel

1. ตั้งค่า `LOCATION_SECRET_TOKEN` และ `LOCATION_VIEWER_TOKEN` ใน `.env` ให้เป็นคนละค่า โดยสร้างค่า 32 ตัวอักษรด้วยคำสั่งนี้สองครั้ง:

   ```sh
   openssl rand -hex 16
   ```

   ตัวแรกใช้ใน OwnTracks ของทั้งสองเครื่อง ตัวที่สองเป็นรหัสในลิงก์แผนที่ เก็บสองค่านี้เป็นความลับ

2. ตั้ง `DISCORD_WEBHOOK_URL` เป็น Incoming Webhook ของห้อง Discord ที่ต้องการรับแจ้งเตือน และตั้ง `RELAY_PIN` สำหรับเปิดเมนูจัดการสถานที่/การแจ้งเตือน
3. หากกำลัง deploy จาก macOS ให้ใช้ `./scripts/deploy.sh` ตามขั้นตอนใน `AGENTS.md` แล้วตรวจสอบว่า Pocket Sentinel เข้าถึงได้ผ่าน HTTPS
4. เก็บค่า token ใน `.env` ให้ตรงกับค่าที่ใช้ระหว่างตั้งค่า ออกจากระบบและเริ่มใหม่แล้วค่าจะยังเดิม

> หากไม่ได้กำหนด token ใน `.env` ระบบจะสร้าง token แบบสุ่มและบันทึกใน SQLite เพื่อคงค่าเดิมหลัง restart แต่แนะนำให้กำหนดเอง เพราะ OwnTracks และลิงก์แผนที่ต้องใช้ token เหล่านี้

## 2. ตั้งค่า OwnTracks บนเครื่องเจ้าของ

ติดตั้ง **OwnTracks** จากแหล่งแอปที่ใช้อยู่ แล้วตั้งค่าดังนี้:

1. เปิด OwnTracks → **Settings** → **Mode** แล้วเลือก **HTTP**
2. ตั้ง URL เป็น `https://spindle-said-hemstitch.ngrok-free.dev/api/location/report?device=me&token=<LOCATION_SECRET_TOKEN>` โดยแทน `<LOCATION_SECRET_TOKEN>` ด้วยค่าจริงจาก `.env` และไม่ใส่วงเล็บมุม
3. เลือก Monitoring เป็น **Significant Changes** เพื่อประหยัดแบตเตอรี่ หรือ **Move** หากต้องการอัปเดตถี่ขึ้น
4. ตั้ง Tracker ID ให้จำง่าย เช่น `ME`
5. อนุญาต Location ให้ OwnTracks ทำงานเบื้องหลัง เปิด notification ของแอป และตั้ง Battery usage เป็น **Unrestricted / Don't optimize** หาก Android หยุดส่งตำแหน่งขณะล็อกจอ
6. กด **Publish / Report location** หนึ่งครั้ง แล้วตรวจว่าแอปส่งสำเร็จ

ตัวอย่าง URL เมื่อใช้เครื่องเจ้าของ:

```text
https://spindle-said-hemstitch.ngrok-free.dev/api/location/report?device=me&token=0123456789abcdef0123456789abcdef
```

## 3. ตั้งค่า OwnTracks บนเครื่องเพื่อน

ทำขั้นตอนเดียวกับเครื่องเจ้าของ แต่ตั้ง URL และ Tracker ID ดังนี้:

```text
https://spindle-said-hemstitch.ngrok-free.dev/api/location/report?device=friend&token=<LOCATION_SECRET_TOKEN>
```

ทั้งสองเครื่องใช้ `LOCATION_SECRET_TOKEN` ค่าเดียวกัน ส่วน `device` ต้องต่างกัน: เจ้าของใช้ `me`, เพื่อนใช้ `friend` ห้ามส่ง URL นี้ให้บุคคลอื่น เพราะ URL มีสิทธิ์ส่งตำแหน่งเข้าระบบ

## 4. เปิดแผนที่และจัดการสถานที่

เปิดลิงก์ส่วนตัวนี้ในเบราว์เซอร์ของโทรศัพท์หรือคอมพิวเตอร์ โดยแทน `<LOCATION_VIEWER_TOKEN>` ด้วย token จริง:

```text
https://spindle-said-hemstitch.ngrok-free.dev/live/<LOCATION_VIEWER_TOKEN>
```

แผนที่จะดึงข้อมูลใหม่ทุก 15 วินาที แสดงตำแหน่งล่าสุด ระยะห่าง แบตเตอรี่ ความเร็ว ความแม่นยำ วง geofence และเส้นทางย้อนหลัง 24 ชั่วโมง ปุ่ม `⌁` เปิด/ปิดเส้นทางย้อนหลัง

กดไอคอนตั้งค่ามุมขวาบน แล้วกรอก `RELAY_PIN` เพื่อ:

- เปิด/ปิดแจ้งเตือนพบกัน/แยกย้าย, เข้า/ออก geofence และแบตเตอรี่ต่ำ
- เพิ่ม geofence ด้วยชื่อ พิกัดละติจูด/ลองจิจูด และรัศมีเป็นเมตร
- ลบ geofence ที่ไม่ต้องการ

การเพิ่ม geofence ครั้งแรกที่อุปกรณ์อยู่ข้างในจะนับเป็นการเข้า และส่งการแจ้งเตือนตามตัวเลือกของ geofence

## 5. เงื่อนไขแจ้งเตือน

| เหตุการณ์ | เงื่อนไข |
|---|---|
| พบกันแล้ว | ตำแหน่งทั้งสองมีความแม่นยำไม่เกิน 100 ม. และระยะห่างไม่เกิน 80 ม. |
| แยกย้ายกันแล้ว | อยู่ในสถานะพบกันแล้ว และระยะห่างตั้งแต่ 250 ม. ขึ้นไป |
| ป้องกัน GPS แกว่ง | เว้นอย่างน้อย 5 นาทีระหว่างการเปลี่ยนสถานะพบกัน/แยกย้าย; ระยะ 81–249 ม. ไม่ทำให้แยกย้าย |
| เข้า/ออกสถานที่ | อุปกรณ์ข้ามขอบรัศมี geofence ที่เปิดแจ้งเตือน |
| แบตเตอรี่ต่ำ | แบตเตอรี่ไม่เกิน 15% และสถานะเป็นไม่ได้เสียบชาร์จ; เตือนซ้ำทุก 2 ชั่วโมง |
| รีเซ็ตเตือนแบตเตอรี่ | เมื่อแบตเตอรี่สูงกว่า 25% จะรีเซ็ตช่วงพักเตือน |
| ตรวจพิกัด | ปฏิเสธพิกัดนอกช่วงละติจูด −90 ถึง 90, ลองจิจูด −180 ถึง 180, ความแม่นยำ 0 หรือ timestamp ที่เร็วกว่าเวลารับเกิน 2 นาที; sample ที่ซ้ำ/เก่ากว่าตำแหน่งล่าสุดจะถูก ACK แต่ไม่บันทึกหรือประเมิน alert |
| เก็บข้อมูล | เก็บจุดย้อนหลัง 7 วัน; แผนที่แสดง breadcrumb 24 ชั่วโมงล่าสุด |

การแจ้งเตือนพบกันจะแสดงชื่อ geofence เมื่ออุปกรณ์ทั้งคู่ยังอยู่ในวงเดียวกัน หากไม่มีสถานที่ลงทะเบียนจะแสดงพิกัดโดยประมาณ Discord ใช้เวลาเขต ICT (UTC+7)

## 6. ตรวจสอบเมื่อยังไม่เห็นตำแหน่ง

1. ตรวจว่าเปิด URL ถูกเครื่อง (`me`/`friend`) และ token ยังตรงกับ `.env`
2. ตรวจ HTTPS/tunnel และกด Publish ใน OwnTracks ขณะเปิดแอป
3. ตรวจสิทธิ์ Location แบบเบื้องหลัง, notification, และ battery optimization ของ Android
4. ตรวจ log ของ Pocket Sentinel โดยไม่คัดลอกหรือส่ง token ออกไป
5. ลิงก์แผนที่ต้องใช้ `LOCATION_VIEWER_TOKEN`; เมนูตั้งค่าต้องใช้ `RELAY_PIN`

ตำแหน่งปัจจุบันเป็นข้อมูลส่วนบุคคล อย่าโพสต์ viewer link, OwnTracks URL, token หรือ Relay PIN ในแชตสาธารณะ

## เอกสาร OwnTracks

- [HTTP mode และ response acknowledgement](https://owntracks.org/booklet/tech/http/)
- [การตั้งค่า OwnTracks บน Android](https://owntracks.org/booklet/guide/app/android/)
- [Android background operation และ battery optimization](https://owntracks.org/booklet/features/android/)
