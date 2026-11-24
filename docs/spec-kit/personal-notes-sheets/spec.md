# Pocket Studio — Personal Notes & Lightweight Sheets Specification

**Status:** Approved Design Specification  
**Product:** Pocket Sentinel (vivo 1804 Termux ARM64) + Embedded Dashboard + Vercel Hub  
**Target Modules:** `modules/workspace` (backend), `modules/dashboard` (PWA UI), `modules/bot` (Telegram), `web/` (Public Hub)  
**Date:** 2026-11-24  
**Author:** Antigravity AI & System Architect  

---

## 1. Intent & Problem Statement

Pocket Sentinel runs autonomously on a vivo 1804 (V11) Android device via Termux. While it excels at system telemetry, monitoring, alerting, location tracking, and AI queries, users currently lack a dedicated, private, and lightweight personal workspace for capturing notes, maintaining checklists, and working with tabular calculations.

Existing off-the-shelf solutions fail to meet the project's strict physical and operational constraints:
- **Heavy Resource Footprint:** Self-hosted tools like Notion-clones (AppFlowy, Outline, Memos with Node/Docker) or spreadsheets (NocoDB, Grist, Ethercalc, LibreOffice Online) require hundreds of megabytes of RAM, Node.js runtimes, or multi-container stacks. Running them on v11 would immediately exceed the **300MB RAM budget**, triggering Android Low Memory Killer (LMK) and crashing the core monitoring services.
- **Cloud Dependency & Privacy Concerns:** Storing personal daily notes, financial calculations, or private server credentials in commercial cloud platforms (Google Keep, Google Sheets, Notion) violates the user's requirement for sovereign data privacy and local ownership.
- **Fragmented Workflow:** Switching between external note applications and Pocket Sentinel disrupts monitoring workflows. The user cannot quickly capture thoughts directly from Telegram while checking server health, nor link server logs/AI insights to notes.

### The Solution:
Build **Pocket Studio (Personal Notes & Lightweight Sheets)** as a native, embedded module inside Pocket Sentinel:
1. **Single-Binary Zero Daemon Architecture:** Runs entirely within the existing Go process (`v11-server`) and pure-Go SQLite driver (`modernc.org/sqlite`), adding **less than 15MB RAM overhead**.
2. **Personal Notes Module:** Markdown-first note engine with live preview, interactive checkboxes (`- [ ]`), tagging, full-text search, and instant capture via Telegram bot (`/note <text>`).
3. **Lightweight Sheets Module:** Interactive web data grid mimicking essential Google Sheets workflows: keyboard navigation, cell styling, cell merging, automatic summaries (`SUM`, `AVG`, `COUNT`), basic formula evaluation (`=A1+B1`, `=SUM(...)`), and CSV import/export.
4. **Hybrid Storage & Privacy:** Stored locally in `./data.db` on v11 by default. Provides an optional, selective sync flag to publish whitelisted documents to Firestore for viewing on the Vercel Public Hub.
5. **Integrated AI Assistant:** Leverages existing Fast LLMs (Groq & Gemini in `shared/ai`) to summarize notes, extract action items, or analyze sheet tables on-demand.

---

## 2. Goals & Non-goals

### Goals
- **Strict RAM & Process Budget:** Total memory increase must remain $< 15\text{MB}$. Zero additional background daemons or Node.js runtimes under PM2.
- **Offline-First Resilience:** Embedded dashboard UI functions 100% offline via pre-embedded client assets in Go (`go:embed static/*`) and Service Worker cache.
- **Keyboard-First Spreadsheet UX:**
  - Standard spreadsheet navigation (`Arrow keys`, `Tab`, `Enter`, `F2` to edit).
  - Cell formatting: Bold, italic, text color, background color, horizontal alignment.
  - Cell merging (`rowSpan` & `colSpan`) with clean re-rendering.
  - Formula computation: Basic arithmetic (`+`, `-`, `*`, `/`), range aggregation (`SUM`, `AVERAGE`, `COUNT`, `MIN`, `MAX`).
  - Data import/export: 1-click CSV export and upload import.
- **Markdown-Centric Note Taking:**
  - Split-view / tabbed Live Preview using embedded `marked.min.js`.
  - Interactive checklists (`- [ ]`) toggleable with a click without manual text editing.
  - Instant note capture from Telegram: `/note <text> #tag`.
- **Workspace Navigation:** Clean top tab bar `[ 📊 Monitor | 📝 Notes | 📈 Sheets ]` on Dashboard with a 1-click **Focus Mode** that maximizes screen real estate.
- **Built-in Fast AI Assist:** One-click AI actions powered by Groq (`openai/gpt-oss-120b`) and Gemini (`gemini-2.5-flash`) for note summaries and sheet insights.

