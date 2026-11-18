// =============================================================================
// Pocket Sentinel — Resource Trends & Charts Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('chartsComponent', () => ({
    hours: 24,
    samples: [],
    charts: {},
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
      this.initCharts();
      window.addEventListener('sentinel:history-loaded', (e) => {
        this.processHistory(e.detail);
      });
      window.addEventListener('sentinel:theme-changed', (e) => {
        this.updateTheme(e.detail.theme);
      });
    },

    async setHours(h) {
      if (this.hours === h) return;
      this.hours = h;
      try {
        const data = await getJSON(`/api/telemetry/history?hours=${this.hours}&limit=500`);
        this.processHistory(data);
      } catch (e) {
        console.warn('Failed to switch chart range:', e);
      }
    },

    makeChart(canvasId, label, unit, color, maxValue) {
      const el = document.getElementById(canvasId);
      if (!el || !window.Chart) return null;

      const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
      const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : '#f0f0f0';
      const tickColor = isDark ? '#8e8e93' : '#86868b';

      return new Chart(el, {
        type: 'line',
        data: {
          labels: [],
          datasets: [{
            label,
            data: [],
            borderColor: color,
            backgroundColor: `${color}18`,
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
                  if (unit === ' MB' && y >= 500) {
                    return `${label}: ${formatNumber(y / 1024, 2)} GB (${formatNumber(y, 0)} MB)`;
                  }
                  return `${label}: ${formatNumber(y, 1)}${unit}`;
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
              ...(maxValue ? { max: maxValue } : {}),
              grid: { color: gridColor },
              ticks: { color: tickColor, maxTicksLimit: 5, font: { size: 10 }, callback: (v) => `${v}${unit}` },
              border: { display: false }
            }
          }
        }
      });
    },

    initCharts() {
      this.charts.hostRam = this.makeChart('host-ram-chart', 'Host RAM Used', ' MB', '#2997ff');
      this.charts.memory = this.makeChart('memory-chart', 'Allocated', ' MB', '#0071e3');
      this.charts.disk = this.makeChart('disk-chart', 'Available', ' GB', '#30d158');
      this.charts.battery = this.makeChart('battery-chart', 'Level', '%', '#ffd60a', 100);
    },

    updateSingleChart(chartKey, samples, key, label, unit, digits = 1) {
      const chart = this.charts[chartKey];
      if (!chart) return;

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
        chart.data.labels = usable.map((s) => formatTime(s.timestamp));
        chart.data.datasets[0].data = usable.map((s) => Number(s[key]));
        chart.update('none');

        const lastVal = Number(usable[usable.length - 1][key]);
        const valStr = (key === 'host_memory_used_mb' && lastVal >= 500)
          ? `${formatNumber(lastVal / 1024, 2)} GB (${formatNumber(lastVal, 0)} MB)`
          : `${formatNumber(lastVal, digits)}${unit}`;
        this.currentValues[chartKey] = `${valStr} · latest`;
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

      Object.values(this.charts).forEach((chart) => {
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
