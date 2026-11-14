package dashboard

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed index.html
var indexHTML []byte

// Module handles serving the Pocket Sentinel status dashboard.
type Module struct{}

// New creates a new dashboard Module.
func New() *Module {
	return &Module{}
}

// RegisterRoutes mounts the dashboard endpoint on the root engine.
func (m *Module) RegisterRoutes(r *gin.Engine) {
	r.GET("/", m.HandleDashboard)
}

// HandleDashboard serves the single-page responsive dashboard.
func (m *Module) HandleDashboard(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
}
