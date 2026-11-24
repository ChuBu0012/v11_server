# Pocket Studio — Implementation Tasks Breakdown

**Feature:** Personal Notes & Lightweight Sheets (Pocket Studio)  
**Spec Document:** [spec.md](file:///Users/most-too/Desktop/v11_server/docs/spec-kit/personal-notes-sheets/spec.md)  
**Plan Document:** [plan.md](file:///Users/most-too/Desktop/v11_server/docs/spec-kit/personal-notes-sheets/plan.md)  
**Branch:** `feat/personal-notes-sheets`  
**Status:** Ready for Execution  

---

## Task Overview Checklist

### Phase 1: Database Migration & Core Store
- [ ] **Task 1.1:** Add SQLite schema migrations in `shared/db/db.go`
  - Create table `workspace_notes` with indexes on `updated_at` and `is_pinned`
  - Create table `workspace_sheets` with index on `updated_at`
- [ ] **Task 1.2:** Implement `modules/workspace/types.go`
  - Define `Note`, `CreateNoteRequest`, `UpdateNoteRequest`, `NoteFilter`
  - Define `Sheet`, `CreateSheetRequest`, `UpdateSheetRequest`, `SheetCell`, `SheetData`
- [ ] **Task 1.3:** Implement `modules/workspace/store.go`
  - `CreateNote`, `GetNote`, `UpdateNote`, `DeleteNote`, `ListNotes` (with tag and keyword search)
  - `CreateSheet`, `GetSheet`, `UpdateSheet`, `DeleteSheet`, `ListSheets`
- [ ] **Task 1.4:** Unit tests for store operations in `modules/workspace/store_test.go`
  - Verify CRUD operations on temporary SQLite instance
  - Verify tag extraction, pinning, and sorting

### Phase 2: Formula Engine & Business Service
- [ ] **Task 2.1:** Implement formula engine in `modules/workspace/formula.go`
  - Coordinate parser: letters/numbers to `(col, row)` coordinates
  - Range expander: `A1:B3` to coordinate array
  - Tokenizer and expression evaluator for basic math (`+`, `-`, `*`, `/`)
  - Aggregation functions: `SUM`, `AVERAGE` / `AVG`, `COUNT`, `MIN`, `MAX`
  - Directed cycle detector with maximum recursion depth protection (`maxDepth = 32`)
- [ ] **Task 2.2:** Unit tests for formula engine in `modules/workspace/formula_test.go`
  - Test simple math (`=1+2*3`)
  - Test cell references (`=A1+B1`)
  - Test ranges and aggregations (`=SUM(A1:A5)`)
  - Test circular reference detection (`A1=B1, B1=A1`) returns `#REF! CIRCULAR`
- [ ] **Task 2.3:** Implement `modules/workspace/service.go`
  - Business validations (length checks, JSON structure checks)
  - Automatic hashtag extraction from note body
  - Integration with formula evaluator for sheet validation
- [ ] **Task 2.4:** Implement AI Assistant actions in `modules/workspace/ai.go`
  - Note actions: `summarize`, `action_items`, `polish` via `shared/ai/engine.go`
  - Sheet actions: `explain`, `formula_suggest`
  - Integration with `bot.PromptGuard` to sanitize AI prompts

### Phase 3: REST API Handlers & Server Integration
- [ ] **Task 3.1:** Implement `modules/workspace/handler.go`
  - Handlers for `GET /api/notes`, `POST /api/notes`, `GET /api/notes/:id`, `PUT /api/notes/:id`, `DELETE /api/notes/:id`
  - Handlers for `POST /api/notes/:id/ai`
  - Handlers for `GET /api/sheets`, `POST /api/sheets`, `GET /api/sheets/:id`, `PUT /api/sheets/:id`, `DELETE /api/sheets/:id`
  - Handlers for `POST /api/sheets/:id/ai`
- [ ] **Task 3.2:** Register workspace routes in `main.go` under `setupRouter`
- [ ] **Task 3.3:** HTTP handler integration tests in `modules/workspace/handler_test.go`
  - Test all HTTP status codes, error payloads, and validation constraints

### Phase 4: Telegram Bot Integration
- [ ] **Task 4.1:** Add Telegram command handlers in `modules/bot/router.go`
  - Case `/note`: Parse title, body, and tags; call `workspace.Service.CreateNote`
  - Case `/notes`: Query recent notes, return inline keyboard with view/delete actions
  - Callback query handler: Handle inline button clicks for note actions
- [ ] **Task 4.2:** Create message formatting templates in `modules/bot/workspace_templates.go`
  - HTML escaping, blockquotes, and tag formatting
- [ ] **Task 4.3:** Update `/help` command description and `MakeHelpKeyboard()` in `modules/bot/router.go`
- [ ] **Task 4.4:** Unit tests in `modules/bot/workspace_test.go`
  - Test command parsing, markdown note creation, and hashtag extraction

### Phase 5: Embedded Dashboard Frontend (Notes)
- [ ] **Task 5.1:** Add Workspace Tab navigation to `modules/dashboard/index.html`
  - Tab selector: `[ 📊 Monitor | 📝 Notes | 📈 Sheets ]`
  - Focus Mode toggle button `[ ⛶ Focus ]`
- [ ] **Task 5.2:** Create stylesheet `modules/dashboard/static/css/workspace.css`
  - Responsive tab styling, split-pane layout, markdown editor styling, checklist styling
- [ ] **Task 5.3:** Create `modules/dashboard/static/js/components/workspace.js`
  - Tab switching logic and full-screen / focus mode state
- [ ] **Task 5.4:** Create `modules/dashboard/static/js/components/notes.js`
  - Note list sidebar with tag filter pills and search bar
  - Markdown editor with live preview split view using `marked.min.js`
  - Clickable interactive checkboxes (`- [ ]`) with automatic debounced save
  - AI Assistant popup modal (Summarize, Extract Actions, Polish)

### Phase 6: Embedded Dashboard Frontend (Lightweight Sheets)
- [ ] **Task 6.1:** Create `modules/dashboard/static/js/components/sheets.js`
  - Non-reactive DOM Grid Engine (outside Alpine proxy to satisfy AGENTS.md rule 3)
  - Keyboard navigation (`Arrow keys`, `Tab`, `Enter`, `F2`)
  - Cell formatting toolbar: Bold, Italic, Color, Align, Merge Cells, Unmerge
  - Formula bar with live cell coordinate display and formula entry
  - Auto-calculating summary footer: `SUM`, `AVG`, `COUNT` for current selection
  - CSV Import & Export handlers (RFC 4180)
- [ ] **Task 6.2:** Integrate Sheet component container into `modules/dashboard/index.html`
- [ ] **Task 6.3:** Service Worker cache update in `modules/dashboard/sw.js`
  - Add `/static/css/workspace.css` and all new component scripts to `SHELL_ASSETS`
  - Bump `CACHE_NAME` to `sentinel-shell-v50`
- [ ] **Task 6.4:** Update `modules/dashboard/dashboard_test.go` static file verification list

### Phase 7: Public Hub (Vercel) Sync & Public View
- [ ] **Task 7.1:** Update `modules/cloudsync/` to support syncing notes/sheets with `sync_public = 1` to Firestore collection `sentinel_workspace`
- [ ] **Task 7.2:** Add public read-only viewer for shared notes and sheets in `web/src/components/WorkspacePublicView.tsx`

### Phase 8: Verification, Benchmarking & Deployment
- [ ] **Task 8.1:** Full unit test suite run (`go test ./...`)
- [ ] **Task 8.2:** Web build verification (`cd web && npm run build`)
- [ ] **Task 8.3:** Cross-compile and deploy to vivo v11 (`./scripts/deploy.sh -m "feat: Pocket Studio personal notes and lightweight sheets"`)
- [ ] **Task 8.4:** Verify PM2 process memory footprint on v11 stays $< 35\text{MB}$ RAM
- [ ] **Task 8.5:** Live verification of `/note` on Telegram and Web UI on Tailscale (`http://100.92.194.87:8080`)
- [ ] **Task 8.6:** Commit all changes and push to `origin feat/personal-notes-sheets`
