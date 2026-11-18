# Pocket Sentinel — Quiet Schedule Specification

**Status:** Approved; implementation complete, awaiting review
**Product name:** Quiet Schedule
**Route:** `/schedule`
**Timezone:** Asia/Bangkok

## Intent

Quiet Schedule is a lightweight personal reminder calendar for the device owner. It makes common reminders fast to create, keeps the next few commitments visible, and sends discreet Telegram messages without sound or vibration. It is intentionally simpler than a general-purpose calendar.

Example: “Dentist, Friday 10:00, remind me 30 and 10 minutes before, bring insurance card.” The reminder appears in the calendar, then Telegram sends silent notifications with Snooze and Done actions.

## Goals

- Create one-time or weekly recurring reminders with a short mobile-friendly form.
- Support multiple advance-notice offsets, including 5, 10, and 30 minutes.
- Store a title and a free-form details field.
- Show a modern, restrained calendar at `/schedule`, with an obvious “Next up” view and a quick-add action.
- Send Telegram notifications silently (`disable_notification=true`).
- Include inline Snooze (5/10/30 minutes) and Done actions.
- Retry delivery failures every minute for up to 10 minutes, then mark the delivery failed and show it in the schedule UI.
- Use Asia/Bangkok for all creation, display, recurrence, and delivery calculations.
- Protect schedule mutations with a separate `REMINDER_PIN`; allow read-only viewing without PIN.
- Persist reminders and delivery state in SQLite across server restarts.

## Non-goals

- Full calendar suite features such as invitees, shared calendars, color taxonomies, attachments, complex recurrence rules, or natural-language AI parsing.
- Synchronization with Google Calendar or other providers.
- Sounding alarms on the phone; Telegram messages are explicitly silent.
- Monthly recurrence in v1.

## Approaches considered

1. **Focused personal schedule with one-time and weekly reminders — selected.** Provides the everyday cases requested with a simple form and predictable recurrence.
2. One-time reminders only. Simplest, but requires recreating weekly routines.
3. Full recurring calendar with daily, weekly, monthly, and custom rules. More expressive, but adds complexity the user wants to avoid.
4. Telegram-only command interface. Fast for simple tasks, but poor for reviewing and editing a schedule.

## User experience

### Schedule page

`/schedule` is a standalone responsive page using Pocket Sentinel dashboard tokens and typography. It should feel like a calm personal planner, not an admin console.

Desktop layout:

```text
┌─────────────────────────────────────────────────────────┐
│ Quiet Schedule                         [+ New reminder]  │
│ Next up                                                  │
│ Today · 14:30  Dentist — bring insurance card            │
│                                                         │
│ [Today] [Week]                              June 2026    │
│ Mon       Tue       Wed       Thu       Fri       Sat/Sun│
│ ... concise event cards inside date cells ...            │
└─────────────────────────────────────────────────────────┘
```

Mobile layout defaults to an agenda list grouped by day, with “Next up” at the top and a sticky Add button. A compact week strip may be used for navigation, but do not squeeze a seven-column desktop calendar onto a phone. Tapping an event opens a small detail sheet with Edit, Done, and Delete actions.

### Create/edit flow

Use one progressive form with visible essentials and sensible defaults:

1. Title (required)
2. Date and time (required, default to the next practical time)
3. Repeat: Does not repeat / Every week
4. Remind me: checkbox chips for at time, 5, 10, and 30 minutes before; multiple choices allowed
5. Details (optional, multiline)
6. Save reminder

Avoid a separate field for each advance offset and avoid asking for timezone; the page states “เวลาไทย” near the time control.

### Telegram message

At each scheduled offset, send a silent Telegram message with title, local time, details when present, and inline buttons:

```text
⏰ Dentist
วันนี้ 14:30 · อีก 10 นาที
นำบัตรประกันไปด้วย

[เลื่อน 5 นาที] [เลื่อน 10 นาที]
[เสร็จแล้ว]
```

Snooze creates one new pending delivery for the selected delay. Done marks the current occurrence complete and cancels later advance notices for that occurrence. Callback actions are accepted only from the configured owner chat.

## Reminder behavior

- For a one-time reminder, calculate each selected delivery time from the scheduled event time. “At time” is an offset of zero.
- For a weekly reminder, repeat on the same weekday and local wall-clock time in Asia/Bangkok.
- Advance notice offsets run only when their resulting delivery time is in the future. If the reminder is created after one or more offsets have passed, those missed notices are skipped; the remaining future notices are retained.
- If the server was offline at a due time, send overdue notifications after restart only when they are no more than 10 minutes late. Older deliveries are marked missed, not sent as stale alerts.
- Retry failed Telegram delivery every minute for up to 10 minutes. After that, mark it failed and surface a visible failure state in the schedule UI. Do not silently drop it.
- When a one-time reminder is Done, archive it from upcoming views while keeping it available in recent history for 30 days.
- For weekly reminders, Done applies to the current occurrence only; future weeks remain scheduled.
- Editing an upcoming reminder recalculates pending delivery times and leaves already delivered history intact.
- Deleting a reminder cancels future pending deliveries and requires explicit confirmation.

## Security and configuration

- `REMINDER_PIN` is a separate secret with no insecure default. Mutations fail clearly while it is unset; read-only schedule remains available.
- Create, edit, complete, snooze, pause/resume recurrence, and delete require PIN validation. Telegram inline callbacks are authorized using the existing owner chat authorization and do not ask for the PIN again.
- Never include the PIN in Telegram messages, logs, or API responses.

## Data and interfaces

Persist schedules and each outbound delivery separately so retries and restart recovery are idempotent.

```text
reminders
  id, title, details, starts_at_utc, timezone, recurrence,
  notice_offsets_json, enabled, created_at, updated_at

reminder_occurrences
  id, reminder_id, scheduled_at_utc, state, completed_at

reminder_deliveries
  id, occurrence_id, offset_minutes, due_at_utc, state,
  attempts, last_error, sent_at, telegram_message_id
```

States should distinguish pending, sent, failed, missed, and canceled. Unique constraints must prevent duplicate occurrence/offset delivery after restart or concurrent worker ticks.

The API should provide read-only schedule/list and upcoming endpoints, plus PIN-protected create, edit, complete, snooze, pause/resume, and delete actions. Telegram callback data must identify a delivery/occurrence and action without embedding sensitive text.

## Constraints

- Go + Gin, SQLite through `modernc.org/sqlite` (no CGO), Telegram Bot API, Alpine.js, and existing dashboard styles.
- Must stay within the vivo V11 memory budget and use cancellable workers.
- Any dashboard asset edit requires a Service Worker cache version bump.
- Dates persist in UTC and render/calculate recurrence in Asia/Bangkok.

## Acceptance criteria

- Owner can create a one-time reminder with title, details, a local date/time, and multiple advance offsets from a phone.
- Owner can create a weekly reminder and sees the next occurrence correctly after a restart.
- Each due Telegram notice is silent and sent at most once, except bounded retries after delivery errors.
- Snooze schedules one later silent notification; Done prevents remaining notices for the current occurrence.
- Dashboard presents upcoming reminders and delivery failures clearly.
- All mutation actions enforce `REMINDER_PIN`; reads remain available without it.
- The layout works on narrow mobile screens without a squeezed month grid or horizontal overflow.
