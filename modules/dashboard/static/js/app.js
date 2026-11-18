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
document.addEventListener('alpine:init', () => {
  Alpine.store('sentinel', {
    theme: localStorage.getItem('sentinel-theme') || 'dark',
    busy: false,
    heroTitle: 'Checking your server',
    heroCopy: 'Loading the latest device and service status.',
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
          this.heroTitle = 'Unable to load server status';
          this.heroCopy = currentRes.reason.message || 'The status endpoint did not respond.';
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
        this.heroTitle = 'Waiting for telemetry';
        this.heroCopy = 'The server is available, but it has not stored a device sample yet.';
        this.statusDotClass = 'warning';
      } else if (collectorError || stale) {
        this.heroTitle = collectorError ? 'Collection needs attention' : 'Telemetry is out of date';
        this.heroCopy = collectorError ? String(data.last_error) : 'No new sample has arrived for more than two collection intervals.';
        this.statusDotClass = 'error';
      } else {
        const active = this.conditions.filter((c) => c.active);
        this.heroTitle = active.length ? 'Attention required' : 'All systems operational';
        this.heroCopy = active.length ? `${active.length} condition${active.length === 1 ? '' : 's'} need attention.` : 'Your pocket server and device telemetry are up to date.';
        this.statusDotClass = active.length ? 'warning' : '';
      }

      if (!lastCollected || Number.isNaN(lastCollected.getTime())) {
        this.syncText = 'Connecting…';
        this.syncDotClass = 'sync-dot stale';
        this.syncTitle = 'No telemetry collection recorded yet';
      } else if (collectorError) {
        this.syncText = 'Sync Error';
        this.syncDotClass = 'sync-dot error';
        this.syncTitle = `Collection error: ${data.last_error}`;
      } else if (stale) {
        this.syncText = `Delayed · ${formatTime(lastCollected)}`;
        this.syncDotClass = 'sync-dot stale';
        this.syncTitle = `Delayed: last update was ${lastCollected.toLocaleString()}`;
      } else {
        this.syncText = formatTime(lastCollected);
        this.syncDotClass = 'sync-dot live';
        this.syncTitle = `Last synced: ${lastCollected.toLocaleString()}`;
      }

      this.sampleTimeText = sample
        ? `Sampled ${formatTime(sample.timestamp, { hour: '2-digit', minute: '2-digit', second: '2-digit' })}`
        : 'Waiting for first sample';

      // Notify other components (e.g. 3D Energy Core)
      window.dispatchEvent(new CustomEvent('sentinel:current-loaded', { detail: data }));
    }
  });
});
