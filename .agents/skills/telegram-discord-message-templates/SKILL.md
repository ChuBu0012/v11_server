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
- Use a short heading and compact bullets. Keep webhook content below Discord's 2000-character message limit; this project truncates near 1950 characters.
- Prevent unintended mentions from dynamic content. Escape or suppress `@everyone`, role, and user mentions unless notification is an explicit feature.
- Use embeds only when the current notifier supports structured embeds; do not make a content-only webhook depend on embed rendering.

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
