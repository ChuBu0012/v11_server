---
name: telegram-discord-message-templates
description: Use when creating or changing Pocket Sentinel Telegram or Discord messages, alerts, command menus, Relay composer templates, or message-formatting helpers.
---

# Telegram and Discord Message Templates

Use this skill whenever a change writes, edits, or formats text sent to Telegram or Discord in this repository.

## Workflow

1. Trace the message from its producer to the platform sender. Read the current command, alert, Relay, or digest path before editing.
2. Name the message's purpose and the next action the reader should take.
3. Keep the template short and ordered: title, key facts, optional next action, timestamp or source when useful.
4. Put dynamic values in named placeholders such as `{condition}`, `{value}`, and `{time}`. Keep data collection separate from rendering.
5. Render the same meaning in each platform's native format; do not send Telegram HTML to Discord directly.
6. Review empty values, special characters, long values, and action-button behavior before finishing.

## Telegram

- Use the parse mode configured by the sender. Pocket Sentinel bot and alerts use HTML.
- Use a short `<b>` title, readable line breaks, and `<code>` only for commands, identifiers, or exact values.
- Escape every dynamic value before inserting it into HTML. Never interpolate user text into markup unescaped.
- Keep command examples copyable. Use Telegram inline `copy_text` for command drafts; copying a task draft must never execute it.
- Use callback buttons only for actions that are explicitly safe and already supported by the bot.
- Keep ordinary bot messages comfortably below Telegram's 4096-character text limit after entity parsing.

## Discord

- Use Discord Markdown (`**bold**`, `*italic*`, and backticks); remove or translate Telegram HTML before sending.
- Use a short heading and compact inline details. Keep webhook content below Discord's 2000-character message limit; this project truncates near 1950 characters.
- Prevent unintended mentions from dynamic content. Escape or suppress `@everyone`, role, and user mentions unless notification is an explicit feature.
- Use embeds only when the current notifier supports structured embeds; do not make a content-only webhook depend on embed rendering.

## Minimal Notification Pattern (Noti แบบเรียบ)

สำหรับ Notification ที่แจ้งเตือนเหตุการณ์สด (เช่น Live Map, Geofence, Hardware Events) ให้ยึดโครงสร้างแบบ **2 บรรทัดกระชับ (Minimal 2-Line)** เสมอ:

1. **บรรทัดที่ 1 (Event Header):** Emoji + ตัวหนา `**` สรุปสิ่งที่เกิดขึ้นทันที (ไม่มีคำเยิ่นเย้อ เช่น ไม่ใส่ "แล้ว", "อยู่ที่นี่ด้วยกันแล้ว")
2. **บรรทัดที่ 2 (Inline Context):** ข้อมูลเสริมรวบยอดคั่นด้วย middle dot (`·`)
3. **ตัดสิ่งรบกวนสายตา (Zero-Clutter Policy):**
   - **ห้ามใช้ Bullet (`•`)** กับ push notification สั้นๆ
   - **ห้ามใส่พิกัดตัวเลขดิบ (Lat/Long)** เช่น `13.7462, 100.5348`
   - **เวลาใช้ `HH:mm`** (เช่น `14:15`) ตัดวินาทีและชื่อโซน (`ICT`) ออก
   - **แบตเตอรี่ใช้ไอคอน `🔋`** และต่อท้ายด้วย `⚡` เมื่อกำลังชาร์จ (เช่น `🔋 85% ⚡`)
   - **ความเร็ว** แสดงเฉพาะเมื่อมีการเคลื่อนที่ชัดเจน (เช่น `≥ 5 km/h`)

## Location & Geofence Alert Templates (Discord)

### 1. เมื่อมาพบกัน (Co-location Meet)
* **กรณีอยู่ในสถานที่ (Geofence):**
```md
🤝 **Chubu & Furturia อยู่ด้วยกันที่ {place}**
ห่าง {distance} · {time} · 🔋 Chubu {batt1} · Furturia {batt2}
```
* **กรณีอยู่นอกสถานที่:**
```md
🤝 **Chubu & Furturia อยู่ด้วยกันแล้ว**
ห่าง {distance} · {time} · 🔋 Chubu {batt1} · Furturia {batt2}
```

### 2. เมื่อแยกย้ายกัน (Co-location Separate)
```md
👋 **Chubu & Furturia แยกย้ายกัน**
ห่าง {distance} · {time}
```

### 3. เมื่อถึงสถานที่ (Geofence Enter)
```md
📍 **{device} ถึง {place}**
{time} · 🔋 {battery}
```

### 4. เมื่อออกจากสถานที่ (Geofence Exit)
```md
🚗 **{device} ออกจาก {place}**
{time} · {speed} · 🔋 {battery}
```
*(หากความเร็ว < 5 km/h ให้ละเว้น `{speed}` เพื่อความกระชับ)*

### 5. แบตเตอรี่ต่ำวิกฤต (Battery Critical ≤ 15% และไม่ได้ชาร์จ)
```md
🪫 **แบตเตอรี่ {device} เหลือ {percent}%**
{time} · {charging_status}
```

## Telegram Live Map Status Card (`/map`)

เมื่อผู้ใช้เรียกดูสถานะสดผ่าน Telegram bot:
```html
📍 <b>Live Map</b> · 🟢 <b>อยู่ด้วยกัน</b> (ห่าง {distance})

• <b>Chubu:</b> 🔋 {battery} · {speed} · {time_ago}
• <b>Furturia:</b> 🔋 {battery} · {speed} · {time_ago}
```
*(ปุ่ม Inline Keyboard: <code>[ 🗺️ เปิดแผนที่สด ]</code> ไม่ต้องมีข้อความแนะนำปุ่มเพิ่มเติม)*

## Pocket Sentinel Template Set

Keep each template editable and explicit about placeholders:

- **Note:** subject and details.
- **Incident report:** symptom, observed time, and checks already tried.
- **Progress update:** done, in progress, blocked, and next step.
- **Diagnostic request:** target service or metric and requested time window.
- **Alert/recovery:** condition, measured value, threshold or result, and local timestamp.
- **Lifecycle notice:** clear startup, shutdown, or restart state with PID, reason when available, and local timestamp.
- **Task draft:** effort and a specific request, copied to the user's input for review before sending.

Example Telegram alert:

```html
🚨 <b>ตรวจพบเงื่อนไข</b>
<b>รายการ:</b> {condition}
<b>ค่าที่วัดได้:</b> <code>{value}</code> (เกณฑ์ <code>{threshold}</code>)
<b>เวลา:</b> {time}
```

Equivalent Discord content:

```md
🚨 **ตรวจพบเงื่อนไข**
• **รายการ:** {condition}
• **ค่าที่วัดได้:** `{value}` (เกณฑ์ `{threshold}`)
• **เวลา:** {time}
```

## Review Checklist

- The first line tells the reader what happened.
- Each dynamic value is escaped for its destination format.
- Empty values are omitted or replaced with a clear unavailable label.
- Command drafts remain editable and never run on copy.
- The Telegram and Discord versions carry the same facts and next action.
- The message fits the target platform's current length limit.
