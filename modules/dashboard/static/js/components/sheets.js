// =============================================================================
// Pocket Sentinel — Lightweight Spreadsheet Component & Outer Grid Engine
// Strict compliance with workspace-data-grid-guard (Non-reactive outer engine)
// =============================================================================

// File-scoped non-reactive Grid Engine
const sheetEngine = {
  activeSheetId: null,
  title: '',
  rowCount: 30,
  colCount: 10,
  cells: {},          // { "A1": { raw: "100", v: 100, style: {} } }
  merges: [],         // [ { start: "A1", end: "B2" } ]
  colWidths: {},
  selection: { start: 'A1', end: 'A1' },
  history: [],
  containerEl: null,
  dirty: false,

  // Coordinate math helpers
  lettersToCol(letters) {
    letters = letters.toUpperCase().trim();
    let col = 0;
    for (let i = 0; i < letters.length; i++) {
      col = col * 26 + (letters.charCodeAt(i) - 64);
    }
    return col - 1;
  },

  colToLetters(col) {
    let result = '';
    col++;
    while (col > 0) {
      const rem = (col - 1) % 26;
      result = String.fromCharCode(65 + rem) + result;
      col = Math.floor((col - 1) / 26);
    }
    return result;
  },

  parseCoord(coord) {
    coord = coord.toUpperCase().trim();
    const match = coord.match(/^([A-Z]+)(\d+)$/);
    if (!match) return null;
    return {
      col: this.lettersToCol(match[1]),
      row: parseInt(match[2], 10) - 1,
      colLetters: match[1],
      rowNum: parseInt(match[2], 10)
    };
  },

  coordToString(col, row) {
    return this.colToLetters(col) + (row + 1);
  },

  expandRange(rangeStr) {
    rangeStr = rangeStr.toUpperCase().trim();
    const parts = rangeStr.split(':');
    if (parts.length === 1) return [parts[0]];
    const start = this.parseCoord(parts[0]);
    const end = this.parseCoord(parts[1]);
    if (!start || !end) return [];

    const minCol = Math.min(start.col, end.col);
    const maxCol = Math.max(start.col, end.col);
    const minRow = Math.min(start.row, end.row);
    const maxRow = Math.max(start.row, end.row);

    const coords = [];
    for (let c = minCol; c <= maxCol; c++) {
      for (let r = minRow; r <= maxRow; r++) {
        coords.push(this.coordToString(c, r));
      }
    }
    return coords;
  },

  // Formula Evaluator with Cycle Detection
  evaluateFormula(expr, currentCoord, visited = new Set(), depth = 0) {
    if (depth > 32) return '#REF! CIRCULAR';
    if (currentCoord) {
      if (visited.has(currentCoord)) return '#REF! CIRCULAR';
      visited.add(currentCoord);
    }

    expr = expr.trim();
    if (expr.startsWith('=')) expr = expr.substring(1).trim();

    // Check Functions: SUM, AVERAGE/AVG, COUNT, MIN, MAX
    const fnMatch = expr.match(/^([A-Z]+)\((.*)\)$/i);
    if (fnMatch) {
      const fnName = fnMatch[1].toUpperCase();
      const argsStr = fnMatch[2].trim();
      const coords = this.expandRange(argsStr);
      const values = [];

      for (const c of coords) {
        const val = this.getNumericCellValue(c, new Set(visited), depth + 1);
        if (typeof val === 'string' && val.startsWith('#')) return val;
        values.push(val);
      }

      switch (fnName) {
        case 'SUM':
          return values.reduce((a, b) => a + b, 0);
        case 'AVERAGE':
        case 'AVG':
          return values.length > 0 ? (values.reduce((a, b) => a + b, 0) / values.length) : '#DIV/0!';
        case 'COUNT':
          return values.length;
        case 'MIN':
          return values.length > 0 ? Math.min(...values) : 0;
        case 'MAX':
          return values.length > 0 ? Math.max(...values) : 0;
        default:
          return '#NAME?';
      }
    }

    // Arithmetic evaluation: replace cell tokens with values
    try {
      const tokenized = expr.replace(/[A-Z]+\d+/gi, (cellCoord) => {
        const num = this.getNumericCellValue(cellCoord.toUpperCase(), new Set(visited), depth + 1);
        if (typeof num === 'string' && num.startsWith('#')) throw new Error(num);
        return String(num);
      });

      // Safe arithmetic evaluator
      if (!/^[\d\s+\-*/.()]+$/.test(tokenized)) {
        return '#VALUE!';
      }
      // eslint-disable-next-line no-new-func
      const result = Function(`"use strict"; return (${tokenized})`)();
      if (!isFinite(result)) return '#DIV/0!';
      return result;
    } catch (err) {
      if (err.message && err.message.startsWith('#')) return err.message;
      return '#VALUE!';
    }
  },

  getNumericCellValue(coord, visited, depth) {
    const cell = this.cells[coord];
    if (!cell) return 0;
    const raw = String(cell.raw || '').trim();
    if (raw.startsWith('=')) {
      const evalVal = this.evaluateFormula(raw, coord, visited, depth);
      if (typeof evalVal === 'string' && evalVal.startsWith('#')) return evalVal;
      return Number(evalVal) || 0;
    }
    if (cell.v !== undefined && cell.v !== null && cell.v !== '') {
      return Number(cell.v) || 0;
    }
    return Number(raw) || 0;
  },

  recalculateAll() {
    for (const coord in this.cells) {
      const cell = this.cells[coord];
      const raw = String(cell.raw || '').trim();
      if (raw.startsWith('=')) {
        cell.v = this.evaluateFormula(raw, coord);
      } else if (raw !== '') {
        const num = Number(raw);
        cell.v = !isNaN(num) ? num : raw;
      } else {
        cell.v = '';
      }
    }
  },

  // Cell Data Accessors
  getRawValue(coord) {
    return this.cells[coord]?.raw ?? '';
  },

  getDisplayValue(coord) {
    const cell = this.cells[coord];
    if (!cell) return '';
    if (cell.v !== undefined && cell.v !== null) {
      if (typeof cell.v === 'number') {
        if (cell.fmt === 'currency') {
          return '฿' + cell.v.toLocaleString('th-TH', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
        }
        if (cell.fmt === 'percent') {
          return (cell.v * 100).toFixed(1) + '%';
        }
        if (cell.fmt === 'decimal') {
          return cell.v.toFixed(2);
        }
        return String(cell.v);
      }
      return String(cell.v);
    }
    return cell.raw ?? '';
  },

  setCellValue(coord, raw) {
    coord = coord.toUpperCase();
    if (!this.cells[coord]) {
      this.cells[coord] = { raw: '', v: '' };
    }
    this.cells[coord].raw = raw;
    this.dirty = true;
    this.recalculateAll();
  },

  setCellStyle(coord, styleKey, styleVal) {
    coord = coord.toUpperCase();
    if (!this.cells[coord]) {
      this.cells[coord] = { raw: '', v: '', style: {} };
    }
    if (!this.cells[coord].style) {
      this.cells[coord].style = {};
    }
    this.cells[coord].style[styleKey] = styleVal;
    this.dirty = true;
  },

  // Merge Cells
  mergeSelection() {
    const start = this.selection.start;
    const end = this.selection.end;
    if (start === end) return;
    this.merges.push({ start, end });
    this.dirty = true;
  },

  unmergeSelection() {
    const coords = this.expandRange(`${this.selection.start}:${this.selection.end}`);
    this.merges = this.merges.filter(m => {
      const mergeCoords = this.expandRange(`${m.start}:${m.end}`);
      return !mergeCoords.some(c => coords.includes(c));
    });
    this.dirty = true;
  },

  getMergeInfo(coord) {
    for (const m of this.merges) {
      const start = this.parseCoord(m.start);
      const end = this.parseCoord(m.end);
      const current = this.parseCoord(coord);
      if (!start || !end || !current) continue;

      const minCol = Math.min(start.col, end.col);
      const maxCol = Math.max(start.col, end.col);
      const minRow = Math.min(start.row, end.row);
      const maxRow = Math.max(start.row, end.row);

      if (current.col >= minCol && current.col <= maxCol && current.row >= minRow && current.row <= maxRow) {
        if (current.col === minCol && current.row === minRow) {
          return {
            isRoot: true,
            colSpan: maxCol - minCol + 1,
            rowSpan: maxRow - minRow + 1
          };
        }
        return { isRoot: false, hidden: true };
      }
    }
    return null;
  },

  calculateSelectionSummary() {
    const range = `${this.selection.start}:${this.selection.end}`;
    const coords = this.expandRange(range);
    let sum = 0;
    let count = 0;

    for (const c of coords) {
      const cell = this.cells[c];
      if (cell && cell.v !== undefined && cell.v !== null && cell.v !== '') {
        const val = Number(cell.v);
        if (!isNaN(val)) {
          sum += val;
          count++;
        }
      }
    }

    if (count === 0) return '';
    const avg = sum / count;
    const fmt = (n) => n.toLocaleString('th-TH', { maximumFractionDigits: 2 });
    return `SUM: ${fmt(sum)} | AVG: ${fmt(avg)} | COUNT: ${count}`;
  },

  // Native DOM Table Renderer (Fast & Bypasses Alpine Proxy)
  renderTable(container, onSelectCallback) {
    this.containerEl = container;
    container.innerHTML = '';

    const table = document.createElement('table');
    table.className = 'sheet-table';

    // 1. Column Header Row (Top Freeze Pane)
    const thead = document.createElement('thead');
    const headerRow = document.createElement('tr');

    const corner = document.createElement('th');
    corner.className = 'corner-header';
    corner.innerText = '';
    headerRow.appendChild(corner);

    for (let c = 0; c < this.colCount; c++) {
      const th = document.createElement('th');
      th.className = 'col-header';
      th.innerText = this.colToLetters(c);
      if (this.colWidths[String(c)]) {
        th.style.width = this.colWidths[String(c)] + 'px';
      }
      headerRow.appendChild(th);
    }
    thead.appendChild(headerRow);
    table.appendChild(thead);

    // 2. Table Body (Rows 1 to rowCount)
    const tbody = document.createElement('tbody');

    for (let r = 0; r < this.rowCount; r++) {
      const tr = document.createElement('tr');

      // Sticky Row Label (Left Freeze Pane)
      const rowHeader = document.createElement('td');
      rowHeader.className = 'row-header';
      rowHeader.innerText = String(r + 1);
      tr.appendChild(rowHeader);

      for (let c = 0; c < this.colCount; c++) {
        const coord = this.coordToString(c, r);
        const mergeInfo = this.getMergeInfo(coord);

        if (mergeInfo && mergeInfo.hidden) {
          continue; // Covered by a merged cell
        }

        const td = document.createElement('td');
        td.className = 'sheet-cell';
        td.dataset.coord = coord;
        td.innerText = this.getDisplayValue(coord);

        if (mergeInfo && mergeInfo.isRoot) {
          td.colSpan = mergeInfo.colSpan;
          td.rowSpan = mergeInfo.rowSpan;
        }

        // Apply Styles
        const style = this.cells[coord]?.style;
        if (style) {
          if (style.b) td.style.fontWeight = 'bold';
          if (style.i) td.style.fontStyle = 'italic';
          if (style.align) td.style.textAlign = style.align;
          if (style.bg) td.style.backgroundColor = style.bg;
          if (style.color) td.style.color = style.color;
        }

        // Active selection styling
        if (coord === this.selection.start) {
          td.classList.add('selected');
        }

        td.addEventListener('mousedown', (e) => {
          this.selection.start = coord;
          this.selection.end = coord;
          this.updateCellHighlights();
          if (onSelectCallback) onSelectCallback(coord);
        });

        td.addEventListener('dblclick', () => {
          this.startInlineEdit(td, coord);
        });

        tr.appendChild(td);
      }
      tbody.appendChild(tr);
    }

    table.appendChild(tbody);
    container.appendChild(table);
  },

  updateCellHighlights() {
    if (!this.containerEl) return;
    const selectedCoords = new Set(this.expandRange(`${this.selection.start}:${this.selection.end}`));
    const allCells = this.containerEl.querySelectorAll('.sheet-cell');

    allCells.forEach(cell => {
      const c = cell.dataset.coord;
      cell.classList.remove('selected', 'range-selected');
      if (c === this.selection.start) {
        cell.classList.add('selected');
      } else if (selectedCoords.has(c)) {
        cell.classList.add('range-selected');
      }
    });
  },

  startInlineEdit(td, coord) {
    if (td.querySelector('.sheet-cell-editor')) return;
    const rawVal = this.getRawValue(coord);
    const input = document.createElement('input');
    input.type = 'text';
    input.className = 'sheet-cell-editor';
    input.value = rawVal;

    td.innerText = '';
    td.appendChild(input);
    input.focus();
    input.select();

    const finish = () => {
      const newVal = input.value;
      this.setCellValue(coord, newVal);
      td.innerText = this.getDisplayValue(coord);
      if (this.onCommitCallback) this.onCommitCallback(coord, newVal);
    };

    input.addEventListener('blur', finish);
    input.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') {
        input.blur();
        this.navigate(0, 1);
      } else if (e.key === 'Tab') {
        e.preventDefault();
        input.blur();
        this.navigate(1, 0);
      } else if (e.key === 'Escape') {
        td.innerText = this.getDisplayValue(coord);
      }
    });
  },

  navigate(dCol, dRow) {
    const current = this.parseCoord(this.selection.start);
    if (!current) return;
    const nextCol = Math.max(0, Math.min(this.colCount - 1, current.col + dCol));
    const nextRow = Math.max(0, Math.min(this.rowCount - 1, current.row + dRow));
    const nextCoord = this.coordToString(nextCol, nextRow);
    this.selection.start = nextCoord;
    this.selection.end = nextCoord;
    this.updateCellHighlights();
    if (this.onSelectCallback) this.onSelectCallback(nextCoord);

    // Scroll into view if needed
    const targetEl = this.containerEl?.querySelector(`[data-coord="${nextCoord}"]`);
    if (targetEl) {
      targetEl.scrollIntoView({ block: 'nearest', inline: 'nearest' });
    }
  },

  // Export RFC 4180 CSV
  exportCSV() {
    const rows = [];
    for (let r = 0; r < this.rowCount; r++) {
      const row = [];
      for (let c = 0; c < this.colCount; c++) {
        const coord = this.coordToString(c, r);
        let val = this.getDisplayValue(coord);
        if (typeof val === 'string' && (val.includes(',') || val.includes('"') || val.includes('\n'))) {
          val = `"${val.replace(/"/g, '""')}"`;
        }
        row.push(val);
      }
      rows.push(row.join(','));
    }
    const csvContent = rows.join('\r\n');
    const blob = new Blob([csvContent], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `${this.title || 'sheet'}.csv`;
    link.click();
  },

  // Import CSV
  importCSVText(text) {
    const lines = text.split(/\r?\n/).filter(l => l.trim() !== '');
    if (lines.length === 0) return;

    this.cells = {};
    this.rowCount = Math.max(30, lines.length);

    let maxCols = 10;
    for (let r = 0; r < lines.length; r++) {
      // Basic CSV splitter respecting quotes
      const cells = lines[r].split(/,(?=(?:(?:[^"]*"){2})*[^"]*$)/);
      maxCols = Math.max(maxCols, cells.length);
      for (let c = 0; c < cells.length; c++) {
        let val = cells[c].trim();
        if (val.startsWith('"') && val.endsWith('"')) {
          val = val.substring(1, val.length - 1).replace(/""/g, '"');
        }
        if (val !== '') {
          const coord = this.coordToString(c, r);
          this.cells[coord] = { raw: val, v: isNaN(Number(val)) ? val : Number(val) };
        }
      }
    }
    this.colCount = Math.min(26, Math.max(10, maxCols));
    this.dirty = true;
    this.recalculateAll();
  }
};

// Alpine Component Definition (Contains ONLY Primitives)
function sheetsComponent() {
  return {
    sheets: [],
    currentSheetId: null,
    currentSheetTitle: '',
    activeCoord: 'A1',
    formulaBarInput: '',
    summaryBarText: '',
    isSaving: false,
    saveTimeout: null,

    // AI Modal State
    aiModalOpen: false,
    aiAction: 'formula_suggest',
    aiPrompt: '',
    aiResult: '',
    aiExplanation: '',
    aiLoading: false,

    async init() {
      sheetEngine.onSelectCallback = (coord) => {
        this.activeCoord = coord;
        this.formulaBarInput = sheetEngine.getRawValue(coord);
        this.summaryBarText = sheetEngine.calculateSelectionSummary();
      };

      sheetEngine.onCommitCallback = (coord, val) => {
        this.formulaBarInput = val;
        this.summaryBarText = sheetEngine.calculateSelectionSummary();
        this.triggerAutoSave();
      };

      // Keyboard navigation listener
      window.addEventListener('keydown', (e) => {
        if (!document.getElementById('sheets-content') || document.activeElement?.tagName === 'INPUT') return;
        switch (e.key) {
          case 'ArrowUp':
            e.preventDefault();
            sheetEngine.navigate(0, -1);
            break;
          case 'ArrowDown':
            e.preventDefault();
            sheetEngine.navigate(0, 1);
            break;
          case 'ArrowLeft':
            e.preventDefault();
            sheetEngine.navigate(-1, 0);
            break;
          case 'ArrowRight':
            e.preventDefault();
            sheetEngine.navigate(1, 0);
            break;
          case 'F2':
          case 'Enter': {
            e.preventDefault();
            const td = document.querySelector(`.sheet-cell[data-coord="${this.activeCoord}"]`);
            if (td) sheetEngine.startInlineEdit(td, this.activeCoord);
            break;
          }
        }
      });

      await this.loadSheetList();
    },

    async loadSheetList() {
      try {
        const res = await fetch('/api/sheets', { headers: { 'Accept': 'application/json' } });
        if (res.ok) {
          this.sheets = await res.json();
          if (this.sheets.length > 0 && !this.currentSheetId) {
            await this.openSheet(this.sheets[0].id);
          }
        }
      } catch (err) {
        console.warn('Failed to load sheets:', err);
      }
    },

    async openSheet(id) {
      try {
        const res = await fetch('/api/sheets/' + id, { headers: { 'Accept': 'application/json' } });
        if (res.ok) {
          const sheet = await res.json();
          this.currentSheetId = sheet.id;
          this.currentSheetTitle = sheet.title;

          sheetEngine.activeSheetId = sheet.id;
          sheetEngine.title = sheet.title;
          sheetEngine.rowCount = sheet.row_count || 30;
          sheetEngine.colCount = sheet.col_count || 10;

          try {
            const parsed = JSON.parse(sheet.data_json || '{}');
            sheetEngine.cells = parsed.cells || {};
            sheetEngine.merges = parsed.merges || [];
            sheetEngine.colWidths = parsed.colWidths || {};
          } catch (e) {
            sheetEngine.cells = {};
            sheetEngine.merges = [];
          }

          sheetEngine.recalculateAll();
          this.renderGrid();
          this.activeCoord = 'A1';
          this.formulaBarInput = sheetEngine.getRawValue('A1');
          this.summaryBarText = sheetEngine.calculateSelectionSummary();
        }
      } catch (err) {
        console.error('Failed to open sheet:', err);
      }
    },

    renderGrid() {
      const container = document.getElementById('sheet-grid-container');
      if (container) {
        sheetEngine.renderTable(container, (coord) => {
          this.activeCoord = coord;
          this.formulaBarInput = sheetEngine.getRawValue(coord);
          this.summaryBarText = sheetEngine.calculateSelectionSummary();
        });
      }
    },

    onFormulaInput(val) {
      this.formulaBarInput = val;
      sheetEngine.setCellValue(this.activeCoord, val);
      const td = document.querySelector(`.sheet-cell[data-coord="${this.activeCoord}"]`);
      if (td) {
        td.innerText = sheetEngine.getDisplayValue(this.activeCoord);
      }
      this.summaryBarText = sheetEngine.calculateSelectionSummary();
      this.triggerAutoSave();
    },

    formatActiveCell(styleKey, styleVal) {
      sheetEngine.setCellStyle(this.activeCoord, styleKey, styleVal);
      this.renderGrid();
      this.triggerAutoSave();
    },

    mergeCells() {
      sheetEngine.mergeSelection();
      this.renderGrid();
      this.triggerAutoSave();
    },

    unmergeCells() {
      sheetEngine.unmergeSelection();
      this.renderGrid();
      this.triggerAutoSave();
    },

    triggerAutoSave() {
      this.isSaving = true;
      if (this.saveTimeout) clearTimeout(this.saveTimeout);
      this.saveTimeout = setTimeout(() => {
        this.saveSheet();
      }, 600);
    },

    async saveSheet() {
      if (!this.currentSheetId) return;
      this.isSaving = true;
      try {
        const dataJSON = JSON.stringify({
          cells: sheetEngine.cells,
          merges: sheetEngine.merges,
          colWidths: sheetEngine.colWidths
        });

        await fetch('/api/sheets/' + this.currentSheetId, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            title: this.currentSheetTitle,
            row_count: sheetEngine.rowCount,
            col_count: sheetEngine.colCount,
            data_json: dataJSON
          })
        });
      } catch (err) {
        console.warn('Auto-save sheet failed:', err);
      } finally {
        this.isSaving = false;
      }
    },

    async createNewSheet() {
      try {
        const res = await fetch('/api/sheets', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            title: 'ชีตใหม่',
            row_count: 30,
            col_count: 10,
            data_json: '{}'
          })
        });
        if (res.ok) {
          const created = await res.json();
          this.sheets.unshift({
            id: created.id,
            title: created.title,
            row_count: created.row_count,
            col_count: created.col_count
          });
          await this.openSheet(created.id);
        }
      } catch (err) {
        console.error('Failed to create sheet:', err);
      }
    },

    exportCSV() {
      sheetEngine.exportCSV();
    },

    importCSV(event) {
      const file = event.target.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = (e) => {
        sheetEngine.importCSVText(e.target.result);
        this.renderGrid();
        this.triggerAutoSave();
      };
      reader.readAsText(file);
    },

    // AI Assist
    openAIModal() {
      this.aiResult = '';
      this.aiExplanation = '';
      this.aiPrompt = '';
      this.aiLoading = false;
      this.aiModalOpen = true;
    },

    async runAIAction() {
      if (!this.currentSheetId) return;
      this.aiLoading = true;
      this.aiResult = '';
      this.aiExplanation = '';
      try {
        const res = await fetch(`/api/sheets/${this.currentSheetId}/ai`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            action: this.aiAction,
            prompt: this.aiPrompt
          })
        });
        if (res.ok) {
          const data = await res.json();
          this.aiResult = data.result;
          this.aiExplanation = data.explanation || '';
        } else {
          const err = await res.json();
          this.aiResult = '⚠️ ' + (err.error || 'AI เกิดข้อผิดพลาด');
        }
      } catch (err) {
        this.aiResult = '⚠️ ไม่สามารถเชื่อมต่อระบบ AI ได้: ' + err.message;
      } finally {
        this.aiLoading = false;
      }
    },

    applyAIFormula() {
      if (!this.aiResult || !this.aiResult.startsWith('=')) return;
      this.onFormulaInput(this.aiResult);
      this.aiModalOpen = false;
    }
  };
}
