---
name: frontend-proxy-guard
description: Best practices and strict architectural patterns for integrating Three.js, Chart.js, and external canvas/WebGL libraries with Alpine.js reactive proxies, eliminating non-configurable property TypeErrors, handling hidden canvas resize lifecycles, and managing PWA Service Worker cache invalidation.
---

# Frontend Proxy Guard: Alpine.js + Three.js & Chart.js Architecture

When building or modifying embedded web dashboards that combine Alpine.js with complex visual libraries (such as Three.js, Chart.js, or WebGL engines), you **MUST** follow these patterns to prevent runtime crashes and silent rendering failures.

---

## 1. The Reactive Proxy Trap

### Root Cause
Alpine.js (like Vue 3) automatically wraps all properties returned inside `Alpine.data('component', () => ({ ... }))` in an ECMAScript `Proxy`. 

Complex libraries like Three.js and Chart.js contain objects with non-configurable, read-only internal properties or prototypes (e.g., `Object3D.prototype.modelViewMatrix`, `CanvasRenderingContext2D`, or private internal symbols).

When JavaScript code accesses a non-configurable property through a Proxy whose target doesn't return the exact invariant, the browser's JavaScript engine throws an unrecoverable exception:
```text
Uncaught TypeError: 'get' on proxy: property 'modelViewMatrix' is a read-only and non-configurable data property on the proxy target but the proxy did not return its actual value
```
In Chart.js, storing `new Chart()` instances inside reactive component state causes `chart.update()` and internal event listeners to fail, silently breaking chart rendering and resizing.

---

## 2. The Outer Closure Pattern (Strict Rule)

**NEVER** place Three.js objects or Chart.js instances inside the object returned by `Alpine.data()`.

### Three.js Correct Pattern
Keep all Three.js scene, camera, renderer, materials, and mesh references in an outer closure outside the `Alpine.data` definition:

```javascript
document.addEventListener('alpine:init', () => {
  // 1. Keep Three.js state strictly outside of Alpine's reactive Proxy
  const tc = {
    scene: null,
    camera: null,
    renderer: null,
    controls: null,
    mesh: null,
    animId: null
  };

  Alpine.data('threeCoreComponent', () => ({
    // 2. Only expose UI state, labels, and primitive values to Alpine
    initialized: false,
    statusText: 'Standby',

    init() {
      tc.scene = new THREE.Scene();
      tc.camera = new THREE.PerspectiveCamera(...);
      tc.renderer = new THREE.WebGLRenderer({ canvas: document.getElementById('my-canvas') });
      ...
    }
  }));
});
```

### Chart.js Correct Pattern
Store chart instances in an outer mapping object, instantiate them on demand, and only expose summary text and boolean sample flags to Alpine:

```javascript
document.addEventListener('alpine:init', () => {
  // Outer chart instances mapping
  const chartInstances = {
    ram: null,
    disk: null
  };

  function getOrCreateChart(key, canvasId, config) {
    if (chartInstances[key]) return chartInstances[key];
    const el = document.getElementById(canvasId);
    if (!el || !window.Chart) return null;
    chartInstances[key] = new Chart(el, config);
    return chartInstances[key];
  }

  Alpine.data('chartsComponent', () => ({
    hasEnoughSamples: { ram: false, disk: false },
    currentValues: { ram: 'No data', disk: 'No data' },

    updateChart(key, samples) {
      const enough = samples.length >= 2;
      this.hasEnoughSamples[key] = enough;

      if (enough) {
        const chart = getOrCreateChart(key, `${key}-chart`, ...);
        if (chart) {
          chart.data.labels = samples.map(s => s.time);
          chart.data.datasets[0].data = samples.map(s => s.value);
          chart.update('none');

          // Ensure canvas renders correctly after unhiding
          if (this.$nextTick) {
            this.$nextTick(() => { try { chart.resize(); } catch (_) {} });
          }
        }
      }
    }
  }));
});
```

---

## 3. Canvas Visibility & Resize Lifecycle

1. **Initial Hidden State:**
   If canvas elements start with `:hidden="!hasEnoughSamples"` or `x-show`, their initial display dimension is `0x0`.
2. **Post-Unhide Recalculation:**
   Chart.js needs to know its parent container's dimensions once the canvas is revealed. Whenever `hasEnoughSamples` transitions to `true`, always trigger `chart.resize()` inside `this.$nextTick()`:
   ```javascript
   this.$nextTick(() => {
     try {
       chart.resize();
     } catch (_) {}
   });
   ```

---

## 4. Preventing Ephemeral Event Race Conditions

Do **not** rely solely on ephemeral DOM CustomEvents (e.g. `sentinel:history-loaded`) for critical initial rendering. If an async store or HTTP fetch finishes before the Alpine component's `init()` runs, the event is lost.

Always provide an explicit fetch fallback inside `init()`:
```javascript
init() {
  // 1. Listen for background updates
  window.addEventListener('sentinel:history-loaded', (e) => this.process(e.detail));

  // 2. Direct fallback in case event was already dispatched before mounting
  this.loadHistory();
}
```

---

## 5. PWA Service Worker Cache Invalidation

When frontend assets are embedded directly into a compiled Go binary (`//go:embed`):
- Browsers and installed PWAs will aggressively cache static assets defined in `sw.js`.
- Whenever modifying any frontend file in `static/js/`, `static/css/`, or `index.html`, **ALWAYS** increment the cache version in `sw.js`:
  ```javascript
  const CACHE_NAME = 'sentinel-shell-v3'; // Bumped from v2
  ```
- This triggers the service worker `activate` event, which purges old caches (`caches.delete(key)`) and claims clients, guaranteeing users see immediate fixes without manual browser cache clearing.
