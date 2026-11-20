// =============================================================================
// Pocket Sentinel — Main App & Global Alpine Store
// =============================================================================

// Common Formatter Helpers
function formatNumber(value, digits = 1) {
  return Number.isFinite(Number(value)) ? Number(value).toFixed(digits) : '—';
}

function formatUptime(seconds) {
  if (!Number.isFinite(Number(seconds))) return '—';
  const total = Math.max(0, Math.floor(Number(seconds)));
  const days = Math.floor(total / 86400);
  const hours = Math.floor((total % 86400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  if (days) return `${days}d ${hours}h`;
  if (hours) return `${hours}h ${minutes}m`;
  return `${minutes}m`;
}

function formatTime(value, options = { hour: '2-digit', minute: '2-digit' }) {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? '—' : date.toLocaleTimeString([], options);
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

async function getJSON(path) {
  const response = await fetch(path, { cache: 'no-store', headers: { Accept: 'application/json' } });
  if (!response.ok) throw new Error(`${path}: HTTP ${response.status}`);
  return response.json();
}

// Immediate Theme Initialization (Prevents White Flash)
(function initTheme() {
  const saved = localStorage.getItem('sentinel-theme');
  const theme = (saved === 'light' || saved === 'dark') ? saved : 'dark';
  document.documentElement.setAttribute('data-theme', theme);
  const meta = document.getElementById('theme-color-meta');
  if (meta) meta.setAttribute('content', theme === 'dark' ? '#0c0d10' : '#ffffff');
})();

// PWA Service Worker Registration
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((err) => {
      console.warn('PWA service worker registration failed:', err);
    });
  });
}

// Alpine Store Initialization
const initialParams = new URLSearchParams(window.location.search);
if (initialParams.get('tab') === 'telegram' || initialParams.get('app') === 'relay') {
  window.location.replace('/relay');
}

