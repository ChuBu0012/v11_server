// Leaflet objects stay outside Alpine.data to avoid proxying library instances.
const liveMapState = {
  map: null,
  markers: { me: null, friend: null },
  connector: null,
  geofenceLayer: null,
  trailLayer: null,
  pollTimer: null,
};

const locationViewerToken = () => decodeURIComponent(window.location.pathname.split('/').filter(Boolean).pop() || '');
const markerIcon = (kind) => L.divIcon({
  className: `tracker-marker tracker-marker-${kind}`,
  html: '<span class="tracker-marker-core"></span><span class="tracker-marker-pulse"></span>',
  iconSize: [34, 34],
  iconAnchor: [17, 17],
});

function updateLeafletLayers(data, showTrail) {
  const map = liveMapState.map;
  if (!map) return;
  const points = [data.me, data.friend].filter(Boolean);
  const coordinates = points.map((point) => [point.latitude, point.longitude]);

  for (const device of ['me', 'friend']) {
    const point = data[device];
    if (!point) {
      if (liveMapState.markers[device]) {
        liveMapState.markers[device].remove();
        liveMapState.markers[device] = null;
      }
      continue;
    }
    const latLng = [point.latitude, point.longitude];
    if (!liveMapState.markers[device]) {
      liveMapState.markers[device] = L.marker(latLng, { icon: markerIcon(device), keyboard: false }).addTo(map);
    } else {
      liveMapState.markers[device].setLatLng(latLng);
    }
  }

  if (coordinates.length === 2) {
    if (!liveMapState.connector) {
      liveMapState.connector = L.polyline(coordinates, { color: '#a78bfa', weight: 2, opacity: 0.82, dashArray: '7 8' }).addTo(map);
    } else {
      liveMapState.connector.setLatLngs(coordinates);
    }
  } else if (liveMapState.connector) {
    liveMapState.connector.remove();
    liveMapState.connector = null;
  }

  liveMapState.geofenceLayer.clearLayers();
  for (const zone of data.geofences || []) {
    L.circle([zone.latitude, zone.longitude], {
      radius: zone.radius_meters,
      color: '#fbbf24',
      weight: 1.5,
      fillColor: '#fbbf24',
      fillOpacity: 0.08,
    }).addTo(liveMapState.geofenceLayer);
  }

  liveMapState.trailLayer.clearLayers();
  if (showTrail) {
    for (const [device, color] of [['me', '#43a7ff'], ['friend', '#38d9a9']]) {
      const trail = (data.breadcrumbs?.[device] || []).map((point) => [point.latitude, point.longitude]);
      if (trail.length > 1) L.polyline(trail, { color, weight: 3, opacity: 0.52 }).addTo(liveMapState.trailLayer);
    }
  }
}

