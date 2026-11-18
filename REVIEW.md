# Code Review: Telegram Relay and App Hub

- **Reviewed commit:** `0ada764` — `feat: add Telegram relay API, web UI, curl support, and Dashboard App Hub launcher`
- **Comparison point:** `HEAD~1` (`cd18aaf`)
- **Working tree:** clean before this review note was added.
- **Spec:** No originating issue or matching spec file was found, so requirement coverage could not be assessed.
- **Verification:** Review only; tests were not run.

## Findings

### 1. Alpine state holds a browser `File` object

**Severity: Standards breach**  
[`modules/dashboard/static/js/components/relay.js:17`](modules/dashboard/static/js/components/relay.js#L17)

`selectedFile` is stored in the object returned by `Alpine.data`, then used for upload. The project AGENTS.md says Alpine state should contain only primitive state, text, and boolean flags; objects with complex prototypes should stay outside Alpine's reactive proxy. Keep the `File` in a module-level store and let Alpine hold a primitive key/flag.

### 2. `/api/relay/send` parses uploads before applying the 20 MB limit

**Severity: Resource-limit bug**  
[`modules/relay/relay.go:233`](modules/relay/relay.go#L233)

`handleSend` calls `Request.FormFile` to detect an upload before dispatching to `handleFileUpload`. The latter installs `http.MaxBytesReader` only at line 169, after multipart parsing has already started. The dashboard uses `/send`, so its uploads can bypass the advertised 20 MB request-body cap and consume memory or temporary disk before rejection. Apply the limit before any multipart parsing, and cover all upload routes consistently.

### 3. Multipart request handling is duplicated

**Severity: Maintainability observation**  
[`modules/bot/bot.go:278`](modules/bot/bot.go#L278)

`SendPhoto` and `SendDocument` repeat multipart construction, HTTP execution, response decoding, and error handling. Consider a shared helper if these methods are expected to evolve together. This is a judgement call, not a hard standards violation.

## Decisions to make

1. **Where should the browser `File` live?** Choose a module-level store outside Alpine's reactive data, with Alpine retaining only simple UI state.
2. **What is the upload limit contract?** Confirm whether 20 MB is the intended hard limit, then enforce it before multipart parsing on `/send`, `/upload`, and any alternate file fields.
3. **Should `SendPhoto` and `SendDocument` share an internal helper?** This is optional; weigh reduced duplication against the helper's added abstraction.
4. **Should the built-in default relay PIN remain `1804`?** The relay is reachable through the public ingress; decide whether deployment should require an explicit `RELAY_PIN` or retain the documented default, and whether failed attempts need rate limiting.

