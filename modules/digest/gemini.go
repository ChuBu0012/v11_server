package digest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

// GeminiClient interacts with Google Gemini API for news summarization.
type GeminiClient struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

// GeminiOption configures the GeminiClient.
type GeminiOption func(*GeminiClient)

// WithBaseURL overrides the API endpoint (useful for unit tests/mocking).
func WithBaseURL(url string) GeminiOption {
	return func(g *GeminiClient) { g.baseURL = url }
}

// WithModel sets the model name (default: gemini-3.5-flash-lite).
func WithModel(model string) GeminiOption {
	return func(g *GeminiClient) { g.model = model }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(client *http.Client) GeminiOption {
	return func(g *GeminiClient) { g.client = client }
}

// NewGeminiClient creates a client instance.
func NewGeminiClient(apiKey string, opts ...GeminiOption) *GeminiClient {
	model := "gemini-3.5-flash-lite"
	if envModel := strings.TrimSpace(os.Getenv("GEMINI_MODEL")); envModel != "" {
		model = envModel
	}
	tr := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}
			return dialer.DialContext(ctx, "tcp4", addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	gc := &GeminiClient{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: "https://generativelanguage.googleapis.com",
		model:   model,
		client:  &http.Client{Timeout: 90 * time.Second, Transport: tr},
	}
	for _, opt := range opts {
		opt(gc)
	}
	return gc
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiRequest struct {
	Contents         []geminiContent `json:"contents"`
	GenerationConfig struct {
		Temperature     float64 `json:"temperature"`
		MaxOutputTokens int     `json:"maxOutputTokens"`
	} `json:"generationConfig"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiResponse struct {
	Candidates []geminiCandidate `json:"candidates"`
	Error      *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// SummarizeNews sends the compiled news items to Gemini and returns a structured Thai digest.
func (g *GeminiClient) SummarizeNews(ctx context.Context, items []NewsItem) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("gemini: API key is not configured")
	}

	var sb strings.Builder
	sb.WriteString("คุณคือ Pocket Sentinel AI ผู้ช่วยสรุปข่าวสารและเกร็ดความรู้ประจำวันสำหรับมือถือ\n")
	sb.WriteString("เป้าหมาย: สรุปให้อ่านง่าย สบายตา ไม่ยาวเกินไป โดยแบ่งออกเป็น 3 ส่วนอย่างชัดเจน และต้องใส่แท็กกำกับส่วนดังนี้:\n\n")

	sb.WriteString("===SECTION:AI===\n")
	sb.WriteString("🤖 **อัปเดตวงการ AI & เทคโนโลยีเด่น**\n")
	sb.WriteString("คัดเลือก 2-3 ประเด็นสำคัญจากค่ายดัง (เช่น OpenAI, Google, Anthropic, Nvidia ฯลฯ)\n")
	sb.WriteString("รูปแบบของแต่ละประเด็น:\n")
	sb.WriteString("* **พาดหัวประเด็น:** คำอธิบายใจความสำคัญและเหตุผลว่าทำไมเรื่องนี้ถึงน่าจับตา (ความยาวไม่เกิน 2 บรรทัดต่อประเด็น)\n\n")

	sb.WriteString("===SECTION:POLITICS===\n")
	sb.WriteString("🏛️ **สรุปประเด็นการเมือง & สถานการณ์สำคัญ**\n")
	sb.WriteString("คัดเลือก 2-3 ประเด็นสถานการณ์สำคัญรอบวันอย่างเป็นกลาง ตรงประเด็น\n")
	sb.WriteString("รูปแบบของแต่ละประเด็น:\n")
	sb.WriteString("* **พาดหัวประเด็น:** สรุปเนื้อหาสำคัญและผลกระทบที่เกิดขึ้น (ความยาวไม่เกิน 2 บรรทัดต่อประเด็น)\n\n")

	sb.WriteString("===SECTION:TRIVIA===\n")
	sb.WriteString("💡 **สาระน่ารู้ & เกร็ดความรู้ประจำวัน (Bite-sized Trivia)**\n")
	sb.WriteString("คัดเลือก 1-2 เกร็ดความรู้สั้น ๆ ชวนคิด (วิทยาศาสตร์, เทคโนโลยี, หรือบทเรียนรอบตัว)\n")
	sb.WriteString("รูปแบบของแต่ละประเด็น:\n")
	sb.WriteString("* **ประเด็นชวนรู้:** ข้อคิดหรือเกร็ดความรู้สั้น ๆ (ไม่เกิน 2 บรรทัด)\n\n")

	sb.WriteString("กฎสำคัญ: ต้องมีแท็ก ===SECTION:AI===, ===SECTION:POLITICS===, ===SECTION:TRIVIA=== คั่นแต่ละส่วนเสมอ\n\n")

	sb.WriteString("=== รายการข่าวสดที่รวบรวมมาได้ ===\n")
	if len(items) == 0 {
		sb.WriteString("(ไม่มีฟีดข่าวสดเข้ามาเป็นพิเศษ ให้คุณวิเคราะห์ข่าว AI เด่นล่าสุดและเกร็ดความรู้ประจำวันแทนได้เลย)\n")
	} else {
		for i, item := range items {
			sb.WriteString(fmt.Sprintf("[%d] หมวด: %s | แหล่งที่มา: %s\nหัวข้อ: %s\nรายละเอียด: %s\nลิงก์: %s\n\n",
				i+1, item.Category, item.Source, item.Title, item.Description, item.Link))
		}
	}

	return g.generateText(ctx, sb.String(), 0.7, 2048)
}

// Generate sends one system + user prompt and returns the model's text.
// It lets other modules (e.g. the morning insight) reuse the Gemini client.
func (g *GeminiClient) Generate(ctx context.Context, system, user string) (string, error) {
	if g.apiKey == "" {
		return "", fmt.Errorf("gemini: API key is not configured")
	}
	prompt := strings.TrimSpace(system) + "\n\n" + strings.TrimSpace(user)
	return g.generateText(ctx, prompt, 0.3, 1024)
}

func (g *GeminiClient) generateText(ctx context.Context, prompt string, temperature float64, maxTokens int) (string, error) {
	reqBody := geminiRequest{}
	reqBody.Contents = []geminiContent{
		{
			Parts: []geminiPart{
				{Text: prompt},
			},
		},
	}
	reqBody.GenerationConfig.Temperature = temperature
	reqBody.GenerationConfig.MaxOutputTokens = maxTokens

	payloadBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("gemini: marshal payload failed: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payloadBytes))
	if err != nil {
		return "", fmt.Errorf("gemini: create request failed: %s", redactKey(err.Error(), g.apiKey))
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gemini: request failed: %s", redactKey(err.Error(), g.apiKey))
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 1024*512))
	if err != nil {
		return "", fmt.Errorf("gemini: read body failed: %w", err)
	}

	var res geminiResponse
	if err := json.Unmarshal(bodyBytes, &res); err != nil {
		return "", fmt.Errorf("gemini: unmarshal response failed: %s (status %d)", redactKey(string(bodyBytes), g.apiKey), resp.StatusCode)
	}

	if res.Error != nil {
		return "", fmt.Errorf("gemini: API error %d: %s", res.Error.Code, redactKey(res.Error.Message, g.apiKey))
	}

	if len(res.Candidates) == 0 || len(res.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: returned no candidates")
	}

	var textBuilder strings.Builder
	for _, part := range res.Candidates[0].Content.Parts {
		textBuilder.WriteString(part.Text)
	}
	text := strings.TrimSpace(textBuilder.String())
	if text == "" {
		return "", fmt.Errorf("gemini: generated text was empty")
	}

	return text, nil
}

func redactKey(input, key string) string {
	if key == "" {
		return input
	}
	return strings.ReplaceAll(input, key, "GEMINI_KEY_REDACTED")
}
