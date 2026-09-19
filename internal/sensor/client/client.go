package client

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/psiloconvalley/404not403/internal/domain"
	"github.com/psiloconvalley/404not403/internal/sensor/collector"
	"github.com/psiloconvalley/404not403/internal/sensor/keychain"
)

type SensorClient struct {
	Config     *keychain.Config
	Collector  collector.SystemCollector
	HTTPClient *http.Client
	PrivateKey ed25519.PrivateKey
	LastState  *domain.SensorStateTelemetry
}

func New(cfg *keychain.Config) (*SensorClient, error) {
	priv, err := keychain.PrivateKeyFromHex(cfg.PrivateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("invalid private key in config: %w", err)
	}

	return &SensorClient{
		Config:     cfg,
		Collector:  collector.New(),
		HTTPClient: &http.Client{Timeout: 15 * time.Second},
		PrivateKey: priv,
	}, nil
}

// Enroll performs first-time sensor enrollment against the server.
func Enroll(serverURL, enrollmentToken string) (*keychain.Config, error) {
	pub, priv, err := keychain.GenerateKeypair()
	if err != nil {
		return nil, err
	}
	pubHex, privHex := keychain.KeypairToHex(pub, priv)

	col := collector.New()
	hw, err := col.CollectHardware()
	if err != nil {
		return nil, fmt.Errorf("failed to collect hardware specs: %w", err)
	}
	osInfo, err := col.CollectOS()
	if err != nil {
		return nil, fmt.Errorf("failed to collect OS info: %w", err)
	}
	st, _ := col.CollectState()

	reqPayload := domain.SensorEnrollRequest{
		OrgEnrollmentToken: enrollmentToken,
		PublicKey:          pubHex,
		SensorVersion:      "0.1.0",
		Hardware:           hw,
		OS:                 osInfo,
		LoggedInUser:       st.LoggedInUser,
	}

	data, err := json.Marshal(reqPayload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/sensor/enroll", serverURL)
	resp, err := http.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("enroll request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enrollment rejected with status: %d", resp.StatusCode)
	}

	var enrollResp domain.SensorEnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&enrollResp); err != nil {
		return nil, fmt.Errorf("invalid enrollment response: %w", err)
	}

	interval := enrollResp.CheckinIntervalSeconds
	if interval <= 0 {
		interval = 900
	}

	return &keychain.Config{
		SensorID:               enrollResp.SensorID,
		OrgID:                  enrollResp.OrgID,
		ConfigItemID:           enrollResp.ConfigItemID,
		ServerURL:              serverURL,
		PrivateKeyHex:          privHex,
		PublicKeyHex:           pubHex,
		CheckinIntervalSeconds: interval,
	}, nil
}

// SendBaseline collects full specs and live state and sends a baseline checkin.
func (c *SensorClient) SendBaseline() error {
	hw, err := c.Collector.CollectHardware()
	if err != nil {
		return err
	}
	osInfo, err := c.Collector.CollectOS()
	if err != nil {
		return err
	}
	st, err := c.Collector.CollectState()
	if err != nil {
		return err
	}
	c.LastState = &st

	body := domain.SensorBaselineBody{
		Hardware: hw,
		OS:       osInfo,
		State:    st,
	}

	return c.sendSignedCheckin(domain.SensorPayloadBaseline, &body, nil)
}

// SendHeartbeat sends a minimal liveness proof.
func (c *SensorClient) SendHeartbeat() error {
	return c.sendSignedCheckin(domain.SensorPayloadHeartbeat, nil, nil)
}

// CheckAndSendDeltas observes live state and pushes a delta checkin if thresholds changed.
func (c *SensorClient) CheckAndSendDeltas() error {
	curr, err := c.Collector.CollectState()
	if err != nil {
		return err
	}

	if c.LastState == nil {
		c.LastState = &curr
		return nil
	}

	var events []domain.SensorDeltaEvent
	now := time.Now().UTC()

	// Check 1: Disk Space Crossed 20% / 10% / 5%
	if curr.TotalDiskBytes > 0 {
		oldPct := float64(c.LastState.FreeDiskBytes) / float64(c.LastState.TotalDiskBytes) * 100
		newPct := float64(curr.FreeDiskBytes) / float64(curr.TotalDiskBytes) * 100

		if oldPct >= 20 && newPct < 20 {
			events = append(events, domain.SensorDeltaEvent{
				Kind:             "disk_low",
				Field:            "free_disk_bytes",
				OldValue:         fmt.Sprintf("%d", c.LastState.FreeDiskBytes),
				NewValue:         fmt.Sprintf("%d", curr.FreeDiskBytes),
				ThresholdCrossed: "20_percent",
				ObservedAt:       now,
			})
		}
		if oldPct >= 10 && newPct < 10 {
			events = append(events, domain.SensorDeltaEvent{
				Kind:             "disk_critical",
				Field:            "free_disk_bytes",
				OldValue:         fmt.Sprintf("%d", c.LastState.FreeDiskBytes),
				NewValue:         fmt.Sprintf("%d", curr.FreeDiskBytes),
				ThresholdCrossed: "10_percent",
				ObservedAt:       now,
			})
		}
	}

	// Check 2: User Switch
	oldUser := ""
	if c.LastState.LoggedInUser != nil {
		oldUser = *c.LastState.LoggedInUser
	}
	newUser := ""
	if curr.LoggedInUser != nil {
		newUser = *curr.LoggedInUser
	}
	if oldUser != newUser && newUser != "" {
		events = append(events, domain.SensorDeltaEvent{
			Kind:       "user_changed",
			Field:      "logged_in_user",
			OldValue:   oldUser,
			NewValue:   newUser,
			ObservedAt: now,
		})
	}

	c.LastState = &curr

	if len(events) > 0 {
		deltaBody := domain.SensorDeltaBody{
			Events: events,
			State:  &curr,
		}
		return c.sendSignedCheckin(domain.SensorPayloadDelta, nil, &deltaBody)
	}

	return nil
}

// ── Signing & Dispatch ────────────────────────────────────────────────────────

func (c *SensorClient) sendSignedCheckin(payloadType string, baseline *domain.SensorBaselineBody, delta *domain.SensorDeltaBody) error {
	nonceBytes := make([]byte, 8)
	_, _ = rand.Read(nonceBytes)
	nonce := hex.EncodeToString(nonceBytes)

	env := domain.SensorEnvelope{
		SensorID:      c.Config.SensorID,
		OrgID:         c.Config.OrgID,
		Timestamp:     time.Now().UTC(),
		Nonce:         nonce,
		PayloadType:   payloadType,
		SensorVersion: "0.1.0",
		SchemaVersion: domain.CurrentSensorSchemaVersion,
	}

	req := domain.SensorCheckinRequest{
		Envelope: env,
		Baseline: baseline,
		Delta:    delta,
	}

	// Sign
	canonical := computeCanonicalMessage(req)
	sig := ed25519.Sign(c.PrivateKey, canonical)
	req.Signature = hex.EncodeToString(sig)

	data, err := json.Marshal(req)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/sensor/checkin", c.Config.ServerURL)
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("checkin network failure: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server rejected checkin: status %d", resp.StatusCode)
	}

	return nil
}

func computeCanonicalMessage(req domain.SensorCheckinRequest) []byte {
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
