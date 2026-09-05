package sensor

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/store"
)

// Enroll handles POST /api/sensor/enroll
// First-contact exchange: validates org token, creates/links CMDB CI, stores public key.
func Enroll(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req domain.SensorEnrollRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
			return
		}

		// Step 1: Validate enrollment token (org API key)
		keyHash := auth.HashAPIKey(req.OrgEnrollmentToken)
		orgID, err := store.GetOrgIDByAPIKey(a.DB, keyHash)
		if err != nil || orgID == "" {
			http.Error(w, `{"error":"invalid enrollment token"}`, http.StatusUnauthorized)
			return
		}

		// Step 2: Validate public key format (must be 32-byte hex)
		pubKeyBytes, err := hex.DecodeString(strings.TrimSpace(req.PublicKey))
		if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
			http.Error(w, `{"error":"invalid ed25519 public key format"}`, http.StatusBadRequest)
			return
		}
		cleanPubKey := hex.EncodeToString(pubKeyBytes)

		// Step 3: Check if this public key is already enrolled
		existingSensor, err := store.GetSensorByPublicKey(a.DB, cleanPubKey)
		if err != nil {
			log.Printf("⚠️ Sensor Enroll: db error checking existing sensor: %v", err)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			return
		}
		if existingSensor != nil {
			// Idempotent re-enrollment response
			ciID := ""
			if existingSensor.ConfigItemID != nil {
				ciID = *existingSensor.ConfigItemID
			}
			json.NewEncoder(w).Encode(domain.SensorEnrollResponse{
				SensorID:               existingSensor.ID,
				OrgID:                  existingSensor.OrgID,
				ConfigItemID:           ciID,
				CheckinIntervalSeconds: 900,
				Status:                 existingSensor.Status,
			})
			return
		}

		// Step 4: Resolve Customer if logged-in user email is provided
		var assignedCustomerID *string
		if req.LoggedInUser != nil && strings.TrimSpace(*req.LoggedInUser) != "" {
			email := strings.ToLower(strings.TrimSpace(*req.LoggedInUser))
			cust, err := store.FindOrCreateCustomerByEmail(a.DB, orgID, email, nil)
			if err == nil && cust != nil {
				assignedCustomerID = &cust.ID
			}
		}

		// Step 5: Find or Create ConfigItem in CMDB
		ciMetadata, _ := json.Marshal(map[string]interface{}{
			"cpu_model":       req.Hardware.CPUModel,
			"cpu_cores":       req.Hardware.CPUCores,
			"total_ram_bytes": req.Hardware.TotalRAMBytes,
			"manufacturer":    req.Hardware.Manufacturer,
			"model":           req.Hardware.Model,
			"architecture":    req.Hardware.Architecture,
			"os_platform":     req.OS.Platform,
			"os_version":      req.OS.Version,
			"os_build":        req.OS.Build,
			"os_kernel":       req.OS.Kernel,
		})

		ciParams := store.CreateConfigItemParams{
			OrgID:        orgID,
			CIType:       store.CITypeLaptop,
			Name:         req.Hardware.Hostname,
			SerialNumber: &req.Hardware.SerialNumber,
			AssignedTo:   assignedCustomerID,
			Metadata:     ciMetadata,
		}
		ci, err := store.CreateConfigItem(a.DB, ciParams)
		if err != nil {
			log.Printf("⚠️ Sensor Enroll: failed to create config item: %v", err)
			http.Error(w, `{"error":"failed to create CMDB asset"}`, http.StatusInternalServerError)
			return
		}

		// Step 6: Create Sensor Record
		sensorParams := store.CreateSensorParams{
			OrgID:         orgID,
			ConfigItemID:  &ci.ID,
			PublicKey:     cleanPubKey,
			SensorVersion: req.SensorVersion,
			Hostname:      req.Hardware.Hostname,
			Platform:      req.OS.Platform,
			Status:        "active",
		}
		newSensor, err := store.CreateSensor(a.DB, sensorParams)
		if err != nil {
			log.Printf("⚠️ Sensor Enroll: failed to create sensor record: %v", err)
			http.Error(w, `{"error":"failed to register sensor"}`, http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(domain.SensorEnrollResponse{
			SensorID:               newSensor.ID,
			OrgID:                  newSensor.OrgID,
			ConfigItemID:           ci.ID,
			CheckinIntervalSeconds: 900,
			Status:                 newSensor.Status,
		})
	}
}

