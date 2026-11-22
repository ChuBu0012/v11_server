// =============================================================================
// Pocket Sentinel — AI Limit Hub Component (Quota, Insights & Trends)
// =============================================================================

// Ensure Chart.js is loaded
let aiLimitChartLibraryPromise;

function ensureAiLimitChartLibrary() {
  if (window.Chart) return Promise.resolve();
  if (aiLimitChartLibraryPromise) return aiLimitChartLibraryPromise;

  aiLimitChartLibraryPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/chart.js@4.6.1/dist/chart.umd.min.js';
    script.async = true;
    script.onload = resolve;
    script.onerror = () => reject(new Error('โหลด Chart.js ไม่สำเร็จ'));
    document.head.appendChild(script);
  });
  return aiLimitChartLibraryPromise;
}

// Keep Chart.js instance strictly OUTSIDE of Alpine's reactive Proxy
// to eliminate TypeError on non-configurable properties and prevent render stalls.
let quotaChartInstance = null;

document.addEventListener('alpine:init', () => {
  Alpine.data('aiLimitComponent', () => ({
    busy: false,
    loading: true,
    chartUnavailable: false,
    hours: 24,
    hasData: false,
    updatedAt: '',
    updatedText: 'กำลังโหลด…',

    // Raw buckets & groups
    buckets: [],
    groups: [],

    // Insight state
    insight: null,
    insightTitle: 'กำลังวิเคราะห์สถานะ…',
    insightText: 'กำลังคำนวณอัตราการใช้โควตา',
    insightLevel: 'normal',

    init() {
      this.loadLimits();
      // Auto refresh every 5 minutes on client
      window.setInterval(() => {
        this.loadLimits(true);
      }, 5 * 60 * 1000);

      // Local countdown ticker every 10 seconds
      window.setInterval(() => {
        this.updateCountdowns();
      }, 10000);

      // Listen to theme changes to recolor charts
      window.addEventListener('sentinel:theme-changed', () => {
        this.renderChart();
      });

      // Tab changed lifecycle resize guard
      window.addEventListener('sentinel:tab-changed', (e) => {
        if (e.detail?.tab === 'overview' && quotaChartInstance) {
          this.$nextTick(() => {
            quotaChartInstance.resize();
          });
        }
      });
    },

    get statusClass() {
      if (this.insightLevel === 'critical') return 'crit';
      if (this.insightLevel === 'warning') return 'warn';
      return 'ok';
    },

    get statusText() {
      if (this.busy) return 'กำลังอัปเดต…';
      if (this.insightLevel === 'critical') return 'โควตาวิกฤต';
      if (this.insightLevel === 'warning') return 'ควรระวัง';
      return 'ปกติ';
    },

    get insightAlertClass() {
      if (this.insightLevel === 'critical') return 'err';
      if (this.insightLevel === 'warning') return 'warn';
      return 'ok';
    },

    async loadLimits(silent = false) {
      if (!silent) this.loading = true;
      try {
        const res = await fetch(`/api/limits?history=true&hours=${this.hours}`);
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        this.applyData(data);
      } catch (err) {
        console.error('[ailimit] load limits failed:', err);
        if (!silent) {
          this.updatedText = 'เชื่อมต่อไม่สำเร็จ';
        }
      } finally {
        this.loading = false;
        this.busy = false;
      }
    },

    async refreshQuota() {
      if (this.busy) return;
      this.busy = true;
      this.updatedText = 'กำลังรัน agy บนเซิร์ฟเวอร์…';

      try {
        const res = await fetch('/api/limits/refresh', { method: 'POST' });
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const data = await res.json();
        this.applyData(data);
      } catch (err) {
        alert(`รีเฟรชโควตาไม่สำเร็จ: ${err.message}`);
        this.loadLimits(true);
      } finally {
        this.busy = false;
      }
    },

    setHours(h) {
      if (this.hours === h) return;
      this.hours = h;
      this.loadLimits(false);
    },

    applyData(data) {
      if (!data) return;

      this.updatedAt = data.updated_at || '';
      if (this.updatedAt) {
        const d = new Date(this.updatedAt);
        this.updatedText = `อัปเดต: ${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')} น.`;
      }

      this.insight = data.insight || null;
      if (this.insight) {
        this.insightTitle = this.insight.title || 'คำแนะนำสถานะโควตา';
        this.insightText = `${this.insight.summary} — ${this.insight.recommendation}`;
        this.insightLevel = this.insight.level || 'normal';
      }

      const rawBuckets = data.buckets || [];
      this.buckets = rawBuckets.map((b) => {
        const remaining = Math.round(b.remaining_percent);
        let badgeClass = 'ok';
        let badgeText = 'ปกติ';
        let meterClass = '';

        if (remaining <= 15) {
          badgeClass = 'crit';
          badgeText = 'วิกฤต';
          meterClass = 'critical';
        } else if (remaining <= 35) {
          badgeClass = 'warn';
          badgeText = 'เหลือน้อย';
          meterClass = 'warning';
        }

        return {
          ...b,
          remaining_percent: remaining,
          badgeClass,
          badgeText,
          meterClass,
          countdown: this.calcCountdown(b.reset_time)
        };
      });

      this.groups = data.groups || [];
      this.hasData = this.buckets.length > 0;

      // Render or update chart with historical snapshots
      this.renderChart(data.history || []);
    },

    updateCountdowns() {
      for (let i = 0; i < this.buckets.length; i++) {
        this.buckets[i].countdown = this.calcCountdown(this.buckets[i].reset_time);
      }
    },

    calcCountdown(resetTimeStr) {
      if (!resetTimeStr) return '—';
      const target = new Date(resetTimeStr).getTime();
      const diff = target - Date.now();
      if (diff <= 0) return 'กำลังรีเซ็ต';

      const d = Math.floor(diff / (1000 * 60 * 60 * 24));
      const h = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
      const m = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

      if (d > 0) return `${d} วัน ${h} ชม.`;
      if (h > 0) return `${h} ชม. ${m} นาที`;
      return `${m} นาที`;
    },

    async renderChart(history = []) {
      const el = document.getElementById('quota-chart');
      if (!el) return;

      try {
        await ensureAiLimitChartLibrary();
      } catch (e) {
        this.chartUnavailable = true;
        return;
      }

      if (!window.Chart) {
        this.chartUnavailable = true;
        return;
      }

      // Group history by bucket_id
      const series = {
        'gemini-5h':     { label: 'Gemini (5h)', color: '#38bdf8', data: [] },
        'gemini-weekly': { label: 'Gemini (Weekly)', color: '#2563eb', data: [] },
        '3p-5h':         { label: 'Claude/GPT (5h)', color: '#f59e0b', data: [] },
        '3p-weekly':     { label: 'Claude/GPT (Weekly)', color: '#ef4444', data: [] }
      };

      const timeSet = new Set();
      history.forEach((h) => {
        if (!h.recorded_at) return;
        const timeKey = new Date(h.recorded_at).toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' });
        timeSet.add(timeKey);
        if (series[h.bucket_id]) {
          series[h.bucket_id].data.push({ x: timeKey, y: Math.round(h.remaining_fraction * 100) });
        }
      });

      const labels = Array.from(timeSet);
      const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
      const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
      const tickColor = isDark ? '#8e8e93' : '#86868b';

      const datasets = Object.values(series).map((s) => ({
        label: s.label,
        data: s.data,
        borderColor: s.color,
        backgroundColor: s.color,
        borderWidth: 2,
        pointRadius: s.data.length > 30 ? 0 : 2,
        pointHoverRadius: 5,
        tension: 0.2,
        spanGaps: true
      }));

      if (quotaChartInstance) {
        quotaChartInstance.data.labels = labels;
        quotaChartInstance.data.datasets = datasets;
        quotaChartInstance.options.scales.x.grid.color = gridColor;
        quotaChartInstance.options.scales.x.ticks.color = tickColor;
        quotaChartInstance.options.scales.y.grid.color = gridColor;
        quotaChartInstance.options.scales.y.ticks.color = tickColor;
        quotaChartInstance.update();
      } else {
        quotaChartInstance = new Chart(el, {
          type: 'line',
          data: { labels, datasets },
          options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            interaction: { intersect: false, mode: 'index' },
            plugins: {
              legend: {
                display: true,
                position: 'top',
                labels: {
                  color: tickColor,
                  boxWidth: 12,
                  boxHeight: 12,
                  usePointStyle: true,
                  font: { size: 11 }
                }
              },
              tooltip: {
                callbacks: {
                  label: (ctx) => `${ctx.dataset.label}: ${ctx.parsed.y}%`
                }
              }
            },
            scales: {
              x: {
                grid: { color: gridColor },
                ticks: { color: tickColor, maxTicksLimit: 8, font: { size: 10 } }
              },
              y: {
                min: 0,
                max: 100,
                grid: { color: gridColor },
                ticks: {
                  color: tickColor,
                  stepSize: 20,
                  callback: (v) => `${v}%`,
                  font: { size: 10 }
                }
              }
            }
          }
        });
      }

      this.$nextTick(() => {
        if (quotaChartInstance) quotaChartInstance.resize();
      });
    }
  }));
});
