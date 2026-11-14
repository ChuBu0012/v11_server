package logs

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type EventLog struct {
	ID        int64     `json:"id"`
	Level     string    `json:"level"`
	Message   string    `json:"message"`
	Metadata  string    `json:"metadata,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LogInput struct {
	Level    string `json:"level"`
	Message  string `json:"message" binding:"required"`
	Metadata string `json:"metadata,omitempty"`
}

type Module struct {
	db *sql.DB
}

func New(db *sql.DB) *Module {
	return &Module{db: db}
}

func (m *Module) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("", m.HandleGetLogs)
	rg.POST("", m.HandleCreateLog)
}

func (m *Module) RecordLog(level, message, metadata string) error {
	_, err := m.db.Exec(
		"INSERT INTO event_logs (level, message, metadata, created_at) VALUES (?, ?, ?, ?)",
		level, message, metadata, time.Now().UTC(),
	)
	return err
}

// PurgeOlderThan removes event logs whose creation time is earlier than cutoff.
func (m *Module) PurgeOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := m.db.ExecContext(ctx,
		"DELETE FROM event_logs WHERE julianday(created_at) < julianday(?)",
		cutoff.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return 0, fmt.Errorf("purge old event logs: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count purged event logs: %w", err)
	}
	return deleted, nil
}

func (m *Module) HandleGetLogs(c *gin.Context) {
	rows, err := m.db.Query(`
		SELECT id, level, message, COALESCE(metadata, ''), created_at
		FROM event_logs
		ORDER BY id DESC
		LIMIT 50
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Query error: %v", err)})
		return
	}
	defer rows.Close()

	results := make([]EventLog, 0)
	for rows.Next() {
		var l EventLog
		if err := rows.Scan(&l.ID, &l.Level, &l.Message, &l.Metadata, &l.CreatedAt); err != nil {
			continue
		}
		results = append(results, l)
	}

	c.JSON(http.StatusOK, results)
}

func (m *Module) HandleCreateLog(c *gin.Context) {
	var input LogInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload", "details": err.Error()})
		return
	}
	if input.Level == "" {
		input.Level = "info"
	}

	if err := m.RecordLog(input.Level, input.Message, input.Metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to insert log: %v", err)})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"status":  "created",
		"level":   input.Level,
		"message": input.Message,
	})
}
