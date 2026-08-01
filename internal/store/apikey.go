package store

import (
	"database/sql"
	"time"
)

// ── APIKey ────────────────────────────────────────────────────────────────────

type APIKey struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	Name      string     `json:"name"`
	KeyHash   string     `json:"-"`
	LastUsed  *time.Time `json:"last_used"`
	ExpiresAt *time.Time `json:"expires_at"`
	Active    bool       `json:"active"`
	CreatedAt time.Time  `json:"created_at"`
}

func CreateAPIKey(db *sql.DB, userID, name, keyHash string) (*APIKey, error) {
	var k APIKey
	err := db.QueryRow(`
		INSERT INTO api_keys (user_id, name, key_hash)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, name, key_hash,
		          last_used, expires_at, active, created_at`,
		userID, name, keyHash,
	).Scan(
		&k.ID, &k.UserID, &k.Name, &k.KeyHash,
		&k.LastUsed, &k.ExpiresAt, &k.Active, &k.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func GetUserByAPIKey(db *sql.DB, keyHash string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT u.id, u.email, u.handle, u.password_hash, u.role,
		       u.mfa_secret, u.mfa_enabled, u.email_verified,
		       u.last_login, u.created_at, u.updated_at
		FROM users u
		JOIN api_keys k ON k.user_id = u.id
		WHERE k.key_hash = $1
		  AND k.active = true
		  AND (k.expires_at IS NULL OR k.expires_at > now())`,
		keyHash,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	db.Exec(
		"UPDATE api_keys SET last_used = now() WHERE key_hash = $1",
		keyHash,
	)

	return &u, nil
}

func ListAPIKeys(db *sql.DB, userID string) ([]APIKey, error) {
	rows, err := db.Query(`
		SELECT id, user_id, name, key_hash,
		       last_used, expires_at, active, created_at
		FROM api_keys
		WHERE user_id = $1 AND active = true
		ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		if err := rows.Scan(
			&k.ID, &k.UserID, &k.Name, &k.KeyHash,
			&k.LastUsed, &k.ExpiresAt, &k.Active, &k.CreatedAt,
		); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func RevokeAPIKey(db *sql.DB, keyID, userID string) error {
	_, err := db.Exec(
		"UPDATE api_keys SET active = false WHERE id = $1 AND user_id = $2",
		keyID, userID,
	)
	return err
}
