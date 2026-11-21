// =============================================================================
// Pocket Sentinel — Gatus Endpoints Health Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('endpointsComponent', () => ({
    endpoints: [],
    loading: true,
    submitting: false,
    status: '',
    message: '',
    gatusUrl: '/status/',
    form: {
      name: '',
      url: '',
      group: 'Websites',
      interval: '60s'
    },

    async init() {
      await this.loadEndpoints();
    },

    async loadEndpoints() {
      try {
        const resp = await fetch('/api/endpoints');
        if (resp.ok) {
          const data = await resp.json();
          this.endpoints = Array.isArray(data.endpoints) ? data.endpoints : [];
          if (data.gatus_url) this.gatusUrl = data.gatus_url;
        }
      } catch (err) {
        console.warn('[endpoints] Failed to load endpoints:', err);
      } finally {
        this.loading = false;
      }
    },

    async addEndpoint() {
      const name = (this.form.name || '').trim();
      const url = (this.form.url || '').trim();
      if (!name || !url) return;

      this.submitting = true;
      this.status = '';
      this.message = '';

      try {
        const resp = await fetch('/api/endpoints', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            name: name,
            url: url,
            group: this.form.group || 'Websites',
            interval: this.form.interval || '60s'
          })
        });
        const data = await resp.json();
        if (resp.ok) {
          this.status = 'ok';
          this.message = data.message || 'เพิ่มสำเร็จ';
          this.form.name = '';
          this.form.url = '';
          await this.loadEndpoints();
          setTimeout(() => { this.message = ''; }, 4000);
        } else {
          this.status = 'error';
          this.message = data.message || 'เกิดข้อผิดพลาดในการบันทึก';
        }
      } catch (err) {
        this.status = 'error';
        this.message = 'เกิดข้อผิดพลาดในการเชื่อมต่อเครือข่าย';
      } finally {
        this.submitting = false;
      }
    },

    async deleteEndpoint(name) {
      if (!name) return;
      if (!confirm(`ต้องการลบ "${name}" ออกจากการมอนิเตอร์ใช่หรือไม่?`)) return;

      this.submitting = true;
      try {
        const resp = await fetch(`/api/endpoints/${encodeURIComponent(name)}`, {
          method: 'DELETE'
        });
        const data = await resp.json();
        if (resp.ok) {
          await this.loadEndpoints();
        } else {
          alert(data.message || 'ไม่สามารถลบได้');
        }
      } catch (err) {
        alert('เกิดข้อผิดพลาดในการเชื่อมต่อ');
      } finally {
        this.submitting = false;
      }
    }
  }));
});
