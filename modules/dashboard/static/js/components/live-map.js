// Leaflet objects stay outside Alpine.data to avoid proxying library instances.
const liveMapState = {
  map: null,
  markers: { me: null, friend: null },
  colocatedMarker: null,
  connector: null,
  geofenceLayer: null,
  trailLayer: null,
  pickerLayer: null,
  pickerMarker: null,
  pickerCircle: null,
  pollTimer: null,
};

const locationViewerToken = () => decodeURIComponent(window.location.pathname.split('/').filter(Boolean).pop() || '');

function haversineMeters(lat1, lon1, lat2, lon2) {
  const R = 6371000;
  const dLat = (lat2 - lat1) * Math.PI / 180;
  const dLon = (lon2 - lon1) * Math.PI / 180;
  const a = Math.sin(dLat / 2) * Math.sin(dLat / 2) +
            Math.cos(lat1 * Math.PI / 180) * Math.cos(lat2 * Math.PI / 180) *
            Math.sin(dLon / 2) * Math.sin(dLon / 2);
  return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}

function formatRelativeShort(dateStr) {
  if (!dateStr) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(dateStr).getTime()) / 1000));
  if (seconds < 60) return `${seconds} วิที่แล้ว`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)} นาทีที่แล้ว`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)} ชม.ที่แล้ว`;
  return `${Math.floor(seconds / 86400)} วันที่แล้ว`;
}

// Single avatar pin marker for individual position
const singleAvatarIcon = (kind) => {
  const isMe = kind === 'me';
  const name = isMe ? 'Chubu' : 'Furturia';
  const avatarSrc = isMe ? '/static/img/avatar-chubu.png?v=22' : '/static/img/avatar-furturia.png?v=22';
  const themeClass = isMe ? 'marker-chubu' : 'marker-furturia';
  return L.divIcon({
    className: `single-avatar-marker ${themeClass}`,
    html: `
      <div class="avatar-marker-pin">
        <div class="avatar-halo"></div>
        <div class="avatar-frame">
          <img src="${avatarSrc}" alt="${name}" />
        </div>
        <span class="avatar-tag">${name}</span>
        <div class="avatar-pointer"></div>
      </div>
    `,
    iconSize: [46, 62],
    iconAnchor: [23, 62],
    popupAnchor: [0, -60],
  });
};

// Life360-style Dual Avatar Co-Location Speech Bubble Pin
const colocatedSpeechBubbleIcon = (title, subtext) => {
  return L.divIcon({
    className: 'colocated-bubble-wrapper',
    html: `
      <div class="life360-bubble">
        <div class="bubble-header-card">
          <span class="bubble-icon">
            <svg width="15" height="15" viewBox="0 0 24 24" fill="#8b5cf6">
              <path d="M12 2C8.13 2 5 5.13 5 9c0 5.25 7 13 7 13s7-7.75 7-13c0-3.87-3.13-7-7-7zm0 9.5c-1.38 0-2.5-1.12-2.5-2.5s1.12-2.5 2.5-2.5 2.5 1.12 2.5 2.5-1.12 2.5-2.5 2.5z"/>
            </svg>
          </span>
          <div class="bubble-titles">
            <span class="bubble-headline">${title}</span>
            <span class="bubble-subline">${subtext}</span>
          </div>
        </div>
        <div class="bubble-avatars-cluster">
          <div class="cluster-avatar chubu">
            <img src="/static/img/avatar-chubu.png?v=22" alt="Chubu" />
          </div>
          <div class="cluster-avatar furturia">
            <img src="/static/img/avatar-furturia.png?v=22" alt="Furturia" />
          </div>
        </div>
        <div class="bubble-tip"></div>
      </div>
    `,
    iconSize: [220, 115],
    iconAnchor: [110, 115],
    popupAnchor: [0, -112],
  });
};

