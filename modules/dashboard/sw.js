// Pocket Sentinel PWA Service Worker
const CACHE_NAME = 'sentinel-shell-v35';
const SHELL_ASSETS = [
  '/',
  '/tele-bridge',
  '/relay',
  '/schedule',
  '/manifest.json',
  '/icon-192.png',
  '/icon-512.png',
  '/icon.svg',
  '/static/css/styles.css',
  '/static/css/dashboard.css',
  '/static/css/schedule.css',
  '/static/css/relay.css',
  '/static/js/app.js',
  '/static/js/components/telemetry.js',
  '/static/js/components/charts.js',
  '/static/js/components/digest.js',
  '/static/js/components/events.js',
  '/static/js/components/relay.js',
  '/static/js/components/schedule-summary.js',
  '/static/js/components/schedule.js',
  '/static/img/avatar-chubu.png',
  '/static/img/avatar-furturia.png'
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => {
      return cache.addAll(SHELL_ASSETS);
    }).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) => {
      return Promise.all(
        keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key))
      );
    }).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const url = new URL(event.request.url);

  // Pass all API and data requests directly through network
  if (url.pathname.startsWith('/api/') || url.pathname.startsWith('/health') || url.pathname.startsWith('/live/')) {
    event.respondWith(fetch(event.request));
    return;
  }

  // Network-first with cache fallback for HTML and assets
  event.respondWith(
    fetch(event.request)
      .then((networkResponse) => {
        if (networkResponse && networkResponse.status === 200 && event.request.method === 'GET') {
          const responseClone = networkResponse.clone();
          caches.open(CACHE_NAME).then((cache) => {
            cache.put(event.request, responseClone);
          });
        }
        return networkResponse;
      })
      .catch(() => {
        return caches.match(event.request).then((cachedResponse) => {
          if (cachedResponse) return cachedResponse;
          if (event.request.headers.get('accept')?.includes('text/html')) {
            return caches.match('/');
          }
          return new Response('Network unavailable', { status: 503, statusText: 'Offline' });
        });
      })
  );
});
