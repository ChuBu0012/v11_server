package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"v11-server/modules/bot"
	"v11-server/shared/ai"
)

// AIGenerator defines the AI provider interface satisfied by *ai.Engine.
type AIGenerator interface {
	GenerateWithProvider(ctx context.Context, req ai.Request) (string, string, error)
}

// AIAssistant provides smart actions for notes and spreadsheets.
type AIAssistant struct {
	engine AIGenerator
	guard  *bot.PromptGuard
}

// NewAIAssistant creates a workspace AI assistant.
func NewAIAssistant(engine AIGenerator) *AIAssistant {
	return &AIAssistant{
		engine: engine,
		guard:  bot.NewPromptGuard(),
	}
}

// AssistNote executes note actions (summarize, action_items, polish).
func (a *AIAssistant) AssistNote(ctx context.Context, note *Note, req *NoteAIRequest) (*NoteAIResponse, error) {
	if a.engine == nil {
		return nil, errors.New("AI engine not configured")
	}

	instruction := strings.TrimSpace(req.Instruction)
	if instruction != "" {
		if err := a.guard.Validate(instruction); err != nil {
			return nil, fmt.Errorf("instruction failed security check: %w", err)
		}
	}

	start := time.Now()
	systemPrompt := "You are a concise, helpful personal workspace assistant on vivo V11. Always respond in Thai unless requested otherwise. Format with clean markdown."
	var userPrompt string

	switch strings.ToLower(req.Action) {
	case "summarize":
		userPrompt = fmt.Sprintf("กรุณาสรุปเนื้อหาโน้ตต่อไปนี้ให้กระชับเป็นหัวข้อย่อย 2-4 ข้อ:\n\nหัวข้อ: %s\nเนื้อหา:\n%s", note.Title, note.Content)
		if instruction != "" {
			userPrompt += "\n\nคำแนะนำเพิ่มเติม: " + instruction
		}
	case "action_items":
		userPrompt = fmt.Sprintf("กรุณาดึงรายการสิ่งที่ต้องทำ (Action Items / Todos) ออกมาจากโน้ตนี้ โดยจัดรูปแบบเป็น Markdown Checklist (- [ ] ...):\n\nหัวข้อ: %s\nเนื้อหา:\n%s", note.Title, note.Content)
		if instruction != "" {
			userPrompt += "\n\nคำแนะนำเพิ่มเติม: " + instruction
		}
	case "polish":
		userPrompt = fmt.Sprintf("กรุณาตรวจทานแก้ไขการสะกดคำ ปรับปรุงโครงสร้างให้อ่านง่ายและเป็นระเบียบ โดยคงเนื้อหาและความหมายเดิมไว้ครบถ้วน:\n\nหัวข้อ: %s\nเนื้อหา:\n%s", note.Title, note.Content)
		if instruction != "" {
			userPrompt += "\n\nคำแนะนำเพิ่มเติม: " + instruction
		}
	default:
		return nil, fmt.Errorf("unsupported note action: %s", req.Action)
	}

	resText, provider, err := a.engine.GenerateWithProvider(ctx, ai.Request{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		JSONMode:     false,
	})
	if err != nil {
		return nil, fmt.Errorf("AI generation failed: %w", err)
	}

	return &NoteAIResponse{
		Action:     req.Action,
		Result:     strings.TrimSpace(resText),
		Provider:   provider,
		DurationMs: time.Since(start).Milliseconds(),
	}, nil
}

// AssistSheet executes spreadsheet actions (explain, formula_suggest).
func (a *AIAssistant) AssistSheet(ctx context.Context, sheet *Sheet, req *SheetAIRequest) (*SheetAIResponse, error) {
	if a.engine == nil {
		return nil, errors.New("AI engine not configured")
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" && req.Action == "formula_suggest" {
		return nil, errors.New("prompt is required for formula suggestion")
	}

	if prompt != "" {
		if err := a.guard.Validate(prompt); err != nil {
			return nil, fmt.Errorf("prompt failed security check: %w", err)
		}
	}

	start := time.Now()

	switch strings.ToLower(req.Action) {
	case "formula_suggest":
		systemPrompt := `You are an expert spreadsheet formula generator. Given a user request and sheet context, return a JSON object with two fields:
"result": the exact spreadsheet formula starting with '=' (e.g. "=SUM(B2:B10)" or "=C2-B2")
"explanation": a short 1-2 sentence explanation in Thai of how the formula works.
Output ONLY raw JSON.`

		userPrompt := fmt.Sprintf("ชื่อชีต: %s\nคำขอของผู้ใช้: %s\nข้อมูลชีต (บางส่วน): %s", sheet.Title, prompt, truncateDataJSON(sheet.DataJSON, 1000))
		resText, provider, err := a.engine.GenerateWithProvider(ctx, ai.Request{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			JSONMode:     true,
		})
		if err != nil {
			return nil, fmt.Errorf("AI formula suggestion failed: %w", err)
		}

		var parsed struct {
			Result      string `json:"result"`
			Explanation string `json:"explanation"`
		}
		if err := json.Unmarshal([]byte(resText), &parsed); err != nil {
			// Fallback: extract formula line
			parsed.Result = strings.TrimSpace(resText)
			parsed.Explanation = "สูตรที่แนะนำตามคำขอ"
		}

		return &SheetAIResponse{
			Action:      req.Action,
			Result:      parsed.Result,
			Explanation: parsed.Explanation,
			Provider:    provider,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil

	case "explain":
		systemPrompt := "You are a concise data analysis assistant on vivo V11. Explain trends, totals, and anomalies in the sheet clearly in Thai with bullet points."
		userPrompt := fmt.Sprintf("กรุณาวิเคราะห์และอธิบายข้อมูลในชีต '%s':\nข้อมูลชีต:\n%s", sheet.Title, truncateDataJSON(sheet.DataJSON, 2000))
		if prompt != "" {
			userPrompt += "\n\nคำถามเฉพาะเจาะจง: " + prompt
		}

		resText, provider, err := a.engine.GenerateWithProvider(ctx, ai.Request{
			SystemPrompt: systemPrompt,
			UserPrompt:   userPrompt,
			JSONMode:     false,
		})
		if err != nil {
			return nil, fmt.Errorf("AI explanation failed: %w", err)
		}

		return &SheetAIResponse{
			Action:      req.Action,
			Result:      strings.TrimSpace(resText),
			Explanation: "การวิเคราะห์ข้อมูลสรุป",
			Provider:    provider,
			DurationMs:  time.Since(start).Milliseconds(),
		}, nil

	default:
		return nil, fmt.Errorf("unsupported sheet action: %s", req.Action)
	}
}

func truncateDataJSON(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "... [truncated]"
}