document.addEventListener('alpine:init', () => {
  Alpine.data('liveMap', () => ({
    me: null,
    friend: null,
    geofences: [],
    breadcrumbs: {},
    distanceMeters: null,
    isColocated: false,
    connected: false,
    mapReady: false,
    mapError: '',
    errorText: '',
    updatedAt: null,
    showTrail: false,
    settingsOpen: false,
    pin: localStorage.getItem('sentinel-relay-pin') || '',
    settingsMessage: '',
    managedGeofences: [],
    savingZone: false,
    settings: { notify_colocation: true, notify_geofence: true, notify_battery: true },
    newZone: { name: '', latitude: null, longitude: null, radius_meters: 150, notify_enter: true, notify_exit: true },

    init() {
      if (!window.L) {
        this.mapError = 'โหลดแผนที่ไม่สำเร็จ กรุณาตรวจสอบอินเทอร์เน็ต';
      } else {
        liveMapState.map = L.map('live-map-canvas', { zoomControl: false, preferCanvas: true }).setView([13.7563, 100.5018], 12);
        L.control.zoom({ position: 'bottomright' }).addTo(liveMapState.map);
        L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {
          maxZoom: 19,
          attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors',
        }).addTo(liveMapState.map);
        liveMapState.geofenceLayer = L.layerGroup().addTo(liveMapState.map);
        liveMapState.trailLayer = L.layerGroup().addTo(liveMapState.map);
        this.mapReady = true;
      }

      this.refresh();
      liveMapState.pollTimer = window.setInterval(() => this.refresh(), 15000);
      window.addEventListener('pagehide', () => {
        if (liveMapState.pollTimer) window.clearInterval(liveMapState.pollTimer);
        liveMapState.map?.remove();
        liveMapState.map = null;
      }, { once: true });
    },

    async refresh() {
      this.errorText = '';
      try {
        const response = await fetch('/api/location/live', {
          headers: { 'X-Location-Viewer-Token': locationViewerToken(), Accept: 'application/json' },
          cache: 'no-store',
        });
        if (!response.ok) throw new Error(response.status === 401 ? 'ลิงก์แผนที่หมดอายุหรือไม่ถูกต้อง' : 'โหลดข้อมูลตำแหน่งไม่สำเร็จ');
        const data = await response.json();
        this.me = data.me || null;
        this.friend = data.friend || null;
        this.geofences = data.geofences || [];
        this.breadcrumbs = data.breadcrumbs || {};
        this.distanceMeters = data.distance_meters ?? null;
        this.isColocated = Boolean(data.is_colocated);
        this.settings = { ...this.settings, ...(data.settings || {}) };
        const times = [this.me?.recorded_at, this.friend?.recorded_at].filter(Boolean).map((value) => new Date(value).getTime());
        this.updatedAt = times.length ? new Date(Math.max(...times)) : null;
        this.connected = true;
        updateLeafletLayers(data, this.showTrail);
      } catch (error) {
        this.connected = false;
        this.errorText = error?.message || 'เชื่อมต่อ Pocket Sentinel ไม่สำเร็จ';
      }
    },

    get proximityText() {
      if (!this.me || !this.friend) return 'กำลังรอตำแหน่งจากอุปกรณ์ทั้งสองเครื่อง';
      return this.isColocated ? 'กำลังอยู่ด้วยกัน' : 'กำลังแยกกันอยู่';
    },

    get distanceText() {
      return this.distanceMeters == null ? '—' : this.formatDistance(this.distanceMeters);
    },

    get updatedText() {
      if (!this.updatedAt) return 'ยังไม่มีข้อมูล';
      return `${this.relativeAge(this.updatedAt)} · ${this.updatedAt.toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit' })}`;
    },

    formatDistance(meters) {
      return meters < 1000 ? `${Math.round(meters)} ม.` : `${(meters / 1000).toFixed(1)} กม.`;
    },

    relativeAge(value) {
      if (!value) return 'ยังไม่มีข้อมูล';
      const seconds = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 1000));
      if (seconds < 60) return `${seconds} วินาทีที่แล้ว`;
      if (seconds < 3600) return `${Math.floor(seconds / 60)} นาทีที่แล้ว`;
      if (seconds < 86400) return `${Math.floor(seconds / 3600)} ชั่วโมงที่แล้ว`;
      return `${Math.floor(seconds / 86400)} วันที่แล้ว`;
    },

    deviceAge(point) {
      return point ? `อัปเดต ${this.relativeAge(point.recorded_at)}` : 'ยังไม่ได้รับตำแหน่ง';
    },

    batteryText(point) {
      if (!point || point.battery_level < 0) return 'ไม่ทราบ';
      const charging = point.battery_status === 2 || point.battery_status === 3;
      return `${point.battery_level}%${charging ? ' ⚡' : ''}`;
    },

    speedText(point) {
      if (!point) return '—';
      return point.speed < 1 ? 'หยุดนิ่ง' : `${point.speed.toFixed(0)} km/h`;
    },

    accuracyText(point) {
      return point ? `±${Math.round(point.accuracy)} ม.` : '—';
    },

    centerOnDevices() {
      const coords = [this.me, this.friend].filter(Boolean).map((point) => [point.latitude, point.longitude]);
      if (!liveMapState.map || !coords.length) return;
      if (coords.length === 1) liveMapState.map.setView(coords[0], 16);
      else liveMapState.map.fitBounds(L.latLngBounds(coords).pad(0.28), { maxZoom: 16 });
    },

    toggleTrail() {
      this.showTrail = !this.showTrail;
      updateLeafletLayers({ me: this.me, friend: this.friend, geofences: this.geofences, breadcrumbs: this.breadcrumbs }, this.showTrail);
    },

    openSettings() {
      this.settingsOpen = true;
      if (this.pin) this.loadManagement();
    },

    async loadManagement() {
      this.settingsMessage = '';
      if (!this.pin) {
        this.settingsMessage = 'กรอก Relay PIN ก่อน';
        return;
      }
      try {
        const response = await fetch('/api/location/geofences', { headers: { 'X-Relay-PIN': this.pin, Accept: 'application/json' }, cache: 'no-store' });
        if (!response.ok) throw new Error(response.status === 401 ? 'PIN ไม่ถูกต้อง' : 'โหลดสถานที่ไม่สำเร็จ');
        this.managedGeofences = await response.json();
        localStorage.setItem('sentinel-relay-pin', this.pin);
        this.settingsMessage = 'ยืนยัน PIN แล้ว';
      } catch (error) {
        this.settingsMessage = error?.message || 'เชื่อมต่อไม่สำเร็จ';
      }
    },

    async saveSettings() {
      if (!this.pin) return this.settingsMessage = 'กรอก Relay PIN ก่อน';
      try {
        const response = await fetch('/api/location/settings', {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json', 'X-Relay-PIN': this.pin },
          body: JSON.stringify({
            notify_colocation: this.settings.notify_colocation,
            notify_geofence: this.settings.notify_geofence,
            notify_battery: this.settings.notify_battery,
          }),
        });
        if (!response.ok) throw new Error('บันทึกการตั้งค่าไม่สำเร็จ ตรวจสอบ PIN');
        this.settings = { ...this.settings, ...(await response.json()) };
        localStorage.setItem('sentinel-relay-pin', this.pin);
        this.settingsMessage = 'บันทึกแล้ว';
      } catch (error) {
        this.settingsMessage = error?.message || 'บันทึกไม่สำเร็จ';
      }
    },

    async createGeofence() {
      if (!this.pin) return this.settingsMessage = 'กรอก Relay PIN ก่อน';
      this.savingZone = true;
      try {
        const response = await fetch('/api/location/geofences', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Relay-PIN': this.pin },
          body: JSON.stringify(this.newZone),
        });
        if (!response.ok) throw new Error('เพิ่มสถานที่ไม่สำเร็จ ตรวจสอบข้อมูลและ PIN');
        this.newZone = { name: '', latitude: null, longitude: null, radius_meters: 150, notify_enter: true, notify_exit: true };
        await this.loadManagement();
        await this.refresh();
      } catch (error) {
        this.settingsMessage = error?.message || 'เพิ่มสถานที่ไม่สำเร็จ';
      } finally {
        this.savingZone = false;
      }
    },

    async deleteGeofence(id) {
      if (!this.pin || !window.confirm('ลบสถานที่นี้หรือไม่?')) return;
      try {
        const response = await fetch(`/api/location/geofences/${encodeURIComponent(id)}`, { method: 'DELETE', headers: { 'X-Relay-PIN': this.pin } });
        if (!response.ok) throw new Error('ลบสถานที่ไม่สำเร็จ ตรวจสอบ PIN');
        await this.loadManagement();
        await this.refresh();
      } catch (error) {
        this.settingsMessage = error?.message || 'ลบสถานที่ไม่สำเร็จ';
      }
    },
  }));
});
