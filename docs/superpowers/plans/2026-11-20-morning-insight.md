# Morning Insight Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a morning insight report. Go computes the facts, and the report is narrated by OpenRouter, then Gemini, then a fixed template. It is delivered by Telegram at 07:00, by `/insight`, and on an Overview card.

**Architecture:** A new package `modules/insight` holds four parts:
- **Pure fact functions** over plain input structs.
- **A narrator** with a provider chain.
- **A store and service** (`insight_reports` table, cooldown, scheduler).
- **HTTP routes.**

Adapters in `modules/insight/sources.go` read the location, telemetry, and reminders stores. `main.go` wires everything up.

**Tech Stack:** Go, SQLite (modernc), Gin, and the existing `shared/ai.Client` and `digest.GeminiClient`. On the frontend, Alpine.

**Spec:** `docs/superpowers/specs/2026-11-20-morning-insight-design.md`

## Global Constraints

- **Timezone:** Asia/Bangkok. "Yesterday" means 00:00–24:00 Bangkok of the previous day.
- **Morning send** happens at 07:00. If the morning report was missed, catch up between 07:00 and 12:00. Send at most one morning report per date.
- **Thresholds**, copied from the spec:

  | Rule | Value |
  |---|---|
  | GPS accuracy cutoff | > 100 m is dropped |
  | Max gap within a segment | 30 min |
  | Walking speed | 1.5–7 km/h |
  | Moving | ≥ 1.5 km/h |
  | Together | ≤ 80 m, with paired samples within 2 min |
  | Battery samples gap | ≤ 5 min |
  | Not-charging time needed for a battery verdict | ≥ 30 min |
  | Battery abnormal | `rate > max(2×base, base+1)` and `rate > 0.5 %/h` |
  | RAM abnormal | +15 pp, or ≥ 90 % |
  | Disk abnormal | > 1 GB/day and > 3× baseline, or free < 5 GB |
  | Phone battery low | < 15 % |
  | Cooldown | 60 s |
  | OpenRouter timeout | 15 s |
  | Gemini timeout | 30 s |
  | AI text length | ≤ 3500 chars |
  | Allowed tags | `b`, `i`, `code` |
  | Facts JSON | ≤ 1200 chars |

- **AI output** is accepted only if it contains every number token that appears in the template.
- **New endpoints are public.** There is no PIN, by the owner's decision.
- **Bump `sw.js`** to `sentinel-shell-v37`. Add `insight.js` to `SHELL_ASSETS`.

## Review Focus

- **v11 always charging:** the report says "ชาร์จอยู่ตลอด" with no verdict. Test: `TestServerFactsAlwaysCharging`.
- **No location points yesterday** for one or both people: no NaN and no division by zero. The section says "ยังไม่มีพิกัดเมื่อวาน". Test: `TestMovementNoPoints` and `TestTemplateEmptySections`.
- **AI returns a valid-looking text that drops or changes a number:** the chain falls through. Test: `TestNarratorRejectsChangedNumbers`.
- **Two morning triggers on the same date** (scheduler tick plus a catch-up after restart): only one Telegram send. Test: `TestMorningSendIsIdempotent`.
- **`/insight` spammed twice within 60 s:** the second call returns the cached report without calling any provider. Test: `TestServiceCooldown`.

---

### Task 1: Facts (pure computation)

**Files:** Create `modules/insight/facts.go`, `modules/insight/facts_test.go`.

**Interfaces (produced):**
- Inputs:
  - `type HealthPoint struct{ At time.Time; RAMUsedPct float64; HasRAM bool; DiskFreeGB float64; Battery float64; HasBattery bool; Charging bool }`
  - `type GeoPoint struct{ At time.Time; Lat, Lng, Accuracy float64; Battery int; Charging bool }`
  - `type Place struct{ Name string; Lat, Lng, Radius float64 }`
  - `type ReminderItem struct{ At time.Time; Title string }`
- Outputs:
  - `type Verdict string` with the constants `VerdictNormal = "normal"`, `VerdictAbnormal = "abnormal"`, `VerdictUnknown = "unknown"`.
  - `type ServerFacts struct{ BatteryRate, BatteryBaseline float64; BatteryVerdict Verdict; AlwaysCharging bool; RAMNow, RAMAvg, RAMBaseline float64; RAMVerdict Verdict; DiskFree, DiskPerDay, DiskBaselinePerDay float64; DiskVerdict Verdict; Attention []string }`
  - `type PersonFacts struct{ Name string; HasData bool; WalkKm, TotalKm float64; MovingMin int; Places []string; MinBattery int; LowBattery bool; WalkKmMedian7 float64 }`
  - `type TogetherFacts struct{ HasData bool; Minutes int; MedianMinutes7 int; LongestStart, LongestEnd time.Time }`
  - `type TodayFacts struct{ Count int; NextAt time.Time; NextTitle string }`
  - `type Facts struct{ Date string; Server ServerFacts; Me, Friend PersonFacts; Together TogetherFacts; Today TodayFacts }`
