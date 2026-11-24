// =============================================================================
// Pocket Sentinel — Workspace Orchestrator & Focus Mode
// =============================================================================

document.addEventListener('alpine:init', () => {
  // Update Alpine store to recognize workspace tabs (notes, sheets)
  const store = Alpine.store('sentinel');
  if (store) {
    const origSetTab = store.setTab.bind(store);
    store.focusMode = false;

    store.toggleFocus = function() {
      this.focusMode = !this.focusMode;
      document.body.classList.toggle('workspace-focus', this.focusMode);
      window.dispatchEvent(new CustomEvent('sentinel:focus-changed', { detail: { focus: this.focusMode } }));
    };

    store.setWorkspaceTab = function(tab) {
      if (this.activeTab === tab) return;
      this.activeTab = tab;
      const url = new URL(window.location.href);
      if (tab === 'overview') {
        url.searchParams.delete('tab');
      } else {
        url.searchParams.set('tab', tab);
      }
      window.history.pushState({ tab }, '', url);
      window.dispatchEvent(new CustomEvent('sentinel:tab-changed', { detail: { tab } }));
    };

    // Check initial query parameter
    const params = new URLSearchParams(window.location.search);
    const initialTab = params.get('tab');
    if (initialTab === 'notes' || initialTab === 'sheets') {
      store.activeTab = initialTab;
    }
  }
});
