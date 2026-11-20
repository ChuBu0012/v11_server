# Pocket Sentinel Web Redesign

## Intent

Make Pocket Sentinel easy to use from a phone first. The interface should answer the user's practical questions quickly: is the server healthy, what changed, what needs attention, what is next, where are the live locations, and how do I send something to Telegram?

## Goals

- Redesign the entire active web experience: dashboard, schedule, Telegram relay, and live location/map.
- Follow `DESIGN.md` RawBlock: black and white surfaces, square corners, strong black borders, typographic hierarchy, and blue reserved for links. Use the specified success, warning, and error colors only for status meaning.
- Use mobile-first layouts and task-oriented navigation that is not the existing top bar plus horizontal nav.
- Make telemetry charts explain their meaning with a computed comparison, direction, and plain-language observation based on returned samples.
- Add restrained transitions and animation, respecting reduced-motion preferences.
- Reduce initial page cost and avoid new dependencies, sessions, and session tracking.
- Temporarily disable Monitor and Watchtower in the running app: remove their visible UI, API routes, and Watchtower scheduler. Keep their module source and stored data available for a later feature return.
- Preserve active functions and endpoints, including reminders, Telegram PIN protection, AI digest, events, and location sharing.

## Non-goals

- Delete the Monitor/Watchtower module implementations or erase their stored data.
- Add authentication/session infrastructure, analytics, or session storage.
- Replace the Go embedded static-file architecture or add a frontend build step.

## Constraints

- Preserve current public page and API behavior where still active.
- Keep Alpine state limited to primitive UI data; keep Chart.js and Three.js instances outside Alpine proxies if used.
- Hidden canvases must resize after they become visible; chart components must retain direct history-load fallback.
- Increment the PWA cache name when frontend assets change and remove dormant Monitor/Watchtower assets from the shell cache.
- No new runtime dependencies. Avoid eager loading of page-specific map/chart libraries when they are not needed.
- Do not write session data or add session-related persistence.

## Approaches

1. **Redesign existing pages and flows (selected):** retain the Go embedded pages and API contracts, replace the shared UI language and page structures, make navigation task-oriented, and derive chart insights in the existing telemetry client. This preserves behavior while allowing a complete visual change with a small runtime footprint.
2. **Consolidate into a single-page app:** would unify transitions, but requires moving schedule and relay behavior into one route and increases routing and regression risk.
3. **Replace the frontend stack:** offers more component structure but adds a build/dependency footprint that conflicts with the fast-load and embedded deployment constraints.

Approach 1 is selected because it allows all active pages to be redesigned without changing the service architecture or adding bundle cost.

## Design

- Use a mobile bottom task bar for Overview, Schedule, Map, and Telegram; at wider screens it becomes a compact task rail that does not repeat the old top navigation.
- Make the dashboard start with a clear operational answer, an attention list when needed, the next reminder, and current resource readings. Present historical charts with range controls and a short insight statement calculated from actual samples; show a clear insufficient-data state when comparisons are not possible.
- Give Schedule a mobile-friendly agenda-first view with the calendar as a secondary way to browse dates, while keeping create/edit/pause/complete/history and PIN behavior.
- Make Telegram Relay a focused compose/send workspace with a clear text/file mode, paste support, size guidance, upload progress, and proofreading preview.
- Make live location a focused map page with concise participant and update context, while lazy-loading map assets where feasible.
- Remove Monitor and Watchtower controls, summaries, page route, and eager frontend scripts from active flows; stop mounting their APIs and Watchtower background scheduler. Keep implementation files and stored data available for later restoration.
- Use compact, purposeful motion for page sections and feedback; disable nonessential motion under `prefers-reduced-motion`.

## Builder decisions

- Retain existing URL and API contracts for active features.
- Use existing vanilla CSS/JavaScript, Alpine.js, and Chart.js; add no framework or library.
- Keep RawBlock tokens dominant across light and dark themes while preserving accessible contrast.
- Remove the 3D core from the critical dashboard path; use typographic status and a lightweight visual indicator instead.
- Keep map scripts/styles page-scoped and load map libraries only on the map route/view.

