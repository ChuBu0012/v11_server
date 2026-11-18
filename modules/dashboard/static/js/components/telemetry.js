// =============================================================================
// Pocket Sentinel — Telemetry Metrics Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('telemetryComponent', () => ({
    get sample() {
      return this.$store.sentinel.currentData?.latest_sample || null;
    },

    // 1. Host RAM (vivo V11 Physical Memory)
    get hostRam() {
      const s = this.sample;
      if (!s || s.host_memory_total_mb == null || s.host_memory_used_mb == null) {
        return { value: '—', sub: 'vivo 1804 · /proc/meminfo', meterWidth: 0, meterClass: '' };
      }
      const hostTotalMB = Number(s.host_memory_total_mb);
      const hostUsedMB = Number(s.host_memory_used_mb);
      const hostFreeMB = s.host_memory_free_mb != null ? Number(s.host_memory_free_mb) : Math.max(0, hostTotalMB - hostUsedMB);
      const hostPercent = hostTotalMB > 0 ? (hostUsedMB / hostTotalMB) * 100 : 0;
      const hostUsedGB = hostUsedMB / 1024;
      const hostTotalGB = hostTotalMB / 1024;
      const hostFreeGB = hostFreeMB / 1024;

      return {
        value: `${formatNumber(hostUsedGB, 2)} GB`,
        sub: `${formatNumber(hostTotalGB, 1)} GB total · ${formatNumber(hostFreeGB, 1)} GB free (${formatNumber(hostPercent, 0)}%)`,
        meterWidth: Math.max(0, Math.min(100, hostPercent)),
        meterClass: hostPercent >= 90 ? 'critical' : hostPercent >= 80 ? 'warning' : ''
      };
    },

    // 2. Go Runtime RAM
    get goRam() {
      const s = this.sample;
      if (!s) {
        return { value: '—', sub: 'of 250 MB restart limit', meterWidth: 0, meterClass: '' };
      }
      const memory = Number(s.memory_alloc_mb);
      const memorySys = Number(s.memory_sys_mb);
      const memoryPercent = (memory / 250) * 100;

      return {
        value: `${formatNumber(memory, 1)} MB`,
        sub: `Sys ${formatNumber(memorySys, 1)} MB · of 250 MB restart limit`,
        meterWidth: Math.max(0, Math.min(100, memoryPercent)),
        meterClass: memoryPercent >= 90 ? 'critical' : memoryPercent >= 75 ? 'warning' : ''
      };
    },

    // 3. Disk Storage
    get disk() {
      const s = this.sample;
      if (!s) {
        return { value: '—', sub: 'Internal storage · /data', meterWidth: 0, meterClass: '' };
      }
      const diskTotal = Number(s.disk_total_gb);
      const diskFree = Number(s.disk_free_gb);
      const diskUsedPercent = diskTotal > 0 ? ((diskTotal - diskFree) / diskTotal) * 100 : 0;

      return {
        value: `${formatNumber(diskFree, 1)} GB`,
        sub: `${formatNumber(diskTotal, 1)} GB total · ${formatNumber(diskUsedPercent, 0)}% used`,
        meterWidth: Math.max(0, Math.min(100, diskUsedPercent)),
        meterClass: diskUsedPercent >= 95 ? 'critical' : diskUsedPercent >= 85 ? 'warning' : ''
      };
    },

    // 4. Battery
    get battery() {
      const s = this.sample;
      if (!s || s.battery_level == null) {
        return { value: '—', sub: 'Checking Termux:API', meterWidth: 0, meterClass: '' };
      }
      const battery = Number(s.battery_level);
      return {
        value: `${formatNumber(battery, 0)}%`,
        sub: s.battery_status || 'Battery status available',
        meterWidth: Math.max(0, Math.min(100, battery)),
        meterClass: battery <= 10 ? 'critical' : battery <= 20 ? 'warning' : ''
      };
    },

    // 5. Server Uptime
    get uptime() {
      const s = this.sample;
      return {
        value: s ? formatUptime(s.uptime_seconds) : '—',
        sub: 'Pocket Sentinel process'
      };
    },

    // Threshold Conditions
    get conditions() {
      return this.$store.sentinel.conditions || [];
    }
  }));
});