- Functions:
  - `func ComputeServer(samples []HealthPoint, now time.Time) ServerFacts`
  - `func ComputePerson(name string, points []GeoPoint, places []Place, day time.Time, loc *time.Location) PersonFacts`. `points` covers 7 days. `day` is yesterday's Bangkok midnight. `WalkKmMedian7` is the median of the 6 previous days that have data.
  - `func ComputeTogether(me, friend []GeoPoint, day time.Time, loc *time.Location) TogetherFacts`
  - `func ComputeToday(items []ReminderItem, now time.Time, loc *time.Location) TodayFacts`
  - `func haversineKm(lat1, lng1, lat2, lng2 float64) float64`

- [ ] **Step 1: Write the failing tests.**
  - `TestServerFactsBatteryAbnormal`: 24 h at −4 %/h against a baseline of −1 %/h gives `abnormal`.
  - `TestServerFactsBatteryNormal`: gives `normal`.
  - `TestServerFactsAlwaysCharging`: `AlwaysCharging=true` and `VerdictUnknown`.
  - `TestServerFactsRAMAndDisk`: RAM 60 % → 80 % is `abnormal`. Free disk of 4 GB is `abnormal`.
  - `TestMovementWalkVsVehicle`: 2 km at 5 km/h plus 10 km at 40 km/h gives WalkKm ≈ 2.0 (±0.1) and TotalKm ≈ 12.0 (±0.2).
  - `TestMovementDropsInaccurateAndGaps`: the accuracy and gap rules hold.
  - `TestMovementPlacesVisited`: two consecutive points inside "บ้าน" list it.
  - `TestMovementNoPoints`: `HasData=false` and zeros.
  - `TestTogetherMinutes`: 90 minutes of paired points within 50 m gives 90 ±2. Pairs 200 m apart are not counted.
  - `TestToday`: the count and the next item.
- [ ] **Step 2: Run** `go test ./modules/insight/`. Expect FAIL (package/undefined).
- [ ] **Step 3: Implement** `facts.go`.
- [ ] **Step 4: Run** `go test ./modules/insight/`. Expect PASS.
- [ ] **Step 5: Commit** with the message `feat(insight): deterministic facts for server health, movement and together time`.

### Task 2: Narration and provider chain

**Files:**
- Create `modules/insight/narrate.go`, `modules/insight/narrate_test.go`.
- Modify `modules/digest/gemini.go`: add a `Generate` method and make `SummarizeNews` use it.

**Interfaces:**
- `type TextGenerator interface{ Generate(ctx context.Context, system, user string) (string, error) }`
- `type Provider struct{ Name string; Gen TextGenerator; Timeout time.Duration }`
- `func Template(f Facts) string`, which returns Telegram HTML.
- `func Narrate(ctx context.Context, f Facts, providers []Provider) (text, source string)`. `source` is the name of the provider that was accepted, or `"template"`.
- `func validNarration(ai, template string) bool`
- `func (g *GeminiClient) Generate(ctx context.Context, system, user string) (string, error)`. It reuses the existing HTTP and redaction code. `shared/ai.Client` already satisfies `TextGenerator`.

- [ ] **Step 1: Write the failing tests.**
  - `TestTemplateContainsSections`: the template contains 🛡️, 🚶, 🤝 and 📅, and the formatted numbers.
  - `TestTemplateEmptySections`: shows "ยังไม่มีพิกัดเมื่อวาน".
  - `TestNarratorUsesFirstValid`: OpenRouter stub OK gives source `openrouter`.
  - `TestNarratorRejectsChangedNumbers`: OpenRouter drops a number and Gemini is OK, so the source is `gemini`.
  - `TestNarratorFallsBackToTemplate`: both providers error, so the result is the template.
  - `TestNarratorRejectsDisallowedTags`: `<a href>` is rejected.
  - `TestGeminiGenerate`: an httptest server returns the candidate text.
