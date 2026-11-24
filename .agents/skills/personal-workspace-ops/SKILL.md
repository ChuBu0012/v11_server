---
name: personal-workspace-ops
description: Operational guidance for reading, creating, updating, and querying personal notes and lightweight spreadsheets in Pocket Sentinel. Covers SQLite schema, REST API contracts, hashtag taxonomy, formula evaluation patterns, and Telegram capture workflows on vivo v11.
---

# Personal Workspace Operations: Notes & Sheets Guide

This skill guides AI agents and developers in managing, querying, creating, and automating personal notes and lightweight spreadsheets inside Pocket Sentinel.

---

## 1. Storage & Database Schema Reference

All workspace data is stored in the local SQLite database (`./data.db`) managed via `shared/db/db.go`.

### 1.1 Notes Table (`workspace_notes`)
```sql
CREATE TABLE workspace_notes (
    id TEXT PRIMARY KEY,           -- Format: "nt_" + nanoid / uuid snippet
    title TEXT NOT NULL,          -- Note title or first line summary
    content TEXT NOT NULL,        -- Full Markdown content
    tags TEXT NOT NULL,           -- Comma-separated tags (e.g. "todo,groceries")
    is_pinned INTEGER DEFAULT 0,  -- 1 = Pinned to top of list, 0 = Normal
    sync_public INTEGER DEFAULT 0,-- 1 = Sync to Firestore/Vercel Hub, 0 = Local only
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 1.2 Sheets Table (`workspace_sheets`)
```sql
CREATE TABLE workspace_sheets (
    id TEXT PRIMARY KEY,           -- Format: "sh_" + nanoid / uuid snippet
    title TEXT NOT NULL,          -- Sheet title
    row_count INTEGER DEFAULT 30, -- Total rows allocated (default 30)
    col_count INTEGER DEFAULT 10, -- Total columns allocated (default 10 = A to J)
    data_json TEXT NOT NULL,      -- Serialized JSON of cells, merges, and styles
    sync_public INTEGER DEFAULT 0,-- 1 = Sync to Firestore/Vercel Hub, 0 = Local only
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 1.3 `data_json` Structure for Sheets:
```json
{
  "cells": {
    "A1": { "raw": "รายการ", "v": "รายการ", "style": { "b": true, "bg": "#f0f2f5" } },
    "B1": { "raw": "จำนวนเงิน", "v": "จำนวนเงิน", "style": { "b": true, "align": "right" } },
    "A2": { "raw": "ค่าไฟ", "v": "ค่าไฟ" },
    "B2": { "raw": "1450", "v": 1450, "fmt": "currency" },
    "A3": { "raw": "รวม", "v": "รวม", "style": { "b": true } },
    "B3": { "raw": "=SUM(B2:B2)", "v": 1450, "style": { "b": true, "align": "right" } }
  },
  "merges": [
    { "start": "A1", "end": "A1" }
  ],
  "colWidths": {
    "0": 180,
    "1": 120
  }
}
```

---

## 2. REST API Integration (`http://localhost:8080` / Tailscale)

### 2.1 Notes API

| Endpoint | Method | Description | Payload / Params |
|---|---|---|---|
| `/api/notes` | `GET` | List notes | Query: `?tag=todo&search=keyword` |
| `/api/notes` | `POST` | Create note | JSON: `{ "title": "...", "content": "...", "tags": ["todo"], "is_pinned": false }` |
| `/api/notes/:id` | `GET` | Get single note | None |
| `/api/notes/:id` | `PUT` | Update note | JSON: Partial or full fields to update |
| `/api/notes/:id` | `DELETE`| Delete note | None |
| `/api/notes/:id/ai` | `POST` | Run AI assist | JSON: `{ "action": "summarize" \| "action_items" \| "polish" }` |

### 2.2 Sheets API

| Endpoint | Method | Description | Payload / Params |
|---|---|---|---|
| `/api/sheets` | `GET` | List all sheet summaries | None |
| `/api/sheets` | `POST` | Create a new sheet | JSON: `{ "title": "...", "row_count": 30, "col_count": 10, "data_json": "{}" }` |
| `/api/sheets/:id` | `GET` | Get full sheet with data | None |
| `/api/sheets/:id` | `PUT` | Update sheet content | JSON: `{ "title": "...", "data_json": "..." }` |
| `/api/sheets/:id` | `DELETE`| Delete sheet | None |
| `/api/sheets/:id/ai` | `POST` | AI table analysis/formula | JSON: `{ "action": "explain" \| "formula_suggest", "prompt": "..." }` |

---

## 3. Telegram Capture Workflows

### 3.1 Creating a Note via Telegram
Users can send notes directly via the Sentinel Bot:
```text
/note รายการของต้องซื้อ: นมสด ไข่ไก่ ขนมปัง #todo #groceries
```
- **Parsing Rule:**
  - If text contains multiple lines, line 1 is `title`, remainder is `content`.
  - Words prefixed with `#` are automatically extracted and saved in the `tags` column.
  - Returns a Telegram HTML preview with inline buttons `[ 🗑 ลบ ]` and `[ 📌 ปักหมุด ]`.

### 3.2 Listing Notes
```text
/notes          -> Returns 5 most recent notes
/notes todo     -> Returns 5 most recent notes tagged with #todo
```

---

## 4. Formula & Calculation Conventions

When writing or generating formulas programmatically for `workspace_sheets`:
- Always prefix formula strings with `=`.
- Cell coordinates: Upper-case column letters + 1-based row number (e.g. `A1`, `B12`).
- Range expressions use a colon `:` (e.g. `A1:A10`).
- Standard aggregations:
  - `=SUM(range)` — sums all numeric values in range.
  - `=AVERAGE(range)` or `=AVG(range)` — arithmetic mean.
  - `=COUNT(range)` — counts non-empty numeric cells.
  - `=MIN(range)` / `=MAX(range)` — minimum / maximum.
- Arithmetic expressions: `=A1+B1`, `=A1*1.07`, `=(B2-B1)/B1`.
- When updating cells programmatically via API, always calculate and provide the scalar evaluation in the `"v"` attribute alongside `"raw"` so clients can render without waiting for calculation:
  ```json
  "C1": { "raw": "=A1+B1", "v": 350 }
  ```

---

## 5. Security & Privacy Rules

- **Default Privacy:** All notes and sheets are `sync_public = 0` (strictly local to v11).
- **Public Whitelist:** A note or sheet is pushed to Firestore / Vercel Hub only when explicitly toggled to `sync_public = 1` by the user.
- **AI Assist Sanitization:** Prompts passed to `/api/notes/:id/ai` or `/api/sheets/:id/ai` must be validated via `bot.PromptGuard` to block prompt injections and command execution attempts.