### Non-goals
- Full Excel replacement (no complex macros, VBA, 3D pivots, or hundreds of obscure statistical functions).
- Real-time multi-user Google Docs collaborative editing (this is a personal single-tenant workspace).
- Storing multi-gigabyte binary attachments or video files inside SQLite.

---

## 3. High-Level Architecture & Data Flow

```
                      ┌─────────────────────────────────────────┐
                      │              USER CLIENTS               │
                      └────┬─────────────────┬────────────────┬─┘
                           │                 │                │
            Telegram Chat  │   Tailscale PWA │   Vercel Hub   │ (Public HTTPS)
            (/note /notes) │ (Private HTTP)  │                │
                           │                 │                │
                           ▼                 ▼                ▼
                     ┌───────────┐     ┌───────────┐    ┌───────────┐
                     │  Telegram │     │  Embedded │    │   Vercel  │
                     │  Bot API  │     │  Gin App  │    │ React Hub │
                     └─────┬─────┘     └─────┬─────┘    └─────┬─────┘
                           │                 │                │
                           │                 │         (Filtered Sync)
                           │                 │                │
                           ▼                 ▼                ▼
                     ┌─────────────────────────────┐    ┌───────────┐
                     │   Pocket Sentinel Backend   │◄───┤ Firestore │
                     │   (modules/workspace)       │    │  Buffer   │
                     ├─────────────────────────────┤    └───────────┘
                     │ • NotesService              │
                     │ • SheetsService             │
                     │ • FormulaEngine             │
                     │ • AIAssistant               │
                     └──────────────┬──────────────┘
                                    │
                                    ▼
                     ┌─────────────────────────────┐
                     │   SQLite Store (data.db)    │
                     │   - notes table             │
                     │   - sheets table            │
                     └─────────────────────────────┘
```

---

## 4. Functional Specifications

### 4.1 Notes Module (`modules/workspace/notes`)

#### Capabilities:
1. **Creation & Editing:**
   - Note fields: `id` (UUID), `title` (string), `content` (Markdown string), `tags` (array of strings), `is_pinned` (boolean), `sync_public` (boolean), `created_at`, `updated_at`.
   - Markdown syntax support: Headers (H1–H6), Bold/Italic, Lists (ordered/unordered), GFM Tables, Blockquotes, Inline Code, Code Blocks.
   - Interactive Checklists: `- [ ]` and `- [x]` rendered as clickable HTML checkboxes. Clicking a checkbox updates the markdown string in memory and dispatches an auto-save request within 500ms debounce.
2. **Organization & Discovery:**
   - Tag filtering: Tags starting with `#` are automatically indexed and presented as filter pills.
   - Pinned notes: Pinned notes float to the top of the note list.
   - Real-time search: Sub-5ms client-side filtering on title, tags, and content.
3. **Telegram Quick Capture:**
   - `/note <text>`: Creates a new note. If the first line is short, it becomes the title; otherwise, current date/time is used as title. Hashtags (e.g. `#todo`, `#ideas`) are extracted as tags.
   - `/notes [tag]`: Lists recent 5 notes matching tag or last 5 modified notes with inline view/delete buttons.
4. **AI Assistant:**
   - Action: `summarize`: Produces a 2–3 bullet point executive summary.
   - Action: `action_items`: Extracts todos and action items into a clean checklist.
   - Action: `polish`: Fixes grammar and improves formatting while keeping original meaning.

---

### 4.2 Sheets Module (`modules/workspace/sheets`)

#### Capabilities:
1. **Grid Model & Data Representation:**
   - Sheet fields: `id` (UUID), `title` (string), `row_count` (int, default 30), `col_count` (int, default 10), `data_json` (serialized JSON of cell records, formulas, merges, and styles), `sync_public` (boolean), `created_at`, `updated_at`.
   - Cell Coordinate System: Column letters (`A`, `B`, `...`, `Z`, `AA`..) and Row numbers (`1`, `2`, `3`...). Example: `B3`.
2. **Interactive Editing & Navigation:**
   - Click cell to select; double-click or press `Enter`/`F2` to enter edit mode.
   - `Arrow keys` move selection. `Tab` moves right, `Shift+Tab` moves left.
   - Top formula bar displays current cell coordinate (e.g., `B2`) and raw value/formula.
3. **Cell Styling & Formatting:**
   - Text styling: **Bold** (`fontWeight: "bold"`), *Italic* (`fontStyle: "italic"`).
   - Text alignment: `left`, `center`, `right`.
   - Colors: Text color, Background fill color (from restrained palette matching UI theme).
   - Number formatting: Plain text, Currency (`฿` or `$`), Decimal (2 places), Percentage (`%`).
