// =============================================================================
// Pocket Sentinel — Telegram Relay & App Hub Component
// =============================================================================

// Architecture Guardrail (frontend-proxy-guard):
// Keep browser File object in module-level scope to avoid Alpine Proxy prototype corruption.
let pendingRelayFile = null;

document.addEventListener('alpine:init', () => {
  Alpine.data('relayComponent', () => ({
    isOpen: window.location.pathname.includes('tele-br') || window.location.pathname === '/relay',
    isStandalonePage: window.location.pathname.includes('tele-br') || window.location.pathname === '/relay',
    activeTab: 'text', // 'text' | 'file'
    pin: localStorage.getItem('sentinel-relay-pin') || '',
    isUnlocked: Boolean(localStorage.getItem('sentinel-relay-pin')),
    tempPin: '',
    showPinModal: false,

    // Form fields (Primitive state only — NO File instances in proxy)
    messageText: '',
    captionText: '',
    hasFile: false,
    filePreviewUrl: null,
    fileName: '',
    fileSizeFormatted: '',
    isImageFile: false,

    // UI state
    isDragging: false,
    isSending: false,
    statusText: '',
    statusType: '', // 'success' | 'error' | 'info' | ''

    init() {
      // 1. Auto-open drawer on main dashboard if query parameter has ?app=relay
      const params = new URLSearchParams(window.location.search);
      if (params.get('app') === 'relay') {
        this.open();
      }

      // If standalone page and no PIN configured, prompt setup
      if (this.isStandalonePage && !this.pin) {
        this.showPinModal = true;
      }

      // 2. Global paste listener for instant clipboard screenshot capture
      window.addEventListener('paste', (e) => {
        if (!this.isOpen && !this.isStandalonePage) return;
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
        if ((this.isOpen || this.isStandalonePage) && (e.metaKey || e.ctrlKey) && e.key === 'Enter') {
          e.preventDefault();
          this.submit();
        }
        if (this.isOpen && !this.isStandalonePage && e.key === 'Escape') {
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
      if (!this.isStandalonePage) {
        document.body.classList.add('drawer-locked');
      }
      if (!this.pin) {
        this.showPinModal = true;
      }
      this.$nextTick(() => {
        const textarea = document.getElementById('relay-message-input');
        if (textarea && !this.showPinModal) textarea.focus();
      });
    },

    close() {
      if (this.isStandalonePage) return;
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
        this.showNotice('❌ ขนาดไฟล์เกิน 20 MB (เพดานสูงสุดที่เซิร์ฟเวอร์อนุญาต)', 'error');
        return;
      }

      // Store in outer unproxied reference
      pendingRelayFile = file;
      this.hasFile = true;
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
      pendingRelayFile = null;
      this.hasFile = false;
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
        }, 3500);
      }
    },

    async readRelayResponse(resp) {
      const contentType = resp.headers.get('content-type') || '';
      const body = await resp.text();
      if (contentType.includes('application/json')) {
        try { return JSON.parse(body); } catch (_) { /* report the response below */ }
      }
      if (/ERR_NGROK_|ngrok gateway error/i.test(body) || contentType.includes('text/html')) {
        throw new Error('เกตเวย์ ngrok ตอบกลับหน้า HTML แทน API (ERR_NGROK_3004) กรุณารอสักครู่แล้วลองใหม่');
      }
      throw new Error(`เซิร์ฟเวอร์ตอบกลับข้อมูลที่อ่านไม่ได้ (HTTP ${resp.status})`);
    },

    async submit() {
      if (this.isSending) return;

      if (!this.pin) {
        this.showPinModal = true;
        this.showNotice('กรุณาตั้งค่าหรือกรอกรหัส PIN ก่อนส่ง', 'error');
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

          const data = await this.readRelayResponse(resp);

          if (!resp.ok || !data?.success) {
            let errMsg = data?.error;
            if (!errMsg) {
              if (resp.status === 502 || resp.status === 504) {
                errMsg = 'เกตเวย์ขัดข้องหรือหมดเวลา (Gateway Timeout / ERR_NGROK_3004)';
              } else {
                errMsg = `เซิร์ฟเวอร์ตอบกลับรหัสข้อผิดพลาด HTTP ${resp.status}`;
              }
            }
            throw new Error(errMsg);
          }

          this.showNotice('✨ ส่งข้อความเข้า Telegram สำเร็จแล้ว!', 'success');
          this.messageText = '';
          if (!this.isStandalonePage) {
            setTimeout(() => {
              if (this.isOpen) this.close();
            }, 1400);
          }
        } catch (err) {
          if (err.message.includes('401') || err.message.toLowerCase().includes('unauthorized')) {
            this.showNotice('❌ PIN ไม่ถูกต้อง กรุณาตรวจสอบรหัส PIN (ค่าเริ่มต้นคือ 1804)', 'error');
            this.showPinModal = true;
          } else {
            this.showNotice(`❌ ส่งไม่สำเร็จ: ${err.message}`, 'error');
          }
        } finally {
          this.isSending = false;
        }

      } else {
        // File / Photo upload
        if (!pendingRelayFile || !this.hasFile) {
          this.showNotice('กรุณาเลือกไฟล์หรือวางรูปภาพก่อน', 'error');
          return;
        }

        this.isSending = true;
        this.statusText = `กำลังอัปโหลด ${this.fileName} (${this.fileSizeFormatted}) ไปยัง Telegram…`;
        this.statusType = 'info';

        const formData = new FormData();
        formData.append('file', pendingRelayFile, this.fileName);
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

          const data = await this.readRelayResponse(resp);

          if (!resp.ok || !data?.success) {
            let errMsg = data?.error;
            if (!errMsg) {
              if (resp.status === 502 || resp.status === 504) {
                errMsg = 'เซิร์ฟเวอร์ส่งไฟล์ไม่สำเร็จหรือหมดเวลา (ERR_NGROK_3004 / Gateway Timeout)';
              } else if (resp.status === 413) {
                errMsg = 'ขนาดไฟล์เกินเพดานสูงสุดที่เซิร์ฟเวอร์อนุญาต (20 MB)';
              } else {
                errMsg = `เซิร์ฟเวอร์ตอบกลับรหัสข้อผิดพลาด HTTP ${resp.status}`;
              }
            }
            throw new Error(errMsg);
          }

          const fileTypeStr = data.type === 'photo' ? 'รูปภาพ' : 'ไฟล์เอกสาร';
          this.showNotice(`✨ ส่ง${fileTypeStr}เข้า Telegram สำเร็จแล้ว!`, 'success');
          this.removeFile();
          this.captionText = '';
          if (!this.isStandalonePage) {
            setTimeout(() => {
              if (this.isOpen) this.close();
            }, 1400);
          }
        } catch (err) {
          if (err.message.includes('401') || err.message.toLowerCase().includes('unauthorized')) {
            this.showNotice('❌ PIN ไม่ถูกต้อง กรุณาตรวจสอบรหัส PIN (ค่าเริ่มต้นคือ 1804)', 'error');
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
