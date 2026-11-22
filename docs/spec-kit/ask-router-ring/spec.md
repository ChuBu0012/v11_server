# Pocket Sentinel — Hybrid `/ask` AI Router, Smart Cache, and Device Ringing (`/ring`) Specification

**Status:** Approved Design Specification  
**Product:** Pocket Sentinel (vivo 1804 Termux ARM64)  
**Target Modules:** `modules/bot`, `shared/ai`, `shared/db`  
**Date:** 2026-11-22  

---

## 1. Intent & Problem Statement

Currently, Pocket Sentinel provides two interactive AI execution channels via Telegram:
1. `<code>/task &lt;command&gt;</code>`: Runs background agent execution via `agy` (`~/.local/bin/agy`).
2. `<code>/ask [low|median|high] &lt;question&gt;</code>`: Intended for questions, but currently invokes `agy` CLI directly as well.

This presents several critical limitations:
- **High Latency for Simple Q&A:** Simple factual questions ("ทำไมท้องฟ้าสีฟ้า", "แปลงเวลา UTC เป็น ICT") take 30–60 seconds because `agy` spins up a full agent harness with tools.
- **Quota & Lock Contention:** `agy` consumes primary model token quotas and acquires an exclusive execution lock. A trivial question blocks background tasks from running.
- **Zero Caching:** Asking the same question repeatedly repeats the entire slow LLM/agent cycle.
- **Inconvenient Syntax:** Requiring square brackets `[...]` for effort tags is cumbersome on mobile soft keyboards.
- **No Remote Physical Locating (`/ring`):** Misplacing the pocket server (vivo V11) in a room or needing immediate physical attention has no remote sound trigger.

This specification unifies:
1. **Hybrid `/ask` AI Router:** Uses ultra-fast LLMs (`shared/ai/engine.go` via Groq <1s / Gemini Flash) for general Q&A, with an intelligent Two-Stage Triage Router that automatically delegates complex agentic tasks (file editing, terminal commands, repo inspection) to `agy`.
2. **Smart TTL Cache:** Persistent SQLite caching with normalized hash keys, 6-hour TTL, automatic bypass on dynamic telemetry queries (battery, RAM, location, quota, git), and manual `--no-cache` / `--fresh` override.
3. **Bracket-free Tag Syntax:** Natural keyword prefix: `<code>/ask [model] [effort] &lt;prompt&gt;</code>` (e.g., `<code>/ask groq high ...</code>`, `<code>/ask agy ...</code>`), with defaults `model=auto`, `effort=medium`.
4. **Remote Device Ringing (`/ring`):** Maximizes volume, plays system alarm sound, vibrates, pops up a lockscreen Heads-Up Notification with instant tap-to-stop, has a 30-second safety auto-off timer, and provides a Telegram `[ 🔇 ปิดเสียง ]` inline button.

---

## 2. Goals & Non-goals

### Goals
- **Sub-2-Second Q&A:** Fast LLM answers standard questions in < 2 seconds via Groq / Gemini Flash without touching `agy` CLI.
- **Autonomous Agent Delegation (Hand-off):** If the prompt requires agent tool usage (code edits, git status/commits, shell execution, repo search), the AI Router transparently dispatches to `AgentRunner` (`agy`) and informs the user.
- **Zero-Latency Cache Hits:** Frequently asked static questions return instantaneously (0ms network round-trip, 0 tokens consumed).
- **Zero Mobile Syntax Friction:** No brackets required. Keywords like `groq`, `gemini`, `agy`, `low`, `high` are parsed cleanly from the front of the text.
- **Physical Device Finder:** `/ring` sounds an alarm on vivo V11 even if muted, with immediate local dismissal upon waking the screen or via Telegram.
- **Resource Discipline:** Respects RAM budget (< 300MB), ARM64 Android Kernel 4.4 constraints, and single-flight lock boundaries.

### Non-goals
- Embedding-based semantic vector cache (too heavy for unprivileged Termux ARM64; SQLite normalized exact match + dynamic heuristic bypass is sufficient).
- Unrestricted public shell execution through `/ask` (prompt injection guards in `guard.go` remain strictly enforced).
- Infinite alarm ringing (hard 30-second safety cutoff to prevent public disturbance).

---

## 3. User Experience & Scenarios

