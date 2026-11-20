(() => {
  const TZ = 'Asia/Bangkok';
  const $ = (selector) => document.querySelector(selector);
  const monthFormatter = new Intl.DateTimeFormat('th-TH', { timeZone: TZ, month: 'long', year: 'numeric' });
  const dateFormatter = new Intl.DateTimeFormat('th-TH', { timeZone: TZ, weekday: 'long', day: 'numeric', month: 'long' });
  const timeFormatter = new Intl.DateTimeFormat('th-TH', { timeZone: TZ, hour: '2-digit', minute: '2-digit' });
  const partsFormatter = new Intl.DateTimeFormat('en-CA', { timeZone: TZ, year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hourCycle: 'h23' });
  const state = { reminders: [], deliveries: [], history: [], year: 0, month: 0, selected: '', openRow: null, editID: null, loadFailed: false, telegramReady: false };

  function parts(value) { return Object.fromEntries(partsFormatter.formatToParts(value).filter((p) => p.type !== 'literal').map((p) => [p.type, p.value])); }
  function dateKey(value) { const p = parts(value); return `${p.year}-${p.month}-${p.day}`; }
  function parseKey(key) { const [year, month, day] = key.split('-').map(Number); return new Date(Date.UTC(year, month - 1, day)); }
  function shiftKey(key, days) { const date = parseKey(key); date.setUTCDate(date.getUTCDate() + days); return `${date.getUTCFullYear()}-${String(date.getUTCMonth()+1).padStart(2,'0')}-${String(date.getUTCDate()).padStart(2,'0')}`; }
  function bangkokDate(key, hour, minute) { const [year, month, day] = key.split('-').map(Number); return new Date(Date.UTC(year, month - 1, day, hour - 7, minute)); }
  function localDateTimeValue(date) { const p = parts(date); return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`; }
  function escapeHTML(value) { return String(value || '').replace(/[&<>"']/g, (ch) => ({ '&':'&amp;', '<':'&lt;', '>':'&gt;', '"':'&quot;', "'":'&#39;' }[ch])); }
  function monthDate(year, month, day) { return new Date(Date.UTC(year, month, day, 12)); }
  function showToast(message) { const toast = $('#schedule-toast'); toast.textContent = message; toast.classList.add('show'); setTimeout(() => toast.classList.remove('show'), 3000); }
  async function api(path, options = {}) {
    const headers = { Accept: 'application/json', ...(options.body ? { 'Content-Type': 'application/json' } : {}), ...options.headers };
    const response = await fetch(path, { ...options, headers, cache: 'no-store' });
    if (!response.ok) { let message = `เกิดข้อผิดพลาด (${response.status})`; try { message = (await response.json()).error || message; } catch (_) {} throw new Error(message); }
    if (response.status === 204) return null;
    return response.json();
  }
  function reminderStart(r) { return new Date(r.starts_at); }
  function eventForDate(reminder, key) {
    if (reminder.paused) return null;
    const start = reminderStart(reminder), startKey = dateKey(start);
    if ((reminder.completed_occurrences || []).some((eventAt) => dateKey(new Date(eventAt)) === key)) return null;
    if (reminder.recurrence !== 'weekly' && startKey !== key) return null;
    if (reminder.recurrence === 'weekly') {
      if (key < startKey) return null;
      const startDay = parseKey(startKey).getUTCDay(), keyDay = parseKey(key).getUTCDay();
      if (startDay !== keyDay) return null;
    }
    const p = parts(start);
    if (reminder.recurrence === 'once') return { reminder, eventAt: start };
    return { reminder, eventAt: bangkokDate(key, Number(p.hour), Number(p.minute)) };
  }
  function eventsForDate(key) { return state.reminders.map((r) => eventForDate(r, key)).filter(Boolean).sort((a,b) => a.eventAt - b.eventAt); }
  function currentKey() { return dateKey(new Date()); }

  function renderTop() {
    const status=$('#telegram-status');status.hidden=state.telegramReady;status.textContent=state.telegramReady?'':'ยังไม่ได้เชื่อม Telegram — การเตือนจะไม่ถูกส่ง';
  }

  function renderMonth() {
    $('#month-label').textContent = monthFormatter.format(monthDate(state.year,state.month,1));
    const first = new Date(Date.UTC(state.year,state.month,1,12)); const offset = (first.getUTCDay()+6)%7; const firstCell = new Date(Date.UTC(state.year,state.month,1-offset,12));
    const grid = $('#month-grid'); grid.replaceChildren();
    for(let i=0;i<42;i++) {
      const day = new Date(firstCell); day.setUTCDate(firstCell.getUTCDate()+i);
      const key = `${day.getUTCFullYear()}-${String(day.getUTCMonth()+1).padStart(2,'0')}-${String(day.getUTCDate()).padStart(2,'0')}`;
      const events = eventsForDate(key); const cell = document.createElement('button'); cell.type='button'; cell.className='calendar-cell'; cell.setAttribute('role','gridcell');
      if(day.getUTCMonth()!==state.month) cell.classList.add('outside'); if(key===currentKey()) cell.classList.add('today'); if(key===state.selected) cell.classList.add('selected'); cell.setAttribute('aria-label',`${dateFormatter.format(bangkokDate(key,12,0))}${events.length?`, ${events.length} รายการ`:''}`); cell.dataset.date=key;
      const number=document.createElement('span');number.className='cell-day';number.textContent=String(day.getUTCDate());cell.append(number);
      if(events.length){cell.classList.add('has-events');const dot=document.createElement('i');dot.className='cell-dot';dot.setAttribute('aria-hidden','true');cell.append(dot)}
      if(key===state.selected) cell.setAttribute('aria-selected','true');
      grid.append(cell);
    }
  }

  function renderAgenda() {
    const root=$('#agenda-list');root.replaceChildren();
    const key=state.selected||currentKey();const events=eventsForDate(key);
    const dayLabel=key===currentKey()?'วันนี้':new Intl.DateTimeFormat('th-TH',{timeZone:TZ,weekday:'short',day:'numeric',month:'short'}).format(bangkokDate(key,12,0));
    $('#day-heading').textContent=`${dayLabel} · ${events.length} รายการ`;
    events.forEach(({reminder,eventAt})=>{
      const rowKey=`${reminder.id}-${eventAt.getTime()}`;const open=state.openRow===rowKey;
      const row=document.createElement('div');row.className='list-item reminder-row'+(open?' is-open':'');
      row.innerHTML=`<time class="reminder-time">${timeFormatter.format(eventAt)}</time><div class="grow"><strong>${escapeHTML(reminder.title)}</strong>${reminder.details?`<small>${escapeHTML(reminder.details)}</small>`:''}${reminder.recurrence==='weekly'?'<small>ทุกสัปดาห์</small>':''}</div><button type="button" class="btn-icon row-more" data-toggle="${rowKey}" aria-expanded="${open}" aria-label="ตัวเลือกของ ${escapeHTML(reminder.title)}"><svg class="icon"><use href="#i-more"/></svg></button><div class="row-actions"${open?'':' hidden'}><button type="button" class="btn btn-sm" data-action="edit" data-id="${reminder.id}">แก้ไข</button>${reminder.recurrence==='weekly'?`<button type="button" class="btn btn-sm" data-action="pause" data-id="${reminder.id}">พัก</button>`:''}<button type="button" class="btn btn-sm" data-action="done" data-id="${reminder.id}" data-event="${eventAt.toISOString()}">เสร็จแล้ว</button><button type="button" class="btn btn-sm btn-danger" data-action="delete" data-id="${reminder.id}">ลบ</button></div>`;
      root.append(row);
    });
    if(!events.length){const empty=document.createElement('div');empty.className='empty';empty.textContent=state.loadFailed?'โหลดตารางไม่สำเร็จ ลองเปิดหน้านี้อีกครั้ง':'ไม่มีการเตือนในวันนี้ กด + เพื่อเพิ่ม';root.append(empty)}
  }

  function renderIssues() {
    const failed=state.deliveries.filter((d)=>d.state==='failed');const root=$('#delivery-issue-list');root.replaceChildren();$('#delivery-issues').hidden=failed.length===0;
    failed.forEach((d)=>{const item=document.createElement('div');item.className='list-item delivery-issue';item.innerHTML=`<div class="grow"><strong>${escapeHTML(d.title)}</strong><small>${dateFormatter.format(new Date(d.event_at))} · ${escapeHTML(d.last_error||'ส่งไม่สำเร็จ')}</small></div>`;root.append(item)});
  }
  function renderPaused() {
    const paused=state.reminders.filter((reminder)=>reminder.paused);const section=$('#schedule-paused');const root=$('#paused-list');root.replaceChildren();section.hidden=paused.length===0;
    paused.forEach((reminder)=>{const row=document.createElement('div');row.className='list-item paused-item';row.innerHTML=`<div class="grow"><strong>${escapeHTML(reminder.title)}</strong><small>ทุกสัปดาห์ · ${timeFormatter.format(reminderStart(reminder))}</small></div><div class="paused-actions"><button type="button" class="btn btn-sm" data-action="resume" data-id="${reminder.id}">เปิดเตือน</button><button type="button" class="btn-icon" data-action="edit" data-id="${reminder.id}" aria-label="แก้ไข ${escapeHTML(reminder.title)}"><svg class="icon"><use href="#i-more"/></svg></button></div>`;root.append(row)});
  }
  function renderHistory() {
    const root=$('#schedule-history-list');root.replaceChildren();
    if(!state.history.length){const empty=document.createElement('p');empty.className='empty';empty.textContent='ยังไม่มีรายการที่ทำเสร็จใน 30 วันที่ผ่านมา';root.append(empty);return;}
    state.history.forEach((entry)=>{const row=document.createElement('div');row.className='history-entry';row.innerHTML=`<strong>${escapeHTML(entry.title)}</strong><time class="muted">${dateFormatter.format(new Date(entry.event_at))} · ทำเสร็จ ${timeFormatter.format(new Date(entry.completed_at))}</time>`;root.append(row)});
  }
  function render() { renderTop();renderMonth();renderAgenda();renderPaused();renderIssues();renderHistory(); }
  async function load() {
    try { const [reminders,upcoming,history,status]=await Promise.all([api('/api/schedule/reminders'),api('/api/schedule/upcoming'),api('/api/schedule/history'),api('/api/schedule/status')]);state.reminders=reminders.items||[];state.deliveries=upcoming.items||[];state.history=history.items||[];state.telegramReady=Boolean(status.telegram_ready);state.loadFailed=false; }
    catch(error){state.loadFailed=true;showToast(error.message)}
    render();
  }
  function openForm(reminder=null,day=state.selected||currentKey()) {
    state.editID=reminder?reminder.id:null;$('#dialog-title').textContent=reminder?'แก้ไขการเตือน':'เพิ่มการเตือน';$('#form-error').textContent='';
    $('#nl-block').hidden=Boolean(reminder);$('#delete-in-form').hidden=!reminder;setManualVisible(Boolean(reminder),false);
    $('#reminder-title').value=reminder?.title||'';$('#reminder-details').value=reminder?.details||'';$('#reminder-recurrence').value=reminder?.recurrence||'once';
    $('#reminder-start').removeAttribute('min');
    if(reminder) $('#reminder-start').value=localDateTimeValue(reminderStart(reminder));
    else { const current=parts(new Date());$('#reminder-start').min=`${current.year}-${current.month}-${current.day}T${current.hour}:${current.minute}`;const now=new Date(Date.now()+3600000);let p=parts(now);let minutes=Math.ceil(Number(p.minute)/5)*5;if(minutes>=60){minutes=0;const shifted=bangkokDate(`${p.year}-${p.month}-${p.day}`,Number(p.hour),0);const next=new Date(shifted.getTime()+3600000);p=parts(next)}$('#reminder-start').value=`${p.year}-${p.month}-${p.day}T${p.hour}:${String(minutes).padStart(2,'0')}`;if(day!==currentKey())$('#reminder-start').value=`${day}T${p.hour}:${String(minutes).padStart(2,'0')}`; }
    const offsets=reminder?.offsets||[0];document.querySelectorAll('input[name="offsets"]').forEach(input=>input.checked=offsets.includes(Number(input.value)));
    if(!$('#reminder-dialog').open)$('#reminder-dialog').showModal();
  }
  async function save(event) {
    event.preventDefault();$('#form-error').textContent='';
    const local=$('#reminder-start').value;if(!local){$('#form-error').textContent='เลือกวันและเวลาก่อนนะ';return;}
    const body={title:$('#reminder-title').value.trim(),details:$('#reminder-details').value.trim(),starts_at:`${local}:00+07:00`,recurrence:$('#reminder-recurrence').value,offsets:[...document.querySelectorAll('input[name="offsets"]:checked')].map((input)=>Number(input.value))};
    const button=$('#save-reminder');button.disabled=true;button.textContent='กำลังบันทึก…';
    try{await api(`/api/schedule/reminders${state.editID?`/${state.editID}`:''}`,{method:state.editID?'PUT':'POST',body:JSON.stringify(body)});$('#reminder-dialog').close();showToast(state.editID?'อัปเดตการเตือนแล้ว':'เพิ่มการเตือนแล้ว');await load();}
    catch(error){$('#form-error').textContent=error.message;}
    finally{button.disabled=false;button.textContent='บันทึกการเตือน';}
  }
  async function act(button) {
    const action=button.dataset.action,id=Number(button.dataset.id);if(action==='edit'){openForm(state.reminders.find(r=>r.id===id));return;}
    if(action==='delete'&&!confirm('ลบการเตือนนี้และการแจ้งเตือนที่ยังไม่ส่งหรือไม่?'))return;
    if(action==='done'&&!confirm('ทำเครื่องหมายรายการนี้ว่าเสร็จแล้วหรือไม่?'))return;
    try{if(action==='delete')await api(`/api/schedule/reminders/${id}`,{method:'DELETE'});else if(action==='pause'||action==='resume')await api(`/api/schedule/reminders/${id}/${action}`,{method:'POST'});else await api('/api/schedule/complete',{method:'POST',body:JSON.stringify({reminder_id:id,event_at:button.dataset.event})});showToast(action==='delete'?'ลบรายการแล้ว':action==='pause'?'พักการเตือนไว้แล้ว':action==='resume'?'เปิดการเตือนแล้ว':'ทำเครื่องหมายเสร็จแล้ว');await load();}
    catch(error){showToast(error.message)}
  }

  async function parseNLPrompt() {
    const prompt = $('#nl-prompt-input')?.value?.trim();
    if (!prompt) { showToast('พิมพ์ข้อความที่ต้องการเตือนก่อนนะ'); $('#nl-prompt-input')?.focus(); return; }

    const btn = $('#nl-parse-btn');
    if (btn) { btn.disabled = true; btn.textContent = 'กำลังวิเคราะห์...'; }

    try {
      const data = await api('/api/schedule/parse', {
        method: 'POST',
        body: JSON.stringify({ prompt })
      });

      if (data?.fallback) { showManualFallback(prompt); return; }
      const preview = data?.preview || {};
      const needsClarification = Boolean(data?.needs_clarification);

      const alertEl = $('#nl-ambiguity-alert');
      if (alertEl) {
        alertEl.hidden = !needsClarification;
        if (needsClarification) {
          const msgEl = $('#nl-ambiguity-msg');
          if (msgEl) msgEl.textContent = data?.question || 'ระบุเวลาไม่ชัดเจน กรุณาระบุหรือเลือกเวลาที่แน่นอนด้านล่าง';
        }
      }

      const titleInput = $('#nl-field-title');
      if (titleInput) titleInput.value = preview.title || prompt;

      const detailsInput = $('#nl-field-details');
      if (detailsInput) detailsInput.value = preview.details || '';

      const recurrenceSelect = $('#nl-field-recurrence');
      if (recurrenceSelect) recurrenceSelect.value = preview.recurrence || 'once';

      const startsAtInput = $('#nl-field-starts-at');
      if (startsAtInput) {
        if (preview.starts_at) {
          try {
            const dt = new Date(preview.starts_at);
            startsAtInput.value = localDateTimeValue(dt);
          } catch (_) {
            startsAtInput.value = '';
          }
        }
        if (!startsAtInput.value) {
          const now = new Date(Date.now() + 3600000);
          startsAtInput.value = localDateTimeValue(now);
        }
      }

      const offsets = Array.isArray(preview.offsets) ? preview.offsets : [0];
      document.querySelectorAll('input[name="nl-offsets"]').forEach(input => {
        input.checked = offsets.includes(Number(input.value));
      });

      const card = $('#nl-preview-card');
      if (card) {
        card.hidden = false;
        setManualVisible(false, false);
        card.scrollIntoView({ behavior: 'smooth', block: 'nearest' });
      }
    } catch (err) {
      showManualFallback(prompt);
    } finally {
      if (btn) { btn.disabled = false; btn.textContent = 'แปลผลด้วย AI'; }
    }
  }

  async function saveNLPreview() {
    const titleInput = $('#nl-field-title');
    const local = $('#nl-field-starts-at')?.value;
    if (!titleInput?.value?.trim()) { showToast('กรุณาระบุเรื่องที่ต้องการเตือน'); titleInput?.focus(); return; }
    if (!local) { showToast('กรุณาเลือกวันและเวลาก่อนนะ'); $('#nl-field-starts-at')?.focus(); return; }

    const offsets = [...document.querySelectorAll('input[name="nl-offsets"]:checked')].map(i => Number(i.value));
    const body = {
      title: titleInput.value.trim(),
      details: $('#nl-field-details')?.value?.trim() || '',
      starts_at: `${local}:00+07:00`,
      recurrence: $('#nl-field-recurrence')?.value || 'once',
      offsets: offsets.length ? offsets : [0]
    };

    const confirmBtn = $('#nl-confirm-save-btn');
    if (confirmBtn) { confirmBtn.disabled = true; confirmBtn.textContent = 'กำลังบันทึก…'; }

    try {
      await api('/api/schedule/reminders', {
        method: 'POST',
        body: JSON.stringify(body)
      });
      const card = $('#nl-preview-card');
      if (card) card.hidden = true;
      const promptInput = $('#nl-prompt-input');
      if (promptInput) promptInput.value = '';
      $('#reminder-dialog').close();
      showToast('เพิ่มการเตือนแล้ว');
      await load();
    } catch (err) {
      showToast(err.message);
    } finally {
      if (confirmBtn) { confirmBtn.disabled = false; confirmBtn.textContent = 'ยืนยันและบันทึก'; }
    }
  }

  function openManualFormFromNL() {
    const title = $('#nl-field-title')?.value || '';
    const details = $('#nl-field-details')?.value || '';
    const recurrence = $('#nl-field-recurrence')?.value || 'once';
    const local = $('#nl-field-starts-at')?.value || '';
    const offsets = [...document.querySelectorAll('input[name="nl-offsets"]:checked')].map(i => Number(i.value));

    const card = $('#nl-preview-card');
    if (card) card.hidden = true;

    openForm();
    setManualVisible(true, false);
    if (title) $('#reminder-title').value = title;
    if (details) $('#reminder-details').value = details;
    if (recurrence) $('#reminder-recurrence').value = recurrence;
    if (local) $('#reminder-start').value = local;
    if (offsets.length) {
      document.querySelectorAll('input[name="offsets"]').forEach(i => {
        i.checked = offsets.includes(Number(i.value));
      });
    }
  }

  // The manual form stays hidden for new reminders until AI parsing is unavailable
  // (fallback/error) or the user asks for it from the preview card.
  function setManualVisible(show,withNote){$('#reminder-form').hidden=!show;$('#nl-fallback-note').hidden=!(show&&withNote);if(show)$('#nl-preview-card').hidden=true;}
  function showManualFallback(prompt){setManualVisible(true,true);const title=$('#reminder-title');if(prompt&&!title.value)title.value=prompt;title.focus();}
  function selectDay(key){state.selected=key;state.openRow=null;renderMonth();renderAgenda();}
  function moveMonth(delta){state.month+=delta;if(state.month<0){state.month=11;state.year--}if(state.month>11){state.month=0;state.year++}const prefix=`${state.year}-${String(state.month+1).padStart(2,'0')}`;if(!state.selected.startsWith(prefix))state.selected=prefix===currentKey().slice(0,7)?currentKey():`${prefix}-01`;state.openRow=null;renderMonth();renderAgenda();}
  function bind(){
    const themeToggle = $('#theme-toggle');
    if (themeToggle) {
      themeToggle.addEventListener('click', () => {
        const isDark = document.documentElement.dataset.theme !== 'light';
        const next = isDark ? 'light' : 'dark';
        document.documentElement.dataset.theme = next;
        localStorage.setItem('sentinel-theme', next);
      });
    }
    $('#add-reminder').addEventListener('click',()=>openForm());$('#dialog-close').addEventListener('click',()=>$('#reminder-dialog').close());$('#cancel-form').addEventListener('click',()=>$('#reminder-dialog').close());$('#reminder-form').addEventListener('submit',save);
    $('#nl-parse-btn')?.addEventListener('click', parseNLPrompt);
    $('#nl-prompt-input')?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); parseNLPrompt(); }
    });
    $('#nl-confirm-save-btn')?.addEventListener('click', saveNLPreview);
    $('#nl-open-modal-btn')?.addEventListener('click', openManualFormFromNL);
    $('#nl-cancel-btn')?.addEventListener('click', () => { $('#nl-preview-card').hidden = true; });
    $('#nl-discard-btn')?.addEventListener('click', () => { $('#nl-preview-card').hidden = true; });
    $('#delete-in-form').addEventListener('click',(event)=>{event.currentTarget.dataset.id=String(state.editID);$('#reminder-dialog').close();act(event.currentTarget)});
    $('#reminder-dialog').addEventListener('click',(event)=>{if(event.target===event.currentTarget)event.currentTarget.close()});
    $('#previous-month').addEventListener('click',()=>moveMonth(-1));$('#next-month').addEventListener('click',()=>moveMonth(1));$('#jump-today').addEventListener('click',()=>{const p=parts(new Date());state.year=Number(p.year);state.month=Number(p.month)-1;selectDay(currentKey())});
    $('#month-grid').addEventListener('click',(event)=>{const cell=event.target.closest('[data-date]');if(!cell)return;const key=cell.dataset.date;const [y,m]=key.split('-').map(Number);if(y!==state.year||m-1!==state.month){state.year=y;state.month=m-1}selectDay(key)});
    $('#agenda-list').addEventListener('click',(event)=>{const toggle=event.target.closest('[data-toggle]');if(toggle){state.openRow=state.openRow===toggle.dataset.toggle?null:toggle.dataset.toggle;renderAgenda();return}const button=event.target.closest('[data-action]');if(button)act(button)});$('#paused-list').addEventListener('click',(event)=>{const button=event.target.closest('[data-action]');if(button)act(button)});
    window.addEventListener('keydown',(event)=>{if(event.key==='Escape')return;if((event.metaKey||event.ctrlKey)&&event.key==='n'){event.preventDefault();openForm()}});
  }
  document.addEventListener('DOMContentLoaded',()=>{const now=parts(new Date());state.year=Number(now.year);state.month=Number(now.month)-1;const requestedDate=new URLSearchParams(location.search).get('date');state.selected=requestedDate?dateKey(new Date(requestedDate)):currentKey();if(requestedDate){const selected=parts(new Date(requestedDate));state.year=Number(selected.year);state.month=Number(selected.month)-1}bind();load();setInterval(load,60000);});
})();
