function scheduleSummaryComponent() {
  return {
    loading: true,
    failed: false,
    items: [],
    async init() {
      await this.refresh();
      window.setInterval(() => this.refresh(), 60000);
    },
    async refresh() {
      try {
        const response = await fetch('/api/schedule/upcoming', { cache: 'no-store' });
        if (!response.ok) throw new Error('schedule unavailable');
        const data = await response.json();
        const unique = new Map();
        for (const item of data.items || []) {
          if (item.state === 'failed') continue;
          const key = `${item.reminder_id}-${item.event_at}`;
          if (!unique.has(key)) unique.set(key, item);
        }
        this.items = [...unique.values()].sort((a, b) => new Date(a.event_at) - new Date(b.event_at)).slice(0, 3);
      } catch (_) { this.items = []; this.failed = true; }
      this.loading = false;
    },
    time(value) { return new Intl.DateTimeFormat('th-TH', { timeZone: 'Asia/Bangkok', hour: '2-digit', minute: '2-digit' }).format(new Date(value)); },
    label(item) { return item.offset_minutes > 0 ? `เตือนล่วงหน้า ${item.offset_minutes} นาที` : 'เตือนตรงเวลา'; }
  };
}
