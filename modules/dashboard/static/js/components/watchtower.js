// =============================================================================
// Pocket Sentinel — Watchtower Monitoring Component & Summary
// =============================================================================

// Architecture Guardrail (frontend-proxy-guard):
// Keep Chart.js instances strictly outside of Alpine's reactive Proxy
// to prevent Uncaught TypeError on non-configurable properties.
const watchtowerCharts = {};

function getOrCreateWatchtowerChart(key, canvasId, config) {
  if (watchtowerCharts[key]) {
    try {
      watchtowerCharts[key].destroy();
    } catch (_) {}
    delete watchtowerCharts[key];
  }
  const el = document.getElementById(canvasId);
  if (!el || !window.Chart) return null;
  watchtowerCharts[key] = new window.Chart(el, config);
  return watchtowerCharts[key];
}

document.addEventListener('alpine:init', () => {

  // 1. Main Watchtower Management Component
  Alpine.data('watchtowerComponent', () => ({
    // Auth state
    pin: localStorage.getItem('sentinel-watchtower-pin') || localStorage.getItem('sentinel-relay-pin') || '',
    isUnlocked: Boolean(localStorage.getItem('sentinel-watchtower-pin') || localStorage.getItem('sentinel-relay-pin')),
    tempPin: '',
    showPinModal: false,

    // Theme state
    theme: localStorage.getItem('sentinel-theme') || 'dark',
    toggleTheme() {
      this.theme = this.theme === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', this.theme);
      localStorage.setItem('sentinel-theme', this.theme);
    },

    // Core data (primitive state & JSON items)
    summary: { total: 0, up: 0, down: 0, paused: 0, status: 'unknown' },
    watchers: [],
    selectedWatcherId: null,
    checksHistory: [],
    loadingWatchers: false,
    loadingChecks: false,
    isDiagnosing: false,
    diagnosingId: null,

    // Modal & Form state
    showFormModal: false,
    isEditing: false,
    formId: null,
    formName: '',
    formUrl: '',
    formExpectedStatus: 200,

    // Notifications
    noticeText: '',
    noticeType: '', // 'success' | 'error' | 'info'

    init() {
      // Prompt PIN if not saved
      if (!this.pin) {
        this.showPinModal = true;
      }
      this.refreshAll();

      // Refresh watchers every 30 seconds
      setInterval(() => {
        this.loadSummary();
        this.loadWatchers(false);
      }, 30000);
    },

    async refreshAll() {
      await Promise.all([this.loadSummary(), this.loadWatchers(true)]);
    },

    showNotice(text, type = 'info') {
      this.noticeText = text;
      this.noticeType = type;
      if (type === 'success') {
        setTimeout(() => {
          if (this.noticeText === text) {
            this.noticeText = '';
            this.noticeType = '';
          }
        }, 4000);
      }
    },

    savePin() {
      const trimmed = this.tempPin.trim();
      if (!trimmed) return;
      this.pin = trimmed;
      this.isUnlocked = true;
      localStorage.setItem('sentinel-watchtower-pin', trimmed);
      this.showPinModal = false;
      this.tempPin = '';
      this.showNotice('🔓 บันทึก PIN สำหรับ Watchtower สำเร็จแล้ว', 'success');
    },

    clearPin() {
      this.pin = '';
      this.isUnlocked = false;
      localStorage.removeItem('sentinel-watchtower-pin');
      this.showPinModal = true;
    },

    async loadSummary() {
      try {
        const resp = await fetch('/api/watchtower/summary', { cache: 'no-store' });
        if (resp.ok) {
          const data = await resp.json();
          this.summary = {
            total: data.total || 0,
            up: data.up || 0,
            down: data.down || 0,
            paused: data.paused || 0,
            status: data.status || 'unknown'
          };
        }
      } catch (err) {
        console.warn('Failed to load watchtower summary:', err);
      }
    },

    async loadWatchers(showSpinner = true) {
      if (showSpinner) this.loadingWatchers = true;
      try {
        const resp = await fetch('/api/watchtower/watchers', { credentials: 'same-origin', cache: 'no-store' });
        if (resp.ok) {
          const data = await resp.json();
          this.watchers = Array.isArray(data.items) ? data.items : [];
          // If a watcher was selected, re-load checks
          if (this.selectedWatcherId) {
            const stillExists = this.watchers.find(w => w.id === this.selectedWatcherId);
            if (stillExists) {
              this.loadChecks(this.selectedWatcherId);
            } else {
              this.selectedWatcherId = null;
              this.checksHistory = [];
            }
          }
        } else {
          this.showNotice(`โหลดรายการ Watchers ไม่สำเร็จ (HTTP ${resp.status})`, 'error');
        }
      } catch (err) {
        this.showNotice(`ไม่สามารถเชื่อมต่อเซิร์ฟเวอร์ Watchtower ได้: ${err.message}`, 'error');
      } finally {
        if (showSpinner) this.loadingWatchers = false;
      }
    },

    async toggleChecks(watcherId) {
      if (this.selectedWatcherId === watcherId) {
        this.selectedWatcherId = null;
        this.checksHistory = [];
        if (watchtowerCharts[watcherId]) {
          try { watchtowerCharts[watcherId].destroy(); } catch (_) {}
          delete watchtowerCharts[watcherId];
        }
        return;
      }
      this.selectedWatcherId = watcherId;
      await this.loadChecks(watcherId);
    },

    async loadChecks(watcherId) {
      this.loadingChecks = true;
      try {
        const resp = await fetch(`/api/watchtower/watchers/${watcherId}/checks`, { cache: 'no-store' });
        if (resp.ok) {
          const data = await resp.json();
          this.checksHistory = Array.isArray(data.items) ? data.items : [];
          this.$nextTick(() => {
            this.renderLatencyChart(watcherId, this.checksHistory);
          });
        }
      } catch (err) {
        console.warn('Failed to load checks history:', err);
      } finally {
        this.loadingChecks = false;
      }
    },

    renderLatencyChart(watcherId, checks) {
      const canvasId = `watcher-chart-${watcherId}`;
      const el = document.getElementById(canvasId);
      if (!el || !checks || checks.length === 0) return;

      // Sort checks chronologically for charting
      const sorted = [...checks].reverse();
      const labels = sorted.map(c => {
        const d = new Date(c.checked_at);
        return d.toLocaleTimeString('th-TH', { timeZone: 'Asia/Bangkok', hour: '2-digit', minute: '2-digit' });
      });
      const dataPoints = sorted.map(c => c.latency_ms);
      const pointColors = sorted.map(c => c.is_up ? 'rgba(48, 209, 88, 0.8)' : 'rgba(255, 69, 58, 0.9)');

      const isDark = document.documentElement.getAttribute('data-theme') !== 'light';
      const gridColor = isDark ? 'rgba(255, 255, 255, 0.08)' : 'rgba(0, 0, 0, 0.06)';
      const textColor = isDark ? '#8e8e93' : '#6e6e73';

      const config = {
        type: 'line',
        data: {
          labels,
          datasets: [{
            label: 'Latency (ms)',
            data: dataPoints,
            borderColor: '#2997ff',
            backgroundColor: 'rgba(41, 151, 255, 0.1)',
            pointBackgroundColor: pointColors,
            pointBorderColor: pointColors,
            pointRadius: 3,
            tension: 0.3,
            fill: true
          }]
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: {
            legend: { display: false },
            tooltip: {
              callbacks: {
                label: (ctx) => ` Latency: ${ctx.parsed.y} ms`
              }
            }
          },
          scales: {
            x: {
              grid: { color: gridColor },
              ticks: { color: textColor, font: { size: 10 } }
            },
            y: {
              beginAtZero: true,
              grid: { color: gridColor },
              ticks: { color: textColor, font: { size: 10 } }
            }
          }
        }
      };

      const chart = getOrCreateWatchtowerChart(watcherId, canvasId, config);
      if (chart && this.$nextTick) {
        this.$nextTick(() => {
          try { chart.resize(); } catch (_) {}
        });
      }
    },

    async checkNow(id) {
      if (this.isDiagnosing) return;
      if (!this.pin) {
        this.showPinModal = true;
        this.showNotice('กรุณากรอก PIN ก่อนใช้งานคำสั่ง Check now', 'error');
        return;
      }

      this.isDiagnosing = true;
      this.diagnosingId = id;
      this.showNotice('🔍 กำลังทดสอบตรวจสอบ URL จาก vivo V11 (Diagnostic Check)...', 'info');

      try {
        const resp = await fetch(`/api/watchtower/watchers/${id}/check`, {
          method: 'POST',
          headers: {
            'X-Watchtower-PIN': this.pin
          }
        });

        if (resp.status === 401) {
          this.showNotice('❌ PIN ไม่ถูกต้อง', 'error');
          this.showPinModal = true;
          return;
        }

        if (!resp.ok) {
          const errData = await resp.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${resp.status}`);
        }

        const res = await resp.json();
        const statusMsg = res.is_up ? 'UP (ปกติ)' : 'DOWN (ขัดข้อง)';
        this.showNotice(`✅ ตรวจสอบทันทีสำเร็จ: สถานะ ${statusMsg} | รหัส ${res.status_code} | เวลา ${res.latency_ms} ms (ไม่กระทบ failure streak)`, 'success');
        
        await Promise.all([this.loadSummary(), this.loadWatchers(false)]);
        if (this.selectedWatcherId === id) {
          await this.loadChecks(id);
        }
      } catch (err) {
        this.showNotice(`❌ การตรวจสอบล้มเหลว: ${err.message}`, 'error');
      } finally {
        this.isDiagnosing = false;
        this.diagnosingId = null;
      }
    },

    async togglePause(watcher) {
      if (!this.pin) {
        this.showPinModal = true;
        this.showNotice('กรุณากรอก PIN ก่อนเปลี่ยนสถานะ Watcher', 'error');
        return;
      }

      const isPaused = watcher.state === 'paused';
      const action = isPaused ? 'resume' : 'pause';
      const actionLabel = isPaused ? 'เปิดทำงานต่อ' : 'พักการตรวจ';

      try {
        const resp = await fetch(`/api/watchtower/watchers/${watcher.id}/${action}`, {
          method: 'POST',
          headers: { 'X-Watchtower-PIN': this.pin }
        });

        if (resp.status === 401) {
          this.showNotice('❌ PIN ไม่ถูกต้อง', 'error');
          this.showPinModal = true;
          return;
        }

        if (!resp.ok) {
          const errData = await resp.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${resp.status}`);
        }

        this.showNotice(`✅ ${actionLabel}สำหรับ ${watcher.name} แล้ว`, 'success');
        await Promise.all([this.loadSummary(), this.loadWatchers(false)]);
      } catch (err) {
        this.showNotice(`❌ ไม่สามารถ${actionLabel}ได้: ${err.message}`, 'error');
      }
    },

    openAddModal() {
      this.isEditing = false;
      this.formId = null;
      this.formName = '';
      this.formUrl = 'https://';
      this.formExpectedStatus = 200;
      this.showFormModal = true;
      this.$nextTick(() => {
        const el = document.getElementById('watcher-form-name');
        if (el) el.focus();
      });
    },

    openEditModal(watcher) {
      this.isEditing = true;
      this.formId = watcher.id;
      this.formName = watcher.name;
      this.formUrl = watcher.url;
      this.formExpectedStatus = watcher.expected_status || 200;
      this.showFormModal = true;
      this.$nextTick(() => {
        const el = document.getElementById('watcher-form-name');
        if (el) el.focus();
      });
    },

    closeFormModal() {
      this.showFormModal = false;
      this.formId = null;
      this.formName = '';
      this.formUrl = '';
      this.formExpectedStatus = 200;
    },

    async saveWatcher() {
      if (!this.pin) {
        this.showPinModal = true;
        this.showNotice('กรุณากรอก PIN ก่อนบันทึก Watcher', 'error');
        return;
      }

      const name = this.formName.trim();
      const url = this.formUrl.trim();
      const expectedStatus = parseInt(this.formExpectedStatus, 10) || 200;

      if (!name || !url) {
        this.showNotice('กรุณาระบุชื่อและ URL ให้ครบถ้วน', 'error');
        return;
      }

      const payload = {
        name,
        url,
        expected_status: expectedStatus
      };

      const urlPath = this.isEditing ? `/api/watchtower/watchers/${this.formId}` : '/api/watchtower/watchers';
      const method = this.isEditing ? 'PATCH' : 'POST';

      try {
        const resp = await fetch(urlPath, {
          method,
          headers: {
            'Content-Type': 'application/json',
            'X-Watchtower-PIN': this.pin
          },
          body: JSON.stringify(payload)
        });

        if (resp.status === 401) {
          this.showNotice('❌ PIN ไม่ถูกต้อง', 'error');
          this.showPinModal = true;
          return;
        }

        if (!resp.ok) {
          const errData = await resp.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${resp.status}`);
        }

        this.showNotice(this.isEditing ? '✅ แก้ไข Watcher สำเร็จแล้ว (รีเซ็ตสถานะเป็น unknown baseline)' : '✅ เพิ่ม Watcher ใหม่สำเร็จแล้ว', 'success');
        this.closeFormModal();
        await Promise.all([this.loadSummary(), this.loadWatchers(false)]);
      } catch (err) {
        this.showNotice(`❌ บันทึกไม่สำเร็จ: ${err.message}`, 'error');
      }
    },

    async deleteWatcher(watcher) {
      if (!this.pin) {
        this.showPinModal = true;
        this.showNotice('กรุณากรอก PIN ก่อนลบ Watcher', 'error');
        return;
      }

      if (!confirm(`คุณแน่ใจหรือไม่ว่าต้องการลบ Watcher "${watcher.name}"?\nประวัติการตรวจสอบย้อนหลังจะถูกลบทั้งหมด`)) {
        return;
      }

      try {
        const resp = await fetch(`/api/watchtower/watchers/${watcher.id}`, {
          method: 'DELETE',
          headers: { 'X-Watchtower-PIN': this.pin }
        });

        if (resp.status === 401) {
          this.showNotice('❌ PIN ไม่ถูกต้อง', 'error');
          this.showPinModal = true;
          return;
        }

        if (!resp.ok && resp.status !== 204) {
          const errData = await resp.json().catch(() => ({}));
          throw new Error(errData.error || `HTTP ${resp.status}`);
        }

        this.showNotice(`🗑️ ลบ Watcher "${watcher.name}" เรียบร้อยแล้ว`, 'success');
        if (this.selectedWatcherId === watcher.id) {
          this.selectedWatcherId = null;
          this.checksHistory = [];
        }
        await Promise.all([this.loadSummary(), this.loadWatchers(false)]);
      } catch (err) {
        this.showNotice(`❌ ลบไม่สำเร็จ: ${err.message}`, 'error');
      }
    },

    formatTime(ts) {
      if (!ts) return 'ยังไม่เคยตรวจ';
      const d = new Date(ts);
      if (isNaN(d.getTime())) return ts;
      return d.toLocaleString('th-TH', { timeZone: 'Asia/Bangkok', month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    },

    getStateBadge(state) {
      switch (state) {
        case 'up':
          return { label: 'OPERATIONAL', class: 'badge-up', icon: 'ri-checkbox-circle-fill' };
        case 'down':
          return { label: 'OUTAGE', class: 'badge-down', icon: 'ri-close-circle-fill' };
        case 'paused':
          return { label: 'PAUSED', class: 'badge-paused', icon: 'ri-pause-circle-line' };
        default:
          return { label: 'UNKNOWN', class: 'badge-unknown', icon: 'ri-question-line' };
      }
    }
  }));

  // 2. Dashboard Watchtower Summary Component (Used in index.html)
  Alpine.data('watchtowerSummaryComponent', () => ({
    summary: { total: 0, up: 0, down: 0, paused: 0, status: 'unknown' },
    watchers: [],
    loading: true,
    failed: false,

    get hasOutage() {
      return this.summary.down > 0;
    },

    init() {
      this.load();
      // Periodically refresh summary on dashboard
      setInterval(() => {
        this.load();
      }, 30000);
    },

    async load() {
      try {
        const [sumResp, watchResp] = await Promise.all([
          fetch('/api/watchtower/summary', { cache: 'no-store' }),
          fetch('/api/watchtower/watchers', { cache: 'no-store' })
        ]);

        if (sumResp.ok) {
          const sumData = await sumResp.json();
          this.summary = {
            total: sumData.total || 0,
            up: sumData.up || 0,
            down: sumData.down || 0,
            paused: sumData.paused || 0,
            status: sumData.status || 'unknown'
          };
        }

        if (watchResp.ok) {
          const watchData = await watchResp.json();
          this.watchers = Array.isArray(watchData.items) ? watchData.items.slice(0, 5) : [];
        }

        this.failed = false;
      } catch (err) {
        console.warn('Watchtower summary load error:', err);
        this.failed = true;
      } finally {
        this.loading = false;
      }
    },

    formatLatency(ms) {
      if (ms === null || ms === undefined || ms === 0) return '—';
      return `${ms} ms`;
    }
  }));

});
