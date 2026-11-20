# Digest One-Message Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The daily digest becomes one Telegram message with category buttons that edit it in place. It gains stocks and gold categories, and shows gold prices in USD and Thai association prices.

**Architecture:** All of this lives in the `modules/digest` package:
- `gold.go` fetches and parses prices.
- `menu.go` renders the views and the keyboard.
- The service publishes through a `Messenger` interface.

`main.go` adapts the bot client and registers a `dg:` callback route.

**Tech Stack:** Go and Gin. Frontend: Alpine (`digest.js`).

**Spec:** `docs/superpowers/specs/2026-11-21-digest-one-message-design.md`

## Global Constraints

- **Gold source URLs, fields and formula:** copied verbatim from the spec. The estimate is always labelled "ราคาประเมินจากราคาโลก".
- **Disclaimer:** the overview and the gold and stocks views each contain "ไม่ใช่คำแนะนำการลงทุน". The prompt forbids recommendations.
- **Callback data:** `dg:<runID>:<key>`, at most 64 bytes. Valid keys are `overview`, `gold`, `stocks`, `ai`, `politics`, `trivia`.
- **Message length:** each rendered view is at most 4000 runes.

## Review Focus

- **Association page changed** (IDs missing): use the estimate and its label. Test: `TestParseAssociationMissing`.
- **Every gold source down:** the overview still renders, with "ดึงราคาไม่ได้". Test: `TestRenderOverviewNoGold`.
- **Button on an old run with no `gold_json`:** the view renders without prices and nothing panics. Test: `TestRenderSectionOldRun`.
- **Unknown or forged callback data** (`dg:abc:xyz`): it is answered and ignored, with no edit. Test: `TestHandleCallbackRejectsBadData`.
- **Bot not configured:** the notifier gets exactly one plain overview message. Test: `TestServiceFallsBackToNotifier`.

---

### Task 1: Gold prices (`gold.go`)

**Produces:**
- `type GoldQuote struct{ SpotUSD float64; BarSell, BarBuy, OrnSell, OrnBuy float64; AnnouncedAt string; Estimated bool; FetchedAt time.Time }` (JSON tagged).
- `func parseAssociation(html string) (bar, barBuy, orn, ornBuy float64, at string, ok bool)`
- `func parseSpot(body []byte) (float64, bool)`
- `func parseTHB(body []byte) (float64, bool)`
- `func estimateBar(spot, thb float64) float64`
- `type GoldFetcher struct{ client *http.Client; assocURL, spotURL, fxURL string }`
- `NewGoldFetcher(client)`
- `(f) Fetch(ctx) GoldQuote`. It never errors; missing values are 0.

**Tests:**
- `TestParseAssociation` uses a fixture string built from the live structure.
- `TestParseAssociationMissing`
- `TestParseSpotAndTHB`
- `TestEstimateBar`: `spot 3541.15 × thb 33.94469` gives ≈ 56,850 ± 100.
- `TestFetchFallsBackToEstimate` uses httptest servers: the association returns 500, so the result has `Estimated=true` and `BarSell > 0`.

### Task 2: Categories, prompt, storage, menu, publish

**Produces:**
- Two feeds added to `DefaultCuratedFeeds` with Category `"Stocks"`.
- `func recategorizeGold(items []NewsItem) []NewsItem`. Matches "ทอง" or case-insensitive "gold" in the title or description, and sets the category to `"Gold"`.
- The prompt's sections become `OVERVIEW`, `AI`, `POLITICS`, `STOCKS`, `GOLD` and `TRIVIA`, plus the no-advice rule.
- Store:
  - Schema: `digest_history.gold_json TEXT`, added with `ALTER TABLE` and the duplicate-column error ignored.
  - `RecordSuccess(ctx, id, summary, items, goldJSON string)`
  - `GetRun(ctx, id) (*RunRecord, error)`
  - `PreviousGold(ctx, beforeID) (*GoldQuote, error)`
  - `RunRecord.GoldJSON`
- `menu.go`:
  - `type Button struct{ Text, Data string }`
  - `func RenderView(run *RunRecord, key string, prev *GoldQuote) string`
  - `func MenuButtons(runID int64, active string) [][]Button`
  - `func ParseCallback(data string) (runID int64, key string, ok bool)`
- `type Messenger interface{ SendMenu(ctx context.Context, text string, buttons [][]Button) error }`, wired through `WithMessenger(m)`.
- `(s *Service) HandleCallback(ctx, data string) (text string, buttons [][]Button, ok bool)`
- Generation fetches gold, injects the price block at the top of the stored `GOLD` section, records the run, and publishes **once**. It uses the Messenger when one is set; otherwise it calls `notifier.Send(RenderView(run, "overview", prev))`.

**Tests:**
- `TestRecategorizeGold`
- `TestMenuButtonsDataUnder64`
- `TestParseCallback`
- `TestHandleCallbackRejectsBadData`
- `TestRenderOverviewIncludesGoldAndDisclaimer`
- `TestRenderOverviewNoGold`
- `TestRenderSectionOldRun`
- `TestServicePublishesOneMessage`
- `TestServiceFallsBackToNotifier`

### Task 3: Bot callback route and main wiring

**Produces:**
- `func WithCallbackRoute(prefix string, h func(ctx context.Context, q *CallbackQuery) error) RouterOption`. `HandleUpdate` checks the registered prefixes before falling back to the existing `qs:` handler and keyboards.
- `main.go`:
  - A messenger adapter that sends via `botClient.SendMessage` to `TelegramChatID`.
  - A `dg:` route that calls `digestSvc.HandleCallback`, then `EditMessageText` and `AnswerCallbackQuery`.
  - The `/digest` digestFunc returns `""` after publishing, and the router sends nothing more when the result is empty.

**Tests:**
- `TestRouter_CallbackRoutePrefix`: `dg:` reaches the route and `qs:` still reaches the callback handler.
- `TestRouter_DigestNoDuplicate`: an empty result means only the acknowledgement is sent.

### Task 4: Dashboard tabs and verification

- `digest.js`:
  - `parseSections` splits on `===SECTION:` tags: ai, politics, stocks, gold, trivia, all.
  - It falls back to the old `###` regexes for older runs.
- `index.html`: add the หุ้น and ทอง tabs.
- `sw.js`: bump to v39.
- Test: the dashboard body contains `tab-btn-stocks` and `tab-btn-gold`.
- Live check:
  - Run `go run` against the real sources once and print the overview.
  - Check the browser at 390 px.
- Commit, then push.
