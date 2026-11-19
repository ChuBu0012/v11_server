// =============================================================================
// Pocket Sentinel — Resource Trends & Charts Component
// =============================================================================

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
    hostRam: { canvasId: 'host-ram-chart', label: 'Host RAM Used', unit: ' MB', color: '#2997ff', maxValue: null },
    memory:  { canvasId: 'memory-chart',   label: 'Allocated',     unit: ' MB', color: '#0071e3', maxValue: null },
    disk:    { canvasId: 'disk-chart',     label: 'Available',     unit: ' GB', color: '#30d158', maxValue: null },
    battery: { canvasId: 'battery-chart',  label: 'Level',         unit: '%',   color: '#ffd60a', maxValue: 100 }
  };

  function createChart(chartKey) {
    const cfg = chartConfigs[chartKey];
    if (!cfg || !window.Chart) return null;

    const el = document.getElementById(cfg.canvasId);
    if (!el) return null;

    const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
    const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
    const tickColor = isDark ? '#8e8e93' : '#86868b';

    return new Chart(el, {
      type: 'line',
      data: {
        labels: [],
        datasets: [{
          label: cfg.label,
          data: [],
          borderColor: cfg.color,
          backgroundColor: `${cfg.color}18`,
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
      this.updateSingleChart('hostRam', this.samples, 'host_memory_used_mb', 'Host RAM', ' MB', 0);
      this.updateSingleChart('memory', this.samples, 'memory_alloc_mb', 'Go Memory', ' MB', 1);
      this.updateSingleChart('disk', this.samples, 'disk_free_gb', 'Disk', ' GB', 1);
      this.updateSingleChart('battery', this.samples, 'battery_level', 'Battery', '%', 0);
    },

    updateTheme(theme) {
      const isDark = theme === 'dark';
      const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
      const tickColor = isDark ? '#8e8e93' : '#86868b';

      Object.values(chartInstances).forEach((chart) => {
        if (!chart || !chart.options || !chart.options.scales) return;
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
