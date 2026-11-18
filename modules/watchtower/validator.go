package watchtower

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

var (
	ErrForbiddenIP    = errors.New("forbidden IP address: private, loopback, or reserved range")
	ErrInvalidScheme  = errors.New("invalid scheme: must be http or https")
	ErrEmptyHost      = errors.New("host cannot be empty")
	ErrHasCredentials = errors.New("url must not contain username or password")
	ErrLocalhost      = errors.New("localhost is forbidden")
)

// List of forbidden CIDR networks (IPv4 and IPv6).
var forbiddenCIDRs = []*net.IPNet{
	// IPv4 Special / Private / Reserved
	mustParseCIDR("0.0.0.0/8"),          // Current network (RFC 1122)
	mustParseCIDR("10.0.0.0/8"),         // Private network (RFC 1918)
	mustParseCIDR("100.64.0.0/10"),      // Shared Address / CGNAT (RFC 6598)
	mustParseCIDR("127.0.0.0/8"),        // Loopback (RFC 1122)
	mustParseCIDR("169.254.0.0/16"),     // Link-Local (RFC 3927)
	mustParseCIDR("172.16.0.0/12"),      // Private network (RFC 1918)
	mustParseCIDR("192.0.0.0/24"),       // IETF Protocol Assignments (RFC 6890)
	mustParseCIDR("192.0.2.0/24"),       // Documentation / TEST-NET-1 (RFC 5737)
	mustParseCIDR("192.88.99.0/24"),     // 6to4 Relay Anycast (RFC 7526)
	mustParseCIDR("192.168.0.0/16"),     // Private network (RFC 1918)
	mustParseCIDR("192.175.48.0/24"),    // Direct Delegation AS112 Service (RFC 7535)
	mustParseCIDR("198.18.0.0/15"),      // Benchmarking (RFC 2544)
	mustParseCIDR("198.51.100.0/24"),    // Documentation / TEST-NET-2 (RFC 5737)
	mustParseCIDR("203.0.113.0/24"),     // Documentation / TEST-NET-3 (RFC 5737)
	mustParseCIDR("224.0.0.0/4"),        // Multicast (RFC 5771)
	mustParseCIDR("240.0.0.0/4"),        // Reserved for future use (RFC 1112)
	mustParseCIDR("255.255.255.255/32"), // Broadcast (RFC 919)

	// IPv6 Special / Private / Reserved
	mustParseCIDR("::/128"),        // Unspecified
	mustParseCIDR("::1/128"),       // Loopback
	mustParseCIDR("64:ff9b::/96"),  // IPv4/IPv6 translation
	mustParseCIDR("100::/64"),      // Discard-only prefix
	mustParseCIDR("2001::/23"),     // IETF Protocol Assignments
	mustParseCIDR("2001:db8::/32"), // Documentation
	mustParseCIDR("2001:10::/28"),  // ORCHIDv2
	mustParseCIDR("fc00::/7"),      // Unique Local Address (ULA)
	mustParseCIDR("fe80::/10"),     // Link-Local Unicast
	mustParseCIDR("ff00::/8"),      // Multicast
}

func mustParseCIDR(s string) *net.IPNet {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR %s: %v", s, err))
	}
	return ipNet
}

// IsForbiddenIP checks whether an IP address belongs to any private, loopback, link-local,
// multicast, CGNAT, documentation, or reserved range.
func IsForbiddenIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// Unwrap IPv4-mapped IPv6 addresses (e.g. ::ffff:127.0.0.1 -> 127.0.0.1)
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}

	for _, block := range forbiddenCIDRs {
		if block.Contains(ip) {
			return true
		}
	}

	return false
}

// URLValidator validates URLs and protects against SSRF.
type URLValidator struct {
	Resolver *net.Resolver
	Timeout  time.Duration
}

// NewURLValidator creates a new URLValidator with sensible defaults.
func NewURLValidator() *URLValidator {
	return &URLValidator{
		Resolver: net.DefaultResolver,
		Timeout:  5 * time.Second,
	}
}

// ValidateURL performs strict URL syntax and pre-flight DNS validation.
func (v *URLValidator) ValidateURL(rawURL string) (*url.URL, error) {
	if strings.TrimSpace(rawURL) == "" {
		return nil, ErrEmptyHost
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidURL, err)
	}

	// 1. Validate Scheme
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, ErrInvalidScheme
	}

	// 2. Reject User Credentials (e.g. http://user:pass@example.com)
	if u.User != nil {
		return nil, ErrHasCredentials
	}

	// 3. Validate Host
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return nil, ErrEmptyHost
	}

	// Reject explicit "localhost"
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return nil, ErrLocalhost
	}

	// 4. Check if host is a literal IP address
	literalIP := net.ParseIP(host)
	if literalIP != nil {
		if IsForbiddenIP(literalIP) {
			return nil, fmt.Errorf("%w: %s", ErrForbiddenIP, literalIP.String())
		}
		return u, nil
	}

	// 5. Pre-flight DNS Resolution
	ctx, cancel := context.WithTimeout(context.Background(), v.Timeout)
	defer cancel()

	resolver := v.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns resolution failed for %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses found for host %q", host)
	}

	// Verify that ALL resolved IPs are strictly public.
	// If any resolved IP is in a forbidden range, reject immediately!
	for _, ipAddr := range ips {
		if IsForbiddenIP(ipAddr.IP) {
			return nil, fmt.Errorf("%w: %s resolves to forbidden address %s", ErrForbiddenIP, host, ipAddr.IP.String())
		}
	}

	return u, nil
}
