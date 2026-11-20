package digest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGeminiGenerate(t *testing.T) {
	var gotPrompt string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotPrompt = req.Contents[0].Parts[0].Text
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"  สวัสดีตอนเช้า  "}]}}]}`))
	}))
	defer srv.Close()
	g := NewGeminiClient("test-key", WithBaseURL(srv.URL))
	out, err := g.Generate(context.Background(), "SYSTEM", "USER")
	if err != nil || out != "สวัสดีตอนเช้า" {
		t.Fatalf("Generate = %q err=%v", out, err)
	}
	if !strings.Contains(gotPrompt, "SYSTEM") || !strings.Contains(gotPrompt, "USER") {
		t.Fatalf("prompt = %q", gotPrompt)
	}
	if _, err := NewGeminiClient("").Generate(context.Background(), "a", "b"); err == nil {
		t.Fatal("missing key must error")
	}
}