4. **Cell Merging:**
   - User selects a range (e.g., `A1:C1`) and clicks "Merge Cells".
   - Top-left cell retains data; other merged cells display empty and are un-editable.
   - Clicking "Unmerge" restores individual cells.
5. **Formula & Calculation Engine:**
   - Built-in evaluator handles formula strings beginning with `=`.
   - Supported Functions:
     - `=SUM(A1:A10)`: Calculates total.
     - `=AVERAGE(A1:A10)` or `=AVG(A1:A10)`: Calculates arithmetic mean.
     - `=COUNT(A1:A10)`: Counts numeric cells.
     - `=MIN(A1:A10)` / `=MAX(A1:A10)`: Finds minimum / maximum values.
     - Basic arithmetic expressions: `=A1+B1`, `=A1*1.07`, `=(A1-B1)/C1`.
   - Live Selection Summary Bar: When user drags to select multiple numeric cells, bottom right bar shows: `SUM: 1,420 | AVG: 284.0 | COUNT: 5`.
6. **Portability:**
   - **Export CSV:** Exports currently viewed sheet data to RFC 4180 compliant CSV file downloaded in browser.
   - **Import CSV:** Parses uploaded CSV file and populates the active sheet.
7. **AI Assistant:**
   - Action: `explain`: Explains trends or anomalies in the selected data range.
   - Action: `formula_suggest`: User asks in natural language (e.g., "คิดภาษี 7% ของคอลัมน์ B"), AI generates the exact formula.

---

### 4.3 UI & Navigation Layout

#### Workspace Tabs:
- Located at top navigation rail of Dashboard:
  - `[ 📊 Monitor ]` — System telemetry, charts, live map, logs, hardware controls.
  - `[ 📝 Notes ]` — Personal Markdown notes & quick memos.
  - `[ 📈 Sheets ]` — Interactive spreadsheet grid & calculations.
- **Focus Mode Toggle:** A prominent button `[ ⛶ Focus ]` expands the active tab to full viewport width (hiding the telemetry rails and sidebars) for writing and calculating without distraction.

---

## 5. Database Schema & Migration (SQLite)

Migration will be added to `shared/db/db.go` within the central database initialization routine:

```sql
-- Migration: Workspace Notes & Sheets
CREATE TABLE IF NOT EXISTS workspace_notes (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    tags TEXT NOT NULL DEFAULT '',
    is_pinned INTEGER NOT NULL DEFAULT 0,
    sync_public INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workspace_notes_updated 
ON workspace_notes(updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_workspace_notes_pinned 
ON workspace_notes(is_pinned DESC, updated_at DESC);

CREATE TABLE IF NOT EXISTS workspace_sheets (
    id TEXT PRIMARY KEY,
    title TEXT NOT NULL,
    row_count INTEGER NOT NULL DEFAULT 30,
    col_count INTEGER NOT NULL DEFAULT 10,
    data_json TEXT NOT NULL DEFAULT '{}',
    sync_public INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_workspace_sheets_updated 
ON workspace_sheets(updated_at DESC);
```

