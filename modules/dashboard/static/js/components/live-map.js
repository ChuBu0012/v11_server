// Leaflet objects stay outside Alpine.data to avoid proxying library instances.
const liveMapState = {
  map: null,
  markers: { me: null, friend: null },
  colocatedMarker: null,
  connector: null,
  geofenceLayer: null,
  trailLayer: null,
  pickerLayer: null,
  pickerCircle: null,
  pickerOpener: null,
  pollTimer: null,
};

const PICKER_RADIUS_MIN = 25;
const PICKER_RADIUS_MAX = 100000;
const PICKER_SLIDER_MIN = 50;
const PICKER_SLIDER_MAX = 2000;

const locationViewerToken = () => {
  const fromPath = decodeURIComponent(window.location.pathname.split('/').filter(Boolean).pop() || '');
  if (fromPath && fromPath !== 'index.html' && fromPath !== 'live' && fromPath !== 'map' && fromPath.length >= 24) {
    try { localStorage.setItem('sentinel_viewer_token', fromPath); } catch (_) {}
    return fromPath;
  }
  const fromQuery = new URLSearchParams(window.location.search).get('token');
  if (fromQuery && fromQuery.length >= 24) {
    try { localStorage.setItem('sentinel_viewer_token', fromQuery); } catch (_) {}
    return fromQuery;
  }
  try {
    const saved = localStorage.getItem('sentinel_viewer_token');
    if (saved) return saved;
  } catch (_) {}
  return '';
};

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
  const avatarSrc = isMe ? '/static/img/avatar-chubu.png?v=28' : '/static/img/avatar-furturia.png?v=28';
  const themeClass = isMe ? 'marker-chubu' : 'marker-furturia';
  return L.divIcon({
    className: `single-avatar-marker ${themeClass}`,
    html: `
      <div class="avatar-marker-pin" title="${name}">
        <div class="avatar-halo"></div>
        <div class="avatar-frame">
          <img src="${avatarSrc}" alt="${name}" />
        </div>
        <div class="avatar-pointer"></div>
      </div>
    `,
    iconSize: [44, 52],
    iconAnchor: [22, 50],
    popupAnchor: [0, -48],
  });
};

