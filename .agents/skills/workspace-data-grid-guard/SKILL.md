---
name: workspace-data-grid-guard
description: Best practices and strict architectural guardrails for building interactive spreadsheet grids, cell matrix editors, and formula engines with Alpine.js, preventing reactive proxy traps, managing sticky freeze panes on mobile, avoiding virtual keyboard occlusion, and handling Service Worker caching on vivo v11.
---

# Workspace Data Grid Guard: Alpine.js + Spreadsheet Architecture

When building or modifying interactive spreadsheet grids, table calculation engines, or cell-matrix components in Pocket Sentinel with Alpine.js on constrained devices (vivo v11, Android Termux), you **MUST** adhere to these architectural guardrails.

---

## 1. The Reactive Proxy Trap for Grid Matrices

### Root Cause
Alpine.js wraps all properties returned inside `Alpine.data('component', () => ({ ... }))` in an ECMAScript `Proxy`.

A spreadsheet grid involves complex data structures:
- Coordinate maps: `{ "A1": { raw: "=SUM(B1:B5)", v: 100, style: { b: true } } }`
- Cell range selection models: `{ start: { col: 0, row: 0 }, end: { col: 2, row: 5 } }`
- Merged cell definitions: `[{ start: "A1", end: "B2", colSpan: 2, rowSpan: 2 }]`
- Directed dependency graphs for formula recalculation.

If these objects are placed inside Alpine's reactive data object:
1. Deep mutations trigger thousands of micro-reactivity updates across every keystroke, causing significant CPU lockups and freezing low-power mobile devices.
2. Accessing DOM table cell references or prototypes through the proxy causes unrecoverable runtime errors:
```text
Uncaught TypeError: 'get' on proxy: property is read-only and non-configurable
```

---

## 2. The Outer Grid Engine Pattern (Strict Rule)

**NEVER** place the raw cell matrix, cell objects, selection references, or formula dependency graphs inside `Alpine.data()`.

### Correct Pattern
Store the entire grid engine and cell storage in an outer closure or file-scoped object outside the `Alpine.data` definition. Alpine holds only **primitives**, **string coordinates**, and **UI toggles**:

```javascript
// Scope: Outside Alpine closure (modules/dashboard/static/js/components/sheets.js)
const gridStore = {
  sheetId: null,
  cells: {},          // Raw cell map: { "A1": { raw: "100", v: 100 } }
  merges: [],         // Array of merged cell ranges
  colWidths: {},
  selection: { start: 'A1', end: 'A1' },
  
  // Methods that manipulate raw data
  getCellValue(coord) {
    return this.cells[coord]?.v ?? '';
  },
  getRawFormula(coord) {
    return this.cells[coord]?.raw ?? '';
  },
  setCellValue(coord, raw) {
    // 1. Update raw cell
    // 2. Evaluate formula if starts with '='
    // 3. Mark dirty
  },
  calculateSummary() {
    // Calculates SUM, AVG, COUNT for currently selected cells
    return { sum: 1450, avg: 290, count: 5 };
  }
};

// Alpine Component: Exposes ONLY primitives and UI bindings
document.addEventListener('alpine:init', () => {
  Alpine.data('sheetsComponent', () => ({
    activeCoord: 'A1',
    formulaBarInput: '',
    isEditing: false,
    summaryBarText: '',
    isSaving: false,

    init() {
      // Connect DOM events to outer gridStore
    },

    selectCell(coord) {
      this.activeCoord = coord;
      this.formulaBarInput = gridStore.getRawFormula(coord);
      const summary = gridStore.calculateSummary();
      this.summaryBarText = `SUM: ${summary.sum} | AVG: ${summary.avg} | COUNT: ${summary.count}`;
    },

    commitCellEdit(val) {
      gridStore.setCellValue(this.activeCoord, val);
      this.isEditing = false;
    }
  }));
});
```

---

## 3. Mobile Viewport & Sticky Freeze Panes

On narrow smartphone screens (~360px–390px), horizontal and vertical scrolling easily causes users to lose their place.

### Mandatory CSS Layout:
1. **Sticky Header Row (Row 1):** Must stay pinned at `top: 0` with high `z-index: 10`.
2. **Sticky Row Labels (Col A or Index Column):** Must stay pinned at `left: 0` with `z-index: 11`.
3. **Top-Left Corner Header (`A0` or Empty Intersect):** Must stay pinned with `z-index: 20` to prevent under-lapping during simultaneous diagonal scroll.

```css
.sheet-scroll-container {
  overflow: auto;
  max-height: calc(100vh - 220px);
  position: relative;
  -webkit-overflow-scrolling: touch;
}

.sheet-table th.col-header {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--surface);
}

.sheet-table td.row-header {
  position: sticky;
  left: 0;
  z-index: 11;
  background: var(--surface);
}

.sheet-table th.corner-header {
  position: sticky;
  top: 0;
  left: 0;
  z-index: 20;
  background: var(--surface);
}
```

---

## 4. Mobile Floating Input Bar (Preventing Keyboard Occlusion)

### Problem:
When editing a cell in Row 15 on mobile, the on-screen virtual keyboard pops up, covering the lower 50% of the screen. The user cannot see what they are typing into the cell.

### Solution Pattern:
Do not open a textarea directly inside the small table cell on touch devices. Instead:
1. Detect touch viewport (`window.innerWidth <= 640`).
2. Anchor a **Floating Formula Bar** directly above the keyboard or at the top of the viewport.
3. Automatically scroll the active table row into the visible viewport above the keyboard using `element.scrollIntoView({ block: 'center', behavior: 'smooth' })`.

---

## 5. Formula Evaluation & Cycle Detection

### Rules:
1. **Single-Pass Evaluation with Depth Guard:**
   - Formula parsing evaluates expressions like `=SUM(A1:A5)` or `=A1+B1`.
   - Any dependency chain must maintain a `visited` Set per recalculation pass.
   - If a cell is encountered twice in the same chain (e.g., `A1 -> B1 -> A1`), abort immediately and assign `#REF! CIRCULAR` without throwing or locking the UI thread.
   - Limit evaluation depth to `maxDepth = 32`.

2. **Clean Serialization:**
   - Store both `raw` (the formula/text user typed) and `v` (the evaluated scalar result):
   ```json
   {
     "B4": { "raw": "=SUM(B1:B3)", "v": 2149.00, "style": { "b": true } }
   }
   ```
   - This ensures offline viewers and the server can display the computed value without needing to re-parse formulas on every read.

---

## 6. Service Worker Asset Invalidation

Whenever `workspace.css`, `sheets.js`, or `notes.js` are updated:
1. Add new files to `SHELL_ASSETS` in `modules/dashboard/sw.js`.
2. **BUMP `CACHE_NAME`** (e.g. `sentinel-shell-v49` $\rightarrow$ `sentinel-shell-v50`).
3. Verify with `go test ./modules/dashboard` before deploying to Termux.