### JSON Structure for `workspace_sheets.data_json`:
```json
{
  "cells": {
    "A1": { "raw": "รายการ", "v": "รายการ", "style": { "b": true, "align": "center", "bg": "#f0f2f5" } },
    "B1": { "raw": "จำนวนเงิน", "v": "จำนวนเงิน", "style": { "b": true, "align": "right", "bg": "#f0f2f5" } },
    "A2": { "raw": "ค่าไฟ", "v": "ค่าไฟ" },
    "B2": { "raw": "1450", "v": 1450, "fmt": "currency" },
    "A3": { "raw": "ค่าเน็ต", "v": "ค่าเน็ต" },
    "B3": { "raw": "699", "v": 699, "fmt": "currency" },
    "A4": { "raw": "รวมทั้งสิ้น", "v": "รวมทั้งสิ้น", "style": { "b": true } },
    "B4": { "raw": "=SUM(B2:B3)", "v": 2149, "style": { "b": true, "align": "right" }, "fmt": "currency" }
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

## 6. REST API Specification

### 6.1 Notes API

#### `GET /api/notes`
- Query Params: `tag` (string, optional), `search` (string, optional)
- Returns: `200 OK`
```json
[
  {
    "id": "nt_8f91a2",
    "title": "รายการซื้อของ",
    "content": "- [x] นมสด\n- [ ] ไข่ไก่ 1 แผง\n- [ ] ขนมปังโฮลวีต",
    "tags": ["groceries", "todo"],
    "is_pinned": true,
    "sync_public": false,
    "created_at": "2026-11-24T12:00:00Z",
    "updated_at": "2026-11-24T12:05:00Z"
  }
]
```

#### `POST /api/notes`
- Request Body:
```json
{
  "title": "บันทึกไอเดียใหม่",
  "content": "วางโครงสร้างระบบ workspace บน v11",
  "tags": ["dev", "v11"],
  "is_pinned": false,
  "sync_public": false
}
```
- Returns: `201 Created` with created note object.

#### `PUT /api/notes/:id`
- Request Body: Partial or full fields to update.
- Returns: `200 OK` with updated note object.

#### `DELETE /api/notes/:id`
- Returns: `200 OK` `{"success": true, "id": "nt_8f91a2"}`

#### `POST /api/notes/:id/ai`
- Request Body:
```json
{
  "action": "summarize" | "action_items" | "polish",
  "instruction": "string optional"
}
```
- Returns: `200 OK`
```json
{
  "action": "summarize",
  "result": "1. ซื้อของใช้จำเป็น\n2. ตรวจสอบของที่เหลือในตู้เย็น",
  "provider": "groq",
  "duration_ms": 420
}
```

---

### 6.2 Sheets API

#### `GET /api/sheets`
- Returns: `200 OK` List of sheet headers (`id`, `title`, `row_count`, `col_count`, `updated_at`, `sync_public`).

#### `GET /api/sheets/:id`
- Returns: `200 OK` Full sheet object including `data_json`.

#### `POST /api/sheets`
- Request Body:
```json
{
  "title": "บันทึกรายรับรายจ่าย พ.ย. 2026",
  "row_count": 30,
  "col_count": 10,
  "data_json": "{}"
}
```
- Returns: `201 Created`

#### `PUT /api/sheets/:id`
- Request Body: `title`, `row_count`, `col_count`, `data_json`, `sync_public`
- Returns: `200 OK`

#### `DELETE /api/sheets/:id`
- Returns: `200 OK` `{"success": true, "id": "sh_1a2b3c"}`

#### `POST /api/sheets/:id/ai`
- Request Body:
```json
{
  "action": "explain" | "formula_suggest",
  "prompt": "ขอสูตรคำนวณกำไรสุทธิจากคอลัมน์ C ลบ B"
}
```
- Returns: `200 OK`
```json
{
  "result": "=C2-B2",
  "explanation": "นำคอลัมน์รายได้ (C2) ลบด้วยต้นทุน (B2) เพื่อคำนวณกำไรสุทธิของแถวนั้น",
  "provider": "groq",
  "duration_ms": 380
}
```

---

## 7. Security, Privacy & Guardrails

1. **Alpine.js Proxy Guard Compliance (CRITICAL):**
   - In accordance with repository architecture rules, internal data structures representing the spreadsheet grid (such as DOM table cell matrices or calculation dependency graphs) **MUST NOT** be stored as reactive properties inside `Alpine.data(...)`.
   - Grid instances and formula calculation caches must live in a module-scoped variable `const sheetGridStore = { ... }`, with Alpine holding only plain primitives (`sheetId`, `title`, `isEditing`, `activeCell`).
2. **Strict Size Limits:**
   - Single note content capped at 1 MB (plenty for thousands of lines of text).
   - Single sheet grid capped at $100 \times 26$ cells (2,600 cells) to preserve sub-second calculation and prevent memory spikes on mobile browsers.
3. **Prompt Injection & Redaction:**
   - AI requests passing through `/api/notes/:id/ai` or `/api/sheets/:id/ai` validate input through `bot.PromptGuard` to block escape sequences and command injection.
4. **Service Worker Cache Hygiene:**
   - When frontend templates or scripts in `modules/dashboard/` are modified, `CACHE_NAME` in `modules/dashboard/sw.js` must be bumped, and new static assets added to `SHELL_ASSETS`.

---

## 8. Definition of Done & Success Criteria

1. **Compilation & Testing:**
   - `go test ./...` passes with 100% success rate (including unit tests for Store, Service, Handlers, and Formula Engine).
   - `go vet ./...` clean without warnings.
   - `scripts/deploy.sh` builds and deploys successfully to vivo v11.
2. **Operational Verification:**
   - `go-server` memory footprint on v11 stays $< 35\text{MB}$ RAM after editing notes and running spreadsheet calculations.
   - Telegram bot responds to `/note` and `/notes` accurately within 1.5 seconds.
   - Dashboard PWA opens on mobile (`http://100.92.194.87:8080`) with zero console errors.
   - Markdown checklists toggle dynamically; spreadsheet formulas (`=SUM`, `=A1+B1`) evaluate accurately without circular dependency deadlocks.