function renderUserPopup(device, point) {
  if (!point) return '';
  const isMe = device === 'me';
  const name = isMe ? 'Chubu' : 'Furturia';
  const avatarSrc = isMe ? '/static/img/avatar-chubu.png?v=28' : '/static/img/avatar-furturia.png?v=28';
  const relTime = formatRelativeShort(point.recorded_at);
  const ageMs = Date.now() - new Date(point.recorded_at).getTime();
  const isLive = ageMs < 5 * 60 * 1000;
  const isStale = ageMs >= 5 * 60 * 1000 && ageMs < 60 * 60 * 1000;
  const statusLabel = isLive ? 'ออนไลน์' : (isStale ? 'สัญญาณนิ่ง' : 'ออฟไลน์');
  const statusClass = isLive ? 'live' : (isStale ? 'stale' : 'offline');
  const speedText = point.speed < 1 ? 'หยุดนิ่ง' : `${Math.round(point.speed)} km/h`;
  const batt = Math.max(0, Math.min(100, point.battery_level || 0));
  const isCharging = point.battery_status === 2 || point.battery_status === 3;
  const battClass = batt > 25 ? 'batt-good' : (batt > 15 ? 'batt-mid' : 'batt-low');

  return `
    <div class="custom-profile-card">
      <div class="cpc-header">
        <img src="${avatarSrc}" alt="${name}" class="cpc-avatar">
        <div class="cpc-info">
          <strong class="cpc-name">${name}</strong>
          <span class="cpc-time">${relTime || 'ล่าสุด'}</span>
        </div>
        <span class="cpc-status-badge ${statusClass}">● ${statusLabel}</span>
      </div>
      <div class="cpc-metrics-grid">
        <div class="cpc-metric">
          <span class="cpc-metric-lbl">แบตเตอรี่</span>
          <strong class="cpc-metric-val ${battClass}">${batt}% ${isCharging ? '⚡' : ''}</strong>
          <div class="cpc-batt-bar"><div class="cpc-batt-fill ${battClass}" style="width: ${batt}%"></div></div>
        </div>
        <div class="cpc-metric">
          <span class="cpc-metric-lbl">ความเร็ว</span>
          <strong class="cpc-metric-val">${speedText}</strong>
        </div>
        <div class="cpc-metric">
          <span class="cpc-metric-lbl">ความแม่นยำ</span>
          <strong class="cpc-metric-val">±${Math.round(point.accuracy || 0)} ม.</strong>
        </div>
      </div>
    </div>
  `;
}

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
            <img src="/static/img/avatar-chubu.png?v=28" alt="Chubu" />
          </div>
          <div class="cluster-avatar furturia">
            <img src="/static/img/avatar-furturia.png?v=28" alt="Furturia" />
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
      liveMapState.markers[device].bindPopup(renderUserPopup(device, point), { className: 'custom-profile-popup', minWidth: 230 });
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

  // Geofences with subtle translucent modern styling
  liveMapState.geofenceLayer.clearLayers();
  for (const zone of data.geofences || []) {
    const circle = L.circle([zone.latitude, zone.longitude], {
      radius: zone.radius_meters,
      color: '#10b981',
      weight: 1.5,
      dashArray: '4 5',
      fillColor: '#10b981',
      fillOpacity: 0.08,
    }).addTo(liveMapState.geofenceLayer);
    circle.bindPopup(`
      <div style="font-family: inherit; font-size: 11px; padding: 2px;">
        <strong style="display:block; font-size: 13px; color: #10b981; margin-bottom: 2px;">📍 ${zone.name}</strong>
        <div>รัศมี: <b>${zone.radius_meters} เมตร</b></div>
        <div style="color: #64748b; font-size: 10px; margin-top: 2px;">${Number(zone.latitude).toFixed(5)}, ${Number(zone.longitude).toFixed(5)}</div>
      </div>
    `, { className: 'custom-profile-popup' });
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
    pickingLocation: false,
    pickerStep: 1,
    pickerError: '',
    radiusSlider: 150,
    geofenceError: '',
    showLogs: false,
    sheetExpanded: false,
    sheetDragStartY: null,
    pin: localStorage.getItem('sentinel-relay-pin') || '',
    showPin: false,
    pinSavedNotice: false,
    theme: localStorage.getItem('sentinel-theme') || 'dark',
    settingsMessage: '',
    savingZone: false,
    expandedGeofenceId: null,
    placeSuggestions: ['🏠 บ้าน', '💼 ที่ทำงาน', '🏫 มหาวิทยาลัย', '🏋️ ฟิตเนส', '☕ ร้านกาแฟ', '🏬 ห้างสรรพสินค้า'],
    settings: { notify_colocation: true, notify_geofence: true, notify_battery: true },
    newZone: { name: '', latitude: null, longitude: null, radius_meters: 150, notify_enter: true, notify_exit: true },

    toggleTheme() {
      if (window.Alpine && Alpine.store && Alpine.store('sentinel')) {
        Alpine.store('sentinel').toggleTheme();
        this.theme = Alpine.store('sentinel').theme;
        return;
      }
      this.theme = this.theme === 'dark' ? 'light' : 'dark';
      document.documentElement.setAttribute('data-theme', this.theme);
      const meta = document.querySelector('meta[name="theme-color"]');
      if (meta) meta.setAttribute('content', this.theme === 'dark' ? '#0c0d10' : '#f5f5f7');
      try { localStorage.setItem('sentinel-theme', this.theme); } catch (_) {}
      window.dispatchEvent(new CustomEvent('sentinel:theme-changed', { detail: { theme: this.theme } }));
    },

    savePinExplicit() {
      if (!this.pin) return;
      try {
        localStorage.setItem('sentinel-relay-pin', this.pin);
        this.pinSavedNotice = true;
        setTimeout(() => { this.pinSavedNotice = false; }, 2500);
      } catch (_) {}
    },

    toggleGeofenceDetails(id) {
      this.expandedGeofenceId = this.expandedGeofenceId === id ? null : id;
    },

    setPlaceSuggestion(suggestion) {
      const cleaned = suggestion.replace(/^[^\w\u0E00-\u0E7F]+\s*/, '').trim();
      this.newZone.name = cleaned;
      this.pickerError = '';
      this.$nextTick(() => document.getElementById('place-name')?.focus());
    },

    toggleSheet(force) {
      this.sheetExpanded = typeof force === 'boolean' ? force : !this.sheetExpanded;
      if (!this.sheetExpanded) {
        const sheet = document.querySelector('.map-sheet');
        if (sheet) sheet.scrollTop = 0;
      }
      this.$nextTick(() => liveMapState.map?.invalidateSize({ pan: false }));
    },

    sheetPointerDown(event) {
      this.sheetDragStartY = event.clientY;
      // Listen on window so a swipe that ends outside the handle still counts.
      const finish = (e) => {
        window.removeEventListener('pointerup', finish);
        window.removeEventListener('pointercancel', finish);
        if (e.type === 'pointercancel') { this.sheetDragStartY = null; return; }
        this.sheetPointerUp(e);
      };
      window.addEventListener('pointerup', finish);
      window.addEventListener('pointercancel', finish);
    },

    sheetPointerUp(event) {
      if (this.sheetDragStartY == null) return;
      const delta = event.clientY - this.sheetDragStartY;
      this.sheetDragStartY = null;
      if (delta < -40) this.toggleSheet(true);
      else if (delta > 40) this.toggleSheet(false);
      else this.toggleSheet();
    },

    friendHeadline() {
      if (!this.friend) return 'ยังไม่มีพิกัดของ Furturia';
      if (this.isColocated) return 'อยู่ด้วยกันกับ Furturia';
      if (this.distanceMeters == null) return 'Furturia · รอตำแหน่งของคุณ';
      return `Furturia ห่าง ${this.distanceText}`;
    },

    friendSubline() {
      if (!this.friend) return 'รอให้เครื่องของ Furturia ส่งตำแหน่งเข้ามา';
      const moving = this.friend.speed >= 1 ? 'กำลังเคลื่อนที่' : 'อยู่นิ่ง';
      return `${this.deviceAge(this.friend)} · ${moving} · แบต ${this.batteryText(this.friend)}`;
    },

    zoomIn() {
      liveMapState.map?.zoomIn();
    },

    zoomOut() {
      liveMapState.map?.zoomOut();
    },

    connectionStatus(point) {
      if (!point || !point.recorded_at) return { label: 'ออฟไลน์', class: 'offline', dot: 'offline' };
      const ageMs = Date.now() - new Date(point.recorded_at).getTime();
      if (ageMs < 5 * 60 * 1000) return { label: 'ออนไลน์', class: 'live', dot: 'live' };
      if (ageMs < 30 * 60 * 1000) return { label: 'พักชั่วคราว', class: 'stale', dot: 'stale' };
      return { label: 'สัญญาณหาย', class: 'offline', dot: 'offline' };
    },

    batteryPercent(point) {
      if (!point || point.battery_level == null || point.battery_level < 0) return 0;
      return Math.max(0, Math.min(100, point.battery_level));
    },

    batteryBarClass(point) {
      const p = this.batteryPercent(point);
      if (p > 25) return 'is-good';
      if (p > 15) return 'is-mid';
      return 'is-low';
    },

    isCharging(point) {
      return point ? (point.battery_status === 2 || point.battery_status === 3) : false;
    },

    init() {
      window.addEventListener('sentinel:theme-changed', (e) => {
        if (e.detail && e.detail.theme && e.detail.theme !== this.theme) {
          this.theme = e.detail.theme;
          document.documentElement.setAttribute('data-theme', this.theme);
        }
      });
      window.addEventListener('sentinel:tab-changed', (e) => {
        if (e.detail && e.detail.tab === 'map') {
          setTimeout(() => {
            if (liveMapState.map) {
              liveMapState.map.invalidateSize();
            }
          }, 80);
        }
      });
      const canvasEl = document.getElementById('live-map-canvas');
      if (!window.L || !canvasEl) {
        if (!window.L) this.mapError = 'โหลดแผนที่ไม่สำเร็จ กรุณาตรวจสอบอินเทอร์เน็ต';
      } else if (!liveMapState.map) {
        liveMapState.map = L.map('live-map-canvas', { zoomControl: false, attributionControl: false, preferCanvas: true }).setView([13.6516, 100.4939], 15);
        L.control.attribution({ position: 'topleft', prefix: false }).addTo(liveMapState.map);

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

        // While choosing a place, the center reticle stays fixed and the
        // selected coordinate follows the map after each pan/zoom.
        liveMapState.map.on('moveend', () => {
          if (!this.pickingLocation) return;
          this.setPickerCoordinates(liveMapState.map.getCenter());
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
      let token = locationViewerToken();
      if (!token) {
        try {
          const resp = await fetch('/api/location/viewer-token');
          if (resp.ok) {
            const data = await resp.json();
            if (data.token) {
              token = data.token;
              try { localStorage.setItem('sentinel_viewer_token', token); } catch (_) {}
            }
          }
        } catch (_) {}
      }
      try {
        const response = await fetch('/api/location/live', {
          headers: { 'X-Location-Viewer-Token': token, Accept: 'application/json' },
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
        // Default view: frame both people once, as soon as the first coordinates arrive.
        if (!liveMapState.didInitialFit && (this.me || this.friend) && !this.pickingLocation) {
          liveMapState.didInitialFit = true;
          this.centerOnDevices();
        }
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

    get orderedGeofences() {
      return [...this.geofences].sort((a, b) => String(a.name).localeCompare(String(b.name), 'th', { numeric: true, sensitivity: 'base' }));
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
      // Keep both pins clear of the floating status chip/controls and the bottom sheet.
      const sheet = document.querySelector('.map-sheet');
      const sheetTop = sheet ? sheet.getBoundingClientRect().top : window.innerHeight;
      const bottomCover = Math.max(0, window.innerHeight - sheetTop);
      liveMapState.map.fitBounds(L.latLngBounds(coords), {
        paddingTopLeft: [40, 90],
        paddingBottomRight: [72, bottomCover + 40],
        maxZoom: 16,
      });
    },

    toggleTrail() {
      this.showTrail = !this.showTrail;
      updateLeafletLayers({ me: this.me, friend: this.friend, geofences: this.geofences, breadcrumbs: this.breadcrumbs, distance_meters: this.distanceMeters, is_colocated: this.isColocated }, this.showTrail);
    },

    get pickerStepTitle() {
      return ['เลือกตำแหน่งและปรับรัศมี', 'ตั้งชื่อสถานที่', 'การแจ้งเตือนและบันทึก'][this.pickerStep - 1] || 'เพิ่มสถานที่';
    },

    // Guided map-first place picker.
    startMapPicker() {
      if (this.pickingLocation) return;
      liveMapState.pickerOpener = document.activeElement;
      this.newZone = { name: '', latitude: null, longitude: null, radius_meters: 150, notify_enter: true, notify_exit: true };
      this.radiusSlider = 150;
      this.pickerStep = 1;
      this.pickerError = '';
      this.geofenceError = '';
      this.pickingLocation = true;
      this.sheetExpanded = false;
      const container = liveMapState.map?.getContainer();
      if (container) container.classList.add('map-picking-cursor');

      const center = this.me
        ? { lat: this.me.latitude, lng: this.me.longitude }
        : liveMapState.map?.getCenter() || { lat: 13.6516, lng: 100.4939 };
      this.setPickerCoordinates(center);
      liveMapState.map?.setView(center, this.me ? 16 : liveMapState.map.getZoom());
      this.renderPickerPreview();
      this.$nextTick(() => {
        liveMapState.map?.invalidateSize({ pan: false });
        document.getElementById('picker-title')?.focus();
      });
    },

    toggleMapPicker() {
      if (this.pickingLocation) {
        this.cancelMapPicker();
      } else {
        this.startMapPicker();
      }
    },

    setPickerCoordinates(latLng) {
      if (!latLng || !Number.isFinite(latLng.lat) || !Number.isFinite(latLng.lng)) return;
      this.newZone.latitude = Number(latLng.lat.toFixed(6));
      this.newZone.longitude = Number(latLng.lng.toFixed(6));
    },

    useDeviceLocation(device) {
      const point = device === 'friend' ? this.friend : this.me;
      if (!point || !liveMapState.map) return;
      const center = { lat: point.latitude, lng: point.longitude };
      this.setPickerCoordinates(center);
      liveMapState.map.setView(center, 17);
      this.renderPickerPreview();
      this.pickerError = '';
    },

    nextPickerStep() {
      this.pickerError = '';
      if (this.pickerStep === 1) {
        if (!Number.isFinite(this.newZone.latitude) || !Number.isFinite(this.newZone.longitude)) {
          this.pickerError = 'กรุณาเลื่อนแผนที่ให้จุดศูนย์กลางอยู่ใต้เป้า หรือเลือกตำแหน่งอุปกรณ์';
          return;
        }
        const radius = Number(this.newZone.radius_meters);
        if (!Number.isFinite(radius) || radius < PICKER_RADIUS_MIN || radius > PICKER_RADIUS_MAX) {
          this.pickerError = `รัศมีต้องอยู่ระหว่าง ${PICKER_RADIUS_MIN} ถึง ${PICKER_RADIUS_MAX} เมตร`;
          return;
        }
        this.pickerStep = 2;
        this.$nextTick(() => {
          document.getElementById('place-name')?.focus();
        });
        return;
      }
      if (this.pickerStep === 2) {
        if (!this.newZone.name?.trim()) {
          this.pickerError = 'กรุณาระบุชื่อสถานที่เพื่อให้จำได้ง่าย';
          document.getElementById('place-name')?.focus();
          return;
        }
        this.newZone.name = this.newZone.name.trim();
        this.pickerStep = 3;
        return;
      }
    },

    previousPickerStep() {
      this.pickerError = '';
      this.pickerStep = Math.max(1, this.pickerStep - 1);
    },

    renderPickerPreview() {
      if (!liveMapState.map || !Number.isFinite(this.newZone.latitude) || !Number.isFinite(this.newZone.longitude)) return;
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

    },

    updatePickerRadius() {
      const radius = Number(this.newZone.radius_meters);
      if (liveMapState.pickerCircle && Number.isFinite(radius) && radius >= PICKER_RADIUS_MIN && radius <= PICKER_RADIUS_MAX) liveMapState.pickerCircle.setRadius(radius);
    },

    setRadius(r) {
      this.newZone.radius_meters = r;
      this.radiusSlider = Math.max(PICKER_SLIDER_MIN, Math.min(PICKER_SLIDER_MAX, r));
      this.updatePickerRadius();
    },

    applyRadiusSlider() {
      this.newZone.radius_meters = Number(this.radiusSlider);
      this.pickerError = '';
      this.updatePickerRadius();
    },

    applyExactRadius() {
      const radius = Number(this.newZone.radius_meters);
      if (!Number.isFinite(radius)) return;
      this.radiusSlider = Math.max(PICKER_SLIDER_MIN, Math.min(PICKER_SLIDER_MAX, radius));
      this.pickerError = '';
      this.updatePickerRadius();
    },

    normalizeExactRadius() {
      if (this.newZone.radius_meters === '' || this.newZone.radius_meters == null) return;
      const radius = Number(this.newZone.radius_meters);
      if (!Number.isFinite(radius) || radius < PICKER_RADIUS_MIN || radius > PICKER_RADIUS_MAX) {
        this.pickerError = `ใส่รัศมีระหว่าง ${PICKER_RADIUS_MIN} ถึง ${PICKER_RADIUS_MAX} เมตร`;
        return;
      }
      this.pickerError = '';
      this.newZone.radius_meters = Math.round(radius);
      this.applyExactRadius();
    },

    cancelMapPicker() {
      this.pickingLocation = false;
      this.pickerStep = 1;
      this.pickerError = '';
      const container = liveMapState.map?.getContainer();
      if (container) container.classList.remove('map-picking-cursor');
      if (liveMapState.pickerLayer) {
        liveMapState.pickerLayer.clearLayers();
      }
      this.$nextTick(() => {
        liveMapState.map?.invalidateSize({ pan: false });
        liveMapState.pickerOpener?.focus?.();
        liveMapState.pickerOpener = null;
      });
      this.radiusSlider = 150;
      this.newZone = { name: '', latitude: null, longitude: null, radius_meters: 150, notify_enter: true, notify_exit: true };
    },

    async saveSettings() {
      const pin = this.pin || localStorage.getItem('sentinel-relay-pin') || '1804';
      if (!pin) {
        this.settingsMessage = 'กรุณากรอก Relay PIN';
        return;
      }
      try {
        const response = await fetch('/api/location/settings', {
          method: 'PATCH',
          headers: { 'Content-Type': 'application/json', 'X-Relay-PIN': pin },
          body: JSON.stringify({
            notify_colocation: this.settings.notify_colocation,
            notify_geofence: this.settings.notify_geofence,
            notify_battery: this.settings.notify_battery,
          }),
        });
        if (!response.ok) throw new Error('บันทึกไม่สำเร็จ ตรวจสอบ PIN');
        this.settings = { ...this.settings, ...(await response.json()) };
        this.pin = pin;
        localStorage.setItem('sentinel-relay-pin', pin);
        this.settingsMessage = '✅ บันทึกการตั้งค่าแล้ว';
        setTimeout(() => { this.settingsMessage = ''; }, 3000);
      } catch (error) {
        this.settingsMessage = `❌ ${error?.message || 'บันทึกไม่สำเร็จ'}`;
      }
    },

    async createGeofence() {
      const pin = this.pin || localStorage.getItem('sentinel-relay-pin') || '1804';
      if (!this.newZone.name?.trim() || !Number.isFinite(this.newZone.latitude) || !Number.isFinite(this.newZone.longitude) ||
          !Number.isFinite(Number(this.newZone.radius_meters)) || Number(this.newZone.radius_meters) < PICKER_RADIUS_MIN || Number(this.newZone.radius_meters) > PICKER_RADIUS_MAX) {
        this.pickerError = 'ตรวจชื่อสถานที่ ตำแหน่ง และรัศมีก่อนบันทึก';
        return;
      }
      this.savingZone = true;
      this.pickerError = '';
      try {
        const response = await fetch('/api/location/geofences', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Relay-PIN': pin },
          body: JSON.stringify(this.newZone),
        });
        if (!response.ok) {
          const errData = await response.json().catch(() => ({}));
          throw new Error(errData.error || 'เพิ่มสถานที่ไม่สำเร็จ ตรวจสอบ PIN');
        }
        this.pin = pin;
        localStorage.setItem('sentinel-relay-pin', pin);
        this.settingsMessage = 'บันทึกสถานที่สำเร็จ';
        this.cancelMapPicker();
        await this.refresh();
        setTimeout(() => { this.settingsMessage = ''; }, 3500);
      } catch (error) {
        this.pickerError = error?.message || 'เพิ่มสถานที่ไม่สำเร็จ';
      } finally {
        this.savingZone = false;
      }
    },

    focusGeofence(zone) {
      if (!zone || !liveMapState.map) return;
      if (this.pickingLocation) this.cancelMapPicker();
      const bounds = L.circle([zone.latitude, zone.longitude], { radius: zone.radius_meters }).getBounds();
      liveMapState.map.fitBounds(bounds.pad(0.2), { maxZoom: 18, animate: true });
      this.geofenceError = '';
      document.getElementById('live-map-canvas')?.scrollIntoView({ behavior: 'smooth', block: 'center' });
    },

    async deleteGeofence(id) {
      const pin = this.pin || localStorage.getItem('sentinel-relay-pin') || '1804';
      if (!pin) {
        this.geofenceError = 'กรุณากรอก Relay PIN ก่อนลบสถานที่';
        return;
      }
      if (!window.confirm('คุณต้องการลบสถานที่นี้ใช่หรือไม่?')) return;
      this.geofenceError = '';
      try {
        const response = await fetch(`/api/location/geofences/${encodeURIComponent(id)}`, {
          method: 'DELETE',
          headers: { 'X-Relay-PIN': pin },
        });
        if (!response.ok) throw new Error('ลบไม่สำเร็จ ตรวจสอบ PIN');
        this.pin = pin;
        localStorage.setItem('sentinel-relay-pin', pin);
        this.settingsMessage = '✅ ลบสถานที่สำเร็จ';
        await this.refresh();
        setTimeout(() => { this.settingsMessage = ''; }, 3000);
      } catch (error) {
        this.geofenceError = error?.message || 'ลบไม่สำเร็จ';
      }
    },
  }));
});
