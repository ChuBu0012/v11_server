# Hybrid `/ask` AI Router, Smart Cache, and Device Ringing (`/ring`) — Tasks & Evidence of Done

**Status key:** `TODO` → `IN PROGRESS` → `BLOCKED` / `DONE`

---

## T001 — SQLite Schema Migration for `ask_cache`
- **Status:** DONE
- **Evidence:** `go test -v ./shared/db -run 'TestInitDB'` → PASS; table `ask_cache` and its indexes exist without error on SQLite WAL mode.
- **Scope:**
  - Add `ask_cache` table creation in `shared/db/db.go`.
  - Add indexes `idx_ask_cache_hash` and `idx_ask_cache_expires`.
- **Target Artifacts:**
  - `shared/db/db.go`
  - `shared/db/db_test.go`
- **Acceptance Criteria:**
  - `InitDB()` initializes cleanly without error or breaking existing tables.
  - Passes all unit tests in `shared/db`.

---

## T002 — Bracket-Free Ask Parser (`ParseAskInput`)
- **Status:** DONE
- **Evidence:** `go test -v ./modules/bot -run 'TestParseAskInput'` → PASS (100% table-driven test cases).
- **Scope:**
  - Implement `AskOptions` struct and `ParseAskInput(input string) AskOptions`.
  - Support model prefixes: `auto`, `groq`, `gemini`, `openrouter`, `agy`.
  - Support effort prefixes: `low`, `med`, `medium`, `median`, `high`, `max`.
  - Support legacy bracket tags: `[low]`, `[high]`, etc.
  - Support cache flags: `--no-cache`, `--fresh`.
  - Default fallback: model=`auto`, effort=`medium`, noCache=`false`.
- **Target Artifacts:**
  - `modules/bot/ask_parser.go`
  - `modules/bot/ask_parser_test.go`
- **Acceptance Criteria:**
  - Accurately strips prefix keywords without touching prompt body.
  - Preserves exact prompts when no prefixes are provided.
  - Handles case-insensitivity and arbitrary whitespace.

---

## T003 — SQLite Ask Cache Repository (`AskCacheStore`)
- **Status:** DONE
- **Evidence:** `go test -v ./modules/bot -run 'TestAskCacheStore'` → PASS.
- **Scope:**
  - Implement `AskCacheStore` with methods:
    - `Get(ctx, prompt, model) (*AskCacheEntry, error)`
    - `Set(ctx, prompt, model, response, ttl) error`
    - `PruneExpired(ctx) (int64, error)`
    - `ComputeHash(prompt, model) string`
  - Normalizes text: lowercase, trimmed, unified whitespace.
  - Auto-prunes expired rows on `Set()`.
- **Target Artifacts:**
  - `modules/bot/ask_cache.go`
  - `modules/bot/ask_cache_test.go`
- **Acceptance Criteria:**
  - Correctly stores and retrieves cached responses before expiration.
  - Returns `nil, nil` on expired or non-existent keys.
  - Thread-safe and resilient against SQLite write contention.

---

## T004 — Fast LLM Q&A & Two-Stage Triage Router
- **Status:** DONE
- **Evidence:** `go test -v ./modules/bot -run 'TestIsDynamicQuery|TestRequiresAgent|TestAskTriageRouter'` → PASS.
- **Scope:**
  - Implement dynamic status keyword detector (`IsDynamicQuery(prompt) bool`).
  - Implement agentic keyword heuristic (`RequiresAgent(prompt) bool`).
  - Wire Fast LLM call via `shared/ai/engine.go` (Groq/Gemini).
  - Implement Triage logic:
    - If `RequiresAgent` or model is `agy` → Hand-off to `AgentRunner`.
    - If ambiguous under `auto` → Fast Groq triage check.
    - If general knowledge → Fast LLM generates answer in < 2s.
- **Target Artifacts:**
  - `modules/bot/ask_router.go`
  - `modules/bot/ask_router_test.go`
- **Acceptance Criteria:**
  - Correctly routes code/git/shell requests to `agy`.
  - Correctly routes knowledge/definition questions to Fast LLM.
  - Bypasses cache when `IsDynamicQuery` returns true.

---

## T005 — Device Sound & Alarm Controller (`RingController`)
- **Status:** DONE
- **Evidence:** `go test -v ./modules/bot -run 'TestRingController'` → PASS.
- **Scope:**
  - Implement `RingController` struct with mutex, state, timer, and command runner.
  - Methods: `Ring(ctx)`, `Stop(ctx)`, `IsRinging()`.
  - Safety auto-off timer set to 30 seconds.
  - Shell command execution:
    - Sets alarm/music volume to 15 via `termux-volume`.
    - Plays `/system/media/audio/alarms/Clock_Alert.ogg` via `termux-media-player`.
    - Pulses vibration via `termux-vibrate`.
    - Posts Heads-Up Notification with action button `🔕 ปิดเสียง` and swipe-to-dismiss hook via `termux-notification`.
