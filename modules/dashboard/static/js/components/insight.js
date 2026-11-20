// =============================================================================
// Pocket Sentinel — Morning Insight card (Overview)
// Shows the latest report from /api/insight/latest; "สร้างใหม่" runs a new one.
// =============================================================================

// Telegram HTML from the server → safe HTML: escape everything, then restore
// only <b>, <i>, <code> (the same tags the server allows) and turn \n into <br>.
function sanitizeInsightHTML(text) {
  const escaped = String(text || '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
  return escaped
    .replace(/&lt;(\/?)(b|i|code)&gt;/g, '<$1$2>')
    .replace(/\n/g, '<br>');
}

const INSIGHT_SOURCE_LABEL = { openrouter: 'สรุปโดย OpenRouter', gemini: 'สรุปโดย Gemini', template: 'ข้อความสำรอง (AI ไม่ว่าง)' };

document.addEventListener('alpine:init', () => {
  Alpine.data('insightCard', () => ({
    loading: true,
    failed: false,
    empty: false,
    busy: false,
    html: '',
    source: '',
    createdAt: '',
    note: '',

    init() {
      this.load();
    },

    apply(report) {
      this.html = sanitizeInsightHTML(report.text_html);
      this.source = report.source || '';
      this.createdAt = report.created_at || '';
      this.empty = false;
      this.failed = false;
    },

    async load() {
      this.loading = true;
      try {
        const res = await fetch('/api/insight/latest', { cache: 'no-store' });
        if (res.status === 404) { this.empty = true; return; }
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        this.apply(await res.json());
      } catch (_) {
        this.failed = true;
      } finally {
        this.loading = false;
      }
    },

    async run() {
      if (this.busy) return;
      this.busy = true;
      this.note = '';
      try {
        const res = await fetch('/api/insight/run', { method: 'POST' });
        if (res.status === 429) this.note = 'เพิ่งสร้างไปเมื่อสักครู่ แสดงฉบับล่าสุดให้';
        else if (!res.ok) throw new Error(`HTTP ${res.status}`);
        this.apply(await res.json());
      } catch (_) {
        this.note = 'สร้างสรุปไม่สำเร็จ ลองใหม่อีกครั้ง';
      } finally {
        this.busy = false;
      }
    },

    get footer() {
      if (!this.createdAt) return '';
      const time = new Date(this.createdAt).toLocaleString('th-TH', { timeZone: 'Asia/Bangkok', day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
      return `${INSIGHT_SOURCE_LABEL[this.source] || this.source} · ${time}`;
    },
  }));
});
