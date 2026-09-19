package sensor

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/psiloconvalley/404not403/internal/app"
	"github.com/psiloconvalley/404not403/internal/auth"
	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/store"
)

func TestSensorLifecycle(t *testing.T) {
	// 1. Setup Test Database (skip if no DATABASE_URL is set in local test environment)
	dbURL := "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		t.Skip("Skipping test: no local postgres available on default port")
		return
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Skip("Skipping test: could not ping local postgres")
		return
	}

	// Clean up tables
	db.Exec("TRUNCATE TABLE sensor_events, sensors, sensor_nonce_cache, config_items, api_keys, org_members, users, organizations CASCADE")

	a := &app.App{DB: db}

	// 2. Generate Test User and Org
	user, err := store.CreateUser(db, "admin@test-org.com", "admin-handle", "argon2id-hash-string-that-is-valid")
	if err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}

	_, err = store.CreateOrg(db, "test-org", "test-org-slug", user.ID)
	if err != nil {
		t.Fatalf("failed to create test org: %v", err)
	}

	rawToken := "404_key_test_token_string_123"
	tokenHash := auth.HashAPIKey(rawToken)
	_, err = store.CreateAPIKey(db, user.ID, "test-key", tokenHash)
	if err != nil {
		t.Fatalf("failed to create API Key: %v", err)
	}

	// 3. Generate Ed25519 Keypair
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate keypair: %v", err)
	}
	pubHex := hex.EncodeToString(pubKey)

	// 4. Test Enroll Handler
	enrollReq := domain.SensorEnrollRequest{
		OrgEnrollmentToken: rawToken,
		PublicKey:          pubHex,
		SensorVersion:      "0.1.0",
		Hardware: domain.SensorHardwareInfo{
			SerialNumber:  "TEST-SERIAL-12345",
			Hostname:      "test-macbook",
			Manufacturer:  "Apple",
			Model:         "MacBookPro18,2",
			Architecture:  "arm64",
			CPUModel:      "Apple M1 Max",
			CPUCores:      10,
			TotalRAMBytes: 34359738368,
		},
		OS: domain.SensorOSInfo{
			Platform: "darwin",
			Version:  "14.5",
			Build:    "23F79",
			Kernel:   "23.5.0",
		},
	}

	enrollBody, _ := json.Marshal(enrollReq)
	rec := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/api/sensor/enroll", bytes.NewReader(enrollBody))
	Enroll(a).ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected enroll status 201, got %d, body: %s", rec.Code, rec.Body.String())
	}

	var enrollResp domain.SensorEnrollResponse
	json.Unmarshal(rec.Body.Bytes(), &enrollResp)

	if enrollResp.SensorID == "" || enrollResp.ConfigItemID == "" {
		t.Errorf("enroll response missing sensor_id or config_item_id: %+v", enrollResp)
	}

	// Test Idempotency (enrolling with same public key should return existing details)
	rec2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/sensor/enroll", bytes.NewReader(enrollBody))
	Enroll(a).ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("expected enroll idempotency status 200, got %d", rec2.Code)
	}

	// 5. Test Checkin Handler (Heartbeat)
	env := domain.SensorEnvelope{
		SensorID:      enrollResp.SensorID,
		OrgID:         enrollResp.OrgID,
		Timestamp:     time.Now().UTC(),
		Nonce:         "nonce_1",
		PayloadType:   domain.SensorPayloadHeartbeat,
		SensorVersion: "0.1.0",
		SchemaVersion: 1,
	}

	checkinReq := domain.SensorCheckinRequest{
		Envelope: env,
	}

	// Sign envelope
	canonical := computeCanonicalSignatureMessage(checkinReq)
	sig := ed25519.Sign(privKey, canonical)
	checkinReq.Signature = hex.EncodeToString(sig)

	checkinBody, _ := json.Marshal(checkinReq)
	recCheck := httptest.NewRecorder()
	reqCheck, _ := http.NewRequest(http.MethodPost, "/api/sensor/checkin", bytes.NewReader(checkinBody))
	Checkin(a).ServeHTTP(recCheck, reqCheck)

	if recCheck.Code != http.StatusOK {
		t.Errorf("expected checkin status 200, got %d, body: %s", recCheck.Code, recCheck.Body.String())
	}

	// Test Replay Attack (resending same nonce)
	recReplay := httptest.NewRecorder()
	reqReplay, _ := http.NewRequest(http.MethodPost, "/api/sensor/checkin", bytes.NewReader(checkinBody))
	Checkin(a).ServeHTTP(recReplay, reqReplay)
	if recReplay.Code != http.StatusForbidden {
		t.Errorf("expected replay request to be rejected with 403, got %d", recReplay.Code)
	}

	// Test Timestamp Skew Protection (old timestamp)
	oldEnv := env
	oldEnv.Timestamp = time.Now().Add(-10 * time.Minute)
	oldEnv.Nonce = "nonce_skew"
	skewReq := domain.SensorCheckinRequest{
		Envelope: oldEnv,
	}
	skewCanonical := computeCanonicalSignatureMessage(skewReq)
	skewSig := ed25519.Sign(privKey, skewCanonical)
	skewReq.Signature = hex.EncodeToString(skewSig)

	skewBody, _ := json.Marshal(skewReq)
	recSkew := httptest.NewRecorder()
	reqSkew, _ := http.NewRequest(http.MethodPost, "/api/sensor/checkin", bytes.NewReader(skewBody))
	Checkin(a).ServeHTTP(recSkew, reqSkew)

	if recSkew.Code != http.StatusForbidden {
		t.Errorf("expected skewed checkin to be rejected with 403, got %d", recSkew.Code)
	}
}
