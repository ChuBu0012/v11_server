// =============================================================================
// Pocket Sentinel — Telegram Relay & App Hub Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('relayComponent', () => ({
    isOpen: false,
    activeTab: 'text', // 'text' | 'file'
    pin: localStorage.getItem('sentinel-relay-pin') || '',
    isUnlocked: Boolean(localStorage.getItem('sentinel-relay-pin')),
    tempPin: '',
    showPinModal: false,

    // Form fields
    messageText: '',
    captionText: '',
    selectedFile: null,
    filePreviewUrl: null,
    fileName: '',
    fileSizeFormatted: '',
    isImageFile: false,

    // UI state
    isDragging: false,
    isSending: false,
    statusText: '',
    statusType: '', // 'success' | 'error' | ''

    init() {
      // 1. Auto-open if query parameter has ?app=relay
      const params = new URLSearchParams(window.location.search);
      if (params.get('app') === 'relay') {
        this.open();
      }

      // 2. Global paste listener for instant clipboard screenshot capture
      window.addEventListener('paste', (e) => {
        if (!this.isOpen) return;
        const items = e.clipboardData?.items;
        if (!items) return;

        for (let i = 0; i < items.length; i++) {
          if (items[i].type.indexOf('image') !== -1) {
            const blob = items[i].getAsFile();
            if (blob) {
              const ext = blob.type.split('/')[1] || 'png';
              const name = `screenshot_${new Date().toISOString().replace(/[:.]/g, '-')}.${ext}`;
              this.setFile(blob, name);
              this.activeTab = 'file';
              this.showNotice('📋 วางรูปจากคลิปบอร์ดแล้ว', 'success');
              break;
            }
          }
        }
      });

      // 3. Global keyboard shortcut (Cmd+Enter or Ctrl+Enter to send)
      window.addEventListener('keydown', (e) => {
        if (this.isOpen && (e.metaKey || e.ctrlKey) && e.key === 'Enter') {
          e.preventDefault();
          this.submit();
        }
        if (this.isOpen && e.key === 'Escape') {
          this.close();
        }
      });

      // 4. Listen for open event from App Hub cards or topbar
      window.addEventListener('sentinel:open-relay', () => {
        this.open();
      });
    },

    open() {
      this.isOpen = true;
      document.body.classList.add('drawer-locked');
      if (!this.pin) {
        this.showPinModal = true;
      }
      this.$nextTick(() => {
        const textarea = document.getElementById('relay-message-input');
        if (textarea && !this.showPinModal) textarea.focus();
      });
    },

    close() {
      this.isOpen = false;
      document.body.classList.remove('drawer-locked');
      this.statusText = '';
      this.statusType = '';
    },

    savePin() {
      const trimmed = this.tempPin.trim();
      if (!trimmed) return;
      this.pin = trimmed;
      this.isUnlocked = true;
      localStorage.setItem('sentinel-relay-pin', trimmed);
      this.showPinModal = false;
      this.tempPin = '';
      this.showNotice('🔓 บันทึก PIN สำเร็จ (จำบนอุปกรณ์นี้แล้ว)', 'success');
    },

    clearPin() {
      this.pin = '';
      this.isUnlocked = false;
      localStorage.removeItem('sentinel-relay-pin');
      this.showPinModal = true;
    },

    handleDragOver(e) {
      e.preventDefault();
      this.isDragging = true;
    },

    handleDragLeave(e) {
      e.preventDefault();
      this.isDragging = false;
    },

    handleDrop(e) {
      e.preventDefault();
      this.isDragging = false;
      if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
        const f = e.dataTransfer.files[0];
        this.setFile(f, f.name);
        this.activeTab = 'file';
      }
    },

    handleFileSelect(e) {
      if (e.target.files && e.target.files.length > 0) {
        const f = e.target.files[0];
        this.setFile(f, f.name);
      }
    },

    setFile(file, name) {
      // Size check: 20 MB cap
      if (file.size > 20 * 1024 * 1024) {
        this.showNotice('❌ ขนาดไฟล์เกิน 20 MB (เพดานสูงสุดที่รองรับ)', 'error');
        return;
      }

      this.selectedFile = file;
      this.fileName = name || file.name || 'unnamed_file';
      this.fileSizeFormatted = this.formatBytes(file.size);

      // Check if image for preview
      this.isImageFile = file.type.startsWith('image/') || /\.(jpg|jpeg|png|gif|webp)$/i.test(this.fileName);
      if (this.isImageFile) {
        const reader = new FileReader();
        reader.onload = (e) => {
          this.filePreviewUrl = e.target.result;
        };
        reader.readAsDataURL(file);
      } else {
        this.filePreviewUrl = null;
      }
    },

    removeFile() {
      this.selectedFile = null;
      this.filePreviewUrl = null;
      this.fileName = '';
      this.fileSizeFormatted = '';
      this.isImageFile = false;
      const fileInput = document.getElementById('relay-file-input');
      if (fileInput) fileInput.value = '';
    },

    formatBytes(bytes) {
      if (bytes === 0) return '0 B';
      const k = 1024;
      const sizes = ['B', 'KB', 'MB', 'GB'];
      const i = Math.floor(Math.log(bytes) / Math.log(k));
      return `${(bytes / Math.pow(k, i)).toFixed(1)} ${sizes[i]}`;
    },

    showNotice(text, type = 'info') {
      this.statusText = text;
      this.statusType = type;
      if (type === 'success') {
        setTimeout(() => {
          if (this.statusText === text) {
            this.statusText = '';
            this.statusType = '';
          }
        }, 3000);
      }
    },

    async submit() {
      if (this.isSending) return;

      if (!this.pin) {
        this.showPinModal = true;
        this.showNotice('กรุณาใส่ PIN เพื่อส่งข้อความ', 'error');
        return;
      }

      if (this.activeTab === 'text') {
        const text = this.messageText.trim();
        if (!text) {
          this.showNotice('กรุณากรอกข้อความที่ต้องการส่ง', 'error');
          return;
        }

        this.isSending = true;
        this.statusText = 'กำลังส่งข้อความไปยัง Telegram…';
        this.statusType = 'info';

        try {
          const resp = await fetch('/api/relay/send', {
            method: 'POST',
            headers: {
              'Content-Type': 'application/json',
              'X-Relay-PIN': this.pin
            },
            body: JSON.stringify({ message: text })
          });

          const data = await resp.json();
          if (!resp.ok || !data.success) {
            throw new Error(data.error || `HTTP ${resp.status}`);
          }

          this.showNotice('✨ ส่งข้อความเข้า Telegram สำเร็จแล้ว!', 'success');
          this.messageText = '';
          setTimeout(() => {
            if (this.isOpen) this.close();
          }, 1400);
        } catch (err) {
          if (err.message.includes('401') || err.message.toLowerCase().includes('unauthorized')) {
            this.showNotice('❌ PIN ไม่ถูกต้อง กรุณาเปลี่ยนรหัส PIN', 'error');
            this.showPinModal = true;
          } else {
            this.showNotice(`❌ ส่งไม่สำเร็จ: ${err.message}`, 'error');
          }
        } finally {
          this.isSending = false;
        }

      } else {
        // File / Photo upload
        if (!this.selectedFile) {
          this.showNotice('กรุณาเลือกไฟล์หรือวางรูปภาพก่อน', 'error');
          return;
        }

        this.isSending = true;
        this.statusText = `กำลังอัปโหลด ${this.fileName} (${this.fileSizeFormatted})…`;
        this.statusType = 'info';

        const formData = new FormData();
        formData.append('file', this.selectedFile, this.fileName);
        if (this.captionText.trim()) {
          formData.append('caption', this.captionText.trim());
        }

        try {
          const resp = await fetch('/api/relay/send', {
            method: 'POST',
            headers: {
              'X-Relay-PIN': this.pin
            },
            body: formData
          });

          const data = await resp.json();
          if (!resp.ok || !data.success) {
            throw new Error(data.error || `HTTP ${resp.status}`);
          }

          const fileTypeStr = data.type === 'photo' ? 'รูปภาพ' : 'ไฟล์เอกสาร';
          this.showNotice(`✨ ส่ง${fileTypeStr}เข้า Telegram สำเร็จแล้ว!`, 'success');
          this.removeFile();
          this.captionText = '';
          setTimeout(() => {
            if (this.isOpen) this.close();
          }, 1400);
        } catch (err) {
          if (err.message.includes('401') || err.message.toLowerCase().includes('unauthorized')) {
            this.showNotice('❌ PIN ไม่ถูกต้อง กรุณาตรวจสอบรหัส PIN', 'error');
            this.showPinModal = true;
          } else {
            this.showNotice(`❌ อัปโหลดไม่สำเร็จ: ${err.message}`, 'error');
          }
        } finally {
          this.isSending = false;
        }
      }
    }
  }));
});