// Checkin handles POST /api/sensor/checkin
// Ingests heartbeats, baselines, and delta telemetry with signature verification and anti-replay.
func Checkin(a *app.App) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")

		var req domain.SensorCheckinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid json body"}`, http.StatusBadRequest)
			return
		}

		// Step 1: Look up Sensor
		sensor, err := store.GetSensorByID(a.DB, req.Envelope.SensorID)
		if err != nil || sensor == nil {
			http.Error(w, `{"error":"unknown sensor"}`, http.StatusUnauthorized)
			return
		}
		if sensor.Status != "active" {
			http.Error(w, `{"error":"sensor not active"}`, http.StatusForbidden)
			return
		}

		// Step 2: Anti-Replay: Validate timestamp window (±120 seconds)
		timeDiff := math.Abs(time.Since(req.Envelope.Timestamp).Seconds())
		if timeDiff > 120 {
			http.Error(w, `{"error":"timestamp skew exceeded"}`, http.StatusForbidden)
			return
		}

		// Step 3: Anti-Replay: Check and record Nonce
		fresh, err := store.CheckAndRecordNonce(a.DB, req.Envelope.Nonce, sensor.ID)
		if err != nil || !fresh {
			http.Error(w, `{"error":"duplicate nonce / replay detected"}`, http.StatusForbidden)
			return
		}

		// Step 4: Cryptographic Verification (Ed25519)
		pubKeyBytes, err := hex.DecodeString(sensor.PublicKey)
		if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
			http.Error(w, `{"error":"corrupted sensor key"}`, http.StatusInternalServerError)
			return
		}

		sigBytes, err := hex.DecodeString(strings.TrimSpace(req.Signature))
		if err != nil || len(sigBytes) != ed25519.SignatureSize {
			http.Error(w, `{"error":"invalid signature format"}`, http.StatusUnauthorized)
			return
		}

		canonicalData := computeCanonicalSignatureMessage(req)
		if !ed25519.Verify(pubKeyBytes, canonicalData, sigBytes) {
			http.Error(w, `{"error":"signature verification failed"}`, http.StatusUnauthorized)
			return
		}

		// Step 5: Process Payload by Type
		now := time.Now().UTC()

		switch req.Envelope.PayloadType {
		case domain.SensorPayloadHeartbeat:
			_ = store.UpdateSensorLastSeen(a.DB, sensor.ID, nil)

		case domain.SensorPayloadBaseline:
			_ = store.UpdateSensorLastSeen(a.DB, sensor.ID, &now)
			if req.Baseline != nil && sensor.ConfigItemID != nil {
				updateConfigItemFromBaseline(a, sensor.OrgID, *sensor.ConfigItemID, req.Baseline)
			}

		case domain.SensorPayloadDelta:
			_ = store.UpdateSensorLastSeen(a.DB, sensor.ID, nil)
			if req.Delta != nil {
				for _, evt := range req.Delta.Events {
					var oldVal *string
					if evt.OldValue != "" {
						oldVal = &evt.OldValue
					}
					var thresh *string
					if evt.ThresholdCrossed != "" {
						thresh = &evt.ThresholdCrossed
					}
					_ = store.RecordSensorEvent(a.DB, store.RecordSensorEventParams{
						SensorID:         sensor.ID,
						OrgID:            sensor.OrgID,
						Kind:             evt.Kind,
						Field:            evt.Field,
						OldValue:         oldVal,
						NewValue:         evt.NewValue,
						ThresholdCrossed: thresh,
						ObservedAt:       evt.ObservedAt,
					})
				}
				if req.Delta.State != nil && sensor.ConfigItemID != nil {
					updateConfigItemLiveState(a, sensor.OrgID, *sensor.ConfigItemID, req.Delta.State)
				}
			}
		}

		// Step 6: Return Response with Hints
		json.NewEncoder(w).Encode(domain.SensorCheckinResponse{
			Status: "ok",
		})
	}
}

