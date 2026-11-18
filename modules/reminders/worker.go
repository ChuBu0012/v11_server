package reminders

import (
	"context"
	"log"
	"time"
)

func (s *Service) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			if err := s.RunOnce(runCtx, time.Now()); err != nil && ctx.Err() == nil {
				log.Printf("[quiet-schedule] delivery pass: %v", err)
			}
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}
