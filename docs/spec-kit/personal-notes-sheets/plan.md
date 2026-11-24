# Pocket Studio — Technical Implementation Plan

**Target Module:** `modules/workspace`, `modules/dashboard`, `modules/bot`, `shared/db`  
**Dependencies:** Standard Go library, `modernc.org/sqlite`, Gin HTTP framework, `marked.min.js`  
**Status:** Approved Architecture Plan  
**Date:** 2026-11-24  

---

## 1. Architectural Strategy & Seams

The Pocket Studio system is divided into three cleanly isolated layers to ensure stability, testability, and zero impact on core monitoring:

1. **Storage Layer (`modules/workspace/store.go`):**
   - Directly interfaces with the initialized `*sql.DB` from `shared/db`.
   - Uses parameterized queries exclusively.
   - Enforces optimistic concurrency via `updated_at` timestamps.
2. **Domain & Calculation Service (`modules/workspace/service.go` + `formula.go`):**
   - Manages Notes business logic (slug/id generation, hashtag extraction, markdown sanitization).
   - Contains a pure Go deterministic formula evaluator (`formula.go`) that parses cell references (`A1`, `B2`), arithmetic expressions, and aggregation functions (`SUM`, `AVG`, `COUNT`, `MIN`, `MAX`) with cycle detection.
   - Integrates with `shared/ai/engine.go` for on-demand AI assist.
3. **Delivery Layers:**
   - **HTTP API (`modules/workspace/handler.go`):** RESTful endpoints plugged into Gin engine in `main.go`.
   - **Telegram Router (`modules/bot/router.go`):** Direct bot commands (`/note`, `/notes`) calling `workspace.Service`.
   - **Client UI (`modules/dashboard/`):** Embedded PWA Alpine.js views + non-reactive Grid Engine.

---

## 2. Component Design & Directory Structure

```
modules/workspace/
├── store.go             # SQLite database operations for notes & sheets
├── store_test.go        # Unit tests for store persistence & query edge cases
├── service.go           # Business logic, note processing, sheet metadata validation
├── service_test.go      # Service mock tests
├── formula.go           # Pure Go spreadsheet formula parser & calculation engine
├── formula_test.go      # Comprehensive formula tests (arithmetic, functions, cycle detection)
├── handler.go           # Gin HTTP REST handlers
├── handler_test.go      # HTTP request/response tests
├── types.go             # Data structs, DTOs, and request/response models
└── ai.go                # AI-assist actions (summarize, action items, formula generator)

modules/dashboard/
├── index.html           # Embedded Workspace Tab Bar & layout containers
├── sw.js                # Service Worker asset list & cache invalidation
└── static/
    ├── css/
    │   └── workspace.css # Styling for Workspace tabs, Markdown editor, and Spreadsheet grid
    └── js/
        └── components/
            ├── workspace.js # Workspace tab orchestration & focus mode
            ├── notes.js     # Notes Alpine component (Markdown editor + live preview)
            └── sheets.js    # Sheets Alpine component + Native DOM Grid Engine
```

---

## 3. Spreadsheet Grid & Formula Engine Architecture

### 3.1 Strict Alpine.js Proxy Guardrails (CRITICAL)
In accordance with repository guidelines:
> *Complex JavaScript objects with heavy prototypes or DOM references MUST NOT be placed inside `Alpine.data(...)` reactive state to avoid non-configurable property TypeErrors.*

#### Implementation Pattern:
```javascript
// Scope: Outside Alpine closure (modules/dashboard/static/js/components/sheets.js)
const sheetEngine = {
  activeSheetId: null,
  cells: {},          // Raw cell data: { "A1": { raw: "=SUM(B1:B5)", v: 100, style: {} } }
  merges: [],         // Merged cell definitions
  selection: { start: null, end: null }, // Selected range
  history: [],        // Undo/Redo stack
  
  initGrid(containerEl) { ... },
  render(containerEl) { ... },
  evaluateFormulas() { ... },
  getCellValue(coord) { ... },
  setCellValue(coord, rawValue) { ... }
};

// Alpine Component: Holds only primitive UI states
Alpine.data('sheetsApp', () => ({
  sheetList: [],
  currentSheetId: null,
  currentSheetTitle: '',
  selectedCoord: 'A1',
  formulaBarValue: '',
  isEditing: false,
  summaryText: '',    // "SUM: 100 | AVG: 20 | COUNT: 5"
  isSaving: false,

  init() {
    this.loadSheetList();
  },
  
  onCellSelected(coord) {
    this.selectedCoord = coord;
    this.formulaBarValue = sheetEngine.getRawValue(coord);
    this.summaryText = sheetEngine.calculateSelectionSummary();
  }
}));
```

### 3.2 Formula Parsing & Evaluation Specification
The formula engine in `modules/workspace/formula.go` parses cell coordinates into zero-based column and row indices:
- Letters to Column index: `A` $\rightarrow 0$, `B` $\rightarrow 1$, `Z` $\rightarrow 25$, `AA` $\rightarrow 26$.
- Numbers to Row index: `1` $\rightarrow 0$, `2` $\rightarrow 1$.
- Range expression: `A1:B3` expands to `[A1, A2, A3, B1, B2, B3]`.

