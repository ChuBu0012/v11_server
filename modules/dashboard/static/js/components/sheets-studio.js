// Pocket Sheets Studio - x-spreadsheet Integration
(function () {
  let xs = null;
  let currentSheet = null;
  let saveTimer = null;
  let sheetList = [];

  const titleInput = document.getElementById('sheet-title');
  const saveBadge = document.getElementById('save-badge');
  const container = document.getElementById('xspreadsheet-container');

  // Initialize
  async function init() {
    initSpreadsheet();
    await loadSheetList();
    setupEventListeners();
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
        height: () => window.innerHeight - 52,
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

    window.addEventListener('resize', () => {
      if (xs && xs.reRender) {
        xs.reRender();
      }
    });
  }

  async function loadSheetList() {
    try {
      const res = await fetch('/api/sheets');
      if (!res.ok) throw new Error('Fetch failed');
      sheetList = await res.json();

      // Check URL param ?id=...
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
      // Fallback default
      titleInput.value = 'สเปรดชีตออฟไลน์';
    }
  }

  async function loadSheet(id) {
    try {
      setSaveStatus('กำลังโหลด...');
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
      setSaveStatus('✓ บันทึกแล้ว');
    } catch (err) {
      console.error('Error loading sheet:', err);
      setSaveStatus('โหลดไม่สำเร็จ');
    }
  }

  async function createNewSheet(title) {
    try {
      setSaveStatus('กำลังสร้าง...');
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
      setSaveStatus('✓ บันทึกแล้ว');
    } catch (err) {
      console.error('Create error:', err);
      setSaveStatus('สร้างไม่สำเร็จ');
    }
  }

  function triggerAutoSave() {
    setSaveStatus('กำลังบันทึก...');
    clearTimeout(saveTimer);
    saveTimer = setTimeout(async () => {
      await saveCurrentSheet();
    }, 1200);
  }

  async function saveCurrentSheet() {
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
      setSaveStatus('✓ บันทึกแล้ว');
    } catch (err) {
      console.error('Save error:', err);
      setSaveStatus('บันทึกผิดพลาด');
    }
  }

  function setSaveStatus(text) {
    if (!saveBadge) return;
    saveBadge.textContent = text;
    if (text.includes('กำลัง')) {
      saveBadge.classList.add('saving');
    } else {
      saveBadge.classList.remove('saving');
    }
  }

  // CSV Export
  function exportCSV() {
    if (!xs) return;
    const data = xs.getData();
    if (!data || data.length === 0) return;

    const sheet = data[0];
    const rows = sheet.rows || {};
    let csvContent = '';

    const maxRow = Math.max(20, Object.keys(rows).length);
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

  // CSV Import
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

  function setupEventListeners() {
    titleInput.addEventListener('input', () => {
      triggerAutoSave();
    });

    document.getElementById('btn-export')?.addEventListener('click', exportCSV);

    const fileInput = document.getElementById('csv-file-input');
    document.getElementById('btn-import')?.addEventListener('click', () => {
      fileInput?.click();
    });
    fileInput?.addEventListener('change', (e) => {
      if (e.target.files && e.target.files[0]) {
        importCSV(e.target.files[0]);
      }
    });

    document.getElementById('btn-new')?.addEventListener('click', async () => {
      const name = prompt('ชื่อสเปรดชีตใหม่:', 'สเปรดชีต ' + (sheetList.length + 1));
      if (name) {
        await createNewSheet(name);
        await loadSheetList();
      }
    });

    // Sheet Switcher Modal
    const switcherModal = document.getElementById('sheet-switcher-modal');
    document.getElementById('btn-sheets')?.addEventListener('click', async () => {
      await renderSheetSwitcher();
      switcherModal.style.display = 'flex';
    });
    document.getElementById('close-switcher')?.addEventListener('click', () => {
      switcherModal.style.display = 'none';
    });

    // AI Modal
    const aiModal = document.getElementById('ai-modal');
    document.getElementById('btn-ai')?.addEventListener('click', () => {
      aiModal.style.display = 'flex';
      document.getElementById('ai-prompt').focus();
    });
    document.getElementById('close-ai')?.addEventListener('click', () => {
      aiModal.style.display = 'none';
    });

    document.getElementById('btn-run-ai')?.addEventListener('click', runAIAssist);
  }

  async function renderSheetSwitcher() {
    const listContainer = document.getElementById('switcher-list');
    if (!listContainer) return;
    listContainer.innerHTML = 'กำลังโหลดรายการ...';

    const res = await fetch('/api/sheets');
    if (res.ok) {
      sheetList = await res.json();
    }

    if (sheetList.length === 0) {
      listContainer.innerHTML = '<p style="color:var(--studio-muted)">ยังไม่มีสเปรดชีตอื่น</p>';
      return;
    }

    listContainer.innerHTML = '';
    sheetList.forEach((sh) => {
      const div = document.createElement('div');
      div.className = `sheet-list-item ${currentSheet && currentSheet.id === sh.id ? 'active' : ''}`;
      div.innerHTML = `
        <div>
          <strong>${escapeHtml(sh.title)}</strong>
          <div style="font-size:11px;color:var(--studio-muted)">แก้ไขล่าสุด: ${new Date(sh.updated_at).toLocaleString('th-TH')}</div>
        </div>
        <button class="studio-btn btn-del" style="color:#ef4444;border:none;background:transparent;padding:4px 8px" title="ลบ">✕</button>
      `;

      div.querySelector('strong').parentElement.addEventListener('click', async () => {
        document.getElementById('sheet-switcher-modal').style.display = 'none';
        await loadSheet(sh.id);
      });

      div.querySelector('.btn-del').addEventListener('click', async (e) => {
        e.stopPropagation();
        if (confirm(`ต้องการลบ "${sh.title}" ใช่หรือไม่?`)) {
          await fetch(`/api/sheets/${sh.id}`, { method: 'DELETE' });
          await renderSheetSwitcher();
          if (currentSheet && currentSheet.id === sh.id) {
            await loadSheetList();
          }
        }
      });

      listContainer.appendChild(div);
    });
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
    resultBox.textContent = 'กำลังประมวลผล...';

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
      runBtn.textContent = 'ส่งคำขอ';
    }
  }

  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  document.addEventListener('DOMContentLoaded', init);
})();
