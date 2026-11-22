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

    // Unified Single Gauge State ("หลอดเดียวเข้าใจเลย")
    unifiedQuota: {
      percent: 100,
      badgeClass: 'ok',
      badgeText: 'ปกติ',
      meterClass: '',
      fiveHourPercent: 100,
      fiveHourCountdown: '—',
      weeklyPercent: 100,
      weeklyCountdown: '—',
      nextResetText: '—'
    },

    // Raw Gemini bucket references for ticker
    gemini5hReset: '',
    geminiWeeklyReset: '',

    // Insight state
    insightTitle: 'กำลังวิเคราะห์สถานะ…',
    insightText: 'กำลังคำนวณอัตราการใช้โควตา',
    insightLevel: 'normal',
    showInsight: false,

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

      // Filter out Claude / 3P models completely as requested by user
      const rawBuckets = data.buckets || [];
      const geminiBuckets = rawBuckets.filter((b) => b.bucket_id && b.bucket_id.startsWith('gemini'));

      const b5h = geminiBuckets.find((b) => b.window === '5h') || null;
      const bWeekly = geminiBuckets.find((b) => b.window === 'weekly') || null;

      let fiveHourPercent = 100;
      let weeklyPercent = 100;

      if (b5h) {
        fiveHourPercent = Math.round(b5h.remaining_percent);
        this.gemini5hReset = b5h.reset_time || '';
      }
      if (bWeekly) {
        weeklyPercent = Math.round(bWeekly.remaining_percent);
        this.geminiWeeklyReset = bWeekly.reset_time || '';
      }

      // Single Unified Available Limit: bottleneck of 5h and weekly
      let effPercent = 100;
      if (b5h && bWeekly) {
        effPercent = Math.min(fiveHourPercent, weeklyPercent);
      } else if (b5h) {
        effPercent = fiveHourPercent;
      } else if (bWeekly) {
        effPercent = weeklyPercent;
      }

      let badgeClass = 'ok';
      let badgeText = 'ปกติ';
      let meterClass = '';

      if (effPercent <= 15) {
        badgeClass = 'crit';
        badgeText = 'วิกฤต';
        meterClass = 'critical';
        this.insightLevel = 'critical';
        this.insightTitle = '⚠️ โควตา Gemini วิกฤต!';
        this.insightText = `โควตาพร้อมใช้เหลือเพียง ${effPercent}% แนะนำลดระดับ Effort หรือพักคำสั่งงานหนัก`;
        this.showInsight = true;
      } else if (effPercent <= 40) {
        badgeClass = 'warn';
        badgeText = 'เหลือน้อย';
        meterClass = 'warning';
        this.insightLevel = 'warning';
        this.insightTitle = '⚡ โควตา Gemini เริ่มเหลือน้อย';
        this.insightText = `โควตาพร้อมใช้เหลือ ${effPercent}% ควรบริหารจัดการรอบการส่งคำสั่งอย่างประหยัด`;
        this.showInsight = true;
      } else {
        this.insightLevel = 'normal';
        this.insightTitle = '✅ โควตา Gemini พร้อมใช้งาน';
        this.insightText = `โควตาพร้อมใช้ ${effPercent}% อยู่ในเกณฑ์ปกติ สามารถส่งคำสั่งงานต่อได้ตามปกติ`;
        this.showInsight = false;
      }

      const fiveHourCountdown = this.calcCountdown(this.gemini5hReset);
      const weeklyCountdown = this.calcCountdown(this.geminiWeeklyReset);

      this.unifiedQuota = {
        percent: effPercent,
        badgeClass,
        badgeText,
        meterClass,
        fiveHourPercent,
        fiveHourCountdown,
        weeklyPercent,
        weeklyCountdown,
        nextResetText: fiveHourCountdown
      };

      this.hasData = geminiBuckets.length > 0;

      // Render single-line unified trend chart
      this.renderChart(data.history || []);
    },

    updateCountdowns() {
      if (this.gemini5hReset) {
        this.unifiedQuota.fiveHourCountdown = this.calcCountdown(this.gemini5hReset);
        this.unifiedQuota.nextResetText = this.unifiedQuota.fiveHourCountdown;
      }
      if (this.geminiWeeklyReset) {
        this.unifiedQuota.weeklyCountdown = this.calcCountdown(this.geminiWeeklyReset);
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

      // Group history snapshots by recorded_at timestamp (minute-bucketed)
      // and calculate single effective percentage per sample
      const snapshotMap = new Map();
      const sorted = [...history].sort((a, b) => new Date(a.recorded_at) - new Date(b.recorded_at));

      sorted.forEach((h) => {
        if (!h.recorded_at) return;
        // Ignore Claude / 3P models completely
        if (!h.bucket_id || !h.bucket_id.startsWith('gemini')) return;

        const dateObj = new Date(h.recorded_at);
        const timeKey = dateObj.toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' });
        const slotKey = Math.floor(dateObj.getTime() / (60 * 1000));

        if (!snapshotMap.has(slotKey)) {
          snapshotMap.set(slotKey, { timeKey, h5: null, weekly: null });
        }
        const item = snapshotMap.get(slotKey);
        const pct = Math.round(h.remaining_fraction * 100);
        if (h.bucket_id === 'gemini-5h') {
          item.h5 = pct;
        } else if (h.bucket_id === 'gemini-weekly') {
          item.weekly = pct;
        }
      });

      const labels = [];
      const dataPoints = [];
      const metadata = [];

      snapshotMap.forEach((item) => {
        labels.push(item.timeKey);
        let eff = 100;
        if (item.h5 !== null && item.weekly !== null) {
          eff = Math.min(item.h5, item.weekly);
        } else if (item.h5 !== null) {
          eff = item.h5;
        } else if (item.weekly !== null) {
          eff = item.weekly;
        }
        dataPoints.push(eff);
        metadata.push({ h5: item.h5, weekly: item.weekly });
      });

      const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
      const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
      const tickColor = isDark ? '#8e8e93' : '#86868b';

      // One unified line: Clean, intuitive, Apple HIG styled
      const dataset = {
        label: 'โควตา Gemini คงเหลือ (%)',
        data: dataPoints,
        borderColor: '#38bdf8',
        backgroundColor: 'rgba(56, 189, 248, 0.15)',
        fill: true,
        borderWidth: 2.5,
        pointRadius: dataPoints.length > 30 ? 0 : 3,
        pointHoverRadius: 6,
        pointBackgroundColor: '#38bdf8',
        tension: 0.25,
        spanGaps: true
      };

      if (quotaChartInstance) {
        quotaChartInstance.data.labels = labels;
        quotaChartInstance.data.datasets = [dataset];
        quotaChartInstance.options.scales.x.grid.color = gridColor;
        quotaChartInstance.options.scales.x.ticks.color = tickColor;
        quotaChartInstance.options.scales.y.grid.color = gridColor;
        quotaChartInstance.options.scales.y.ticks.color = tickColor;
        quotaChartInstance.update();
      } else {
        quotaChartInstance = new Chart(el, {
          type: 'line',
          data: { labels, datasets: [dataset] },
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
                  label: (ctx) => {
                    const idx = ctx.dataIndex;
                    const meta = metadata[idx] || {};
                    const parts = [];
                    if (meta.h5 !== null && meta.h5 !== undefined) parts.push(`5 ชม.: ${meta.h5}%`);
                    if (meta.weekly !== null && meta.weekly !== undefined) parts.push(`รายสัปดาห์: ${meta.weekly}%`);
                    const detail = parts.length > 0 ? ` (${parts.join(', ')})` : '';
                    return `โควตาพร้อมใช้: ${ctx.parsed.y}%${detail}`;
                  }
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
