package watchtower

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// SafeDialer resolves hostnames and strictly validates every resolved IP address
// at socket connect time, defending against DNS rebinding attacks.
type SafeDialer struct {
	Resolver *net.Resolver
	Timeout  time.Duration
}

// NewSafeDialer creates a SafeDialer with the specified resolver and timeout.
func NewSafeDialer(resolver *net.Resolver, timeout time.Duration) *SafeDialer {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &SafeDialer{
		Resolver: resolver,
		Timeout:  timeout,
	}
}

// DialContext validates that every resolved IP address for the target host is public.
// If any forbidden IP (private/reserved/loopback) is found, connection is aborted immediately.
func (d *SafeDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", addr, err)
	}

	// 1. Literal IP check
	if literalIP := net.ParseIP(host); literalIP != nil {
		if IsForbiddenIP(literalIP) {
			return nil, fmt.Errorf("%w: dial-time check rejected %s", ErrForbiddenIP, literalIP.String())
		}
		nd := &net.Dialer{Timeout: d.Timeout}
		return nd.DialContext(ctx, network, addr)
	}

	// 2. DNS Resolution at Dial Time
	ips, err := d.Resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dial-time dns resolution failed for %q: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no IP addresses resolved for host %q", host)
	}

	// Verify ALL resolved IPs; reject if ANY forbidden IP is present
	var selectedIP net.IP
	for _, ipAddr := range ips {
		if IsForbiddenIP(ipAddr.IP) {
			return nil, fmt.Errorf("%w: dial-time check rejected %s (%s)", ErrForbiddenIP, host, ipAddr.IP.String())
		}
		if selectedIP == nil {
			selectedIP = ipAddr.IP
		}
	}

	// Dial the validated public IP directly while preserving the destination port
	targetAddr := net.JoinHostPort(selectedIP.String(), port)
	nd := &net.Dialer{Timeout: d.Timeout}
	return nd.DialContext(ctx, network, targetAddr)
}

// Checker executes safe HTTP availability checks against target URLs.
type Checker struct {
	client    *http.Client
	transport *http.Transport
	validator *URLValidator
}

// NewChecker creates a Checker configured with safe dialer, no environment proxy,
// no redirects, and bounded response body reading.
func NewChecker(customResolver *net.Resolver) *Checker {
	dialer := NewSafeDialer(customResolver, 5*time.Second)

	transport := &http.Transport{
		// Explicitly disable environment proxy bypass
		Proxy:                 nil,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   CheckTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Do not follow redirects automatically (prevent open redirect SSRF)
			return http.ErrUseLastResponse
		},
	}

	val := NewURLValidator()
	if customResolver != nil {
		val.Resolver = customResolver
	}

	return &Checker{
		client:    client,
		transport: transport,
		validator: val,
	}
}

// ProbeOnce executes a single HTTP request against targetURL and compares the returned
// status against expectedStatus.
func (c *Checker) ProbeOnce(ctx context.Context, targetURL string, expectedStatus int) (bool, int, int64, error) {
	// 1. Pre-flight URL validation
	if _, err := c.validator.ValidateURL(targetURL); err != nil {
		return false, 0, 0, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return false, 0, 0, fmt.Errorf("create request failed: %w", err)
	}

	req.Header.Set("User-Agent", "PocketSentinel-Watchtower/1.0 (+https://github.com/most-too/v11_server)")
	req.Header.Set("Accept", "*/*")

	start := time.Now()
	resp, err := c.client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		// Clean error string to remove secrets
		cleanErr := SanitizeError(err.Error())
		return false, 0, latency, errors.New(cleanErr)
	}
	defer resp.Body.Close()

	// 2. Bound body read up to 64KB to prevent OOM
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, MaxResponseBodyLimit))

	statusCode := resp.StatusCode
	if statusCode == expectedStatus {
		return true, statusCode, latency, nil
	}

	return false, statusCode, latency, fmt.Errorf("unexpected status code: %d (expected %d)", statusCode, expectedStatus)
}

// CheckCycle executes up to MaxRetriesPerCheckCycle (3) attempts with bounded backoff
// inside an end-to-end deadline.
func (c *Checker) CheckCycle(ctx context.Context, targetURL string, expectedStatus int) CheckResult {
	cycleCtx, cancel := context.WithTimeout(ctx, CheckTimeout)
	defer cancel()

	result := CheckResult{
		CheckedAt:    time.Now().UTC(),
		AttemptCount: 0,
	}

	var lastErr error
	var lastStatus int
	var lastLatency int64

	backoffs := []time.Duration{0, 500 * time.Millisecond, 1000 * time.Millisecond}

	for attempt := 1; attempt <= MaxRetriesPerCheckCycle; attempt++ {
		result.AttemptCount = attempt

		// Apply backoff if not first attempt
		if attempt > 1 && attempt-1 < len(backoffs) {
			select {
			case <-time.After(backoffs[attempt-1]):
			case <-cycleCtx.Done():
				result.Success = false
				result.StatusCode = lastStatus
				result.LatencyMs = lastLatency
				result.Error = SanitizeError(cycleCtx.Err().Error())
				return result
			}
		}

		success, statusCode, latency, err := c.ProbeOnce(cycleCtx, targetURL, expectedStatus)
		lastStatus = statusCode
		lastLatency = latency
		lastErr = err

		if success {
			result.Success = true
			result.StatusCode = statusCode
			result.LatencyMs = latency
			result.Error = ""
			return result
		}

		// If error is a permanent SSRF/forbidden IP or validation error, do not retry
		if errors.Is(err, ErrForbiddenIP) || errors.Is(err, ErrInvalidScheme) ||
			errors.Is(err, ErrHasCredentials) || errors.Is(err, ErrLocalhost) {
			break
		}
	}

	result.Success = false
	result.StatusCode = lastStatus
	result.LatencyMs = lastLatency
	if lastErr != nil {
		result.Error = SanitizeError(lastErr.Error())
	} else {
		result.Error = fmt.Sprintf("failed after %d attempts", result.AttemptCount)
	}

	return result
}
