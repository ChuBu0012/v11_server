# Morning Insight — Design

Date: 2026-11-20

## Intent and why

The owner wants the free OpenRouter API to be useful. They also want a summary that answers their morning questions without opening several pages:

1. **Is the server healthy?** That means: is the v11's battery, RAM or disk dropping abnormally fast?
2. **Movement stats** for Chubu, for Furturia, and for the two of them together. Walking and total travel are reported **separately** (the owner's explicit choice).
3. **Anything else interesting.**

OpenRouter's free tier fails often. The summary must still arrive, and must still be correct, when it is down.

## Goals

- **Delivery surfaces.** Every one must be reachable (AGENTS.md §8):
  - **Telegram, every morning at 07:00 Asia/Bangkok.**
  - **`/insight`** in Telegram regenerates a report now and replies with it. It is listed in `/help` and in the help keyboard.
  - **An "Insight" card on the Overview page** shows the latest report, its source, and a "สร้างใหม่" button.
- **Deterministic facts.** Go code computes every number and every "abnormal" verdict. AI never computes or decides.
- **AI narration** with a fallback chain: **OpenRouter (`openrouter/free`) → Gemini → plain template.** The template alone is a complete, correct report; AI only rephrases it and adds 1–2 "💡 น่าสนใจ" lines picked from the facts.
- **Every report records its source** (`openrouter` / `gemini` / `template`) and states it in small text, so the owner can see when the fallbacks kick in.

## Non-goals

- No new data collection; the report uses only what is already stored.
- No push alerts in the middle of the day. Existing condition alerts already cover that.
- No paid models.

## Anti-goals

- Numbers in the message that differ from the computed facts. AI output is validated (see below).
- No message at all on a morning when the AI providers are down.
- Two morning messages on the same day. This includes a restart at 07:00.

## Data available (verified)

- **`health_samples`:** one sample per minute, 72 h retention. It holds host RAM used/total, Go RAM, disk free/total, and v11 battery level/status.
- **`location_points`:** 7 days retention, for both devices. Each point has lat/lng, accuracy, speed (km/h, as reported by OwnTracks), battery and battery_status, and recorded_at.
- **`location_geofences`:** named places.
- **Reminders:** today's events, via the reminders store.

## Facts computed (Go, `modules/insight/facts.go`)

All rates exclude charging periods. "Yesterday" means 00:00–24:00 Bangkok of the previous day. The **baseline** is the median of the same metric over the earlier days still in retention.

**Server (v11)**
- **Battery drain, %/h:** a level drop is counted only across consecutive samples where the status is not charging and the gap is ≤ 5 min. Compute the last 24 h rate against the baseline over the previous 48 h. The verdict is `abnormal` when `rate > max(2 × baseline, baseline + 1 %/h)` and the rate is above 0.5 %/h. With no usable baseline, the rate is reported without a verdict.
- **RAM:** the average host RAM used % over the last 24 h against the previous 48 h. The verdict is `abnormal` when it is up more than 15 percentage points, or the current value is ≥ 90 %.
- **Disk:** the free-GB slope per day over the last 24 h against the previous 48 h. The verdict is `abnormal` when it loses more than 1 GB/day and more than 3× the baseline, or free space is < 5 GB (matching the existing `DiskThresholdGB`).
- **Overall status** is `ok` / `attention`, plus the list of abnormal items.
- **Battery that never discharges.** The v11 is usually plugged in. When fewer than 30 minutes of the window are not charging, the report says "ชาร์จอยู่ตลอด" and gives no drain verdict. (Claude's call: a rate built from a few minutes of data would be noise.)

**Movement (yesterday, per device)**
- **Filtering:** drop points with accuracy > 100 m. Segments are consecutive point pairs with a gap ≤ 30 min. (Claude's call: OwnTracks reports sparsely while moving fast or standing still, so a 10-minute cap would drop real car trips. A long gap with little distance still counts as not moving.)
- **Segment speed** is the haversine distance divided by the time gap.
- **Walking:** segments at 1.5–7 km/h. **Travel total:** segments ≥ 1.5 km/h, plus the walking distance. Report walking km, total km, moving minutes, and the top speed segment class (walk / vehicle).
- **Places visited:** places whose radius contains at least 2 consecutive points of the device, shown as a list of names in order of first visit.
- **Phone battery:** the lowest level yesterday, and whether it fell below 15 %.
- **7-day comparison:** yesterday's walking km against the median of the previous 6 days.

**Together**
- **Minutes together:** paired samples where both devices have a point within 2 min of each other and the distance is ≤ 80 m (the same co-location threshold the map uses). Sum those minutes, and compare against the median of the previous 6 days.
- **Longest together stretch:** its start and end time.

**Today**
- The count of today's reminders, and the next one (time and title).

The facts struct marshals to JSON of at most about 1,200 characters, so the OpenRouter prompt stays under its 2,000-rune limit.

## Narration (`modules/insight/narrate.go`)

- **`Template(facts) string`:** a deterministic Telegram-HTML report in the agreed layout. Section order is 🛡️ เครื่อง v11, 🚶 เมื่อวาน (per person: walking / total / places), 🤝 ด้วยกัน, 📅 วันนี้. Empty sections say why, for example "ยังไม่มีพิกัดเมื่อวาน".
- **AI prompt:** the facts JSON plus the template, with a fixed instruction:
  - Rewrite this report in friendly, concise Thai.
  - Keep every number exactly as given.
  - Keep the section order.
  - Add at most 2 "💡" lines, drawn only from the facts.
  - Output Telegram HTML using only `<b>`, `<i>`, and `<code>`.
- **Validation:** the AI text is accepted only if:
  - it is non-empty and ≤ 3,500 characters, and
  - every number in the template appears in it (string match on the formatted numbers), and
  - it contains no tags other than b, i, and code.
  
  If any check fails, fall through to the next provider.
- **Provider chain:** OpenRouter via the existing `shared/ai.Client`, with a 15 s timeout. Then Gemini, through a new generic `Generate(ctx, prompt)` method on `digest.GeminiClient` (refactored out of `SummarizeNews`), with a 30 s timeout. Then the template.

## Storage and API

- **Table `insight_reports`:** `id`, `report_date` (Bangkok `YYYY-MM-DD`), `kind` (`morning` | `manual`), `source`, `facts_json`, `text_html`, `created_at`, and `sent_at` (nullable).
- **`GET /api/insight/latest`** returns the latest report: `{report_date, kind, source, text_html, created_at}`, or 404.
- **`POST /api/insight/run`** generates a manual report and returns it. There is a 60 s cooldown that returns 429 with the existing report. The endpoint is public, matching the owner's no-PIN decision.

## Scheduling (`modules/insight/scheduler.go`)

- An in-process loop sleeps until the next 07:00 Bangkok.
- When it wakes, it generates a `morning` report and sends it through the existing `alerts.Notifier` (Telegram HTML), then sets `sent_at`.
- **Catch-up:** on startup, if the time is between 07:00 and 12:00 and no `morning` report with `sent_at` exists for today, it sends one. This covers a restart or deploy at 07:00.
- **Idempotence:** before sending, it re-checks that today has no sent morning report.

## UI

- **Overview card ("Insight เช้านี้")** placed after the hero, before "สถานะตอนนี้":
  - It renders `text_html` (sanitised to b, i, and code, with newlines turned into `<br>`).
  - A small muted footer shows the source and time, for example "สรุปโดย OpenRouter · 07:00" or "ข้อความสำรอง (AI ไม่ว่าง)".
  - A "สร้างใหม่" button calls `POST /api/insight/run`.
  - Empty state: "ยังไม่มีสรุป · กดสร้างใหม่".
- The card's component lives in `static/js/components/insight.js`, which is added to `sw.js`.

## Telegram

- **`/insight`:** the bot replies "⏳ กำลังสรุป…", then edits or sends the report. It shares the 60 s cooldown with the API.
- **`/help`** lists `/insight` and the help keyboard gets a "☀️ Insight" copy button (AGENTS.md §8).

## Approaches considered

1. **Go computes the facts, AI narrates, with an OpenRouter → Gemini → template chain.** Chosen by the owner: accurate numbers, and always delivered.
2. **Send raw data to the AI.** Rejected: it exceeds the 2,000-rune limit, free models get the arithmetic wrong, and nothing is sent when AI is down.
3. **Deterministic only.** Rejected: it does not use OpenRouter, which was the goal.

## Left to the builder

- Exact Thai copy of the template, within the layout above.
- The internal file split inside `modules/insight`.

## Verification

- **Unit tests with fixture samples:**
  - Battery verdicts: normal, abnormal, charging excluded, no baseline.
  - RAM and disk verdicts.
  - Walking vs vehicle split.
  - Places visited.
  - Minutes together.
  - The 7-day comparison.
- **Narration tests:**
  - OpenRouter success is accepted.
  - OpenRouter returns changed numbers, so the result falls through to Gemini.
  - Both providers fail, so the template is used.
  - Disallowed tags are rejected.
- **Scheduler tests:**
  - Computing the next 07:00.
  - Catch-up inside the window.
  - No double send.
- **API and bot tests:**
  - `/api/insight/latest` 404 and 200.
  - Cooldown returns 429.
  - `/insight` replies.
  - `/help` contains `/insight`.
- **Browser check** at 390 px: the card in dark and light, and the "สร้างใหม่" button with no AI key locally, so the template source is shown.
