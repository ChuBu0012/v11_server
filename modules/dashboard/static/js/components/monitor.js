// =============================================================================
// Pocket Sentinel — Monitor and Insight Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('monitorComponent', () => ({
    loading: true,
    analyzing: false,
    error: '',
    insightError: '',
    insight: null,
    current: null,
    history: [],
    watchtower: { total: 0, up: 0, down: 0, paused: 0 },

    get hasRelayPin() {
      return Boolean(localStorage.getItem('sentinel-relay-pin'));
    },

    init() {
      this.load();
      window.addEventListener('sentinel:current-loaded', () => this.load());
    },

    get sample() {
      return this.current?.latest_sample || null;
    },

    get conditions() {
      return (this.current?.conditions || []).filter((condition) => condition.active);
    },

    get statusLabel() {
      if (!this.sample) return 'รอข้อมูล telemetry';
      const collectedAt = this.current?.last_collected ? new Date(this.current.last_collected).getTime() : 0;
      const intervalMs = Math.max(5, Number(this.current?.sample_interval_seconds) || 60) * 1000;
      if (this.current?.last_error || !collectedAt || Date.now() - collectedAt > intervalMs * 2) return 'ต้องตรวจสอบการเก็บข้อมูล';
      if (this.conditions.length) return 'พบเงื่อนไขที่ต้องติดตาม';
      return 'ระบบทำงานปกติ';
    },

    get statusClass() {
      if (!this.sample || this.current?.last_error || !this.current?.last_collected) return 'warning';
      if (Date.now() - new Date(this.current.last_collected).getTime() > Math.max(5, Number(this.current.sample_interval_seconds) || 60) * 2000) return 'warning';
      return this.conditions.length ? 'warning' : 'healthy';
    },

    get sampledAt() {
      return this.sample?.timestamp ? `เก็บข้อมูลล่าสุด ${formatTime(this.sample.timestamp, { hour: '2-digit', minute: '2-digit', second: '2-digit' })}` : 'ยังไม่มีตัวอย่างข้อมูล';
    },

    get metrics() {
      const s = this.sample;
      if (!s) return { hostRam: '—', hostRamSub: 'รอข้อมูล', goRam: '—', uptime: '—', disk: '—', diskSub: 'รอข้อมูล', battery: '—', batteryStatus: 'รอข้อมูล' };
      const used = Number(s.host_memory_used_mb || 0);
      const total = Number(s.host_memory_total_mb || 0);
      const diskTotal = Number(s.disk_total_gb || 0);
      const diskFree = Number(s.disk_free_gb || 0);
      return {
        hostRam: total ? `${formatNumber(used / 1024, 2)} GB` : '—',
        hostRamSub: total ? `${formatNumber(used / total * 100, 0)}% ใช้ · ${formatNumber(total / 1024, 1)} GB รวม` : 'ไม่พบข้อมูล RAM อุปกรณ์',
        goRam: `${formatNumber(s.memory_alloc_mb, 1)} MB`,
        uptime: `ทำงานมา ${formatUptime(s.uptime_seconds)}`,
        disk: `${formatNumber(diskFree, 1)} GB`,
        diskSub: `${formatNumber(diskTotal ? (diskTotal - diskFree) / diskTotal * 100 : 0, 0)}% ใช้ · ${formatNumber(diskTotal, 1)} GB รวม`,
        battery: s.battery_level == null ? '—' : `${formatNumber(s.battery_level, 0)}%`,
        batteryStatus: s.battery_status || 'สถานะแบตเตอรี่'
      };
    },

    get trend() {
      if (this.history.length < 2) return [];
      const newest = this.history[0];
      const oldest = this.history[this.history.length - 1];
      const deltaRow = (label, key, unit, lowerIsBetter) => {
        const start = Number(oldest[key]);
        const end = Number(newest[key]);
        if (key === 'battery_level' && (oldest[key] == null || newest[key] == null)) return null;
        if (!Number.isFinite(start) || !Number.isFinite(end)) return null;
        const delta = end - start;
        const tolerance = key === 'disk_free_gb' ? 0.1 : 1;
        const tone = Math.abs(delta) < tolerance ? 'neutral' : ((delta > 0) === lowerIsBetter ? 'warning' : 'positive');
        const prefix = delta > 0 ? '+' : '';
        return { label, text: `${formatNumber(end, 1)} ${unit}`, delta: `${prefix}${formatNumber(delta, 1)} ${unit} จาก ${this.history.length} จุด`, tone };
      };
      return [
        deltaRow('Go RAM', 'memory_alloc_mb', 'MB', true),
        deltaRow('พื้นที่ว่าง', 'disk_free_gb', 'GB', false),
        deltaRow('แบตเตอรี่', 'battery_level', '%', false)
      ].filter(Boolean);
    },

    async load() {
      this.loading = true;
      this.error = '';
      try {
        const [current, history, watchtower] = await Promise.all([
          getJSON('/api/telemetry/current'),
          getJSON('/api/telemetry/history?hours=24&limit=24'),
          getJSON('/api/watchtower/summary').catch(() => ({ total: 0, up: 0, down: 0, paused: 0 }))
        ]);
        this.current = current;
        this.history = Array.isArray(history) ? history : [];
        this.watchtower = { total: Number(watchtower.total || 0), up: Number(watchtower.up || 0), down: Number(watchtower.down || 0), paused: Number(watchtower.paused || 0) };
      } catch (err) {
        this.error = err.message || 'โหลดข้อมูล Monitor ไม่สำเร็จ';
      } finally {
        this.loading = false;
      }
    },

    async analyze() {
      if (this.analyzing) return;
      const relayPin = localStorage.getItem('sentinel-relay-pin') || '';
      if (!relayPin) {
        this.insight = null;
        this.insightError = 'ตั้งค่า Relay PIN ก่อน เพื่อยืนยันสิทธิ์ใช้งาน AI insight';
        return;
      }
      const s = this.sample;
      this.analyzing = true;
      this.insightError = '';
      this.insight = null;
      try {
        const response = await fetch('/api/monitor/insight', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-Relay-PIN': relayPin },
          body: JSON.stringify({
            status: this.statusLabel,
            host_ram_used_mb: Number(s?.host_memory_used_mb || 0),
            host_ram_total_mb: Number(s?.host_memory_total_mb || 0),
            go_ram_mb: Number(s?.memory_alloc_mb || 0),
            disk_free_gb: Number(s?.disk_free_gb || 0),
            disk_total_gb: Number(s?.disk_total_gb || 0),
            battery_pct: s?.battery_level == null ? 0 : Number(s.battery_level),
            battery_available: s?.battery_level != null,
            uptime_seconds: Number(s?.uptime_seconds || 0),
            conditions: this.conditions.slice(0, 10).map((condition) => String(condition.type || '').slice(0, 40)),
            trend: this.history.slice(0, 24).map((point) => ({ go_ram_mb: Number(point.memory_alloc_mb || 0), disk_free_gb: Number(point.disk_free_gb || 0), battery_pct: point.battery_level == null ? 0 : Number(point.battery_level), battery_available: point.battery_level != null })),
            watchtower: this.watchtower
          })
        });
        const data = await response.json().catch(() => ({}));
        if (!response.ok) throw new Error(data.error || `HTTP ${response.status}`);
        this.insight = data;
      } catch (err) {
        this.insightError = err.message || 'AI insight ไม่สำเร็จ';
      } finally {
        this.analyzing = false;
      }
    }
  }));
});