### Scenario 1: General Knowledge (Fast LLM + Cache Miss → Cache Hit)
1. User sends: `<code>/ask ทำไมทองคำถึงราคาขึ้นในสัปดาห์นี้</code>`
2. Parser identifies: `model=auto`, `effort=medium`.
3. Router checks Cache: Miss.
4. Fast Heuristic + Triage: Pure knowledge question → routes to Fast LLM (`shared/ai`).
5. Response arrives in ~1.2s via Groq/Gemini. Result is stored in `ask_cache` with a 6-hour TTL.
6. 10 minutes later, user sends the exact same question.
7. Router checks Cache: **Hit** → Returns immediately with footer `<i>⚡ ตอบทันทีจาก Cache (บันทึกเมื่อ 10 นาทีที่แล้ว)</i>`.

### Scenario 2: Dynamic Telemetry (Automatic Cache Bypass)
1. User sends: `<code>/ask แบตเตอรี่ v11 เหลือเท่าไหร่และ RAM เป็นอย่างไร</code>`
2. Dynamic Heuristic detects live status keywords (`แบตเตอรี่`, `RAM`, `v11`).
3. Cache is bypassed automatically.
4. Response is generated fresh using current telemetry context without serving stale data.

### Scenario 3: Agent Task Delegation (Hand-off to `agy`)
1. User sends: `<code>/ask ตรวจสอบ git status ล่าสุดและดูว่ามีบั๊กใน runner.go ไหม</code>`
2. Heuristic detects keywords: `git`, `runner.go`, `บั๊ก`.
3. Triage classifies: **AGY required** (needs workspace file inspection).
4. Bot edits Telegram ACK in-place:  
   `💬 <b>รับคำถามแล้ว:</b> <i>"ตรวจสอบ git status..."</i>`  
   `🔄 <b>ต้องใช้ Agent เชิงลึก:</b> กำลังส่งต่อให้ agy ดำเนินการพร้อมเครื่องมือตรวจสอบ...`
5. Dispatches to `AgentRunner.DispatchAsync()`. Heartbeat ticker begins every 15s.
6. `agy` finishes; formatted summary and execution details are delivered.

### Scenario 4: Explicit Model & Effort Selection
- `<code>/ask groq อธิบาย microservices ใน 3 ข้อ</code>` → Forces Groq provider.
- `<code>/ask gemini high วิเคราะห์ความต่างระหว่าง SQLite กับ PostgreSQL</code>` → Forces Gemini provider with high reasoning effort.
- `<code>/ask agy ดึง commit ล่าสุด</code>` → Bypasses triage, forces direct `agy` background runner.

### Scenario 5: Finding Misplaced Phone (`/ring`)
1. User misplaces vivo V11 and sends: `<code>/ring</code>` (or taps `[ 🔔 ส่งเสียงหาเครื่อง ]` in `/panel`).
2. Pocket Sentinel:
   - Sets alarm and music streams to maximum volume (`termux-volume stream alarm 15`).
   - Plays `/system/media/audio/alarms/Clock_Alert.ogg` in loop.
   - Triggers pulse vibration (`termux-vibrate`).
   - Displays lockscreen Heads-Up Notification with big button `[ 🔕 ปิดเสียง ]`.
   - Starts 30-second safety auto-off timer.
   - Replies to Telegram: `🔔 <b>กำลังส่งเสียงเรียกโทรศัพท์...</b>` with inline button `[ 🔇 ปิดเสียง ]`.
3. User picks up phone, presses Power button to wake screen:
   - Sees the Heads-Up notification banner.
   - Taps `[ 🔕 ปิดเสียง ]` (or swipes notification away) → Sound stops instantly on device!
4. Alternatively, if found from desk, user taps `[ 🔇 ปิดเสียง ]` in Telegram → Sound stops.
5. If ignored, sound shuts off automatically after 30 seconds.

---

## 4. Technical Architecture & Component Contracts

