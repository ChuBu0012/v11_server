---
name: spa-reverse-proxy
description: Best practices and strict guardrails for reverse-proxying Single Page Applications (Vue, React, Svelte) under subpaths (e.g. /status/), eliminating router base mismatches, handling root asset routing, and preventing PWA Service Worker caching collisions.
---

# SPA Subpath Reverse Proxy & Service Worker Guardrails

When mounting an external Single Page Application (e.g. Gatus, Grafana, custom Vue/React/Svelte dashboards) under a subpath (such as `/status/`) via a Go reverse proxy, you **MUST** follow these guardrails to avoid silent rendering failures, 404 asset cascades, and Service Worker collisions.

---

## 1. The Silent Router Trap (Base Path Mismatch)

### Symptom
The page loads its static shell (header, logo container, footer), but the main body content, dashboard cards, and views are **completely blank**. No errors appear in server logs, and the API endpoints return 200 OK.

### Root Cause
Production SPAs are almost always bundled with a hardcoded root router history:
- Vue Router: `history: createWebHistory("/")`
- React Router: `<BrowserRouter basename="/">`

When the browser visits `https://domain.com/status/`:
1. `window.location.pathname` is `/status/`.
2. The router evaluates `/status/` against its route definitions (`/`, `/endpoints/:key`, etc.).
3. Because the base is `"/"`, `/status/` matches **none** of the routes.
4. `<router-view>` or `<Outlet />` renders **nothing**, causing an empty white/dark box.

### Solution: Dynamic History Base
Patch the router initialization to detect the subpath dynamically:
```javascript
// Before (breaks on subpath):
history: createWebHistory("/")

// After (supports both direct root access and subpath reverse proxy):
history: createWebHistory(location.pathname.startsWith("/status") ? "/status/" : "/")
```
This guarantees:
- Access via subpath reverse proxy (`/status/`): Router uses base `/status/`, matches route `/`, and renders views.
- Direct root access (e.g. private Tailscale port `:8081/`): Router uses base `/`, functioning normally.

---

## 2. Root Asset 404 Cascade

### Symptom
Browser console shows 404 for assets:
- `GET /img/logo.svg 404 (Not Found)`
- `GET /favicon.ico 404 (Not Found)`
- Missing JS chunks, CSS files, or SVG badges.

### Root Cause
SPAs are built with Webpack/Vite `publicPath: "/"`. Even when the HTML page is delivered from `/status/`, the browser parses HTML/JS and requests assets relative to host root (`/img/*`, `/js/*`, `/css/*`, `/favicon.ico`, `/manifest.json`, and `/api/v1/*`).

### Solution: Explicit Route Proxying
In the reverse proxy server (`main.go`), you **must** forward both the subpath and the root asset prefixes required by the SPA:

```go
// 1. The main subpath (with trailing slash redirect)
r.GET("/status", func(c *gin.Context) {
    c.Redirect(http.StatusMovedPermanently, "/status/")
})
r.Any("/status/*path", func(c *gin.Context) {
    c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
    gatusProxy.ServeHTTP(c.Writer, c.Request)
})

// 2. SPA static asset bundles and API prefixes
r.Any("/api/v1/*path", func(c *gin.Context) {
    gatusProxy.ServeHTTP(c.Writer, c.Request)
})
r.Any("/js/*path", func(c *gin.Context) {
    c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
    gatusProxy.ServeHTTP(c.Writer, c.Request)
})
r.Any("/css/*path", func(c *gin.Context) {
    gatusProxy.ServeHTTP(c.Writer, c.Request)
})
r.Any("/img/*path", func(c *gin.Context) {
    gatusProxy.ServeHTTP(c.Writer, c.Request)
})
for _, p := range []string{"/favicon.ico", "/favicon-16x16.png", "/favicon-32x32.png", "/apple-touch-icon.png"} {
    r.Any(p, func(c *gin.Context) {
        gatusProxy.ServeHTTP(c.Writer, c.Request)
    })
}
```

---

## 3. PWA Service Worker Hijack

### Symptom
- Fixed assets still return broken images or 404s after deploy.
- Subpath page serves the parent PWA shell offline fallback instead of the proxy content.
- Browser keeps executing old cached `app.js`.

### Root Cause
If the parent application runs a PWA Service Worker (e.g. `modules/dashboard/sw.js`), it intercepts **every** network request under the origin. Any successful or failed GET request may be cached into the PWA cache storage.

### Solution: Strict Service Worker Bypass & Cache Invalidation
1. **Bypass the proxied paths in `sw.js`:**
   ```javascript
   // Pass all proxied subpath and external asset requests directly through network
   if (
     url.pathname.startsWith('/status') ||
     url.pathname.startsWith('/js/') ||
     url.pathname.startsWith('/css/') ||
     url.pathname.startsWith('/img/') ||
     url.pathname.startsWith('/api/') ||
     url.pathname.startsWith('/health') ||
     url.pathname.startsWith('/live/')
   ) {
     event.respondWith(fetch(event.request));
     return;
   }
   ```
2. **Bump `CACHE_NAME` in `sw.js`:**
   Increment the cache name (e.g. `sentinel-shell-v41` -> `sentinel-shell-v42`) so outdated caches are deleted in `self.addEventListener('activate')`.
3. **Send `no-cache` HTTP headers on HTML & JS proxy routes:**
   ```go
   c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
   ```

---

## 4. Go Integration Test Guardrail: `http.CloseNotifier` Panic

### Symptom
Unit or integration tests testing `httputil.ReverseProxy` fail with:
```text
panic: interface conversion: *httptest.ResponseRecorder is not http.CloseNotifier: missing method CloseNotify
```

### Root Cause
Go's `httputil.ReverseProxy.ServeHTTP` checks if `ResponseWriter` implements `http.CloseNotifier`. Gin wraps `ResponseWriter` with `gin.responseWriter`, which unconditionally type-asserts to `http.CloseNotifier`. Standard `*httptest.ResponseRecorder` does **not** implement `CloseNotify()`, causing a runtime panic during testing.

### Solution: Test CloseNotifier Recorder
Wrap `httptest.ResponseRecorder` with a struct implementing `CloseNotify()`:
```go
type testCloseNotifierRecorder struct {
    *httptest.ResponseRecorder
    closed chan bool
}

func (cn *testCloseNotifierRecorder) CloseNotify() <-chan bool {
    return cn.closed
}

// In test:
rec := httptest.NewRecorder()
cnRec := &testCloseNotifierRecorder{
    ResponseRecorder: rec,
    closed:           make(chan bool, 1),
}
router.ServeHTTP(cnRec, req)
```