// ── Internal Helpers ──────────────────────────────────────────────────────────

func computeCanonicalSignatureMessage(req domain.SensorCheckinRequest) []byte {
	h := sha256.New()
	fmt.Fprintf(h, "%s:%s:%d:%s:%s",
		req.Envelope.SensorID,
		req.Envelope.OrgID,
		req.Envelope.Timestamp.Unix(),
		req.Envelope.Nonce,
		req.Envelope.PayloadType,
	)

	if req.Baseline != nil {
		b, _ := json.Marshal(req.Baseline)
		h.Write(b)
	}
	if req.Delta != nil {
		b, _ := json.Marshal(req.Delta)
		h.Write(b)
	}
	return h.Sum(nil)
}

func updateConfigItemFromBaseline(a *app.App, orgID, ciID string, b *domain.SensorBaselineBody) {
	meta, _ := json.Marshal(map[string]interface{}{
		"cpu_model":              b.Hardware.CPUModel,
		"cpu_cores":              b.Hardware.CPUCores,
		"total_ram_bytes":        b.Hardware.TotalRAMBytes,
		"manufacturer":           b.Hardware.Manufacturer,
		"model":                  b.Hardware.Model,
		"architecture":           b.Hardware.Architecture,
		"os_platform":            b.OS.Platform,
		"os_version":             b.OS.Version,
		"os_build":               b.OS.Build,
		"os_kernel":              b.OS.Kernel,
		"uptime_seconds":         b.State.UptimeSeconds,
		"available_ram_bytes":    b.State.AvailableRAMBytes,
		"total_disk_bytes":       b.State.TotalDiskBytes,
		"free_disk_bytes":        b.State.FreeDiskBytes,
		"battery_percent":        b.State.BatteryPercent,
		"battery_health_percent": b.State.BatteryHealthPercent,
		"on_ac_power":            b.State.OnACPower,
		"last_boot_at":           b.State.LastBootAt,
	})
	_ = store.UpdateConfigItemMetadata(a.DB, orgID, ciID, meta)

	if b.State.LoggedInUser != nil && *b.State.LoggedInUser != "" {
		email := strings.ToLower(strings.TrimSpace(*b.State.LoggedInUser))
		cust, err := store.FindOrCreateCustomerByEmail(a.DB, orgID, email, nil)
		if err == nil && cust != nil {
			_ = store.UpdateConfigItemAssignment(a.DB, orgID, ciID, &cust.ID)
		}
	}
}

func updateConfigItemLiveState(a *app.App, orgID, ciID string, s *domain.SensorStateTelemetry) {
	existingCI, err := store.GetConfigItemByID(a.DB, orgID, ciID)
	if err != nil || existingCI == nil {
		return
	}

	var m map[string]interface{}
	_ = json.Unmarshal(existingCI.Metadata, &m)
	if m == nil {
		m = make(map[string]interface{})
	}

	m["uptime_seconds"] = s.UptimeSeconds
	m["available_ram_bytes"] = s.AvailableRAMBytes
	m["total_disk_bytes"] = s.TotalDiskBytes
	m["free_disk_bytes"] = s.FreeDiskBytes
	if s.BatteryPercent != nil {
		m["battery_percent"] = *s.BatteryPercent
	}
	if s.BatteryHealthPercent != nil {
		m["battery_health_percent"] = *s.BatteryHealthPercent
	}
	if s.OnACPower != nil {
		m["on_ac_power"] = *s.OnACPower
	}
	if s.LastBootAt != nil {
		m["last_boot_at"] = *s.LastBootAt
	}

	metaBytes, _ := json.Marshal(m)
	_ = store.UpdateConfigItemMetadata(a.DB, orgID, ciID, metaBytes)
}