### 4.1 Parser (`modules/bot/ask_parser.go`)
Contract:
```go
type AskOptions struct {
    Model       string        // "auto", "groq", "gemini", "openrouter", "agy"
    Effort      string        // "low", "medium", "high"
    NoCache     bool          // true if --no-cache or --fresh specified
    CleanPrompt string        // Stripped user prompt
}

func ParseAskInput(input string) AskOptions
```
Parsing rules:
- Strips leading words matching models: `auto`, `groq`, `gemini`, `openrouter`, `agy`.
- Strips next word matching effort: `low`, `med`, `medium`, `median`, `high`, `max`.
- Checks for `--no-cache` or `--fresh` flags anywhere in text and removes them.
- Defaults: `Model = "auto"`, `Effort = "medium"`, `NoCache = false`.

### 4.2 SQLite Cache Store (`modules/bot/ask_cache.go`)
Schema:
```sql
CREATE TABLE IF NOT EXISTS ask_cache (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    prompt_hash TEXT NOT NULL UNIQUE,
    prompt_text TEXT NOT NULL,
    model_name TEXT NOT NULL,
    response_text TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_ask_cache_hash ON ask_cache(prompt_hash);
CREATE INDEX IF NOT EXISTS idx_ask_cache_expires ON ask_cache(expires_at);
```
- Hash: SHA-256 of lowercase, whitespace-normalized prompt + model name.
- Pruning: Auto-delete expired rows (`expires_at < CURRENT_TIMESTAMP`) during writes or background housekeeping.

### 4.3 AI Router & Triage Engine
1. **Dynamic Keyword Bypass:**
   Keywords: `แบต`, `battery`, `ram`, `disk`, `uptime`, `พิกัด`, `gps`, `location`, `โควต้า`, `quota`, `git`, `log`, `ตอนนี้`, `กี่โมง`, `สถานะ`.
   If matched → `NoCache = true`.
2. **Two-Stage Triage:**
   - Heuristic matches agent words: `แก้`, `fix`, `เขียนไฟล์`, `รัน`, `run`, `exec`, `commit`, `deploy`, `โค้ด`, `code`.
   - If heuristic is positive or `Model == "agy"` → Route to `AgentRunner`.
   - If ambiguous and `Model == "auto"`: Groq triage check (`Respond ONLY with "FAST" or "AGY"`).
   - If `FAST` → `aiEngine.Generate(ctx, prompt)`.

### 4.4 Ring Controller (`modules/bot/ring.go`)
```go
type RingController struct {
    mu           sync.Mutex
    isRinging    bool
    stopTimer    *time.Timer
    runner       CommandRunner
}

func (c *RingController) Ring(ctx context.Context) error
func (c *RingController) Stop(ctx context.Context) error
func (c *RingController) IsRinging() bool
```
- Shell commands executed:
  - `termux-volume stream alarm 15`
  - `termux-volume stream music 15`
  - `termux-media-player play /system/media/audio/alarms/Clock_Alert.ogg`
  - `termux-notification --id sentinel_ring --ongoing --priority high --title "🔔 Pocket Sentinel กำลังส่งเสียง" --content "แตะปุ่มเพื่อปิดเสียงทันที" --button1 "🔕 ปิดเสียง" --button1-action "termux-media-player stop && termux-notification-remove sentinel_ring" --on-delete "termux-media-player stop"`
  - `termux-vibrate -d 3000 -f`
- Auto-off: `time.AfterFunc(30*time.Second, func() { c.Stop(context.Background()) })`

---

## 5. Telegram Integration & Command Routing

- **Command:** `<code>/ask</code>`
  - Fully backward compatible with legacy prompts.
  - Returns answer directly with footer indicating source: `[Groq 0.8s]`, `[Gemini 1.5s]`, `[Cache Hit]`, or `[agy Agent]`.
- **Command:** `<code>/ring [on|off]</code>`
  - `/ring` or `/ring on` starts ringing.
  - `/ring off` stops ringing.
  - Callback queries: `callback_data="ring:off"` handles Telegram button clicks.
- **Control Panel (`/panel`):**
  - Add quick action button: `[ 🔔 ส่งเสียงหาเครื่อง ]` in the reply / inline control menu.
- **Help (`/help`):**
  - Document syntax:
    - `<code>/ask [model] [effort] &lt;คำถาม&gt;</code> ถามคำถาม AI ตอบเร็ว (รองรับ auto, groq, gemini, agy)`
    - `<code>/ring [on|off]</code> ส่งเสียงหาโทรศัพท์ (ดังสุด + สั่น + ปิดที่หน้าจอหรือบอท)`
