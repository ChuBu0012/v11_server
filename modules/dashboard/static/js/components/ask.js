// =============================================================================
// Pocket Sentinel — AI Ask & Fast LLM Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('askComponent', () => ({
    prompt: '',
    model: 'auto',
    loading: false,
    response: null,
    provider: '',
    cached: false,
    durationMs: 0,
    error: '',
    copied: false,

    async submit() {
      const q = (this.prompt || '').trim();
      if (!q || this.loading) return;

      this.loading = true;
      this.error = '';
      this.copied = false;

      try {
        const res = await fetch('/api/ask', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'Accept': 'application/json'
          },
          body: JSON.stringify({
            prompt: q,
            model: this.model
          })
        });

        const data = await res.json();
        if (!res.ok) {
          throw new Error(data.error || `HTTP ${res.status}`);
        }

        this.response = data.response;
        this.provider = data.provider || 'AI';
        this.cached = !!data.cached;
        this.durationMs = data.duration_ms || 0;
      } catch (err) {
        this.error = err.message || 'เกิดข้อผิดพลาดในการประมวลผล';
      } finally {
        this.loading = false;
      }
    },

    setPrompt(text) {
      this.prompt = text;
      this.submit();
    },

    clear() {
      this.prompt = '';
      this.response = null;
      this.error = '';
      this.copied = false;
    },

    async copyResponse() {
      if (!this.response) return;
      try {
        await navigator.clipboard.writeText(this.response);
        this.copied = true;
        setTimeout(() => { this.copied = false; }, 2000);
      } catch (_) {}
    },

    get formattedResponse() {
      if (!this.response) return '';
      if (window.marked && typeof window.marked.parse === 'function') {
        try {
          const raw = window.marked.parse(this.response, {
            gfm: true,
            breaks: true,
          });
          return typeof sanitizeHtml === 'function' ? sanitizeHtml(raw) : escapeHtml(this.response).replace(/\n/g, '<br>');
        } catch (e) {
          console.warn('Marked parse error:', e);
        }
      }
      return escapeHtml(this.response).replace(/\n/g, '<br>');
    }
  }));
});
