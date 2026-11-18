# Quiet Schedule — Implementation Plan

## Architecture

```mermaid
flowchart LR
  UI[/schedule UI] --> API[Reminder API]
  API --> Store[(SQLite)]
  Store --> Worker[Due delivery worker]
  Worker --> Telegram[Telegram silent send]
  Telegram --> Callback[Owner inline actions]
  Callback --> API
  Worker --> Retry[Bounded retry and restart recovery]
```

## Module map

```text
modules/reminders/
  models.go       reminder, occurrence, delivery contracts
  schedule.go     recurrence and offset calculation in Asia/Bangkok
  store.go        SQLite CRUD, due deliveries, and idempotency
  service.go      creation, edit, completion, snooze, and state transitions
  worker.go       due delivery loop, retry, and restart recovery
  handlers.go     Gin API and REMINDER_PIN middleware
  telegram.go     silent Telegram delivery and callback action handling

modules/dashboard/
  schedule.html
  static/js/components/schedule.js
  static/css/styles.css
```

## Implementation sequence

1. Add typed `REMINDER_PIN` config and pure recurrence/offset calculation.
2. Add SQLite tables, unique indexes, and store operations.
3. Implement service state transitions, snooze, completion, edit recalculation, and deletion cancellation.
4. Implement a cancellable worker that claims due deliveries transactionally, sends silently, retries every minute for at most 10 minutes, and recovers overdue work after restart.
5. Extend the Telegram client to send silent messages with inline callback buttons; route callbacks through existing authorized bot flow.
6. Add `/schedule` responsive UI with desktop calendar and mobile agenda, progressive editor, confirmation states, and delivery error feedback.
7. Register routes and worker in the existing app lifecycle; bump Service Worker cache version.

## Verification plan

- Recurrence/offset tests around Bangkok midnight and weekly boundaries.
- Delivery idempotency tests across repeated worker ticks and simulated restart.
- Retry tests for first-send failure, recovery, and exhaustion at 10 minutes.
- Callback tests for snooze, done, unauthorized chat, and repeated taps.
- PIN tests for every mutation and read-only access.
- Responsive review at phone, tablet, and desktop widths; keyboard and screen-reader labels for date/time and actions.
- Project regression tests and Android ARM64 deployment check, including idle memory measurement.

## Rollout

1. Configure `REMINDER_PIN` before enabling mutation routes.
2. Deploy and create a test reminder a few minutes ahead with two advance offsets.
3. Verify Telegram messages are silent, Snooze adds one delivery, Done cancels remaining notices, and weekly recurrence advances correctly.
4. Verify restart recovery and retry status in Dashboard, then use the feature for personal reminders.
