package location

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Errors returned by LinkResolver.Resolve.
var (
	ErrLinkNotSupported  = errors.New("only Google Maps links or lat,lng coordinates are supported")
	ErrLinkNoCoordinates = errors.New("link does not contain coordinates")
)

const (
	linkResolveTimeout = 6 * time.Second
	linkMaxRedirects   = 5
	linkMaxBodyBytes   = 64 * 1024
)

// Hosts a short link may pass through. Anything else stops the redirect
// chain before a request is made, so the resolver cannot be aimed elsewhere.
var allowedLinkHosts = map[string]bool{
	"maps.app.goo.gl":    true,
	"goo.gl":             true,
	"www.google.com":     true,
	"google.com":         true,
	"maps.google.com":    true,
	"consent.google.com": true,
}

var (
	plainCoordsPattern = regexp.MustCompile(`^\s*(-?\d{1,3}(?:\.\d+)?)\s*,\s*(-?\d{1,3}(?:\.\d+)?)\s*$`)
	placePinPattern    = regexp.MustCompile(`!3d(-?\d{1,3}\.\d+)!4d(-?\d{1,3}\.\d+)`)
	viewportPattern    = regexp.MustCompile(`@(-?\d{1,3}\.\d+),(-?\d{1,3}\.\d+)`)
	coordQueryKeys     = []string{"q", "query", "ll", "destination", "center"}
)

// ParseMapCoordinates extracts a latitude/longitude from plain "lat, lng"
// text or from a full Google Maps URL. The place pin (!3d…!4d…) wins over the
// viewport centre (@lat,lng) because it is where the place actually is.
func ParseMapCoordinates(raw string) (lat, lng float64, ok bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, false
	}
	if m := plainCoordsPattern.FindStringSubmatch(raw); m != nil {
		return validPair(m[1], m[2])
	}
	if lat, lng, ok := coordsFromText(raw); ok {
		return lat, lng, true
	}
	if u, err := url.Parse(raw); err == nil {
		query := u.Query()
		for _, key := range coordQueryKeys {
			if m := plainCoordsPattern.FindStringSubmatch(query.Get(key)); m != nil {
				if lat, lng, ok := validPair(m[1], m[2]); ok {
					return lat, lng, true
				}
			}
		}
	}
	return 0, 0, false
}

func coordsFromText(text string) (float64, float64, bool) {
	for _, pattern := range []*regexp.Regexp{placePinPattern, viewportPattern} {
		if m := pattern.FindStringSubmatch(text); m != nil {
			if lat, lng, ok := validPair(m[1], m[2]); ok {
				return lat, lng, true
			}
		}
	}
	return 0, 0, false
}

func validPair(latText, lngText string) (float64, float64, bool) {
	lat, err1 := strconv.ParseFloat(latText, 64)
	lng, err2 := strconv.ParseFloat(lngText, 64)
	if err1 != nil || err2 != nil || !ValidCoordinates(lat, lng) {
		return 0, 0, false
	}
	return lat, lng, true
}

// LinkResolver turns Google Maps share links into coordinates.
type LinkResolver struct {
	client *http.Client
}

// NewLinkResolver returns a resolver that uses the default network transport.
func NewLinkResolver() *LinkResolver { return newLinkResolver(nil) }

func newLinkResolver(transport http.RoundTripper) *LinkResolver {
	return &LinkResolver{client: &http.Client{
		Timeout:   linkResolveTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= linkMaxRedirects {
				return fmt.Errorf("too many redirects: %w", ErrLinkNotSupported)
			}
			if !allowedHop(req.URL) {
				return ErrLinkNotSupported
			}
			return nil
		},
	}}
}

func allowedHop(u *url.URL) bool {
	return u != nil && u.Scheme == "https" && allowedLinkHosts[strings.ToLower(u.Hostname())]
}

func isShortLink(u *url.URL) bool {
	if u == nil || u.Scheme != "https" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "maps.app.goo.gl" || (host == "goo.gl" && strings.HasPrefix(u.Path, "/maps"))
}

// Resolve parses coordinates locally when possible and only follows short
// links (maps.app.goo.gl, goo.gl/maps) through the Google host allowlist.
func (r *LinkResolver) Resolve(ctx context.Context, raw string) (float64, float64, error) {
	raw = strings.TrimSpace(raw)
	if lat, lng, ok := ParseMapCoordinates(raw); ok {
		return lat, lng, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return 0, 0, ErrLinkNotSupported
	}
	if !isShortLink(u) {
		if host := strings.ToLower(u.Hostname()); u.Scheme == "https" && (host == "www.google.com" || host == "google.com" || host == "maps.google.com") {
			return 0, 0, ErrLinkNoCoordinates // a full Google Maps URL without coordinates
		}
		return 0, 0, ErrLinkNotSupported
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, 0, ErrLinkNotSupported
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (PocketSentinel link resolver)")
	resp, err := r.client.Do(req)
	if err != nil {
		if errors.Is(err, ErrLinkNotSupported) {
			return 0, 0, ErrLinkNotSupported
		}
		return 0, 0, fmt.Errorf("open link: %w", err)
	}
	defer resp.Body.Close()
	if resp.Request != nil && resp.Request.URL != nil {
		if lat, lng, ok := ParseMapCoordinates(resp.Request.URL.String()); ok {
			return lat, lng, nil
		}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, linkMaxBodyBytes))
	if lat, lng, ok := coordsFromText(string(body)); ok {
		return lat, lng, nil
	}
	return 0, 0, ErrLinkNoCoordinates
}
