# Quiet Schedule — Tasks

**Status key:** `TODO` → `IN PROGRESS` → `BLOCKED` / `DONE`

## T001 — Configuration and schedule rules

**Status:** DONE

- Add `REMINDER_PIN` and pure Bangkok-local recurrence/offset calculations.
- Acceptance: once and weekly reminders calculate each next delivery correctly; missed offsets are skipped.

## T002 — SQLite store and idempotency

**Status:** DONE

- Add reminder, occurrence, and delivery tables, queries, state updates, and uniqueness constraints.
- Acceptance: duplicate worker ticks cannot create duplicate deliveries; done/edited/deleted records update pending deliveries correctly.

## T003 — Reminder service and worker

**Status:** DONE

- Implement create/edit/complete/snooze/pause/delete plus cancellable due-delivery worker.
- Acceptance: restart catches up only deliveries at most 10 minutes overdue; retry stops after 10 minutes and marks failure visibly.

## T004 — Telegram silent delivery and callback actions

**Status:** DONE

- Send `disable_notification=true` with title/details and inline Snooze/Done controls.
- Acceptance: owner can snooze or complete; unauthorized chats cannot apply callback actions; repeated callbacks are idempotent.

## T005 — PIN-protected API

**Status:** DONE

- Add read-only upcoming/list endpoints and `REMINDER_PIN` protected mutation routes.
- Acceptance: missing/wrong PIN denies mutations; reads work without PIN; PIN never appears in logs or response bodies.

## T006 — Schedule UX and responsive design

**Status:** DONE (visual device review pending)

- Add `/schedule`, desktop calendar, mobile agenda, Next up card, progressive create/edit form, and delivery status.
- Acceptance: common reminder can be created quickly on a phone without horizontal scrolling or dense settings.

## T007 — Lifecycle, cache, and release verification

**Status:** IN PROGRESS (build and static checks complete; device/release verification pending)

- Register worker and routes, bump Service Worker cache, verify existing features, and validate on Android ARM64.
- Acceptance: silent delivery, offsets, snooze, done, retry, weekly recurrence, restart recovery, and memory budget verified.
