package location

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestParseMapCoordinates(t *testing.T) {
	cases := []struct {
		in       string
		ok       bool
		lat, lng float64
	}{
		{"13.7563, 100.5018", true, 13.7563, 100.5018},
		{" -33.8688,151.2093 ", true, -33.8688, 151.2093},
		{"https://www.google.com/maps/place/X/@13.7563,100.5018,17z", true, 13.7563, 100.5018},
		{"https://www.google.com/maps/place/X/@13.70,100.40,17z/data=!3m1!4b1!4m6!3m5!1s0x0:0x0!8m2!3d13.7563!4d100.5018", true, 13.7563, 100.5018},
		{"https://maps.google.com/?q=13.7563,100.5018", true, 13.7563, 100.5018},
		{"https://www.google.com/maps/dir/?api=1&destination=13.7563,100.5018", true, 13.7563, 100.5018},
		{"hello", false, 0, 0},
		{"91, 10", false, 0, 0},
		{"10, 181", false, 0, 0},
		{"https://www.google.com/maps?q=Siam+Paragon", false, 0, 0},
	}
	for _, tc := range cases {
		lat, lng, ok := ParseMapCoordinates(tc.in)
		if ok != tc.ok || (ok && (lat != tc.lat || lng != tc.lng)) {
			t.Errorf("ParseMapCoordinates(%q) = %v,%v,%v want %v,%v,%v", tc.in, lat, lng, ok, tc.lat, tc.lng, tc.ok)
		}
	}
}

// hostRouter sends requests for real hostnames to local test handlers so the
// resolver's allowlist sees the real hosts while no network is touched.
type hostRouter struct {
	handlers map[string]http.Handler
	hits     map[string]*int32
}

func (h *hostRouter) RoundTrip(req *http.Request) (*http.Response, error) {
	handler, ok := h.handlers[req.URL.Host]
	if !ok {
		return nil, errors.New("unexpected host " + req.URL.Host)
	}
	if counter := h.hits[req.URL.Host]; counter != nil {
		atomic.AddInt32(counter, 1)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	resp := rr.Result()
	resp.Request = req
	return resp, nil
}

func redirectTo(location string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, location, http.StatusFound)
	})
}

func TestResolveShortLinkFollowsGoogleRedirect(t *testing.T) {
	router := &hostRouter{handlers: map[string]http.Handler{
		"maps.app.goo.gl": redirectTo("https://www.google.com/maps/place/X/@13.70,100.40,17z/data=!3d13.7563!4d100.5018"),
		"www.google.com":  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) }),
	}}
	lat, lng, err := newLinkResolver(router).Resolve(context.Background(), "https://maps.app.goo.gl/abc123")
	if err != nil || lat != 13.7563 || lng != 100.5018 {
		t.Fatalf("resolve = %v,%v err=%v", lat, lng, err)
	}
}

func TestResolveLinkWithoutCoordinates(t *testing.T) {
	router := &hostRouter{handlers: map[string]http.Handler{
		"maps.app.goo.gl": redirectTo("https://www.google.com/maps?q=Siam+Paragon"),
		"www.google.com":  http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>no coords</html>")) }),
	}}
	if _, _, err := newLinkResolver(router).Resolve(context.Background(), "https://maps.app.goo.gl/abc123"); !errors.Is(err, ErrLinkNoCoordinates) {
		t.Fatalf("err = %v, want ErrLinkNoCoordinates", err)
	}
}

func TestResolveRejectsOffListRedirect(t *testing.T) {
	var evilHits int32
	router := &hostRouter{
		handlers: map[string]http.Handler{
			"maps.app.goo.gl": redirectTo("https://evil.example/@13.7,100.5"),
			"evil.example":    http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("pwned")) }),
		},
		hits: map[string]*int32{"evil.example": &evilHits},
	}
	if _, _, err := newLinkResolver(router).Resolve(context.Background(), "https://maps.app.goo.gl/abc123"); !errors.Is(err, ErrLinkNotSupported) {
		t.Fatalf("err = %v, want ErrLinkNotSupported", err)
	}
	if atomic.LoadInt32(&evilHits) != 0 {
		t.Fatalf("off-list host was contacted %d times", evilHits)
	}
}

func TestResolveRejectsUnsupportedHost(t *testing.T) {
	var hits int32
	router := &hostRouter{handlers: map[string]http.Handler{"example.com": redirectTo("https://www.google.com/")}, hits: map[string]*int32{"example.com": &hits}}
	for _, raw := range []string{"https://example.com/x", "http://maps.app.goo.gl/abc", "https://goo.gl/notmaps"} {
		if _, _, err := newLinkResolver(router).Resolve(context.Background(), raw); !errors.Is(err, ErrLinkNotSupported) {
			t.Errorf("%s: err = %v, want ErrLinkNotSupported", raw, err)
		}
	}
	if hits != 0 {
		t.Fatalf("unsupported host contacted %d times", hits)
	}
}

func TestResolveLinkEndpoint(t *testing.T) {
	router, _, _, _ := newLocationTestRouter(t)
	post := func(pin, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/location/resolve-link", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Relay-PIN", pin)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		return rr
	}
	if rr := post("wrong", `{"url":"13.7,100.5"}`); rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong PIN = %d", rr.Code)
	}
	if rr := post("1804", `{"url":"13.7563, 100.5018"}`); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"latitude":13.7563`) {
		t.Fatalf("plain coords = %d %s", rr.Code, rr.Body.String())
	}
	if rr := post("1804", `{"url":"https://example.com/`+url.PathEscape("x")+`"}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("unsupported host = %d %s", rr.Code, rr.Body.String())
	}
	if rr := post("1804", `{"url":""}`); rr.Code != http.StatusBadRequest {
		t.Fatalf("empty = %d", rr.Code)
	}
}
