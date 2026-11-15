# Pocket Sentinel — Product Specification

**Status:** Ready for Implementation  
**Version:** 2.0 (Interactive Telegram Sentinel)  
**Target:** vivo V11 / Android 10 / Termux (Qualcomm Snapdragon 660, 6GB RAM)

## Summary

Pocket Sentinel transforms an old Android phone (vivo V11) into an always-on, low-power personal server and monitoring sentinel. Version 1.0 established local SQLite telemetry sampling, a Chart.js web dashboard, and outbound Telegram alerts. Version 2.0 extends this into a **Two-way Interactive Telegram Sentinel**: allowing the owner to monitor, diagnose, and control hardware remotely from anywhere via Telegram using secure Long Polling, interactive inline buttons, and safe Termux hardware integration.

## Problem

While Version 1.0 alerts the owner when metrics breach thresholds, the owner cannot query the server on demand without opening the web dashboard (which relies on ngrok tunnels that may cycle or reconnect). Furthermore, the owner cannot trigger quick hardware actions (e.g. turning on the flashlight during a power outage or checking live server vitals) or execute basic health diagnostics remotely via mobile chat.

## Goals

- **Interactive Two-way Telegram Bot:** Support on-demand commands (`/status`, `/ping`, `/torch`, `/reboot`, `/exec`) and interactive Inline Keyboard buttons.
- **Tunnel-Independent Long Polling:** Use outbound Telegram `getUpdates` Long Polling so bot commands function 100% reliably even if the ngrok tunnel drops or changes domain.
- **Strict Single-User Security:** Whitelist execution strictly to the configured `TELEGRAM_CHAT_ID`; drop or reject any incoming messages from unauthorized users.
- **Hardware Control & Safety:** Support flashlight control (`termux-torch`) with an automated 3-minute auto-off safety timer to prevent battery drain and thermal buildup.
- **Safe Command Execution:** Allow diagnostic checks (`/exec`) restricted to an immutable whitelist (`ps`, `df`, `free`, `uptime`, `pm2`) with strict 5-second execution timeouts.
- **Lightweight Footprint:** Maintain Go process memory under 25MB (far below the Android 10 LMK 300MB ceiling).

## Non-goals for Version 2.0

- Arbitrary unconstrained shell execution (strict security risk on non-rooted Android).
- Public multi-user bot access or group chat administration.
- Replacing the existing web dashboard or outbound alert manager.
- On-device LLM inference or heavy background video streaming.

## Users and primary flow

The primary user is the device owner. Through Telegram:
1. Owner types `/status` or taps `[🔄 Refresh]` → Bot returns real-time Battery, RAM, Disk, Uptime, and Ngrok URL with inline buttons.
2. Owner types `/torch on` or taps `[🔦 Toggle Torch]` → Bot activates phone LED flash, starts a 3-minute auto-off countdown, and reports status.
3. Owner types `/ping` or taps `[🩺 Ping]` → Bot returns pong latency and heartbeat.
4. Owner types `/exec df -h` or `/exec ps` → Bot executes the whitelisted command with a 5s timeout and returns formatted monospace output.
5. Owner types `/reboot` → Bot acknowledges and cleanly exits, allowing PM2 to instantly resurrect the process.

## Functional requirements

### Outbound Telemetry & Dashboard (Inherited from v1.0)
- `GET /` serves static dashboard with 1h/6h/24h Chart.js graphs and live metrics.
- Background collector samples battery, disk, memory, and load every 60s, retained in SQLite.
- Edge-triggered alerts for low battery (<20%) and low disk (<5GB).

### Interactive Telegram Bot (v2.0)

#### 1. Long Polling Engine (`modules/bot`)
- Runs in a dedicated background worker goroutine with a cancellable context.
- Calls Telegram Bot API `getUpdates` using long-polling (`timeout=30s`, `limit=20`).
- Outbound HTTPS avoids all NAT, firewall, or ngrok tunnel dependency.
- Correctly integrates with `setupAndroidDNS()` to resolve `api.telegram.org` on Android Bionic.

#### 2. Authentication & Whitelisting (`auth.go`)
- Compares incoming `update.Message.Chat.ID` and `update.CallbackQuery.Message.Chat.ID` against `cfg.TelegramChatID`.
- Non-matching sender messages are discarded immediately with a warning recorded in local logs (`modules/logs`).

#### 3. Command Routing & Handlers (`router.go`)
- `/start` & `/help`: Displays available commands, usage instructions, and quick action inline keyboard.
- `/status`: Queries current memory (`runtime.ReadMemStats`), disk (`syscall.Statfs`), battery (`termux-battery-status`), and ngrok tunnel API (`localhost:4040/api/tunnels`). Renders an HTML card with inline keyboard.
- `/ping`: Responds with `🏓 Pong! (Uptime: Xh Ym, Server: OK)`.
- `/torch <on|off|status>`: Controls `termux-torch`. Automatically arms a 3-minute safety timer on `on`; cancels timer on `off`.
- `/reboot`: Responds with `♻️ Restarting Pocket Sentinel via PM2...` and triggers `os.Exit(0)` after 500ms.
- `/exec <cmd>`: Validates `<cmd>` against `[ps, df, free, uptime, pm2]`. Runs via `exec.CommandContext` with 5s timeout and returns `<pre>` formatted stdout/stderr.

#### 4. Inline Keyboards & Callback Queries (`keyboards.go`)
- Attaches an interactive inline keyboard to `/status`:
  - Row 1: `[🔄 Refresh Status]` (callback: `status_refresh`)
  - Row 2: `[🔦 Toggle Torch]` (callback: `torch_toggle`) | `[🩺 Ping]` (callback: `ping_check`)
- Handles `CallbackQuery`: acknowledges via `answerCallbackQuery` and edits the existing message in-place to avoid chat clutter.

## Quality attributes and constraints

- **Memory Limit:** Total Go service memory consumption must stay `< 30MB` (Android LMK limit: 300MB).
- **Graceful Shutdown:** Long Polling HTTP requests and workers must terminate promptly on `SIGINT` / `SIGTERM` within 3 seconds.
- **Fail-Safe Hardware Calls:** If `termux-torch` fails (e.g. camera permission revoked), return an informative error to Telegram without crashing the daemon.
- **Credential Hygiene:** Never echo `TELEGRAM_BOT_TOKEN` in responses, logs, or error outputs.

## Acceptance criteria

1. **Long Polling Verification:** Bot receives updates and responds to `/ping` and `/status` in < 2 seconds without requiring ngrok tunnel.
2. **Whitelist Enforcement:** Simulated or actual requests from an unconfigured Chat ID are dropped silently and logged.
3. **Hardware Torch & Auto-off:** `/torch on` lights the phone LED; `/torch off` turns it off; if left on, it automatically extinguishes after 3 minutes.
4. **Interactive Inline Keyboard:** Tapping `[🔄 Refresh Status]` updates the existing status message with current metrics and timestamp.
5. **Safe Whitelisted Exec:** `/exec df -h` returns disk usage; `/exec rm -rf /` or unknown commands are rejected with a clear permission error.
6. **Graceful Reboot:** `/reboot` replies to Telegram, terminates the Go process cleanly, and PM2 restarts it within 3 seconds.
7. **Automated Unit & Integration Tests:** 100% passing tests for command router, whitelist authentication, safe executor, and keyboard callbacks.