- [ ] **Step 2: Run** `go test ./modules/insight/ ./modules/digest/`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./modules/insight/ ./modules/digest/`. Expect PASS.
- [ ] **Step 5: Commit** with the message `feat(insight): narrate facts via OpenRouter → Gemini → template`.

### Task 3: Store, service, scheduler, HTTP API

**Files:** Create `modules/insight/store.go`, `service.go`, `scheduler.go`, `handlers.go`, plus tests.

**Interfaces:**
- `type Sources interface{ Health(ctx, since time.Time) ([]HealthPoint, error); Points(ctx, device string, from, to time.Time) ([]GeoPoint, error); Places(ctx) ([]Place, error); Reminders(ctx, now time.Time) ([]ReminderItem, error) }`
- `type Report struct{ ID int64; ReportDate, Kind, Source, FactsJSON, TextHTML string; CreatedAt time.Time; SentAt *time.Time }`, with JSON tags in snake_case.
- `func NewStore(db *sql.DB) (*Store, error)` creates the table.
- `func NewService(store *Store, src Sources, providers []Provider, notifier Sender, opts ...Option) *Service`, where `type Sender interface{ Send(ctx context.Context, msg string) error }` and `WithClock(func() time.Time)`.
- `func (s *Service) Generate(ctx context.Context, kind string) (Report, bool, error)`. The bool is true when the cooldown returned the cached report.
- `func (s *Service) Latest(ctx context.Context) (Report, bool, error)`
- `func (s *Service) SendMorning(ctx context.Context) error`. It is idempotent per date.
- `func NextMorning(now time.Time, loc *time.Location) time.Time`
- `func (s *Service) RunScheduler(ctx context.Context)`. It catches up first, then loops.
- Routes:
  - `func (s *Service) RegisterRoutes(r *gin.Engine)`
  - `GET /api/insight/latest` returns 200 or 404.
  - `POST /api/insight/run` returns 200, or 429 with the report body.

- [ ] **Step 1: Write the failing tests.**
  - `TestNextMorning`: 06:59 gives today at 07:00. 07:00:01 gives tomorrow.
  - `TestMorningSendIsIdempotent`: calling `SendMorning` twice sends once.
  - `TestMorningCatchUpWindow`: the scheduler's catch-up sends at 08:00 and does not send at 13:00.
  - `TestServiceCooldown`: the second `Generate` within 60 s returns `cached=true`, and the provider is called once.
  - `TestInsightRoutes`: `latest` returns 404 and then 200 after a run. A second run gets 429.
- [ ] **Step 2: Run** `go test ./modules/insight/`. Expect FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test ./modules/insight/`. Expect PASS.
- [ ] **Step 5: Commit** with the message `feat(insight): report store, cooldown, 07:00 scheduler and API`.

### Task 4: Sources adapters, main wiring, `/insight` bot command

**Files:**
- Create `modules/insight/sources.go`.
- Modify `modules/location/store.go`: add `PointsBetween`.
- Modify `main.go`, `modules/bot/router.go`, `modules/bot/keyboards.go`.
- Tests: `modules/location/store_test.go` and `modules/bot/insight_cmd_test.go`.

**Interfaces:**
- `func (s *Store) PointsBetween(ctx context.Context, device string, from, to time.Time) ([]LocationPoint, error)`. Results are ascending, capped at 20000.
- `func NewSources(tel *telemetry.Repository, loc *location.Store, rem *reminders.Store) Sources`
- Bot:
  - `WithInsightFunc(func(ctx context.Context) (string, error))`
  - `/insight` sends "⏳ กำลังสรุป…" and then the report HTML.
  - `/help` gets a line for `/insight`.
  - The help keyboard gets a `copyCommand("☀️ Insight", "/insight")` button.

- [ ] **Step 1: Write the failing tests.**
  - `TestStorePointsBetween`: returns ascending points within the range.
  - `TestRouter_Insight`: the reply contains the stub text.
  - `TestRouter_HelpListsInsight`.
- [ ] **Step 2: Run** `go test ./modules/location/ ./modules/bot/`. Expect FAIL.
- [ ] **Step 3: Implement**, and wire up `main.go`:
  - Create the store.
  - Build the providers: `openrouter` comes from `aiClient` if it has a key, and `gemini` comes from `digest.NewGeminiClient(cfg.GeminiAPIKey)` if the key is set.
  - Pass the notifier.
  - Register the routes.
  - Start `RunScheduler` with the worker context.
  - Wire the bot `WithInsightFunc`.
- [ ] **Step 4: Run** `go test ./... && go vet ./...`. Expect PASS.
- [ ] **Step 5: Commit** with the message `feat(insight): wire sources, scheduler and /insight command`.

### Task 5: Overview card

**Files:**
- Create `modules/dashboard/static/js/components/insight.js`.
- Modify `modules/dashboard/index.html`, `static/css/dashboard.css`, `sw.js`, `dashboard_test.go`.

**Interfaces:**
- `Alpine.data('insightCard')` with the state `{loading, failed, html, source, createdAt, busy}`.
- It calls `GET /api/insight/latest` on init, and `POST /api/insight/run` when "สร้างใหม่" is pressed.
- The HTML is sanitised: only `b`, `i` and `code` are kept, and `\n` becomes `<br>`.

- [ ] **Step 1: Update the tests.**
  - The dashboard body contains `insightCard` and `id="insight-card"`.
  - `sw.js` lists `insight.js`. `TestShellAssetsAllExist` covers this.
- [ ] **Step 2: Run** `go test ./modules/dashboard/`. Expect FAIL.
- [ ] **Step 3: Implement.** Place the card after the hero section.
- [ ] **Step 4: Run** `go test ./...`. Expect PASS. Then do a browser check at 390 px in dark and light: press สร้างใหม่ with no AI key, and expect the template text and the "ข้อความสำรอง" footer. The console must show no errors.
- [ ] **Step 5: Commit** with the message `feat(insight): overview card`, then run `git push origin main`.
