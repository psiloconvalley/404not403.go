package store

import (
	"database/sql"
	"time"
)

// ── Password Reset ────────────────────────────────────────────────────────────

func CreatePasswordReset(db *sql.DB, userID, tokenHash string, expiresAt time.Time) error {
	_, err := db.Exec(`
		INSERT INTO password_resets (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)`,
		userID, tokenHash, expiresAt,
	)
	return err
}

func GetPasswordReset(db *sql.DB, tokenHash string) (id string, userID string, err error) {
	err = db.QueryRow(`
		SELECT id, user_id FROM password_resets
		WHERE token_hash = $1
		  AND used = false
		  AND expires_at > now()`,
		tokenHash,
	).Scan(&id, &userID)
	if err == sql.ErrNoRows {
		return "", "", nil
	}
	return id, userID, err
}

func MarkResetUsed(db *sql.DB, id string) error {
	_, err := db.Exec(
		"UPDATE password_resets SET used = true WHERE id = $1", id,
	)
	return err
}
