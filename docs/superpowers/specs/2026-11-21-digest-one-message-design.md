# Daily Digest: One Message, Category Buttons, Stocks & Gold — Design

Date: 2026-11-21 · The owner approved the sketch and source choice in chat.

## Intent

The daily news digest currently arrives as 3 Telegram messages fired back to back (AI / Politics / Trivia). The owner wants:

- **One message.** It shows an overview, and inline buttons switch that same message to one category at a time.
- **New categories:**
  - **Stock news**, factual only.
  - **Gold**: news plus prices in **USD (world spot)** and **Thai association prices** (bar and ornament, buy and sell).

**Out of scope, by policy:** recommending stocks to buy, or any buy/sell/target-price advice. The prompt forbids it, and the gold and stock sections carry "ไม่ใช่คำแนะนำการลงทุน".

## Sources (all probed live on 2026-11-21)

| Data | Source | Notes |
|---|---|---|
| World spot USD/oz | `https://xaus.com/api/v1/spot` → `spot_usd_oz` | Free, no key. |
| Thai association prices | `https://classic.goldtraders.or.th/`: `#DetailPlace_uc_goldprices1_lblBLSell`, `lblBLBuy`, `lblOMSell`, `lblOMBuy`, `lblAsTime` | Official numbers. HTML scrape. |
| USD/THB (fallback only) | `https://open.er-api.com/v6/latest/USD` → `rates.THB` | Free, no key. |
| Stock news | `https://www.kaohoon.com/feed`, `https://www.prachachat.net/finance/feed` | RSS. |
| Gold news | Items from all feeds whose title or description contains "ทอง" or "gold" (case-insensitive) | These are re-categorised as Gold. |

- **Thai price fallback (approach A + B):** if the association scrape fails, estimate `bar ≈ spot × THB × (15.244 / 31.1035) × 0.965`. Label it **"ราคาประเมินจากราคาโลก"**, never as the association price.
- **Partial failure:** if any source fails, the rest of the gold block still renders. A missing number shows "ดึงราคาไม่ได้".
- **Day-over-day change:** compare against the gold quote stored with the previous successful run, then show `+150` / `−0.8%`.

## Message

**Overview** (the default view):
- Date header.
- 🥇 ทอง: world USD/oz with change, then Thai bar sell / ornament sell with change.
- One line each for 📈 หุ้น, 🤖 AI, 🏛️ การเมือง, and 💡 สาระ, taken from Gemini's `OVERVIEW` section.
- "ไม่ใช่คำแนะนำการลงทุน".

**Buttons:** `[🥇 ทอง][📈 หุ้น][🤖 AI]` / `[🏛️ การเมือง][💡 สาระ][🏠 ภาพรวม]`. The callback data is `dg:<runID>:<key>`, where key is one of `overview`, `gold`, `stocks`, `ai`, `politics`, `trivia`. Tapping a button **edits the same message** (`editMessageText`) and keeps the keyboard. The active button gets a "• " prefix. The content comes from SQLite by run ID, so buttons keep working on old digests.

**Gold section:** the full price table (bar buy/sell, ornament buy/sell, association announcement time, world spot) followed by the gold news summary.

## Generation

- The Gemini prompt produces the tagged sections `OVERVIEW`, `AI`, `POLITICS`, `STOCKS`, `GOLD`, `TRIVIA`. `OVERVIEW` is exactly one line per category.
- **Rule:** no investment recommendations, buy/sell calls, or target prices. Report facts and news only.
- **Stored text:** the gold price block is injected at the top of the stored `GOLD` section. The run also stores `gold_json` (a new nullable column on `digest_runs`, added with `ALTER TABLE` if missing).

## Delivery

- **Bot configured:** the digest service publishes through a `Messenger` interface (`SendMenu(ctx, text, buttons)`). It is implemented in `main.go` over the bot client.
- **Bot not configured:** it falls back to `alerts.Notifier.Send(overview)`.
- **Callbacks:** the router gains `WithCallbackRoute(prefix, handler)`. The `dg:` handler renders the requested view and edits the message. Existing `qs:` handling is unchanged.
- **`/digest`:** acknowledges, then lets the service publish the single message. The extra duplicate send is removed.

## Dashboard

The digest card's tabs become: AI, การเมือง, หุ้น, ทอง, สาระ, ทั้งหมด. `digest.js` splits on the `===SECTION:X===` tags and keeps the old `###` heading fallback for older runs.

## Verification

- **Unit tests:**
  - Classic-site parser with a fixture, plus the empty and malformed cases.
  - xaus and FX parsers.
  - The estimate formula and its label.
  - Gold re-categorisation by keyword.
  - Section splitting with the new tags.
  - Overview and section rendering, plus the disclaimer.
  - Keyboard callback data under 64 bytes.
  - Callback routing (`dg:` reaches digest and `qs:` still reaches reminders).
  - The service publishes exactly one message.
- **Live check:** fetch the real sources once from the dev machine.
