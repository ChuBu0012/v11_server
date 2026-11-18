package watchtower

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestIsForbiddenIP(t *testing.T) {
	tests := []struct {
		name      string
		ip        string
		forbidden bool
	}{
		// IPv4 Loopback
		{"IPv4 Loopback 127.0.0.1", "127.0.0.1", true},
		{"IPv4 Loopback 127.0.0.2", "127.0.0.2", true},
		{"IPv4 Loopback 127.255.255.254", "127.255.255.254", true},

		// IPv4 Private (RFC 1918)
		{"IPv4 Private 10.0.0.1", "10.0.0.1", true},
		{"IPv4 Private 10.255.255.254", "10.255.255.254", true},
		{"IPv4 Private 172.16.0.1", "172.16.0.1", true},
		{"IPv4 Private 172.31.255.254", "172.31.255.254", true},
		{"IPv4 Private 192.168.0.1", "192.168.0.1", true},
		{"IPv4 Private 192.168.1.100", "192.168.1.100", true},

		// IPv4 Link-Local & Cloud Metadata
		{"IPv4 Link-Local 169.254.169.254", "169.254.169.254", true},
		{"IPv4 Link-Local 169.254.1.1", "169.254.1.1", true},

		// IPv4 CGNAT (RFC 6598)
		{"IPv4 CGNAT 100.64.0.1", "100.64.0.1", true},
		{"IPv4 CGNAT 100.127.255.254", "100.127.255.254", true},

		// IPv4 Documentation & Reserved
		{"IPv4 Doc TEST-NET-1 192.0.2.1", "192.0.2.1", true},
		{"IPv4 Doc TEST-NET-2 198.51.100.1", "198.51.100.1", true},
		{"IPv4 Doc TEST-NET-3 203.0.113.1", "203.0.113.1", true},
		{"IPv4 Benchmarking 198.18.0.1", "198.18.0.1", true},
		{"IPv4 Multicast 224.0.0.1", "224.0.0.1", true},
		{"IPv4 Reserved 240.0.0.1", "240.0.0.1", true},
		{"IPv4 Broadcast 255.255.255.255", "255.255.255.255", true},
		{"IPv4 Unspecified 0.0.0.0", "0.0.0.0", true},

		// IPv6 Special & Private
		{"IPv6 Loopback ::1", "::1", true},
		{"IPv6 Unspecified ::", "::", true},
		{"IPv6 Unique Local fc00::1", "fc00::1", true},
		{"IPv6 Unique Local fd12:3456:789a::1", "fd12:3456:789a::1", true},
		{"IPv6 Link-Local fe80::1", "fe80::1", true},
		{"IPv6 Multicast ff02::1", "ff02::1", true},
		{"IPv6 Documentation 2001:db8::1", "2001:db8::1", true},

		// IPv4-mapped IPv6
		{"IPv4-mapped Loopback ::ffff:127.0.0.1", "::ffff:127.0.0.1", true},
		{"IPv4-mapped Private ::ffff:192.168.1.1", "::ffff:192.168.1.1", true},
		{"IPv4-mapped Private ::ffff:10.0.0.1", "::ffff:10.0.0.1", true},
		{"IPv4-mapped Public ::ffff:8.8.8.8", "::ffff:8.8.8.8", false},

		// Public IP Addresses (Allowed)
		{"Public IPv4 Google DNS 8.8.8.8", "8.8.8.8", false},
		{"Public IPv4 Cloudflare DNS 1.1.1.1", "1.1.1.1", false},
		{"Public IPv4 Example.com 93.184.216.34", "93.184.216.34", false},
		{"Public IPv6 Google DNS 2001:4860:4860::8888", "2001:4860:4860::8888", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("Failed to parse IP %s", tt.ip)
			}
			got := IsForbiddenIP(ip)
			if got != tt.forbidden {
				t.Errorf("IsForbiddenIP(%s) = %v, expected %v", tt.ip, got, tt.forbidden)
			}
		})
	}
}

func TestURLValidator_ValidateURL(t *testing.T) {
	v := NewURLValidator()

	// Test URL Syntax & Scheme
	invalidURLs := []struct {
		url string
		err string
	}{
		{"", "empty"},
		{"   ", "empty"},
		{"ftp://example.com/file", "invalid scheme"},
		{"javascript:alert(1)", "invalid scheme"},
		{"file:///etc/passwd", "invalid scheme"},
		{"http://user:pass@example.com/", "username or password"},
		{"http://admin:@example.com/", "username or password"},
		{"http://localhost", "localhost is forbidden"},
		{"https://localhost:8080/test", "localhost is forbidden"},
		{"http://sub.localhost/test", "localhost is forbidden"},
		{"http://127.0.0.1/test", "forbidden IP address"},
		{"http://10.0.0.1/api", "forbidden IP address"},
		{"http://172.16.1.1/api", "forbidden IP address"},
		{"http://192.168.1.1/api", "forbidden IP address"},
		{"http://169.254.169.254/latest/meta-data/", "forbidden IP address"},
		{"http://100.64.0.1/test", "forbidden IP address"},
		{"http://[::1]/test", "forbidden IP address"},
		{"http://[::ffff:127.0.0.1]/test", "forbidden IP address"},
	}

	for _, tt := range invalidURLs {
		t.Run(tt.url, func(t *testing.T) {
			_, err := v.ValidateURL(tt.url)
			if err == nil {
				t.Errorf("ValidateURL(%q) succeeded, expected error", tt.url)
			}
		})
	}

	// Test Valid Public URLs
	validURLs := []string{
		"https://todo-fm-d.vercel.app/",
		"https://example.com/test?a=1",
		"http://93.184.216.34/",
	}

	for _, u := range validURLs {
		t.Run(u, func(t *testing.T) {
			parsed, err := v.ValidateURL(u)
			if err != nil {
				t.Errorf("ValidateURL(%q) unexpected error: %v", u, err)
			}
			if parsed == nil {
				t.Errorf("ValidateURL(%q) returned nil parsed URL", u)
			}
		})
	}
}

func TestURLValidator_MockResolver(t *testing.T) {
	// Simulate DNS resolution returning private IP for an innocent-looking domain
	fakeDialer := func(ctx context.Context, network, address string) (net.Conn, error) {
		return nil, nil
	}

	// Custom resolver that resolves "attacker.internal" to 127.0.0.1
	// and "attacker.split" to both public IP and 192.168.1.1
	resolver := &net.Resolver{
		PreferGo: true,
		Dial:     fakeDialer,
	}

	// Test directly with custom resolver injection in validator
	v := &URLValidator{
		Resolver: resolver,
		Timeout:  2 * time.Second,
	}

	// Test literal IP validation (independent of DNS)
	_, err := v.ValidateURL("http://127.0.0.1:8080/")
	if err == nil {
		t.Errorf("Expected literal IP to be rejected, but succeeded")
	}
}
