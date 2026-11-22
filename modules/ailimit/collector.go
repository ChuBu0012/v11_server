package ailimit

import (
	"context"
	"log"
	"time"
)

// Collector coordinates periodic background execution of AI quota checks.
type Collector struct {
	service  *Service
	interval time.Duration
}

// NewCollector creates an AI limit background collector.
func NewCollector(service *Service, interval ...time.Duration) *Collector {
	iv := 15 * time.Minute
	if len(interval) > 0 && interval[0] > 0 {
		iv = interval[0]
	}
	return &Collector{
		service:  service,
		interval: iv,
	}
}

// Start spawns the background collection loop.
func (c *Collector) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[ailimit:collector] stopped background collection")
				return
			case <-ticker.C:
				runCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
				_, err := c.service.Collect(runCtx)
				cancel()
				if err != nil && ctx.Err() == nil {
					log.Printf("[ailimit:collector] periodic collection warning: %v", err)
				}
			}
		}
	}()
}
