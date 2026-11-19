# Monitor and Telegram Tabs Design

## Intent

Add a Monitor view that explains current device/server health and trends, with optional AI analysis, and make Telegram accessible as an in-app tab so it no longer sends the user to a differently styled page.

## Goals

- Add Monitor to the shared Dashboard navigation.
- Render Monitor and Telegram within the Dashboard shell and switch between those views without a full page load.
- Generate Thai operational insights from bounded telemetry and Watchtower evidence through the existing server-side OpenRouter client.
- Keep deterministic status and threshold decisions authoritative; AI explains observations and suggests checks, but never declares service state.
- Preserve Telegram text/file sending, PIN setup, clipboard paste, and AI Proofread.

## Non-goals

- Replace existing telemetry collection, alerts, Watchtower, or digest workflows.
- Expose OpenRouter credentials in browser code.
- Remove legacy `/tele-bridge` routes; they remain available and may redirect to the Dashboard Telegram tab.

## Constraints

- Use the existing Go/Gin, Alpine.js, and OpenRouter free-model client.
- Do not put Chart.js or Three.js instances in Alpine state.
- Keep backend work bounded and reject oversized insight requests.
- Update `CACHE_NAME` in `modules/dashboard/sw.js` whenever Dashboard frontend files change.

## Approach

Use the existing Dashboard as a lightweight single-page shell with `?tab=overview|monitor|telegram` navigation, history support, and the current dedicated Watchtower/Schedule routes left intact. Move the Telegram composer experience into the shared shell and retain `/tele-bridge` as a compatible entry point to the Telegram tab. Add a small Monitor component that loads current telemetry, recent history, and Watchtower summary, shows deterministic metric summaries and condition-based insights, and can request an additional Thai explanation from a server endpoint backed by the already-configured OpenRouter client. When AI is unavailable, show the deterministic insight and a clear explanation.

## Builder decisions

- AI analysis is user-triggered from Monitor to control external calls.
- The AI insight endpoint uses the existing Relay PIN so public access to the Dashboard cannot trigger OpenRouter requests without authorization.
- The AI endpoint receives only a bounded summary of health measurements and Watchtower state, not raw logs, secrets, or arbitrary prompt text.
- Existing Overview content remains the default view; navigation state is shareable and browser back/forward aware.
