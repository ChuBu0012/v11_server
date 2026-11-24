package cloudsync

import "time"

// DeviceStatus holds non-sensitive device telemetry metrics.
type DeviceStatus struct {
	BatteryLevel    int     `json:"battery_level"`
	IsCharging      bool    `json:"is_charging"`
	RAMUsedPercent  float64 `json:"ram_used_percent"`
	DiskUsedPercent float64 `json:"disk_used_percent"`
	UptimeSeconds   int64   `json:"uptime_seconds"`
	UptimeHours     float64 `json:"uptime_hours"`
}

// ServicesStatus holds public health status of core server services.
type ServicesStatus struct {
	Server    string `json:"server"`
	Gatus     string `json:"gatus"`
	Tailscale string `json:"tailscale"`
}

// PublicStatusPayload is the sanitized status document for Firestore: sentinel_public/status.
type PublicStatusPayload struct {
	UpdatedAt string         `json:"updated_at"`
	Device    DeviceStatus   `json:"device"`
	Services  ServicesStatus `json:"services"`
	Message   string         `json:"message,omitempty"`
}

// GoldPayload holds current spot and bar prices.
type GoldPayload struct {
	WorldSpot float64 `json:"world_spot"`
	BarBuy    float64 `json:"bar_buy"`
	BarSell   float64 `json:"bar_sell"`
	UpdatedAt string  `json:"updated_at,omitempty"`
}

// StockItem holds a sanitized news/quote headline.
type StockItem struct {
	Title  string `json:"title"`
	Source string `json:"source,omitempty"`
	URL    string `json:"url,omitempty"`
}

// NewsItem holds categorized summary content.
type NewsItem struct {
	Category string `json:"category"`
	Summary  string `json:"summary"`
}

// PublicDigestPayload is the sanitized digest document for Firestore: sentinel_public/digest.
type PublicDigestPayload struct {
	UpdatedAt string       `json:"updated_at"`
	Gold      *GoldPayload `json:"gold,omitempty"`
	Stocks    []StockItem  `json:"stocks,omitempty"`
	News      []NewsItem   `json:"news,omitempty"`
	SummaryMD string       `json:"summary_md,omitempty"`
}

// ActionRequest represents a command submitted from the public web UI.
type ActionRequest struct {
	ID          string     `json:"id"`
	Action      string     `json:"action"`
	Status      string     `json:"status"` // pending, processing, completed, failed
	Prompt      string     `json:"prompt,omitempty"`
	Model       string     `json:"model,omitempty"`
	RequestedAt time.Time  `json:"requested_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Result      string     `json:"result,omitempty"`
}

// PublicNote holds public note item.
type PublicNote struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	UpdatedAt string   `json:"updated_at"`
}

// PublicSheet holds public spreadsheet item.
type PublicSheet struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	RowCount  int    `json:"row_count"`
	ColCount  int    `json:"col_count"`
	DataJSON  string `json:"data_json"`
	UpdatedAt string `json:"updated_at"`
}

// PublicWorkspacePayload holds public workspace documents for Firestore: sentinel_workspace/shared.
type PublicWorkspacePayload struct {
	UpdatedAt string        `json:"updated_at"`
	Notes     []PublicNote  `json:"notes"`
	Sheets    []PublicSheet `json:"sheets"`
}
