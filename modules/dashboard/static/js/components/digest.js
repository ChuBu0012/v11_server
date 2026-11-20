// =============================================================================
// Pocket Sentinel — AI Daily Digest Component
// =============================================================================

document.addEventListener('alpine:init', () => {
  Alpine.data('digestComponent', () => ({
    activeTab: 'ai',
    enabled: true,
    schedule: '13:00',
    metaText: 'Waiting for first digest run',
    statusBadge: {
      text: 'Enabled (Daily 13:00)',
      class: 'badge ok',
      icon: 'ri-checkbox-circle-fill'
    },
    toggleBtn: {
      text: 'Pause',
      icon: 'ri-pause-circle-line'
    },
    sections: { ai: '', politics: '', stocks: '', gold: '', trivia: '', all: '' },
    renderedContent: '<div class="loading">Loading latest AI digest…</div>',
    isBusy: false,

    init() {
      this.loadStatus();
      window.setInterval(() => this.loadStatus(), 60000);
    },

    setTab(tab) {
      this.activeTab = tab;
      this.updateRenderedTab();
    },

    async loadStatus() {
      try {
        const res = await getJSON('/api/digest/status');
        const st = res.settings;
        const latest = res.latest_run;

        if (st) {
          this.enabled = st.enabled;
          this.schedule = st.schedule || '13:00';
          if (st.enabled) {
            this.statusBadge = {
              text: `Active (${this.schedule})`,
              class: 'badge ok',
              icon: 'ri-checkbox-circle-fill'
            };
            this.toggleBtn = {
              text: 'Pause',
              icon: 'ri-pause-circle-line'
            };
          } else {
            this.statusBadge = {
              text: 'Paused',
              class: 'badge active',
              icon: 'ri-pause-circle-fill'
            };
            this.toggleBtn = {
              text: 'Resume',
              icon: 'ri-play-circle-line'
            };
          }
        }

        if (latest && latest.summary_md) {
          this.metaText = `Last run #${latest.id} · ${formatTime(latest.finished_at || latest.started_at)} · ${latest.items_count || 0} items`;
          this.sections = this.parseSections(latest.summary_md);
          this.updateRenderedTab();
        } else if (latest && latest.status === 'failed') {
          this.metaText = `Last run failed · ${formatTime(latest.finished_at || latest.started_at)}`;
          this.renderedContent = `<span class="digest-status warn">Error: ${escapeHtml(latest.error_message || 'Digest generation failed')}</span>`;
        } else {
          this.metaText = 'No digest generated yet';
          this.renderedContent = 'คลิก "Run Now" เพื่อเริ่มดึงข่าวสดและให้ Gemini สรุปข่าว AI, การเมือง และเกร็ดความรู้ทันที';
        }
      } catch (err) {
        console.warn('Failed to load digest status:', err);
      }
    },

    async toggle() {
      this.isBusy = true;
      try {
        await fetch('/api/digest/toggle', { method: 'POST' });
        await this.loadStatus();
      } catch (e) {
        alert('Toggle failed: ' + e.message);
      } finally {
        this.isBusy = false;
      }
    },

    async runNow() {
      this.isBusy = true;
      this.renderedContent = '<div class="loading"><span class="spinner" aria-hidden="true"></span> กำลังดึงข่าวสดและให้ Gemini ประมวลผลบทความสรุป (ใช้เวลาประมาณ 10-15 วินาที)...</div>';
      try {
        const res = await fetch('/api/digest/run', { method: 'POST' });
        const data = await res.json();
        if (!res.ok) throw new Error(data.error || 'Generation failed');
        await this.loadStatus();
      } catch (e) {
        this.renderedContent = `<span class="digest-status err">ล้มเหลว: ${escapeHtml(e.message)}</span>`;
      } finally {
        this.isBusy = false;
      }
    },

    parseSections(raw) {
      const sec = { ai: '', politics: '', stocks: '', gold: '', trivia: '', all: raw || '' };
      if (!raw) return sec;

      if (raw.includes('===SECTION:')) {
        const parts = raw.split('===SECTION:');
        for (const part of parts) {
          const trimmed = part.trim();
          if (!trimmed) continue;
          const idx = trimmed.indexOf('===');
          if (idx === -1) continue;
          const key = trimmed.slice(0, idx).trim().toLowerCase();
          const content = trimmed.slice(idx + 3).trim();
          if (sec.hasOwnProperty(key)) {
            sec[key] = content;
          }
        }
        return sec;
      }

      const aiMatch = raw.match(/###\s*(?:🤖\s*)?AI & Tech[\s\S]*?(?=(?:###|$))/i);
      const polMatch = raw.match(/###\s*(?:🏛️\s*)?การเมือง[\s\S]*?(?=(?:###|$))/i);
      const stockMatch = raw.match(/###\s*(?:📈\s*)?หุ้น[\s\S]*?(?=(?:###|$))/i);
      const goldMatch = raw.match(/###\s*(?:🥇\s*)?ทอง[\s\S]*?(?=(?:###|$))/i);
      const triMatch = raw.match(/###\s*(?:💡\s*)?สาระน่ารู้[\s\S]*?(?=(?:###|$))/i);

      if (aiMatch) sec.ai = aiMatch[0].trim();
      if (polMatch) sec.politics = polMatch[0].trim();
      if (stockMatch) sec.stocks = stockMatch[0].trim();
      if (goldMatch) sec.gold = goldMatch[0].trim();
      if (triMatch) sec.trivia = triMatch[0].trim();

      if (!sec.ai && !sec.politics && !sec.stocks && !sec.gold && !sec.trivia) {
        const lines = raw.split('\n');
        let currentKey = 'ai';
        let currentLines = [];

        for (const line of lines) {
          const lower = line.toLowerCase();
          if (lower.includes('การเมือง') || lower.includes('politics')) {
            if (currentLines.length) sec[currentKey] = currentLines.join('\n');
            currentKey = 'politics';
            currentLines = [line];
          } else if (lower.includes('หุ้น') || lower.includes('stock')) {
            if (currentLines.length) sec[currentKey] = currentLines.join('\n');
            currentKey = 'stocks';
            currentLines = [line];
          } else if (lower.includes('ทอง') || lower.includes('gold')) {
            if (currentLines.length) sec[currentKey] = currentLines.join('\n');
            currentKey = 'gold';
            currentLines = [line];
          } else if (lower.includes('สาระน่ารู้') || lower.includes('trivia')) {
            if (currentLines.length) sec[currentKey] = currentLines.join('\n');
            currentKey = 'trivia';
            currentLines = [line];
          } else if (lower.includes('ai') || lower.includes('tech')) {
            if (currentLines.length) sec[currentKey] = currentLines.join('\n');
            currentKey = 'ai';
            currentLines = [line];
          } else {
            currentLines.push(line);
          }
        }
        if (currentLines.length) sec[currentKey] = currentLines.join('\n');
      }

      return sec;
    },

    updateRenderedTab() {
      const content = this.sections[this.activeTab] || this.sections['all'] || '';
      if (!content) {
        this.renderedContent = '<div class="loading">ไม่มีเนื้อหาสำหรับหมวดนี้</div>';
        return;
      }
      this.renderedContent = this.renderMarkdown(content);
    },

    renderMarkdown(md) {
      if (!md) return '';
      let html = md.replace(/\r\n/g, '\n');

      html = html.replace(/```([\s\S]*?)```/g, (match, p1) => {
        return '<pre><code>' + escapeHtml(p1.trim()) + '</code></pre>';
      });

      html = html.replace(/^### (.*$)/gim, '<h3>$1</h3>');
      html = html.replace(/^## (.*$)/gim, '<h2>$1</h2>');
      html = html.replace(/^# (.*$)/gim, '<h1>$1</h1>');
      html = html.replace(/^---$/gim, '<hr>');
      html = html.replace(/\*\*(.*?)\*\*/g, '<strong>$1</strong>');
      html = html.replace(/\*(.*?)\*/g, '<em>$1</em>');
      html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');

      const lines = html.split('\n');
      let inList = false;
      const processed = [];

      for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        const listMatch = line.match(/^(\*|-|\d+\.)\s+(.*)/);
        if (listMatch) {
          if (!inList) {
            inList = true;
            processed.push('<ul>');
          }
          processed.push(`<li>${listMatch[2]}</li>`);
        } else {
          if (inList) {
            inList = false;
            processed.push('</ul>');
          }
          if (line.trim().length > 0 && !line.startsWith('<h') && !line.startsWith('<hr') && !line.startsWith('<pre')) {
            processed.push(`<p>${line}</p>`);
          } else {
            processed.push(line);
          }
        }
      }
      if (inList) processed.push('</ul>');

      return processed.join('\n');
    }
  }));
});
