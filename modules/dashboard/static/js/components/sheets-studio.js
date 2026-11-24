// Pocket Sheets Studio - Google Sheets Desktop Engine
(function () {
  let xs = null;
  let currentSheet = null;
  let saveTimer = null;
  let sheetList = [];
  let isMenuOpen = false;

  const titleInput = document.getElementById('sheet-title');
  const saveText = document.getElementById('save-text');
  const saveBadge = document.getElementById('save-badge');
  const container = document.getElementById('xspreadsheet-container');

  // Initialize
  async function init() {
    initSpreadsheet();
    await loadSheetList();
    setupMenuBar();
    setupActions();
    setupModals();
  }

  function initSpreadsheet() {
    if (!window.x_spreadsheet) {
      console.error('x_spreadsheet library not loaded');
      return;
    }

    xs = window.x_spreadsheet(container, {
      showToolbar: true,
      showGrid: true,
      showContextmenu: true,
      showBottomBar: true,
      view: {
        height: () => window.innerHeight - 80,
        width: () => window.innerWidth
      },
      row: {
        len: 80,
        height: 26
      },
      col: {
        len: 26,
        width: 100,
        indexWidth: 44,
        minWidth: 44
      }
    });

    xs.change(() => {
      triggerAutoSave();
    });

    setupAutoWidthListener();

    window.addEventListener('resize', () => {
      if (xs && xs.reRender) {
        xs.reRender();
      }
    });
  }

  // Menu Bar Controller (Google Sheets style desktop dropdowns)
  function setupMenuBar() {
    const menuItems = document.querySelectorAll('.gs-menu-item');

    menuItems.forEach((item) => {
      const trigger = item.querySelector('.gs-menu-trigger');

      trigger.addEventListener('click', (e) => {
        e.stopPropagation();
        const wasActive = item.classList.contains('active');
        closeAllMenus();
        if (!wasActive) {
          item.classList.add('active');
          isMenuOpen = true;
        }
      });

      item.addEventListener('mouseenter', () => {
        if (isMenuOpen && !item.classList.contains('active')) {
          closeAllMenus();
          item.classList.add('active');
        }
      });
    });

    document.addEventListener('click', () => {
      closeAllMenus();
    });

    // Dropdown Action clicks
    document.querySelectorAll('.gs-dropdown-item').forEach((btn) => {
      btn.addEventListener('click', (e) => {
        e.stopPropagation();
        const action = btn.getAttribute('data-action');
        closeAllMenus();
        handleAction(action);
      });
    });
  }

  function closeAllMenus() {
    document.querySelectorAll('.gs-menu-item').forEach((i) => i.classList.remove('active'));
    isMenuOpen = false;
  }

  // Action Dispatcher
  function handleAction(action) {
    if (!action) return;

    switch (action) {
      case 'new':
        createNewPrompt();
        break;
      case 'open':
        openSheetSwitcher();
        break;
      case 'save':
        saveCurrentSheet(true);
        break;
      case 'import-csv':
        document.getElementById('csv-file-input')?.click();
        break;
      case 'export-csv':
        exportCSV();
        break;
      case 'delete':
        deleteCurrentSheet();
        break;

      // Edit
      case 'undo':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.undo) {
          xs.sheet.data.undo();
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'redo':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.redo) {
          xs.sheet.data.redo();
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'clear':
        if (xs && xs.sheet && xs.sheet.data) {
          xs.cellText(xs.sheet.data.selector.ri, xs.sheet.data.selector.ci, '');
          xs.reRender();
          triggerAutoSave();
        }
        break;

      // View
      case 'zoom-100':
        setZoom(1);
        break;
      case 'zoom-125':
        setZoom(1.25);
        break;
      case 'zoom-75':
        setZoom(0.75);
        break;
      case 'toggle-theme':
        toggleTheme();
        break;
      case 'fullscreen':
        toggleFullscreen();
        break;

      // Insert
      case 'insert-row':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.insertRow) {
          xs.sheet.data.insertRow(xs.sheet.data.selector.ri);
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'insert-col':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.insertColumn) {
          xs.sheet.data.insertColumn(xs.sheet.data.selector.ci);
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'delete-row':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.deleteRow) {
          xs.sheet.data.deleteRow(xs.sheet.data.selector.ri);
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'delete-col':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.deleteColumn) {
          xs.sheet.data.deleteColumn(xs.sheet.data.selector.ci);
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'formula-sum':
        insertFormula('=SUM(');
        break;
      case 'formula-avg':
        insertFormula('=AVERAGE(');
        break;
      case 'formula-count':
        insertFormula('=COUNT(');
        break;

      // Format
      case 'format-bold':
        toggleStyle('bold');
        break;
      case 'format-italic':
        toggleStyle('italic');
        break;
      case 'format-strike':
        toggleStyle('strike');
        break;
      case 'format-currency':
        setCellFormat('currency');
        break;
      case 'format-percent':
        setCellFormat('percent');
        break;
      case 'auto-width':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.selector) {
          autoFitColumn(xs.sheet.data.selector.ci);
        }
        break;
      case 'format-merge':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.merge) {
          xs.sheet.data.merge();
          xs.reRender();
          triggerAutoSave();
        }
        break;

      // Data
      case 'data-filter':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.autofilter) {
          xs.sheet.data.autofilter();
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'data-sort-asc':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.sort) {
          xs.sheet.data.sort(xs.sheet.data.selector.ci, 'asc');
          xs.reRender();
          triggerAutoSave();
        }
        break;
      case 'data-sort-desc':
        if (xs && xs.sheet && xs.sheet.data && xs.sheet.data.sort) {
          xs.sheet.data.sort(xs.sheet.data.selector.ci, 'desc');
          xs.reRender();
          triggerAutoSave();
        }
        break;

      // AI
      case 'ai-formula':
        openAIModal('ขอสูตรคำนวณ Excel/Sheets สำหรับ: ');
        break;
      case 'ai-analyze':
        openAIModal('ช่วยวิเคราะห์ข้อมูลและสรุปภาพรวมของตารางนี้');
        break;
      case 'ai-template':
        openAIModal('ช่วยแนะนำโครงสร้างตารางและหัวข้อคอลัมน์สำหรับ: ');
        break;

      // Help
      case 'help-shortcuts':
        document.getElementById('shortcuts-modal').style.display = 'flex';
        break;
      case 'help-about':
        alert('Pocket Sheets v1.0\nรันโดยตรงบน vivo v11 (Termux ARM64)\nความสามารถ: สเปรดชีตสูตรคำนวณแบบ Offline 100% พร้อมระบบบันทึกในตัว');
        break;
    }
  }

  function setZoom(scale) {
    if (container) {
      container.style.transform = scale === 1 ? 'none' : `scale(${scale})`;
      container.style.transformOrigin = '0 0';
    }
  }

  function toggleTheme() {
    const current = document.documentElement.dataset.theme;
    const next = current === 'light' ? 'dark' : 'light';
    document.documentElement.dataset.theme = next;
    try {
      localStorage.setItem('sentinel-theme', next);
    } catch (_) {}
  }

  function toggleFullscreen() {
    if (!document.fullscreenElement) {
      document.documentElement.requestFullscreen().catch(() => {});
    } else {
      document.exitFullscreen().catch(() => {});
    }
  }

  // Auto Width Calculation & Column Fit
  function autoFitColumn(ci) {
    if (!xs || !xs.sheet || ci === undefined || ci === null || ci < 0) return;
    const sheet = xs.sheet;
    const data = sheet.data;
    if (!data) return;

    if (!autoFitColumn.ctx) {
      const cvs = document.createElement('canvas');
      autoFitColumn.ctx = cvs.getContext('2d');
    }
    const ctx = autoFitColumn.ctx;

    // Convert column index to letter (A, B, C ... Z, AA, AB ...)
    let colName = '';
    let temp = ci;
    while (temp >= 0) {
      colName = String.fromCharCode((temp % 26) + 65) + colName;
      temp = Math.floor(temp / 26) - 1;
    }

    // Baseline width: header letter + padding
    ctx.font = 'bold 12px "Source Sans Pro", Arial, sans-serif';
    let maxW = ctx.measureText(colName).width + 24;

    const rowsObj = (data.rows && data.rows._) ? data.rows._ : {};
    Object.keys(rowsObj).forEach((ri) => {
      const row = rowsObj[ri];
      if (row && row.cells && row.cells[ci]) {
        const cell = row.cells[ci];
        const text = (cell.text !== undefined && cell.text !== null)
          ? String(cell.text)
          : (cell.v !== undefined && cell.v !== null ? String(cell.v) : '');
        if (text) {
          const lines = text.split('\n');
          lines.forEach((line) => {
            const fontSize = (cell.style && cell.style.fontSize) ? cell.style.fontSize + 'pt' : '13px';
            const isBold = (cell.style && cell.style.bold);
            ctx.font = `${isBold ? 'bold ' : 'normal '} ${fontSize} "Source Sans Pro", Arial, sans-serif`;
            const w = ctx.measureText(line).width;
            if (w > maxW) maxW = w;
          });
        }
      }
    });

    // Add cell padding (28px: 14px each side for comfortable reading)
    const newWidth = Math.max(64, Math.ceil(maxW + 28));

    // Update column width in data model
    data.setColWidth(ci, newWidth);
    if (sheet.table && sheet.table.render) {
      sheet.table.render();
    }
    if (sheet.reload) {
      sheet.reload();
    }
    if (xs.reRender) {
      xs.reRender();
    }
    triggerAutoSave();

    showToast(`📐 ปรับความกว้างคอลัมน์ ${colName} พอดีข้อความ (${newWidth}px)`);
    return newWidth;
  }

  function setupAutoWidthListener() {
    let downX = 0;
    let downY = 0;
    let downTime = 0;
    let targetColIndex = -1;

    // 1. Capture mousedown on the vertical resizer handle
    window.addEventListener('mousedown', (e) => {
      const resizerHover = e.target.closest && e.target.closest('.x-spreadsheet-resizer.vertical .x-spreadsheet-resizer-hover, .x-spreadsheet-resizer.vertical');
      if (resizerHover) {
        downX = e.clientX;
        downY = e.clientY;
        downTime = Date.now();
        if (xs && xs.sheet && xs.sheet.colResizer && xs.sheet.colResizer.cRect) {
          targetColIndex = xs.sheet.colResizer.cRect.ci;
        }
      }
    }, true);

    // 2. Capture mouseup: if click was performed without significant drag (< 4px and < 400ms)
    window.addEventListener('mouseup', (e) => {
      if (downTime > 0) {
        const dist = Math.hypot(e.clientX - downX, e.clientY - downY);
        const duration = Date.now() - downTime;
        downTime = 0;

        if (dist <= 4 && duration <= 400) {
          let ci = targetColIndex;
          if ((ci === undefined || ci < 0) && xs && xs.sheet && xs.sheet.colResizer && xs.sheet.colResizer.cRect) {
            ci = xs.sheet.colResizer.cRect.ci;
          }
          if (ci !== undefined && ci >= 0) {
            autoFitColumn(ci);
          }
        }
      }
    }, true);

    // 3. Double-click on vertical resizer handle (Standard Excel / Google Sheets shortcut)
    window.addEventListener('dblclick', (e) => {
      const resizerHover = e.target.closest && e.target.closest('.x-spreadsheet-resizer.vertical .x-spreadsheet-resizer-hover, .x-spreadsheet-resizer.vertical');
      if (resizerHover) {
        let ci = -1;
        if (xs && xs.sheet && xs.sheet.colResizer && xs.sheet.colResizer.cRect) {
          ci = xs.sheet.colResizer.cRect.ci;
        }
        if (ci >= 0) {
          e.preventDefault();
          e.stopPropagation();
          autoFitColumn(ci);
        }
      }
    }, true);

    // 4. Also support clicking on the top Col Cell header near its right border
    if (container) {
      container.addEventListener('click', (e) => {
        if (!xs || !xs.sheet || !xs.sheet.data) return;
        const data = xs.sheet.data;
        const rect = container.getBoundingClientRect();
        const offsetX = e.clientX - rect.left;
        const offsetY = e.clientY - rect.top;

        const headerHeight = (data.rows && data.rows.height) ? data.rows.height : 26;
        const indexWidth = (data.cols && data.cols.indexWidth) ? data.cols.indexWidth : 44;

        // Check if click was in the column header row
        if (offsetY >= 0 && offsetY <= headerHeight + 35 && offsetX > indexWidth) {
          // If resizer active rect is present
          if (xs.sheet.colResizer && xs.sheet.colResizer.cRect) {
            const cRect = xs.sheet.colResizer.cRect;
            const rightEdge = cRect.left + cRect.width;
            if (Math.abs(offsetX - rightEdge) <= 12) {
              autoFitColumn(cRect.ci);
              return;
            }
          }

          if (data.getCellRectByXY) {
            const u = data.getCellRectByXY(offsetX, offsetY);
            if (u && u.ri === -1 && u.ci >= 0) {
              const rightEdge = u.left + u.width;
              if (Math.abs(offsetX - rightEdge) <= 12) {
                autoFitColumn(u.ci);
              }
            }
          }
        }
      }, true);
    }
  }

  let toastTimer = null;
  function showToast(msg) {
    const el = document.getElementById('gs-toast');
    if (!el) return;
    el.textContent = msg;
    el.style.display = 'block';
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
      el.style.display = 'none';
    }, 2400);
  }

  function insertFormula(formulaText) {
    if (!xs || !xs.sheet || !xs.sheet.data) return;
    const { ri, ci } = xs.sheet.data.selector;
    xs.cellText(ri, ci, formulaText);
    xs.reRender();
    triggerAutoSave();
  }

  function toggleStyle(prop) {
    if (!xs || !xs.sheet || !xs.sheet.data) return;
    const { ri, ci } = xs.sheet.data.selector;
    const style = xs.cellStyle(ri, ci) || {};
    if (prop === 'bold') style.bold = !style.bold;
    if (prop === 'italic') style.italic = !style.italic;
    if (prop === 'strike') style.strike = !style.strike;
    if (xs.sheet.data.setStyle) {
      xs.sheet.data.setStyle(style);
    }
    xs.reRender();
    triggerAutoSave();
  }

  function setCellFormat(fmt) {
    if (!xs || !xs.sheet || !xs.sheet.data) return;
    const { ri, ci } = xs.sheet.data.selector;
    const style = xs.cellStyle(ri, ci) || {};
    style.format = fmt;
    if (xs.sheet.data.setStyle) {
      xs.sheet.data.setStyle(style);
    }
    xs.reRender();
    triggerAutoSave();
  }

  // Sheets CRUD & Remote
  async function loadSheetList() {
    try {
      const res = await fetch('/api/sheets');
      if (!res.ok) throw new Error('Fetch failed');
      sheetList = await res.json();

      const urlParams = new URLSearchParams(window.location.search);
      const targetId = urlParams.get('id');

      let target = null;
      if (targetId) {
        target = sheetList.find((s) => s.id === targetId);
      }
      if (!target && sheetList.length > 0) {
        target = sheetList[0];
      }

      if (target) {
        await loadSheet(target.id);
      } else {
        await createNewSheet('สเปรดชีตแผ่นที่ 1');
      }
    } catch (err) {
      console.error('Error loading sheets:', err);
      titleInput.value = 'สเปรดชีตออฟไลน์';
    }
  }

  async function loadSheet(id) {
    try {
      setSaveStatus('กำลังโหลด...', 'saving');
      const res = await fetch(`/api/sheets/${id}`);
      if (!res.ok) throw new Error('Load failed');
      currentSheet = await res.json();

      titleInput.value = currentSheet.title || 'สเปรดชีตไม่มีชื่อ';
      history.replaceState(null, '', `/sheets?id=${currentSheet.id}`);

      if (currentSheet.data_json && currentSheet.data_json !== '[]') {
        const parsed = JSON.parse(currentSheet.data_json);
        xs.loadData(parsed);
      } else {
        xs.loadData([{ name: 'Sheet1', rows: {} }]);
      }
      setSaveStatus('บันทึกใน v11 แล้ว ✓', 'saved');
    } catch (err) {
      console.error('Error loading sheet:', err);
      setSaveStatus('โหลดไม่สำเร็จ', 'saving');
    }
  }

  async function createNewSheet(title) {
    try {
      setSaveStatus('กำลังสร้าง...', 'saving');
      const defaultData = [{ name: 'Sheet1', rows: {} }];
      const res = await fetch('/api/sheets', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: title || 'สเปรดชีตใหม่',
          data_json: JSON.stringify(defaultData)
        })
      });
      if (!res.ok) throw new Error('Create failed');
      currentSheet = await res.json();
      titleInput.value = currentSheet.title;
      xs.loadData(defaultData);
      history.replaceState(null, '', `/sheets?id=${currentSheet.id}`);
      setSaveStatus('บันทึกใน v11 แล้ว ✓', 'saved');
    } catch (err) {
      console.error('Create error:', err);
      setSaveStatus('สร้างไม่สำเร็จ', 'saving');
    }
  }

  function createNewPrompt() {
    const name = prompt('ชื่อสเปรดชีตใหม่:', 'สเปรดชีต ' + (sheetList.length + 1));
    if (name) {
      createNewSheet(name);
    }
  }

  async function deleteCurrentSheet() {
    if (!currentSheet) return;
    if (confirm(`คุณต้องการลบ "${currentSheet.title}" ใช่หรือไม่?`)) {
      await fetch(`/api/sheets/${currentSheet.id}`, { method: 'DELETE' });
      await loadSheetList();
    }
  }

  function triggerAutoSave() {
    setSaveStatus('กำลังบันทึก...', 'saving');
    clearTimeout(saveTimer);
    saveTimer = setTimeout(async () => {
      await saveCurrentSheet(false);
    }, 1200);
  }

  async function saveCurrentSheet(isManual) {
    if (!currentSheet || !xs) return;
    try {
      const data = xs.getData();
      const title = titleInput.value.trim() || 'สเปรดชีตไม่มีชื่อ';

      const res = await fetch(`/api/sheets/${currentSheet.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: title,
          data_json: JSON.stringify(data)
        })
      });

      if (!res.ok) throw new Error('Save failed');
      currentSheet = await res.json();
      setSaveStatus('บันทึกใน v11 แล้ว ✓', 'saved');
      if (isManual) {
        // Quick visual pulse
        saveBadge.style.transform = 'scale(1.15)';
        setTimeout(() => (saveBadge.style.transform = 'scale(1)'), 200);
      }
    } catch (err) {
      console.error('Save error:', err);
      setSaveStatus('บันทึกผิดพลาด', 'saving');
    }
  }

  function setSaveStatus(text, statusClass) {
    if (!saveText || !saveBadge) return;
    saveText.textContent = text;
    saveBadge.classList.remove('saved', 'saving');
    if (statusClass) {
      saveBadge.classList.add(statusClass);
    }
  }

  // CSV Export & Import
  function exportCSV() {
    if (!xs) return;
    const data = xs.getData();
    if (!data || data.length === 0) return;

    const sheet = data[0];
    const rows = sheet.rows || {};
    let csvContent = '';

    const maxRow = Math.max(25, Object.keys(rows).length);
    for (let r = 0; r < maxRow; r++) {
      const rowData = rows[r];
      const cells = rowData ? rowData.cells || {} : {};
      const rowArr = [];
      const maxCol = 15;

      for (let c = 0; c < maxCol; c++) {
        const cell = cells[c];
        let val = cell ? (cell.text !== undefined ? cell.text : cell.v || '') : '';
        val = String(val).replace(/"/g, '""');
        if (val.includes(',') || val.includes('\n') || val.includes('"')) {
          val = `"${val}"`;
        }
        rowArr.push(val);
      }
      csvContent += rowArr.join(',') + '\r\n';
    }

    const blob = new Blob(['\ufeff' + csvContent], { type: 'text/csv;charset=utf-8;' });
    const link = document.createElement('a');
    link.href = URL.createObjectURL(blob);
    link.download = `${titleInput.value.trim() || 'sheet'}.csv`;
    link.click();
  }

  function importCSV(file) {
    if (!file) return;
    const reader = new FileReader();
    reader.onload = function (e) {
      const text = e.target.result;
      const lines = text.split(/\r\n|\n/);
      const rows = {};

      lines.forEach((line, rIndex) => {
        if (!line.trim()) return;
        const cells = {};
        const items = line.split(',');
        items.forEach((item, cIndex) => {
          let clean = item.trim();
          if (clean.startsWith('"') && clean.endsWith('"')) {
            clean = clean.slice(1, -1).replace(/""/g, '"');
          }
          if (clean) {
            cells[cIndex] = { text: clean };
          }
        });
        if (Object.keys(cells).length > 0) {
          rows[rIndex] = { cells };
        }
      });

      xs.loadData([{ name: 'Imported', rows }]);
      triggerAutoSave();
    };
    reader.readAsText(file);
  }

  // Actions and Modals
  function setupActions() {
    titleInput.addEventListener('input', () => {
      triggerAutoSave();
    });

    const starBtn = document.getElementById('star-btn');
    starBtn?.addEventListener('click', () => {
      starBtn.textContent = starBtn.textContent === '☆' ? '★' : '☆';
      starBtn.style.color = starBtn.textContent === '★' ? '#f59e0b' : 'inherit';
    });

    document.getElementById('btn-export-quick')?.addEventListener('click', exportCSV);
    document.getElementById('btn-ai-quick')?.addEventListener('click', () => openAIModal(''));

    const fileInput = document.getElementById('csv-file-input');
    fileInput?.addEventListener('change', (e) => {
      if (e.target.files && e.target.files[0]) {
        importCSV(e.target.files[0]);
      }
    });
  }

  function setupModals() {
    // Switcher Modal
    const switcherModal = document.getElementById('sheet-switcher-modal');
    document.getElementById('close-switcher')?.addEventListener('click', () => {
      switcherModal.style.display = 'none';
    });
    document.getElementById('btn-create-modal')?.addEventListener('click', () => {
      switcherModal.style.display = 'none';
      createNewPrompt();
    });
    document.getElementById('sheet-search')?.addEventListener('input', (e) => {
      renderSheetSwitcher(e.target.value);
    });

    // AI Modal
    const aiModal = document.getElementById('ai-modal');
    document.getElementById('close-ai')?.addEventListener('click', () => {
      aiModal.style.display = 'none';
    });
    document.getElementById('btn-run-ai')?.addEventListener('click', runAIAssist);

    document.querySelectorAll('.ai-quick-prompt').forEach((btn) => {
      btn.addEventListener('click', () => {
        const p = btn.getAttribute('data-p');
        document.getElementById('ai-prompt').value = p;
        runAIAssist();
      });
    });

    // Shortcuts Modal
    const shortcutsModal = document.getElementById('shortcuts-modal');
    document.getElementById('close-shortcuts')?.addEventListener('click', () => {
      shortcutsModal.style.display = 'none';
    });
  }

  async function openSheetSwitcher() {
    await renderSheetSwitcher('');
    document.getElementById('sheet-switcher-modal').style.display = 'flex';
    document.getElementById('sheet-search').focus();
  }

  async function renderSheetSwitcher(searchFilter) {
    const listContainer = document.getElementById('switcher-list');
    if (!listContainer) return;

    const res = await fetch('/api/sheets');
    if (res.ok) {
      sheetList = await res.json();
    }

    let filtered = sheetList;
    if (searchFilter) {
      const q = searchFilter.toLowerCase();
      filtered = sheetList.filter((s) => s.title.toLowerCase().includes(q));
    }

    if (filtered.length === 0) {
      listContainer.innerHTML = '<p style="color:var(--gs-text-muted);text-align:center;padding:16px">ไม่พบสเปรดชีตที่ค้นหา</p>';
      return;
    }

    listContainer.innerHTML = '';
    filtered.forEach((sh) => {
      const div = document.createElement('div');
      div.className = `sheet-item ${currentSheet && currentSheet.id === sh.id ? 'active' : ''}`;
      div.innerHTML = `
        <div style="display:flex;align-items:center;gap:10px">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#0f9d58" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><polyline points="14 2 14 8 20 8"/><line x1="8" y1="13" x2="16" y2="13"/><line x1="8" y1="17" x2="16" y2="17"/></svg>
          <div>
            <strong style="display:block;font-size:13.5px">${escapeHtml(sh.title)}</strong>
            <small style="color:var(--gs-text-muted)">แก้ไขล่าสุด: ${new Date(sh.updated_at).toLocaleString('th-TH')}</small>
          </div>
        </div>
        <button class="gs-icon-btn btn-del" style="color:#ef4444;font-size:16px" title="ลบแผ่นนี้">✕</button>
      `;

      div.addEventListener('click', async (e) => {
        if (e.target.closest('.btn-del')) return;
        document.getElementById('sheet-switcher-modal').style.display = 'none';
        await loadSheet(sh.id);
      });

      div.querySelector('.btn-del').addEventListener('click', async (e) => {
        e.stopPropagation();
        if (confirm(`คุณแน่ใจหรือไม่ว่าต้องการลบ "${sh.title}"?`)) {
          await fetch(`/api/sheets/${sh.id}`, { method: 'DELETE' });
          await renderSheetSwitcher('');
          if (currentSheet && currentSheet.id === sh.id) {
            await loadSheetList();
          }
        }
      });

      listContainer.appendChild(div);
    });
  }

  function openAIModal(initialPrompt) {
    const aiModal = document.getElementById('ai-modal');
    const promptInput = document.getElementById('ai-prompt');
    if (initialPrompt) {
      promptInput.value = initialPrompt;
    }
    aiModal.style.display = 'flex';
    promptInput.focus();
  }

  async function runAIAssist() {
    const promptInput = document.getElementById('ai-prompt');
    const resultBox = document.getElementById('ai-result');
    const runBtn = document.getElementById('btn-run-ai');
    const p = promptInput.value.trim();
    if (!p || !currentSheet) return;

    runBtn.disabled = true;
    runBtn.textContent = 'กำลังคิด…';
    resultBox.style.display = 'block';
    resultBox.textContent = 'กำลังประมวลผลคำขอจาก AI...';

    try {
      const res = await fetch(`/api/sheets/${currentSheet.id}/ai`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ action: 'formula', prompt: p })
      });
      const data = await res.json();
      if (!res.ok) throw new Error(data.error || 'AI error');
      resultBox.textContent = data.result;
    } catch (err) {
      resultBox.textContent = 'เกิดข้อผิดพลาด: ' + err.message;
    } finally {
      runBtn.disabled = false;
      runBtn.textContent = '🚀 ส่งคำขอ';
    }
  }

  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  document.addEventListener('DOMContentLoaded', init);
})();
