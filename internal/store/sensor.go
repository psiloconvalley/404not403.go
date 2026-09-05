package store

import (
	"database/sql"
	"time"
)

// ── Sensor DB Models ──────────────────────────────────────────────────────────

type Sensor struct {
	ID             string     `json:"id"`
	OrgID          string     `json:"org_id"`
	ConfigItemID   *string    `json:"config_item_id,omitempty"`
	PublicKey      string     `json:"public_key"`
	Status         string     `json:"status"` // active, pending_approval, revoked
	SensorVersion  string     `json:"sensor_version"`
	Hostname       string     `json:"hostname"`
	Platform       string     `json:"platform"`
	LastSeenAt     time.Time  `json:"last_seen_at"`
	LastBaselineAt *time.Time `json:"last_baseline_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type SensorEvent struct {
	ID               string    `json:"id"`
	SensorID         string    `json:"sensor_id"`
	OrgID            string    `json:"org_id"`
	Kind             string    `json:"kind"`
	Field            string    `json:"field"`
	OldValue         *string   `json:"old_value,omitempty"`
	NewValue         string    `json:"new_value"`
	ThresholdCrossed *string   `json:"threshold_crossed,omitempty"`
	ObservedAt       time.Time `json:"observed_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// ── Sensor Queries ────────────────────────────────────────────────────────────

type CreateSensorParams struct {
	OrgID          string
	ConfigItemID   *string
	PublicKey      string
	SensorVersion  string
	Hostname       string
	Platform       string
	Status         string
}

func CreateSensor(db *sql.DB, p CreateSensorParams) (*Sensor, error) {
	if p.Status == "" {
		p.Status = "active"
	}

	var s Sensor
	err := db.QueryRow(`
		INSERT INTO sensors (
			org_id, config_item_id, public_key, sensor_version,
			hostname, platform, status
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, org_id, config_item_id, public_key, status,
		          sensor_version, hostname, platform, last_seen_at,
		          last_baseline_at, created_at, updated_at`,
		p.OrgID, p.ConfigItemID, p.PublicKey, p.SensorVersion,
		p.Hostname, p.Platform, p.Status,
	).Scan(
		&s.ID, &s.OrgID, &s.ConfigItemID, &s.PublicKey, &s.Status,
		&s.SensorVersion, &s.Hostname, &s.Platform, &s.LastSeenAt,
		&s.LastBaselineAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetSensorByID(db *sql.DB, sensorID string) (*Sensor, error) {
	var s Sensor
	err := db.QueryRow(`
		SELECT id, org_id, config_item_id, public_key, status,
		       sensor_version, hostname, platform, last_seen_at,
		       last_baseline_at, created_at, updated_at
		FROM sensors
		WHERE id = $1`,
		sensorID,
	).Scan(
		&s.ID, &s.OrgID, &s.ConfigItemID, &s.PublicKey, &s.Status,
		&s.SensorVersion, &s.Hostname, &s.Platform, &s.LastSeenAt,
		&s.LastBaselineAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func GetSensorByPublicKey(db *sql.DB, publicKey string) (*Sensor, error) {
	var s Sensor
	err := db.QueryRow(`
		SELECT id, org_id, config_item_id, public_key, status,
		       sensor_version, hostname, platform, last_seen_at,
		       last_baseline_at, created_at, updated_at
		FROM sensors
		WHERE public_key = $1`,
		publicKey,
	).Scan(
		&s.ID, &s.OrgID, &s.ConfigItemID, &s.PublicKey, &s.Status,
		&s.SensorVersion, &s.Hostname, &s.Platform, &s.LastSeenAt,
		&s.LastBaselineAt, &s.CreatedAt, &s.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func UpdateSensorLastSeen(db *sql.DB, sensorID string, baselineTime *time.Time) error {
	if baselineTime != nil {
		_, err := db.Exec(`
			UPDATE sensors
			SET last_seen_at = now(), last_baseline_at = $1, updated_at = now()
			WHERE id = $2`,
			*baselineTime, sensorID,
		)
		return err
	}

	_, err := db.Exec(`
		UPDATE sensors
		SET last_seen_at = now(), updated_at = now()
		WHERE id = $1`,
		sensorID,
	)
	return err
}

// ── Nonce Cache (Anti-Replay) ─────────────────────────────────────────────────

// CheckAndRecordNonce attempts to insert a nonce.
// Returns true if nonce is fresh (inserted successfully).
// Returns false if nonce was already seen (replay detected).
func CheckAndRecordNonce(db *sql.DB, nonce, sensorID string) (bool, error) {
	res, err := db.Exec(`
		INSERT INTO sensor_nonce_cache (nonce, sensor_id, seen_at)
		VALUES ($1, $2, now())
		ON CONFLICT (nonce) DO NOTHING`,
		nonce, sensorID,
	)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// ── Sensor Events ─────────────────────────────────────────────────────────────

type RecordSensorEventParams struct {
	SensorID         string
	OrgID            string
	Kind             string
	Field            string
	OldValue         *string
	NewValue         string
	ThresholdCrossed *string
	ObservedAt       time.Time
}

func RecordSensorEvent(db *sql.DB, p RecordSensorEventParams) error {
	_, err := db.Exec(`
		INSERT INTO sensor_events (
			sensor_id, org_id, kind, field, old_value, new_value,
			threshold_crossed, observed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		p.SensorID, p.OrgID, p.Kind, p.Field, p.OldValue, p.NewValue,
		p.ThresholdCrossed, p.ObservedAt,
	)
	return err
}

func ListSensorEvents(db *sql.DB, orgID, sensorID string, limit int) ([]SensorEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}

	rows, err := db.Query(`
		SELECT id, sensor_id, org_id, kind, field, old_value, new_value,
		       threshold_crossed, observed_at, created_at
		FROM sensor_events
		WHERE org_id = $1 AND sensor_id = $2
		ORDER BY observed_at DESC
		LIMIT $3`,
		orgID, sensorID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []SensorEvent
	for rows.Next() {
		var e SensorEvent
		if err := rows.Scan(
			&e.ID, &e.SensorID, &e.OrgID, &e.Kind, &e.Field, &e.OldValue,
			&e.NewValue, &e.ThresholdCrossed, &e.ObservedAt, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
