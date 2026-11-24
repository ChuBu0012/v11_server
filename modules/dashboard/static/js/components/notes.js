// =============================================================================
// Pocket Sentinel — Markdown Notes Alpine Component
// =============================================================================

function notesComponent() {
  return {
    notes: [],
    activeNote: null,
    searchQuery: '',
    selectedTag: '',
    viewMode: window.innerWidth > 768 ? 'split' : 'edit',
    isSaving: false,
    saveTimeout: null,

    // AI Modal State
    aiModalOpen: false,
    aiAction: 'summarize',
    aiInstruction: '',
    aiResult: '',
    aiLoading: false,

    async init() {
      await this.loadNotes();
      window.addEventListener('resize', () => {
        if (window.innerWidth <= 768 && this.viewMode === 'split') {
          this.viewMode = 'edit';
        }
      });

      // Global hook for interactive preview checkboxes
      window.toggleNoteCheckbox = (lineIdx, checked) => {
        this.handleCheckboxToggle(lineIdx, checked);
      };
    },

    async loadNotes() {
      try {
        let url = '/api/notes';
        const params = [];
        if (this.selectedTag) params.push('tag=' + encodeURIComponent(this.selectedTag));
        if (this.searchQuery) params.push('search=' + encodeURIComponent(this.searchQuery));
        if (params.length > 0) url += '?' + params.join('&');

        const res = await fetch(url, { headers: { 'Accept': 'application/json' } });
        if (res.ok) {
          this.notes = await res.json();
          if (this.notes.length > 0 && !this.activeNote) {
            this.selectNote(this.notes[0]);
          } else if (this.activeNote) {
            // refresh active note in list
            const updated = this.notes.find(n => n.id === this.activeNote.id);
            if (updated) this.activeNote = { ...updated };
          }
        }
      } catch (err) {
        console.warn('Failed to load notes:', err);
      }
    },

    get availableTags() {
      const set = new Set();
      for (const n of this.notes) {
        if (Array.isArray(n.tags)) {
          for (const t of n.tags) set.add(t);
        }
      }
      return Array.from(set);
    },

    selectTag(tag) {
      if (this.selectedTag === tag) {
        this.selectedTag = '';
      } else {
        this.selectedTag = tag;
      }
      this.loadNotes();
    },

    selectNote(note) {
      this.activeNote = { ...note };
      // Check localStorage for offline draft
      const draft = localStorage.getItem('sentinel_draft_note_' + note.id);
      if (draft && draft !== note.content) {
        this.activeNote.content = draft;
      }
    },

    async createNewNote() {
      try {
        const res = await fetch('/api/notes', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            title: 'บันทึกใหม่',
            content: '- [ ] รายการแรก',
            tags: this.selectedTag ? [this.selectedTag] : []
          })
        });
        if (res.ok) {
          const created = await res.json();
          this.notes.unshift(created);
          this.selectNote(created);
          if (window.innerWidth <= 768) {
            this.viewMode = 'edit';
          }
        }
      } catch (err) {
        console.error('Failed to create note:', err);
      }
    },

    async deleteActiveNote() {
      if (!this.activeNote || !confirm('ต้องการลบโน้ตนี้ใช่หรือไม่?')) return;
      try {
        const id = this.activeNote.id;
        const res = await fetch('/api/notes/' + id, { method: 'DELETE' });
        if (res.ok) {
          localStorage.removeItem('sentinel_draft_note_' + id);
          this.notes = this.notes.filter(n => n.id !== id);
          this.activeNote = this.notes.length > 0 ? { ...this.notes[0] } : null;
        }
      } catch (err) {
        console.error('Failed to delete note:', err);
      }
    },

    async togglePin() {
      if (!this.activeNote) return;
      const newPin = !this.activeNote.is_pinned;
      this.activeNote.is_pinned = newPin;
      await this.saveNoteImmediately({ is_pinned: newPin });
      await this.loadNotes();
    },

    async togglePublicSync() {
      if (!this.activeNote) return;
      const newSync = !this.activeNote.sync_public;
      this.activeNote.sync_public = newSync;
      await this.saveNoteImmediately({ sync_public: newSync });
    },

    onContentChange() {
      if (!this.activeNote) return;
      this.isSaving = true;
      localStorage.setItem('sentinel_draft_note_' + this.activeNote.id, this.activeNote.content);

      if (this.saveTimeout) clearTimeout(this.saveTimeout);
      this.saveTimeout = setTimeout(() => {
        this.saveNoteImmediately({
          title: this.activeNote.title,
          content: this.activeNote.content
        });
      }, 500);
    },

    async saveNoteImmediately(payload) {
      if (!this.activeNote) return;
      this.isSaving = true;
      try {
        const res = await fetch('/api/notes/' + this.activeNote.id, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        if (res.ok) {
          const updated = await res.json();
          this.activeNote.tags = updated.tags;
          this.activeNote.updated_at = updated.updated_at;
          const idx = this.notes.findIndex(n => n.id === updated.id);
          if (idx !== -1) {
            this.notes[idx] = { ...updated };
          }
          localStorage.removeItem('sentinel_draft_note_' + this.activeNote.id);
        }
      } catch (err) {
        console.warn('Auto-save failed, draft preserved locally:', err);
      } finally {
        this.isSaving = false;
      }
    },

    handleCheckboxToggle(itemIdx, isChecked) {
      if (!this.activeNote || !this.activeNote.content) return;
      const lines = this.activeNote.content.split('\n');
      let currentBox = 0;

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (line.match(/^(\s*[-*+]\s+\[)[ xX](\].*)$/)) {
          if (currentBox === itemIdx) {
            lines[i] = line.replace(/^(\s*[-*+]\s+\[)[ xX](\].*)$/, `$1${isChecked ? 'x' : ' '}$2`);
            break;
          }
          currentBox++;
        }
      }

      this.activeNote.content = lines.join('\n');
      this.onContentChange();
    },

    renderMarkdown(content) {
      if (!content) return '<p class="empty-hint"><i>ไม่มีเนื้อหา</i></p>';
      if (typeof marked === 'undefined') {
        return '<pre>' + escapeHtml(content) + '</pre>';
      }

      let parsed = marked.parse(content);

      // Transform checkboxes to interactive clickable inputs with data-idx
      let boxIdx = 0;
      parsed = parsed.replace(/<input(?:\s+[^>]*)?type="checkbox"([^>]*)>/gi, (match) => {
        const isChecked = match.includes('checked');
        const idx = boxIdx++;
        return `<input type="checkbox" class="interactive-checkbox" ${isChecked ? 'checked' : ''} onchange="window.toggleNoteCheckbox(${idx}, this.checked)">`;
      });

      return parsed;
    },

    // AI Assistant
    openAIModal() {
      this.aiResult = '';
      this.aiInstruction = '';
      this.aiLoading = false;
      this.aiModalOpen = true;
    },

    async runAIAction() {
      if (!this.activeNote) return;
      this.aiLoading = true;
      this.aiResult = '';
      try {
        const res = await fetch(`/api/notes/${this.activeNote.id}/ai`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            action: this.aiAction,
            instruction: this.aiInstruction
          })
        });
        if (res.ok) {
          const data = await res.json();
          this.aiResult = data.result;
        } else {
          const err = await res.json();
          this.aiResult = '⚠️ เกิดข้อผิดพลาด: ' + (err.error || 'AI ไม่ตอบสนอง');
        }
      } catch (err) {
        this.aiResult = '⚠️ ไม่สามารถเชื่อมต่อระบบ AI ได้: ' + err.message;
      } finally {
        this.aiLoading = false;
      }
    },

    applyAIResult() {
      if (!this.aiResult || !this.activeNote) return;
      if (this.aiAction === 'polish') {
        this.activeNote.content = this.aiResult;
      } else {
        this.activeNote.content += '\n\n' + this.aiResult;
      }
      this.onContentChange();
      this.aiModalOpen = false;
    }
  };
}
