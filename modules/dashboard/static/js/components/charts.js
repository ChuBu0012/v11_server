// =============================================================================
// Pocket Sentinel — Resource Trends & Charts Component
// =============================================================================

let chartLibraryPromise;

function ensureChartLibrary() {
  if (window.Chart) return Promise.resolve();
  if (chartLibraryPromise) return chartLibraryPromise;

  chartLibraryPromise = new Promise((resolve, reject) => {
    const script = document.createElement('script');
    script.src = 'https://cdn.jsdelivr.net/npm/chart.js@4.6.1/dist/chart.umd.min.js';
    script.async = true;
    script.onload = resolve;
    script.onerror = () => reject(new Error('โหลดกราฟไม่สำเร็จ'));
    document.head.appendChild(script);
  });
  return chartLibraryPromise;
}

document.addEventListener('alpine:init', () => {
  // Keep Chart.js instances strictly outside of Alpine's reactive Proxy
  // to avoid TypeError on internal Chart.js proxy traps and ensure reliable rendering.
  const chartInstances = {
    hostRam: null,
    memory: null,
    disk: null,
    battery: null
  };

  const chartConfigs = {
    hostRam: { canvasId: 'host-ram-chart', insightId: 'host-ram-insight', label: 'Host RAM Used', unit: ' MB', maxValue: null, digits: 0 },
    memory:  { canvasId: 'memory-chart', insightId: 'memory-insight', label: 'Go Allocated Memory', unit: ' MB', maxValue: null, digits: 1 },
    disk:    { canvasId: 'disk-chart', insightId: 'disk-insight', label: 'Free Disk Space', unit: ' GB', maxValue: null, digits: 1 },
    battery: { canvasId: 'battery-chart', insightId: 'battery-insight', label: 'Battery Level', unit: '%', maxValue: 100, digits: 0 }
  };

  function mean(rows, key) {
    return rows.reduce((sum, sample) => sum + Number(sample[key]), 0) / rows.length;
  }

  function insightFor(samples, key, label, unit, hours, digits) {
    if (samples.length < 4) {
      return 'ข้อมูลย้อนหลังยังไม่พอสำหรับเปรียบเทียบแนวโน้ม ต้องมีอย่างน้อย 4 ตัวอย่าง';
    }

    const third = Math.max(1, Math.floor(samples.length / 3));
    const baseline = mean(samples.slice(0, third), key);
    const recent = mean(samples.slice(-third), key);
    const delta = recent - baseline;
    const change = baseline === 0 ? null : Math.abs(delta / baseline) * 100;
    const magnitude = change === null
      ? `${formatNumber(Math.abs(delta), digits)}${unit}`
      : `${formatNumber(change, 1)}%`;
    const period = hours === 1 ? '1 ชั่วโมง' : `${hours} ชั่วโมง`;
    const direction = delta > 0 ? 'เพิ่มขึ้น' : delta < 0 ? 'ลดลง' : 'คงที่';

    if (change !== null && change < 1) {
      return `${label}คงที่ในช่วง ${period} (ค่าเฉลี่ยเปลี่ยน ${magnitude})`;
    }
    if (Math.abs(delta) < 0.01) {
      return `${label}คงที่ในช่วง ${period} (ค่าเฉลี่ยเปลี่ยน ${magnitude})`;
    }

    const interpretation = key === 'disk_free_gb'
      ? (delta < 0 ? 'พื้นที่ว่างกำลังลดลง' : 'พื้นที่ว่างเพิ่มขึ้น')
      : key === 'battery_level'
        ? (delta < 0 ? 'ระดับแบตเตอรี่กำลังลดลง' : 'ระดับแบตเตอรี่เพิ่มขึ้น')
        : (delta > 0 ? 'ควรสังเกตการใช้ทรัพยากร' : 'การใช้ทรัพยากรลดลง');
    return `${label}${direction} ${magnitude} ในช่วง ${period} — ${interpretation}`;
  }

  function createChart(chartKey) {
    const cfg = chartConfigs[chartKey];
    if (!cfg || !window.Chart) return null;

    const el = document.getElementById(cfg.canvasId);
    if (!el) return null;

    const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
    const lineColor = isDark ? '#ffffff' : '#000000';
    const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
    const tickColor = isDark ? '#8e8e93' : '#86868b';

    return new Chart(el, {
      type: 'line',
      data: {
        labels: [],
        datasets: [{
          label: cfg.label,
          data: [],
          borderColor: lineColor,
          backgroundColor: isDark ? 'rgba(255,255,255,.08)' : 'rgba(0,0,0,.08)',
          borderWidth: 2,
          pointRadius: 0,
          pointHoverRadius: 4,
          tension: 0.25,
          fill: true,
          spanGaps: false
        }]
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        animation: false,
        interaction: { intersect: false, mode: 'index' },
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              label: (ctx) => {
                const y = ctx.parsed.y;
                if (cfg.unit === ' MB' && y >= 500) {
                  return `${cfg.label}: ${formatNumber(y / 1024, 2)} GB (${formatNumber(y, 0)} MB)`;
                }
                return `${cfg.label}: ${formatNumber(y, 1)}${cfg.unit}`;
              }
            }
          }
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: { color: tickColor, maxTicksLimit: 6, maxRotation: 0, autoSkip: true, font: { size: 10 } },
            border: { display: false }
          },
          y: {
            beginAtZero: true,
            ...(cfg.maxValue ? { max: cfg.maxValue } : {}),
            grid: { color: gridColor },
            ticks: { color: tickColor, maxTicksLimit: 5, font: { size: 10 }, callback: (v) => `${v}${cfg.unit}` },
            border: { display: false }
          }
        }
      }
    });
  }

  function getOrCreateChart(chartKey) {
    if (chartInstances[chartKey]) return chartInstances[chartKey];
    const instance = createChart(chartKey);
    if (instance) {
      chartInstances[chartKey] = instance;
    }
    return instance;
  }

  Alpine.data('chartsComponent', () => ({
    hours: 24,
    samples: [],
    chartUnavailable: false,
    currentValues: {
      hostRam: 'No sample yet',
      memory: 'No sample yet',
      disk: 'No sample yet',
      battery: 'No sample yet'
    },
    hasEnoughSamples: {
      hostRam: false,
      memory: false,
      disk: false,
      battery: false
    },

    init() {
      // 1. Listen for background telemetry history updates
      window.addEventListener('sentinel:history-loaded', (e) => {
        this.processHistory(e.detail);
      });

      // 2. Listen for theme change
      window.addEventListener('sentinel:theme-changed', (e) => {
        this.updateTheme(e.detail.theme);
      });

      window.addEventListener('sentinel:tab-changed', (e) => {
        if (e.detail?.tab !== 'overview') return;
        this.$nextTick(() => {
          Object.values(chartInstances).forEach((chart) => {
            if (!chart) return;
            try { chart.resize(); } catch (_) {}
          });
        });
      });

      // 3. Fallback direct load on init in case event was dispatched before component mounted
      this.loadHistory();

      const chartSection = document.querySelector('.chart-grid');
      if (!chartSection) return;
      const loadCharts = () => {
        ensureChartLibrary()
          .then(() => {
            this.chartUnavailable = false;
            this.processHistory(this.samples);
          })
          .catch((error) => {
            this.chartUnavailable = true;
            console.warn('Chart library unavailable:', error);
          });
      };
      if ('IntersectionObserver' in window) {
        const observer = new IntersectionObserver((entries) => {
          if (!entries.some((entry) => entry.isIntersecting)) return;
          observer.disconnect();
          loadCharts();
        }, { rootMargin: '300px 0px' });
        observer.observe(chartSection);
      } else {
        loadCharts();
      }
    },

    onToggle(e) {
      if (e && e.target && e.target.open) {
        this.$nextTick(() => {
          Object.values(chartInstances).forEach((c) => {
            if (c) {
              try { c.resize(); } catch (_) {}
            }
          });
        });
      }
    },

    async loadHistory() {
      try {
        const data = await getJSON(`/api/telemetry/history?hours=${this.hours}&limit=500`);
        this.processHistory(data);
      } catch (e) {
        console.warn('Failed to load telemetry history:', e);
      }
    },

    async setHours(h) {
      if (this.hours === h) return;
      this.hours = h;
      await this.loadHistory();
    },

    updateSingleChart(chartKey, samples, key, label, unit, digits = 1) {
      const usable = samples.filter((s) => s[key] !== null && s[key] !== undefined && Number.isFinite(Number(s[key])));
      const enough = usable.length >= 2;
      this.hasEnoughSamples[chartKey] = enough;
      const insightEl = document.getElementById(chartConfigs[chartKey]?.insightId);
      if (insightEl) {
        insightEl.textContent = insightFor(usable, key, label, unit, this.hours, digits);
      }

      if (!enough) {
        if (usable.length === 1) {
          const val = Number(usable[0][key]);
          const valStr = (key === 'host_memory_used_mb' && val >= 500)
            ? `${formatNumber(val / 1024, 2)} GB (${formatNumber(val, 0)} MB)`
            : `${formatNumber(val, digits)}${unit}`;
          this.currentValues[chartKey] = `${valStr} · 1 sample`;
        } else {
          this.currentValues[chartKey] = 'No sample in this period';
        }
      } else {
        const lastVal = Number(usable[usable.length - 1][key]);
        const valStr = (key === 'host_memory_used_mb' && lastVal >= 500)
          ? `${formatNumber(lastVal / 1024, 2)} GB (${formatNumber(lastVal, 0)} MB)`
          : `${formatNumber(lastVal, digits)}${unit}`;
        this.currentValues[chartKey] = `${valStr} · latest`;

        // Render chart data
        const chart = getOrCreateChart(chartKey);
        if (chart) {
          chart.data.labels = usable.map((s) => formatTime(s.timestamp));
          chart.data.datasets[0].data = usable.map((s) => Number(s[key]));
          chart.update('none');

          // Resize canvas after it unhides in the DOM
          if (this.$nextTick) {
            this.$nextTick(() => {
              try {
                chart.resize();
              } catch (_) {}
            });
          }
        }
      }
    },

    processHistory(historyData) {
      this.samples = Array.isArray(historyData) ? historyData.slice().reverse() : [];
      this.updateSingleChart('hostRam', this.samples, 'host_memory_used_mb', 'RAM เครื่อง ', ' MB', 0);
      this.updateSingleChart('memory', this.samples, 'memory_alloc_mb', 'RAM เซิร์ฟเวอร์ ', ' MB', 1);
      this.updateSingleChart('disk', this.samples, 'disk_free_gb', 'พื้นที่ว่าง ', ' GB', 1);
      this.updateSingleChart('battery', this.samples, 'battery_level', 'แบตเตอรี่ ', '%', 0);
    },

    updateTheme(theme) {
      const isDark = theme === 'dark';
      const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
      const tickColor = isDark ? '#8e8e93' : '#86868b';

      Object.values(chartInstances).forEach((chart) => {
        if (!chart || !chart.options || !chart.options.scales) return;
        const lineColor = isDark ? '#ffffff' : '#000000';
        chart.data.datasets.forEach((dataset) => {
          dataset.borderColor = lineColor;
          dataset.backgroundColor = isDark ? 'rgba(255,255,255,.08)' : 'rgba(0,0,0,.08)';
        });
        if (chart.options.scales.y) {
          chart.options.scales.y.grid.color = gridColor;
          chart.options.scales.y.ticks.color = tickColor;
        }
        if (chart.options.scales.x) {
          chart.options.scales.x.ticks.color = tickColor;
        }
        chart.update('none');
      });
    },

    get recentSamples() {
      return this.samples.slice(-12).reverse().map((s) => {
        const hostRamStr = (s.host_memory_used_mb != null && s.host_memory_total_mb != null)
          ? `${formatNumber(s.host_memory_used_mb / 1024, 2)} GB (${formatNumber((s.host_memory_used_mb / s.host_memory_total_mb) * 100, 0)}%)`
          : '—';
        return {
          time: new Date(s.timestamp).toLocaleString(),
          hostRam: hostRamStr,
          goRam: `${formatNumber(s.memory_alloc_mb, 2)} MB`,
          disk: `${formatNumber(s.disk_free_gb, 2)} GB`,
          battery: s.battery_level == null ? 'N/A' : `${formatNumber(s.battery_level, 0)}%`,
          uptime: formatUptime(s.uptime_seconds)
        };
      });
    }
  }));
});
