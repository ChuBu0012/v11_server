// =============================================================================
// Pocket Sentinel — Recent Events & Logs Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('eventsComponent', () => ({
    logs: [],
    loading: true,

    init() {
      window.addEventListener('sentinel:logs-loaded', (e) => {
        this.logs = (Array.isArray(e.detail) ? e.detail : []).slice(0, 5);
        this.loading = false;
      });
    },

    formatRowTime(created) {
      return formatTime(created);
    },

    formatRowLevel(lvl) {
      return (lvl || 'info').toLowerCase();
    },

    formatRowMessage(entry) {
      return entry.metadata ? `${entry.message || ''} · ${entry.metadata}` : (entry.message || '');
    }
  }));
});
