package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Passkey represents a WebAuthn credential stored for a user.
type Passkey struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	CredentialID []byte     `json:"-"`
	PublicKey    []byte     `json:"-"`
	AAGUID       []byte     `json:"-"`
	SignCount    int64      `json:"sign_count"`
	Name         string     `json:"name"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

// CreatePasskey stores a new WebAuthn credential.
func CreatePasskey(db *sql.DB, userID string, credentialID, publicKey, aaguid []byte, name string) (*Passkey, error) {
	var p Passkey
	err := db.QueryRow(`
		INSERT INTO passkeys (user_id, credential_id, public_key, aaguid, name)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, credential_id, public_key, aaguid,
		          sign_count, name, created_at, last_used_at`,
		userID, credentialID, publicKey, aaguid, name,
	).Scan(
		&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey, &p.AAGUID,
		&p.SignCount, &p.Name, &p.CreatedAt, &p.LastUsedAt,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// GetPasskeysForUser returns all passkeys for a user.
func GetPasskeysForUser(db *sql.DB, userID string) ([]Passkey, error) {
	rows, err := db.Query(`
		SELECT id, user_id, credential_id, public_key, aaguid,
		       sign_count, name, created_at, last_used_at
		FROM passkeys
		WHERE user_id = $1
		ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var passkeys []Passkey
	for rows.Next() {
		var p Passkey
		if err := rows.Scan(
			&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey, &p.AAGUID,
			&p.SignCount, &p.Name, &p.CreatedAt, &p.LastUsedAt,
		); err != nil {
			return nil, err
		}
		passkeys = append(passkeys, p)
	}
	return passkeys, rows.Err()
}

// UpdatePasskeySignCount increments the sign count and updates last_used_at.
func UpdatePasskeySignCount(db *sql.DB, credentialID []byte, newCount int64) error {
	result, err := db.Exec(`
		UPDATE passkeys
		SET sign_count = $1, last_used_at = now()
		WHERE credential_id = $2`,
		newCount, credentialID,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("no passkey found with credential_id (len=%d)", len(credentialID))
	}
	return nil
}

// DeletePasskey removes a passkey by ID and user.
func DeletePasskey(db *sql.DB, userID, passkeyID string) error {
	_, err := db.Exec(`
		DELETE FROM passkeys
		WHERE id = $1 AND user_id = $2`,
		passkeyID, userID,
	)
	return err
}