- **Target Artifacts:**
  - `modules/bot/ring.go`
  - `modules/bot/ring_test.go`
- **Acceptance Criteria:**
  - Concurrency-safe: calling `Ring()` when already ringing does not spawn duplicate processes.
  - `Stop()` cancels auto-off timer, stops media player, removes notification, and resets state.
  - Timer automatically triggers `Stop()` after 30s.

---

## T006 — Telegram Bot Router Integration (`/ask`, `/ring`, Callbacks)
- **Status:** DONE
- **Evidence:** `go test -v ./modules/bot -run 'TestRouter_Ask|TestRouter_Ring'` → PASS.
- **Scope:**
  - Refactor `case "/ask"` in `modules/bot/router.go` to integrate Parser → Cache → Triage → Fast LLM / agy.
  - Add `case "/ring"` to start/stop ringing with authorization check.
  - Add callback handler for `ring:off` in inline keyboards.
  - Format output with clean source attribution footer (`[Groq 0.8s]`, `[Cache Hit]`, `[agy Agent]`).
- **Target Artifacts:**
  - `modules/bot/router.go`
  - `modules/bot/router_test.go`
- **Acceptance Criteria:**
  - Fast questions receive immediate reply without `agy` spin-up.
  - Hand-off questions update ACK message and invoke `AgentRunner`.
  - `/ring` returns confirmation message with `[ 🔇 ปิดเสียง ]` button.
  - Unauthorized chat IDs are blocked with 403 response.

---

## T007 — Control Panel (`/panel`), Keyboards & Documentation
- **Status:** DONE
- **Evidence:** `go test -v ./modules/bot -run 'TestKeyboards'` → PASS; `/help` outputs updated syntax.
- **Scope:**
  - Add `[ 🔔 ส่งเสียงหาเครื่อง ]` button in `/panel` quick actions or services menu.
  - Update `MakeHelpKeyboard()` and `/help` command text with new `/ask` and `/ring` usage.
  - Update `.agents/skills/telegram-discord-message-templates/SKILL.md` if any message templates change.
- **Target Artifacts:**
  - `modules/bot/keyboards.go`
  - `modules/bot/router.go`
- **Acceptance Criteria:**
  - Buttons render cleanly on Telegram mobile UI.
  - `/help` clearly explains model prefixes without square brackets.

---

## T008 — End-to-End Build, Vet & Quality Hook Verification
- **Status:** DONE
- **Evidence:** `./scripts/hook-validate.sh` passes 100%; `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -o server .` succeeds; all unit tests pass with zero errors (`go test -count=1 ./...`).
- **Scope:**
  - Run `gofmt -w` and `go vet ./...` across the repository.
  - Run full test suite: `go test -v ./...`.
  - Cross-compile for ARM64 Android: `CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -o server .`.
- **Target Artifacts:**
  - All touched Go files
- **Acceptance Criteria:**
  - Zero compiler errors, zero vet warnings, 100% passing tests.

---

## T009 — On-Device Deployment & Live Verification (vivo V11)
- **Status:** TODO
- **Evidence:** Binary deployed to vivo V11 Termux; PM2 `go-server` online; HTTP health check 200 OK; Telegram verification of `/ask`, cache hit, and `/ring`.
- **Scope:**
  - Commit and push changes to `origin/main`.
  - Run `./scripts/deploy-v11.sh -m "feat(bot): hybrid /ask fast router, smart cache, and /ring device alarm"`.
  - Verify live Telegram bot:
    1. `/ask ทำไมท้องฟ้าสีฟ้า` → ตอบใน < 2 วินาที
    2. `/ask ทำไมท้องฟ้าสีฟ้า` → ตอบทันทีจาก Cache
    3. `/ask ตรวจสอบ git status` → ส่งต่อให้ agy อัตโนมัติ
    4. `/ring` → เสียงดัง, หน้าจอมือถือขึ้นแจ้งเตือนปิดเสียง, และปุ่ม Telegram ปิดเสียงทำงาน
- **Target Artifacts:**
  - Live deployment on vivo 1804 (Termux arm64)
- **Acceptance Criteria:**
  - Memory consumption stays < 30MB RAM.
  - All live features respond properly as specified.
