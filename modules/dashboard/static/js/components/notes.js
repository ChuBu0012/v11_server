// =============================================================================
// Pocket Sentinel — Minimalist Notes & Checklists (Keep-style)
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('notesComponent', () => ({
    notes: [],
    loading: true,
    saving: false,
    searchQuery: '',
    filterMode: 'active', // 'active' | 'pinned' | 'archived'
    composerOpen: false,
    toastMsg: '',
    editing: null,

    draft: {
      title: '',
      content: '',
      kind: 'note',
      items: [{ text: '', done: false }],
      tags: '',
      pinned: false,
    },

    init() {
      this.loadNotes();
    },

    get visibleNotes() {
      if (this.filterMode === 'pinned') {
        return this.notes.filter((n) => n.pinned);
      }
      return this.notes;
    },

    get pinnedNotes() {
      if (this.filterMode === 'archived') return [];
      return this.visibleNotes.filter((n) => n.pinned);
    },

    get regularNotes() {
      if (this.filterMode === 'pinned') return [];
      if (this.filterMode === 'archived') return this.visibleNotes;
      return this.visibleNotes.filter((n) => !n.pinned);
    },

    async loadNotes() {
      this.loading = true;
      try {
        const params = new URLSearchParams();
        if (this.searchQuery.trim()) params.set('q', this.searchQuery.trim());
        if (this.filterMode === 'archived') params.set('archived', '1');
        const data = await getJSON('/api/notes?' + params.toString());
        this.notes = Array.isArray(data.items) ? data.items : [];
      } catch (err) {
        this.showToast('โหลดโน้ตไม่สำเร็จ');
      } finally {
        this.loading = false;
      }
    },

    setFilter(mode) {
      this.filterMode = mode;
      this.loadNotes();
    },

    searchByTag(tag) {
      this.searchQuery = tag;
      this.loadNotes();
    },

    openComposer(kind) {
      this.draft = {
        title: '',
        content: '',
        kind: kind || 'note',
        items: [{ text: '', done: false }],
        tags: '',
        pinned: false,
      };
      this.composerOpen = true;
    },

    closeComposer() {
      this.composerOpen = false;
    },

    switchDraftKind(kind) {
      if (this.draft.kind === kind) return;
      if (kind === 'checklist') {
        const lines = (this.draft.content || '')
          .split('\n')
          .map((l) => l.trim())
          .filter(Boolean);
        this.draft.items = lines.length ? lines.map((text) => ({ text, done: false })) : [{ text: '', done: false }];
      } else {
        this.draft.content = (this.draft.items || [])
          .map((i) => i.text.trim())
          .filter(Boolean)
          .join('\n');
      }
      this.draft.kind = kind;
    },

    addDraftItem() {
      this.draft.items.push({ text: '', done: false });
    },

    removeDraftItem(idx) {
      this.draft.items.splice(idx, 1);
      if (this.draft.items.length === 0) {
        this.draft.items.push({ text: '', done: false });
      }
    },

    async createNote() {
      if (this.saving) return;
      const cleanItems = (this.draft.items || [])
        .map((i) => ({ text: (i.text || '').trim(), done: !!i.done }))
        .filter((i) => i.text !== '');

      if (!this.draft.title.trim() && !this.draft.content.trim() && cleanItems.length === 0) {
        this.closeComposer();
        return;
      }

      this.saving = true;
      try {
        const res = await fetch('/api/notes', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            title: this.draft.title.trim(),
            content: this.draft.kind === 'note' ? this.draft.content.trim() : '',
            kind: this.draft.kind,
            items: this.draft.kind === 'checklist' ? cleanItems : [],
            tags: this.draft.tags.trim(),
            pinned: !!this.draft.pinned,
          }),
        });
        if (!res.ok) throw new Error('create failed');
        this.closeComposer();
        await this.loadNotes();
        this.showToast('บันทึกโน้ตแล้ว');
      } catch (_) {
        this.showToast('บันทึกไม่สำเร็จ');
      } finally {
        this.saving = false;
      }
    },

    async toggleCheckItem(note, idx) {
      const items = (note.items || []).map((it, i) => (i === idx ? { ...it, done: !it.done } : it));
      note.items = items;
      try {
        await fetch(`/api/notes/${note.id}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ items }),
        });
      } catch (_) {
        this.loadNotes();
      }
    },

    async togglePin(note) {
      try {
        await fetch(`/api/notes/${note.id}/pin`, { method: 'POST' });
        await this.loadNotes();
      } catch (_) {}
    },

    async toggleArchive(note) {
      try {
        await fetch(`/api/notes/${note.id}/archive`, { method: 'POST' });
        await this.loadNotes();
        this.showToast(note.archived ? 'นำออกจากคลังแล้ว' : 'เก็บเข้าคลังแล้ว');
      } catch (_) {}
    },

    async autoTag(note) {
      this.showToast('✨ AI กำลังคิดแท็ก...');
      try {
        const res = await fetch(`/api/notes/${note.id}/auto-tag`, { method: 'POST' });
        if (!res.ok) throw new Error('auto-tag failed');
        await this.loadNotes();
        this.showToast('ติดแท็กให้เรียบร้อยแล้ว');
      } catch (_) {
        this.showToast('ติดแท็กไม่สำเร็จ');
      }
    },

    async sendToSchedule(note) {
      this.showToast('⏰ กำลังสร้างนัดเตือน...');
      try {
        const res = await fetch(`/api/notes/${note.id}/to-schedule`, { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'failed');
        this.showToast(`ตั้งเตือน "${data.title}" ใน /schedule แล้ว`);
      } catch (_) {
        this.showToast('สร้างนัดเตือนไม่สำเร็จ');
      }
    },

    async deleteNote(note) {
      if (!confirm('ลบโน้ตนี้ถาวรใช่ไหม?')) return;
      try {
        await fetch(`/api/notes/${note.id}`, { method: 'DELETE' });
        await this.loadNotes();
        this.showToast('ลบโน้ตแล้ว');
      } catch (_) {}
    },

    openEdit(note) {
      this.editing = {
        id: note.id,
        title: note.title || '',
        content: note.content || '',
        kind: note.kind || 'note',
        items: (note.items && note.items.length > 0)
          ? note.items.map((i) => ({ text: i.text, done: !!i.done }))
          : [{ text: '', done: false }],
        tags: note.tags || '',
      };
      const dlg = document.getElementById('note-edit-dialog');
      if (dlg && typeof dlg.showModal === 'function') dlg.showModal();
    },

    closeEdit() {
      const dlg = document.getElementById('note-edit-dialog');
      if (dlg && dlg.open) dlg.close();
      this.editing = null;
    },

    switchEditKind(kind) {
      if (!this.editing || this.editing.kind === kind) return;
      if (kind === 'checklist') {
        const lines = (this.editing.content || '').split('\n').map((l) => l.trim()).filter(Boolean);
        this.editing.items = lines.length ? lines.map((text) => ({ text, done: false })) : [{ text: '', done: false }];
      } else {
        this.editing.content = (this.editing.items || []).map((i) => i.text.trim()).filter(Boolean).join('\n');
      }
      this.editing.kind = kind;
    },

    addEditItem() {
      if (this.editing) this.editing.items.push({ text: '', done: false });
    },

    removeEditItem(idx) {
      if (!this.editing) return;
      this.editing.items.splice(idx, 1);
      if (this.editing.items.length === 0) this.editing.items.push({ text: '', done: false });
    },

    async saveEdit() {
      if (!this.editing || this.saving) return;
      this.saving = true;
      const cleanItems = (this.editing.items || [])
        .map((i) => ({ text: (i.text || '').trim(), done: !!i.done }))
        .filter((i) => i.text !== '');

      try {
        const res = await fetch(`/api/notes/${this.editing.id}`, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            title: this.editing.title.trim(),
            content: this.editing.kind === 'note' ? this.editing.content.trim() : '',
            kind: this.editing.kind,
            items: this.editing.kind === 'checklist' ? cleanItems : [],
            tags: this.editing.tags.trim(),
          }),
        });
        if (!res.ok) throw new Error('save failed');
        this.closeEdit();
        await this.loadNotes();
        this.showToast('อัปเดตโน้ตแล้ว');
      } catch (_) {
        this.showToast('บันทึกไม่สำเร็จ');
      } finally {
        this.saving = false;
      }
    },

    splitTags(raw) {
      return String(raw || '')
        .split(',')
        .map((s) => s.trim())
        .filter(Boolean);
    },

    formatDate(ts) {
      if (!ts) return '';
      const d = new Date(ts);
      if (Number.isNaN(d.getTime())) return '';
      return d.toLocaleDateString('th-TH', { day: 'numeric', month: 'short' });
    },

    showToast(msg) {
      this.toastMsg = msg;
      setTimeout(() => {
        if (this.toastMsg === msg) this.toastMsg = '';
      }, 2500);
    },
  }));
});
