package domain

import (
	"time"
)

// ── Sensor Payload Types ───────────────────────────────────────────────────────

const (
	SensorPayloadEnroll     = "enroll"
	SensorPayloadBaseline   = "baseline"
	SensorPayloadDelta      = "delta"
	SensorPayloadHeartbeat  = "heartbeat"
	SensorPayloadDiagnostic = "diagnostic"
)

// Current schema version. Evolving schemas increment this integer.
const CurrentSensorSchemaVersion = 1

// ── Envelope & Request Container ──────────────────────────────────────────────

// SensorEnvelope carries metadata required to authenticate and route checkins.
type SensorEnvelope struct {
	SensorID      string    `json:"sensor_id"`
	OrgID         string    `json:"org_id"`
	Timestamp     time.Time `json:"timestamp"`
	Nonce         string    `json:"nonce"`
	PayloadType   string    `json:"payload_type"`
	SensorVersion string    `json:"sensor_version"`
	SchemaVersion int       `json:"schema_version"`
}

// SensorCheckinRequest is the signed payload received from a sensor.
// Signature covers SHA256(canonical(Envelope) + canonical(Body)).
type SensorCheckinRequest struct {
	Envelope  SensorEnvelope      `json:"envelope"`
	Signature string              `json:"signature"` // hex or base64-encoded Ed25519 signature
	Baseline  *SensorBaselineBody `json:"baseline,omitempty"`
	Delta     *SensorDeltaBody    `json:"delta,omitempty"`
}

// SensorEnrollRequest is sent on first contact to exchange the org enrollment token
// and establish the sensor's public key.
type SensorEnrollRequest struct {
	OrgEnrollmentToken string             `json:"enrollment_token"`
	PublicKey          string             `json:"public_key"` // hex-encoded Ed25519 public key (32 bytes)
	SensorVersion      string             `json:"sensor_version"`
	Hardware           SensorHardwareInfo `json:"hardware"`
	OS                 SensorOSInfo       `json:"os"`
	LoggedInUser       *string            `json:"logged_in_user,omitempty"`
}

// ── Telemetry & System Specs ──────────────────────────────────────────────────

// SensorHardwareInfo represents physical machine specs.
type SensorHardwareInfo struct {
	SerialNumber  string `json:"serial_number"`
	Hostname      string `json:"hostname"`
	Manufacturer  string `json:"manufacturer"`
	Model         string `json:"model"`
	Architecture  string `json:"architecture"` // arm64, amd64
	CPUModel      string `json:"cpu_model"`
	CPUCores      int    `json:"cpu_cores"`
	TotalRAMBytes uint64 `json:"total_ram_bytes"`
}

// SensorOSInfo represents operating system details.
type SensorOSInfo struct {
	Platform string `json:"platform"` // darwin, windows, linux
	Version  string `json:"version"`  // e.g. 14.5
	Build    string `json:"build"`    // e.g. 23F79
	Kernel   string `json:"kernel"`   // e.g. 23.5.0
}

// SensorStateTelemetry represents live system health and resource consumption.
type SensorStateTelemetry struct {
	LoggedInUser         *string    `json:"logged_in_user,omitempty"`
	UptimeSeconds        uint64     `json:"uptime_seconds"`
	AvailableRAMBytes    uint64     `json:"available_ram_bytes"`
	TotalDiskBytes       uint64     `json:"total_disk_bytes"`
	FreeDiskBytes        uint64     `json:"free_disk_bytes"`
	BatteryPercent       *int       `json:"battery_percent,omitempty"`
	BatteryHealthPercent *int       `json:"battery_health_percent,omitempty"`
	OnACPower            *bool      `json:"on_ac_power,omitempty"`
	LastBootAt           *time.Time `json:"last_boot_at,omitempty"`
}

// SensorBaselineBody is sent daily or upon significant state reset.
type SensorBaselineBody struct {
	Hardware SensorHardwareInfo   `json:"hardware"`
	OS       SensorOSInfo         `json:"os"`
	State    SensorStateTelemetry `json:"state"`
}

// ── Delta Events ──────────────────────────────────────────────────────────────

// SensorDeltaEvent records a specific state change observed on the machine.
type SensorDeltaEvent struct {
	Kind             string    `json:"kind"` // e.g. disk_low, ram_pressure_high, os_updated, user_changed
	Field            string    `json:"field"`
	OldValue         string    `json:"old_value,omitempty"`
	NewValue         string    `json:"new_value"`
	ThresholdCrossed string    `json:"threshold_crossed,omitempty"`
	ObservedAt       time.Time `json:"observed_at"`
}

// SensorDeltaBody contains one or more state transition events.
type SensorDeltaBody struct {
	Events []SensorDeltaEvent    `json:"events"`
	State  *SensorStateTelemetry `json:"state,omitempty"` // optional updated state snapshot
}

// ── Server Response & Hints ───────────────────────────────────────────────────

// SensorEnrollResponse returns the permanent sensor credentials and org context.
type SensorEnrollResponse struct {
	SensorID               string `json:"sensor_id"`
	OrgID                  string `json:"org_id"`
	ConfigItemID           string `json:"config_item_id"`
	CheckinIntervalSeconds int    `json:"checkin_interval_seconds"`
	Status                 string `json:"status"` // active, pending_approval
}

// SensorCheckinResponse acknowledges receipt and provides server-side hints.
type SensorCheckinResponse struct {
	Status string            `json:"status"` // ok, error
	Hints  *SensorHintsBlock `json:"hints,omitempty"`
}

// SensorHintsBlock allows the server to request specific actions without arbitrary command execution.
type SensorHintsBlock struct {
	NextCheckinSeconds     *int                   `json:"next_checkin_seconds,omitempty"`
	RequestDiagnostic      *DiagnosticRequestHint `json:"request_diagnostic,omitempty"`
	SensorVersionAvailable *string                `json:"sensor_version_available,omitempty"`
}

// DiagnosticRequestHint requests Tier 3 diagnostics scoped to an active ticket.
type DiagnosticRequestHint struct {
	TicketID   string   `json:"ticket_id"`
	Categories []string `json:"categories"` // e.g. ["top_processes", "recent_crashes"]
}