document.addEventListener('alpine:init', () => {
  const initialTab = 'overview';

  Alpine.store('sentinel', {
    theme: localStorage.getItem('sentinel-theme') || 'dark',
    activeTab: initialTab,
    busy: false,
    heroTitle: 'กำลังตรวจสอบเซิร์ฟเวอร์',
    heroCopy: 'กำลังโหลดสถานะอุปกรณ์และบริการล่าสุด',
    statusDotClass: '',
    syncDotClass: 'sync-dot',
    syncText: 'Connecting…',
    syncTitle: 'Telemetry sync status',
    sampleTimeText: 'Waiting for first sample',
    currentData: null,
    conditions: [],
    dataNotice: { text: '', error: false, show: false },

    init() {
      this.applyTheme(this.theme);
      this.refreshAll();
      window.setInterval(() => this.refreshAll(), 60000);
      window.addEventListener('popstate', () => {
        this.activateTab('overview');
      });
    },

    setTab(tab) {
      if (tab !== 'overview' || tab === this.activeTab) return;
      const url = new URL(window.location.href);
      url.searchParams.delete('app');
      url.searchParams.delete('tab');
      window.history.pushState({ tab }, '', url);
      this.activateTab(tab);
    },

    activateTab(tab) {
      this.activeTab = tab;
      window.dispatchEvent(new CustomEvent('sentinel:tab-changed', { detail: { tab } }));
    },

    toggleTheme() {
      this.applyTheme(this.theme === 'dark' ? 'light' : 'dark');
    },

    applyTheme(theme) {
      this.theme = theme;
      document.documentElement.setAttribute('data-theme', theme);
      const meta = document.getElementById('theme-color-meta');
      if (meta) meta.setAttribute('content', theme === 'dark' ? '#0c0d10' : '#ffffff');
      localStorage.setItem('sentinel-theme', theme);
      window.dispatchEvent(new CustomEvent('sentinel:theme-changed', { detail: { theme } }));
    },

    async refreshAll() {
      if (this.busy) return;
      this.busy = true;
      this.dataNotice.show = false;

      try {
        const [currentRes, historyRes, logsRes] = await Promise.allSettled([
          getJSON('/api/telemetry/current'),
          getJSON('/api/telemetry/history?hours=24&limit=500'),
          getJSON('/api/logs')
        ]);

        if (currentRes.status === 'fulfilled') {
          this.processCurrent(currentRes.value);
        } else {
          this.heroTitle = 'ไม่สามารถโหลดสถานะเซิร์ฟเวอร์';
          this.heroCopy = currentRes.reason.message || 'ระบบสถานะไม่ตอบกลับ';
          this.statusDotClass = 'error';
          this.syncDotClass = 'sync-dot error';
          this.syncText = 'Offline';
        }

        if (historyRes.status === 'fulfilled') {
          window.dispatchEvent(new CustomEvent('sentinel:history-loaded', { detail: historyRes.value }));
        } else {
          this.dataNotice = { text: `Could not load chart history: ${historyRes.reason.message || 'request failed'}`, error: true, show: true };
        }

        if (logsRes.status === 'fulfilled') {
          window.dispatchEvent(new CustomEvent('sentinel:logs-loaded', { detail: logsRes.value }));
        } else {
          window.dispatchEvent(new CustomEvent('sentinel:logs-loaded', { detail: [] }));
        }
      } catch (err) {
        this.dataNotice = { text: err.message || 'Unable to refresh dashboard data.', error: true, show: true };
      } finally {
        this.busy = false;
      }
    },

    processCurrent(data) {
      this.currentData = data;
      const sample = data.latest_sample;
      this.conditions = data.conditions || [];

      const lastCollected = data.last_collected ? new Date(data.last_collected) : null;
      const intervalMs = Math.max(5, Number(data.sample_interval_seconds) || 60) * 1000;
      const stale = !lastCollected || Number.isNaN(lastCollected.getTime()) || Date.now() - lastCollected.getTime() > intervalMs * 2;
      const collectorError = Boolean(data.last_error);

      if (!sample) {
        this.heroTitle = 'รอข้อมูลอุปกรณ์';
        this.heroCopy = 'เซิร์ฟเวอร์พร้อม แต่ยังไม่มีข้อมูลอุปกรณ์ที่บันทึกไว้';
        this.statusDotClass = 'warning';
      } else if (collectorError || stale) {
        this.heroTitle = collectorError ? 'ระบบเก็บข้อมูลมีปัญหา' : 'ข้อมูลล่าสุดล่าช้า';
        this.heroCopy = collectorError ? String(data.last_error) : 'ไม่มีข้อมูลใหม่เกิน 2 รอบการเก็บข้อมูล';
        this.statusDotClass = 'error';
      } else {
        const active = this.conditions.filter((c) => c.active);
        this.heroTitle = active.length ? 'ต้องตรวจสอบระบบ' : 'ระบบทำงานปกติ';
        this.heroCopy = active.length ? `พบ ${active.length} รายการที่ต้องตรวจสอบ` : 'เซิร์ฟเวอร์และข้อมูลอุปกรณ์อัปเดตตามปกติ';
        this.statusDotClass = active.length ? 'warning' : '';
      }

      if (!lastCollected || Number.isNaN(lastCollected.getTime())) {
        this.syncText = 'กำลังเชื่อมต่อ';
        this.syncDotClass = 'sync-dot stale';
        this.syncTitle = 'No telemetry collection recorded yet';
      } else if (collectorError) {
        this.syncText = 'ซิงก์ไม่สำเร็จ';
        this.syncDotClass = 'sync-dot error';
        this.syncTitle = `Collection error: ${data.last_error}`;
      } else if (stale) {
        this.syncText = `ล่าช้า · ${formatTime(lastCollected)}`;
        this.syncDotClass = 'sync-dot stale';
        this.syncTitle = `Delayed: last update was ${lastCollected.toLocaleString()}`;
      } else {
        this.syncText = formatTime(lastCollected);
        this.syncDotClass = 'sync-dot live';
        this.syncTitle = `Last synced: ${lastCollected.toLocaleString()}`;
      }

      this.sampleTimeText = sample
        ? `Sampled ${formatTime(sample.timestamp, { hour: '2-digit', minute: '2-digit', second: '2-digit' })}`
        : 'รอข้อมูลตัวอย่างแรก';

      // Notify other components (e.g. 3D Energy Core)
      window.dispatchEvent(new CustomEvent('sentinel:current-loaded', { detail: data }));
    }
  });
});