function updateLeafletLayers(data, showTrail) {
  const map = liveMapState.map;
  if (!map) return;
  const me = data.me;
  const friend = data.friend;
  const isColocated = Boolean(data.is_colocated && me && friend);

  if (isColocated) {
    // Hide individual markers when co-located
    for (const device of ['me', 'friend']) {
      if (liveMapState.markers[device]) {
        liveMapState.markers[device].remove();
        liveMapState.markers[device] = null;
      }
    }
    if (liveMapState.connector) {
      liveMapState.connector.remove();
      liveMapState.connector = null;
    }

    // Determine midpoint
    const midLat = (me.latitude + friend.latitude) / 2;
    const midLng = (me.longitude + friend.longitude) / 2;
    const midPoint = [midLat, midLng];

    // Check if in any Geofence
    let matchingZone = null;
    for (const zone of data.geofences || []) {
      const d = haversineMeters(midLat, midLng, zone.latitude, zone.longitude);
      if (d <= zone.radius_meters) {
        matchingZone = zone;
        break;
      }
    }

    const title = matchingZone
      ? `อยู่ที่ ${matchingZone.name} ด้วยกัน`
      : 'อยู่ที่นี่ด้วยกัน';

    const latestTime = Math.max(
      new Date(me.recorded_at).getTime(),
      new Date(friend.recorded_at).getTime()
    );
    const subtext = `ห่างกัน ${Math.round(data.distance_meters || 0)} ม. · ${formatRelativeShort(latestTime)}`;

    const icon = colocatedSpeechBubbleIcon(title, subtext);
    if (!liveMapState.colocatedMarker) {
      liveMapState.colocatedMarker = L.marker(midPoint, { icon, keyboard: false }).addTo(map);
    } else {
      liveMapState.colocatedMarker.setLatLng(midPoint);
      liveMapState.colocatedMarker.setIcon(icon);
    }

    // Attach popup with details
    const popupHTML = `
      <div style="font-family: inherit; font-size: 11px; line-height: 1.5; color: #1e293b;">
        <strong style="display:block; font-size: 13px; margin-bottom: 4px; color: #0f172a;">📍 ${title}</strong>
        <div>ระยะห่าง: <b>${Math.round(data.distance_meters || 0)} เมตร</b></div>
        <div style="margin-top: 4px; border-top: 1px solid #e2e8f0; padding-top: 4px;">
          <div><b>Chubu:</b> แบต ${me.battery_level}% · ${me.speed < 1 ? 'หยุดนิ่ง' : me.speed + ' km/h'}</div>
          <div><b>Furturia:</b> แบต ${friend.battery_level}% · ${friend.speed < 1 ? 'หยุดนิ่ง' : friend.speed + ' km/h'}</div>
        </div>
      </div>
    `;
    liveMapState.colocatedMarker.bindPopup(popupHTML);

  } else {
    // Not colocated: remove colocated marker
    if (liveMapState.colocatedMarker) {
      liveMapState.colocatedMarker.remove();
      liveMapState.colocatedMarker = null;
    }

    // Update single markers
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
        liveMapState.markers[device] = L.marker(latLng, { icon: singleAvatarIcon(device), keyboard: false }).addTo(map);
      } else {
        liveMapState.markers[device].setLatLng(latLng);
        liveMapState.markers[device].setIcon(singleAvatarIcon(device));
      }
    }

    // Polyline connector
    if (me && friend) {
      const coordinates = [[me.latitude, me.longitude], [friend.latitude, friend.longitude]];
      if (!liveMapState.connector) {
        liveMapState.connector = L.polyline(coordinates, { color: '#a78bfa', weight: 2, opacity: 0.85, dashArray: '6 8' }).addTo(map);
      } else {
        liveMapState.connector.setLatLngs(coordinates);
      }
    } else if (liveMapState.connector) {
      liveMapState.connector.remove();
      liveMapState.connector = null;
    }
  }

  // Geofences
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

  // Breadcrumb Trail
  liveMapState.trailLayer.clearLayers();
  if (showTrail) {
    for (const [device, color] of [['me', '#43a7ff'], ['friend', '#38d9a9']]) {
      const trail = (data.breadcrumbs?.[device] || []).map((point) => [point.latitude, point.longitude]);
      if (trail.length > 1) {
        L.polyline(trail, { color, weight: 3, opacity: 0.55 }).addTo(liveMapState.trailLayer);
      }
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
    pickingLocation: false,
    showLogs: true,
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
        liveMapState.map = L.map('live-map-canvas', { zoomControl: false, preferCanvas: true }).setView([13.6516, 100.4939], 15);
        L.control.zoom({ position: 'bottomright' }).addTo(liveMapState.map);

        // Google Maps Clean Roadmap tiles with Esri fallback
        const baseLayer = L.tileLayer('https://mt{s}.google.com/vt/lyrs=m&x={x}&y={y}&z={z}', {
          maxZoom: 20,
          subdomains: ['0', '1', '2', '3'],
          attribution: '&copy; Google Maps',
        });
        baseLayer.on('tileerror', function() {
          if (!this._hasFallenBack) {
            this._hasFallenBack = true;
            this.setUrl('https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/{z}/{y}/{x}');
          }
        });
        baseLayer.addTo(liveMapState.map);

        liveMapState.geofenceLayer = L.layerGroup().addTo(liveMapState.map);
        liveMapState.trailLayer = L.layerGroup().addTo(liveMapState.map);
        liveMapState.pickerLayer = L.layerGroup().addTo(liveMapState.map);

        // Map click for geofence picker
        liveMapState.map.on('click', (e) => {
          if (!this.pickingLocation) return;
          this.newZone.latitude = parseFloat(e.latlng.lat.toFixed(6));
          this.newZone.longitude = parseFloat(e.latlng.lng.toFixed(6));
          this.renderPickerPreview();
        });

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
      return this.isColocated ? 'Chubu & Furturia อยู่ที่นี่ด้วยกัน' : 'กำลังแยกกันอยู่';
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

    get recentLogs() {
      const meLogs = (this.breadcrumbs?.me || []).map((p) => ({ ...p, device: 'me', name: 'Chubu' }));
      const friendLogs = (this.breadcrumbs?.friend || []).map((p) => ({ ...p, device: 'friend', name: 'Furturia' }));
      return [...meLogs, ...friendLogs].sort((a, b) => new Date(b.recorded_at).getTime() - new Date(a.recorded_at).getTime()).slice(0, 15);
    },

    formatTimeDetail(dateStr) {
      if (!dateStr) return '';
      const d = new Date(dateStr);
      const time = d.toLocaleTimeString('th-TH', { hour: '2-digit', minute: '2-digit', second: '2-digit' });
      const rel = this.relativeAge(dateStr);
      return `${time} (${rel})`;
    },

    centerOnDevices() {
      const coords = [this.me, this.friend].filter(Boolean).map((point) => [point.latitude, point.longitude]);
      if (!liveMapState.map || !coords.length) return;
      if (coords.length === 1) liveMapState.map.setView(coords[0], 16);
      else liveMapState.map.fitBounds(L.latLngBounds(coords).pad(0.28), { maxZoom: 16 });
    },

    toggleTrail() {
      this.showTrail = !this.showTrail;
      updateLeafletLayers({ me: this.me, friend: this.friend, geofences: this.geofences, breadcrumbs: this.breadcrumbs, distance_meters: this.distanceMeters, is_colocated: this.isColocated }, this.showTrail);
    },

    openSettings() {
      this.settingsOpen = true;
      if (this.pin) this.loadManagement();
    },

    // Map Picker Handlers
    toggleMapPicker() {
      this.pickingLocation = !this.pickingLocation;
      if (this.pickingLocation) {
        this.settingsOpen = false; // Hide drawer so map is fully visible
        const container = liveMapState.map?.getContainer();
        if (container) container.classList.add('map-picking-cursor');

        // If no lat/lng yet, set default from current user or map center
        if (!this.newZone.latitude) {
          const center = liveMapState.map.getCenter();
          this.newZone.latitude = parseFloat(center.lat.toFixed(6));
          this.newZone.longitude = parseFloat(center.lng.toFixed(6));
        }
        this.renderPickerPreview();
      } else {
        this.cancelMapPicker();
      }
    },

    renderPickerPreview() {
      if (!liveMapState.map || !this.newZone.latitude) return;
      liveMapState.pickerLayer.clearLayers();

      const pos = [this.newZone.latitude, this.newZone.longitude];
      
      // Radius Circle
      liveMapState.pickerCircle = L.circle(pos, {
        radius: this.newZone.radius_meters || 150,
        color: '#10b981',
        weight: 2,
        dashArray: '5 6',
        fillColor: '#10b981',
        fillOpacity: 0.16,
      }).addTo(liveMapState.pickerLayer);

      // Draggable Target Pin
      const pinIcon = L.divIcon({
        className: 'picker-pin-marker',
        html: `<div class="picker-target-pin">🎯</div>`,
        iconSize: [36, 36],
        iconAnchor: [18, 18],
      });

      liveMapState.pickerMarker = L.marker(pos, {
        icon: pinIcon,
        draggable: true,
      }).addTo(liveMapState.pickerLayer);

      liveMapState.pickerMarker.on('drag', (e) => {
        const latLng = e.target.getLatLng();
        this.newZone.latitude = parseFloat(latLng.lat.toFixed(6));
        this.newZone.longitude = parseFloat(latLng.lng.toFixed(6));
        if (liveMapState.pickerCircle) {
          liveMapState.pickerCircle.setLatLng(latLng);
        }
      });
    },

    updatePickerRadius() {
      if (liveMapState.pickerCircle) {
        liveMapState.pickerCircle.setRadius(this.newZone.radius_meters);
      }
    },

    setRadius(r) {
      this.newZone.radius_meters = r;
      this.updatePickerRadius();
    },

    finishMapPicker() {
      this.pickingLocation = false;
      const container = liveMapState.map?.getContainer();
      if (container) container.classList.remove('map-picking-cursor');
      this.settingsOpen = true; // reopen drawer to complete form
    },

    cancelMapPicker() {
      this.pickingLocation = false;
      const container = liveMapState.map?.getContainer();
      if (container) container.classList.remove('map-picking-cursor');
      liveMapState.pickerLayer.clearLayers();
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
        this.cancelMapPicker();
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
