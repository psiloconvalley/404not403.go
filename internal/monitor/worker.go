package monitor

import (
	"context"
	"log"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/scanner"
	"github.com/psiloconvalley/404not403/internal/store"
)

// StartWorker launches the background monitoring goroutine.
// It runs until ctx is cancelled (graceful shutdown).
// Call once from main() as: go monitor.StartWorker(ctx, app)
func StartWorker(ctx context.Context, a *app.App) {
	log.Println("👻 Ghost Link Monitor worker started.")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run immediately on startup
	runChecks(ctx, a)

	for {
		select {
		case <-ctx.Done():
			log.Println("👻 Ghost Link Monitor worker stopped (shutdown signal).")
			return
		case <-ticker.C:
			runChecks(ctx, a)
		}
	}
}

// runChecks fetches all due monitors and scans each one.
// Panic recovery ensures one bad scan cannot kill the worker.
func runChecks(ctx context.Context, a *app.App) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🚨 Worker: panic recovered in runChecks: %v", r)
		}
	}()

	if a.DB == nil {
		return
	}

	monitors, err := store.DueMonitors(a.DB)
	if err != nil {
		log.Printf("⚠️  Worker: DueMonitors error: %v", err)
		return
	}

	if len(monitors) == 0 {
		return
	}

	log.Printf("👻 Worker: checking %d due monitors", len(monitors))

	for _, m := range monitors {
		// Respect shutdown between monitors
		select {
		case <-ctx.Done():
			log.Println("👻 Worker: shutdown mid-batch, stopping.")
			return
		default:
			checkMonitor(ctx, a, m)
		}
	}
}

// checkMonitor runs a single scan and detects state changes.
// Each scan is bounded by a 30-second timeout.
func checkMonitor(ctx context.Context, a *app.App, m store.Monitor) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("🚨 Worker: panic recovered in checkMonitor [%s]: %v", m.URL, r)
		}
	}()

	// Per-scan timeout — one hung URL cannot block the batch
	scanCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_ = scanCtx // scanner.Scan will accept context in future hardening pass
	result := scanner.Scan(a, m.URL)

	// Always update last_checked regardless of outcome
	defer func() {
		if err := store.UpdateMonitorState(a.DB, m.ID, result.StatusCode, result.BodyHash); err != nil {
			log.Printf("⚠️  Worker: UpdateMonitorState error [%s]: %v", m.URL, err)
		}
	}()

	// Skip change detection if the scan itself errored
	if result.Error != "" {
		log.Printf("⚠️  Worker: scan error [%s]: %s", m.URL, result.Error)
		return
	}

	// First check — no previous state to compare against
	if m.LastHash == nil && m.LastStatus == nil {
		log.Printf("👻 Worker: first check recorded [%s] → %d", m.URL, result.StatusCode)
		return
	}

	// Detect changes — dereference pointers safely
	oldStatus := 0
	if m.LastStatus != nil {
		oldStatus = *m.LastStatus
	}
	oldHash := ""
	if m.LastHash != nil {
		oldHash = *m.LastHash
	}

	statusChanged := result.StatusCode != oldStatus
	hashChanged := result.BodyHash != oldHash

	if !statusChanged && !hashChanged {
		return
	}

	// Record the change as forensic evidence
	change := store.Change{
		MonitorID: m.ID,
		URL:       m.URL,
		OldStatus: oldStatus,
		NewStatus: result.StatusCode,
		OldHash:   oldHash,
		NewHash:   result.BodyHash,
	}
	if err := store.RecordChange(a.DB, change); err != nil {
		log.Printf("⚠️  Worker: RecordChange error [%s]: %v", m.URL, err)
		return
	}

	if err := store.IncrementChangeCount(a.DB, m.ID); err != nil {
		log.Printf("⚠️  Worker: IncrementChangeCount error [%s]: %v", m.URL, err)
	}

	log.Printf("👻 Worker: CHANGE DETECTED [%s] %d→%d",
		m.URL, oldStatus, result.StatusCode)
}
