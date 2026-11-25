package bot

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterAskRoutes mounts the fast AI ask HTTP endpoint on the root Gin engine.
func RegisterAskRoutes(r *gin.Engine, askRouter *AskTriageRouter) {
	r.POST("/api/ask", func(c *gin.Context) {
		var req struct {
			Prompt string `json:"prompt"`
			Model  string `json:"model"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload: " + err.Error()})
			return
		}
		prompt := strings.TrimSpace(req.Prompt)
		if prompt == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt cannot be empty"})
			return
		}

		if askRouter == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI service not configured"})
			return
		}

		guard := NewPromptGuard()
		if _, err := guard.ValidateAndFrameQuestion(prompt); err != nil {
			if errors.Is(err, ErrPromptTooLong) {
				c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("prompt is too long (max %d characters)", MaxPromptLength)})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": "prompt failed security check"})
			return
		}

		opts := ParseAskInput(prompt)
		if req.Model != "" && req.Model != "auto" {
			opts.Model = req.Model
		}

		res, err := askRouter.ExecuteFast(c.Request.Context(), opts)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"response":    res.ResponseText,
			"provider":    res.Provider,
			"cached":      res.IsCached,
			"duration_ms": res.Duration.Milliseconds(),
		})
	})
}
