package monitor

import (
	"log"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/scanner"
	"github.com/psiloconvalley/404not403/internal/store"
)

// StartWorker launches the background monitoring goroutine.
// It runs forever, waking every 60 seconds to check due monitors.
// Call this once from main() as: go monitor.StartWorker(app)
func StartWorker(a *app.App) {
	log.Println("👻 Ghost Link Monitor worker started.")
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Run immediately on startup, then on every tick
	runChecks(a)

	for range ticker.C {
		runChecks(a)
	}
}

// runChecks fetches all due monitors and scans each one.
func runChecks(a *app.App) {
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
		checkMonitor(a, m)
	}
}

// checkMonitor runs a single scan and detects state changes.
func checkMonitor(a *app.App, m store.Monitor) {
	result := scanner.Scan(a, m.URL)

	// Always update the last_checked timestamp regardless of outcome
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
	if m.LastHash == "" && m.LastStatus == 0 {
		log.Printf("👻 Worker: first check recorded [%s] → %d", m.URL, result.StatusCode)
		return
	}

	// Detect changes
	statusChanged := result.StatusCode != m.LastStatus
	hashChanged   := result.BodyHash != m.LastHash

	if !statusChanged && !hashChanged {
		return
	}

	// Record the change as forensic evidence
	change := store.Change{
		MonitorID: m.ID,
		URL:       m.URL,
		OldStatus: m.LastStatus,
		NewStatus: result.StatusCode,
		OldHash:   m.LastHash,
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
		m.URL, m.LastStatus, result.StatusCode)
}
