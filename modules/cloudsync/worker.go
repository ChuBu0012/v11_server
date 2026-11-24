package cloudsync

import (
	"context"
	"log"
	"sync"
	"time"

	"v11-server/modules/digest"
	"v11-server/modules/telemetry"
)

// TelemetryProvider abstracts querying the latest telemetry sample.
type TelemetryProvider interface {
	GetLatestSample(ctx context.Context) (*telemetry.HealthSample, error)
}

// DigestProvider abstracts querying the latest digest run.
type DigestProvider interface {
	GetLatestRun(ctx context.Context) (*digest.RunRecord, error)
}

// WorkspaceProvider supplies public workspace documents.
type WorkspaceProvider interface {
	GetPublicWorkspace(ctx context.Context) (PublicWorkspacePayload, error)
}

// ServicesProvider supplies current health of background services.
type ServicesProvider func() ServicesStatus

// Worker coordinates periodic Firestore synchronization and actions processing.
type Worker struct {
	client       Client
	telemetry    TelemetryProvider
	digest       DigestProvider
	workspace    WorkspaceProvider
	dispatcher   *ActionDispatcher
	servicesProv ServicesProvider
	interval     time.Duration

	mu      sync.Mutex
	running bool
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// WithServicesProvider sets the provider for service status reports.
func WithServicesProvider(p ServicesProvider) WorkerOption {
	return func(w *Worker) { w.servicesProv = p }
}

// WithWorkspaceProvider configures the public workspace provider.
func WithWorkspaceProvider(p WorkspaceProvider) WorkerOption {
	return func(w *Worker) { w.workspace = p }
}

// WithActionDispatcher configures the action dispatcher.
func WithActionDispatcher(d *ActionDispatcher) WorkerOption {
	return func(w *Worker) { w.dispatcher = d }
}

// NewWorker constructs a cloudsync Worker.
func NewWorker(client Client, tel TelemetryProvider, dig DigestProvider, interval time.Duration, opts ...WorkerOption) *Worker {
	if interval <= 0 {
		interval = 60 * time.Second
	}

	w := &Worker{
		client:    client,
		telemetry: tel,
		digest:    dig,
		interval:  interval,
		servicesProv: func() ServicesStatus {
			return ServicesStatus{
				Server:    "healthy",
				Gatus:     "healthy",
				Tailscale: "connected",
			}
		},
	}

	for _, opt := range opts {
		opt(w)
	}

	return w
}

// Start begins background periodic synchronization until ctx is cancelled.
func (w *Worker) Start(ctx context.Context) {
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.mu.Unlock()

	log.Printf("[cloudsync] Worker started (interval: %v, dry-run: %t)", w.interval, w.client.IsDryRun())

	// Execute initial sync immediately
	if err := w.SyncOnce(ctx); err != nil {
		log.Printf("[cloudsync] Initial sync notice: %v", err)
	}

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[cloudsync] Worker stopping...")
			return
		case <-ticker.C:
			if err := w.SyncOnce(ctx); err != nil {
				log.Printf("[cloudsync] Sync failed: %v", err)
			}
		}
	}
}

// SyncOnce performs a single pass of status sync, digest sync, and action processing.
func (w *Worker) SyncOnce(ctx context.Context) error {
	// 1. Sync Telemetry Status
	if w.telemetry != nil {
		sample, err := w.telemetry.GetLatestSample(ctx)
		if err != nil {
			log.Printf("[cloudsync] Failed to read latest telemetry: %v", err)
		} else {
			services := w.servicesProv()
			payload := SanitizeTelemetry(sample, services)
			if err := w.client.SyncStatus(ctx, payload); err != nil {
				log.Printf("[cloudsync] Sync status error: %v", err)
			}
		}
	}

	// 2. Sync Digest
	if w.digest != nil {
		run, err := w.digest.GetLatestRun(ctx)
		if err != nil {
			log.Printf("[cloudsync] Failed to read latest digest: %v", err)
		} else if run != nil {
			payload := SanitizeDigest(run)
			if err := w.client.SyncDigest(ctx, payload); err != nil {
				log.Printf("[cloudsync] Sync digest error: %v", err)
			}
		}
	}

	// 3. Process Pending Actions
	if w.dispatcher != nil && !w.client.IsDryRun() {
		actions, err := w.client.GetPendingActions(ctx)
		if err != nil {
			log.Printf("[cloudsync] Query pending actions error: %v", err)
		} else {
			for _, action := range actions {
				log.Printf("[cloudsync] Processing action %s (type: %s)", action.ID, action.Action)
				result, dispErr := w.dispatcher.Dispatch(ctx, action)
				if dispErr != nil {
					action.Status = "failed"
					action.Result = dispErr.Error()
				} else {
					action.Status = "completed"
					action.Result = result
				}
				if updErr := w.client.UpdateAction(ctx, action); updErr != nil {
					log.Printf("[cloudsync] Failed to update action %s: %v", action.ID, updErr)
				}
			}
		}
	}

	// 4. Sync Public Workspace
	if w.workspace != nil {
		wsPayload, err := w.workspace.GetPublicWorkspace(ctx)
		if err != nil {
			log.Printf("[cloudsync] Failed to read public workspace: %v", err)
		} else {
			if err := w.client.SyncWorkspace(ctx, wsPayload); err != nil {
				log.Printf("[cloudsync] Sync workspace error: %v", err)
			}
		}
	}

	return nil
}
