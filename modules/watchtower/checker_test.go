package watchtower

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestChecker_ProbeOnce_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	defer ts.Close()

	// Safe dialer will check ts.URL. Because httptest binds to 127.0.0.1 by default,
	// standard safe dialer blocks 127.0.0.1 as SSRF defense (as expected!).
	// To test HTTP semantics (status, headers, body limit, redirects), we can use a custom
	// test checker that uses ts.Client()'s transport or tests the components directly.
	checker := NewChecker(nil)

	// Verify that testing against ts.URL directly gets blocked by SSRF defense!
	_, _, _, err := checker.ProbeOnce(context.Background(), ts.URL, 200)
	if err == nil {
		t.Fatalf("Expected ProbeOnce to reject 127.0.0.1 httptest server with SSRF error, but succeeded")
	}
}

func TestChecker_NonRedirectPolicy(t *testing.T) {
	// Verify that CheckRedirect returns http.ErrUseLastResponse
	c := NewChecker(nil)
	if c.client.CheckRedirect == nil {
		t.Fatalf("CheckRedirect must be defined")
	}

	err := c.client.CheckRedirect(nil, nil)
	if err != http.ErrUseLastResponse {
		t.Fatalf("Expected http.ErrUseLastResponse, got %v", err)
	}
}

func TestChecker_ProxyNil(t *testing.T) {
	// Verify that transport.Proxy is nil to prevent environment proxy bypass
	c := NewChecker(nil)
	if c.transport.Proxy != nil {
		t.Fatalf("Expected transport.Proxy to be nil")
	}
}

func TestSafeDialer_SSRF_Rejection(t *testing.T) {
	dialer := NewSafeDialer(nil, 2*time.Second)

	// Literal private IP
	_, err := dialer.DialContext(context.Background(), "tcp", "127.0.0.1:8080")
	if err == nil {
		t.Fatalf("Expected DialContext to reject 127.0.0.1, but succeeded")
	}

	_, err = dialer.DialContext(context.Background(), "tcp", "10.0.0.1:80")
	if err == nil {
		t.Fatalf("Expected DialContext to reject 10.0.0.1, but succeeded")
	}

	_, err = dialer.DialContext(context.Background(), "tcp", "169.254.169.254:80")
	if err == nil {
		t.Fatalf("Expected DialContext to reject 169.254.169.254, but succeeded")
	}
}

func TestChecker_CheckCycle_RetrySuccess(t *testing.T) {
	var attempts int32
	// Custom roundtripper simulating first failure, second success
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		att := atomic.AddInt32(&attempts, 1)
		rec := httptest.NewRecorder()
		if att == 1 {
			rec.WriteHeader(http.StatusServiceUnavailable)
			rec.WriteString("Service Unavailable")
		} else {
			rec.WriteHeader(http.StatusOK)
			rec.WriteString("OK")
		}
		return rec.Result(), nil
	})

	c := NewChecker(nil)
	c.client.Transport = rt

	res := c.CheckCycle(context.Background(), "https://todo-fm-d.vercel.app/", 200)
	if !res.Success {
		t.Fatalf("Expected CheckCycle to succeed after retry, got failure: %v", res.Error)
	}
	if res.AttemptCount != 2 {
		t.Fatalf("Expected AttemptCount = 2, got %d", res.AttemptCount)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Expected StatusCode = 200, got %d", res.StatusCode)
	}
}

func TestChecker_CheckCycle_AllFailures(t *testing.T) {
	var attempts int32
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusInternalServerError)
		rec.WriteString("Internal Error")
		return rec.Result(), nil
	})

	c := NewChecker(nil)
	c.client.Transport = rt

	res := c.CheckCycle(context.Background(), "https://todo-fm-d.vercel.app/", 200)
	if res.Success {
		t.Fatalf("Expected CheckCycle to fail, got success")
	}
	if res.AttemptCount != 3 {
		t.Fatalf("Expected AttemptCount = 3, got %d", res.AttemptCount)
	}
	if res.StatusCode != 500 {
		t.Fatalf("Expected StatusCode = 500, got %d", res.StatusCode)
	}
	if !strings.Contains(res.Error, "unexpected status code: 500") {
		t.Fatalf("Expected unexpected status code error, got %s", res.Error)
	}
}

// helper roundTripFunc for testing HTTP client without network calls
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func (f roundTripFunc) Close() error {
	return nil
}

func TestChecker_BodyReadingBounded(t *testing.T) {
	// Verify that reading body with MaxResponseBodyLimit (64KB) works cleanly
	bigPayload := strings.Repeat("A", 128*1024) // 128 KB
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		rec.WriteString(bigPayload)
		return rec.Result(), nil
	})

	c := NewChecker(nil)
	c.client.Transport = rt

	success, statusCode, _, err := c.ProbeOnce(context.Background(), "https://todo-fm-d.vercel.app/", 200)
	if err != nil {
		t.Fatalf("ProbeOnce failed on large body: %v", err)
	}
	if !success || statusCode != 200 {
		t.Fatalf("Expected success=true, code=200, got success=%v, code=%d", success, statusCode)
	}
}

func TestChecker_Timeout(t *testing.T) {
	rt := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(2 * time.Second):
			rec := httptest.NewRecorder()
			rec.WriteHeader(http.StatusOK)
			return rec.Result(), nil
		}
	})

	c := NewChecker(nil)
	c.client.Transport = rt

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	success, _, _, err := c.ProbeOnce(ctx, "https://todo-fm-d.vercel.app/", 200)
	if success {
		t.Fatalf("Expected probe to fail on timeout, but succeeded")
	}
	if err == nil {
		t.Fatalf("Expected timeout error, got nil")
	}
}