#### Cycle Detection Algorithm:
A directed dependency graph tracks cell evaluation. A depth counter (`maxDepth = 32`) and a `visited` map detect circular references (e.g., `A1 = B1 + 1` and `B1 = A1 + 1`). If a cycle is detected, the formula immediately returns `#REF! CIRCULAR` without crashing or freezing the Go server or browser.

---

## 4. Telegram Integration Architecture

### Commands:
- `/note <text>`:
  - If text contains multiple lines, line 1 is `title`, subsequent lines are `content`.
  - Extracts `#hashtags` from the text and populates `tags`.
  - Saves to `workspace_notes` with `is_pinned = false`.
  - Replies with formatted Telegram HTML confirming note creation with inline action buttons `[ 🗑 Delete ]` and `[ 📌 Pin ]`.
- `/notes [tag]`:
  - Fetches the 5 most recently updated notes matching `tag` (or top 5 if empty).
  - Formats output as an HTML list with preview snippets and inline buttons to view or delete.

#### Telegram HTML Message Template (`modules/bot/workspace_templates.go`):
Following `.agents/skills/telegram-discord-message-templates/SKILL.md`:
```html
📝 <b>บันทึกโน้ตสำเร็จ</b>
<b>หัวข้อ:</b> รายการของต้องซื้อ
<b>หมวดหมู่:</b> <code>#groceries</code> <code>#todo</code>
<blockquote>• นมสด
• ไข่ไก่ 1 แผง
• กาแฟดำ</blockquote>
<i>บันทึกใน Pocket Sentinel เรียบร้อย</i>
```

---

## 5. UI/UX Interaction Details

### 5.1 Workspace Tab Bar & Layout
```
+-------------------------------------------------------------------------+
| [ 🛡 Pocket Sentinel ]      [ 📊 Monitor ] [ 📝 Notes ] [ 📈 Sheets ]   |
+-------------------------------------------------------------------------+
| [ + โน้ตใหม่ ] [ 🔍 ค้นหา... ] [ ⛶ โหมดขยายจอเต็ม ]                     |
|-------------------------------------------------------------------------|
| [รายการโน้ต 30%]              | [ตัวแก้ไข Markdown / พรีวิว 70%]        |
| - 📌 รายการซื้อของ (#todo)     | หัวข้อ: [ รายการซื้อของ              ] |
| - แผนงานเซิร์ฟเวอร์ (#dev)     | แท็ก:   [ #groceries #todo           ] |
| - บันทึกการเงิน (#finance)    | [ B ] [ I ] [ ≡ ] [ - [ ] ] [ 🤖 สรุป ] |
|                               | --------------------------------------- |
|                               | - [x] นมสด                              |
|                               | - [ ] ไข่ไก่ 1 แผง                      |
|                               | - [ ] กาแฟ                              |
+-------------------------------------------------------------------------+
```

### 5.2 Lightweight Sheets Layout
```
+-------------------------------------------------------------------------+
| [ + ชีตใหม่ ] [ 📂 เปิดไฟล์ ] [ 📥 นำเข้า CSV ] [ 📤 ส่งออก CSV ] [ ⛶ ]|
|-------------------------------------------------------------------------|
| [ B2 ] | fx: [ =SUM(B2:B3)                                            ] |
|-------------------------------------------------------------------------|
| [ B ] [ I ] [ 🎨 สี ] [ ⬍ ผสานเซลล์ ] [ ⬅ ] [ ⬛ ] [ ➡ ] [ 🤖 ผู้ช่วย ]|
|-------------------------------------------------------------------------|
|   |        A        |        B        |        C        |       D       |
| 1 | รายการ          | จำนวนเงิน       | หมายเหตุ        |               |
| 2 | ค่าไฟ           | 1,450.00        | บิลเดือน พ.ย.   |               |
| 3 | ค่าเน็ต          |   699.00        | ทรูไฟเบอร์      |               |
| 4 | รวมทั้งสิ้น     | 2,149.00        |                 |               |
| 5 |                 |                 |                 |               |
|-------------------------------------------------------------------------|
| ชีต 1 | [ + เพิ่มแท็บ ]                 SUM: 2,149.00 | COUNT: 2        |
+-------------------------------------------------------------------------+
```

---

## 6. Service Worker & Offline Strategy
- Update `modules/dashboard/sw.js`:
  - Bump `CACHE_NAME` to `sentinel-shell-v50`.
  - Add `/static/css/workspace.css`, `/static/js/components/workspace.js`, `/static/js/components/notes.js`, and `/static/js/components/sheets.js` to `SHELL_ASSETS`.
- Add local storage fallback:
  - If network drops while editing a note or sheet, the client stores a draft in `localStorage` under `sentinel_draft_note_<id>` or `sentinel_draft_sheet_<id>` and flushes to the server automatically once connectivity is restored.

---

## 7. Memory & Performance Guardrails on vivo v11

1. **Android Kernel 4.4 / Termux Constraints:**
   - Database operations use single-connection write locking (WAL mode enabled in `shared/db/db.go`), preventing SQLite busy lockups.
2. **Binary Compilation:**
   - Compiled via standard `./scripts/deploy.sh` with `CGO_ENABLED=0 GOOS=android GOARCH=arm64`.
   - Zero dynamic C library linking.
3. **RAM Ceiling Verification:**
   - Verification test must assert that resident memory (`RSS`) does not surge by more than 15MB when holding 100 notes and 10 sheets with formulas in SQLite.
