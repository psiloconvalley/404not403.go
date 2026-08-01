package store

import (
	"database/sql"
	"time"
)

// ── User ──────────────────────────────────────────────────────────────────────

type User struct {
	ID            string     `json:"id"`
	Email         string     `json:"email"`
	Handle        string     `json:"handle"`
	PasswordHash  string     `json:"-"`
	Role          string     `json:"role"`
	MFASecret     *string    `json:"-"`
	MFAEnabled    bool       `json:"mfa_enabled"`
	EmailVerified bool       `json:"email_verified"`
	LastLogin     *time.Time `json:"last_login"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func CreateUser(db *sql.DB, email, handle, passwordHash string) (*User, error) {
	var u User
	err := db.QueryRow(`
		INSERT INTO users (email, handle, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, email, handle, password_hash, role,
		          mfa_secret, mfa_enabled, email_verified,
		          last_login, created_at, updated_at`,
		email, handle, passwordHash,
	).Scan(
		&u.ID, &u.Email, &u.Handle, &u.PasswordHash, &u.Role,
		&u.MFASecret, &u.MFAEnabled, &u.EmailVerified,
		&u.LastLogin, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, email, handle, password_hash, role,
		       mfa_secret, mfa_enabled, email_verified,
		       last_login, created_at, updated_at
		FROM users WHERE email = $1`,
		email,
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
	return &u, nil
}

func GetUserByID(db *sql.DB, id string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, email, handle, password_hash, role,
		       mfa_secret, mfa_enabled, email_verified,
		       last_login, created_at, updated_at
		FROM users WHERE id = $1`,
		id,
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
	return &u, nil
}

func GetUserByHandle(db *sql.DB, handle string) (*User, error) {
	var u User
	err := db.QueryRow(`
		SELECT id, email, handle, password_hash, role,
		       mfa_secret, mfa_enabled, email_verified,
		       last_login, created_at, updated_at
		FROM users WHERE handle = $1`,
		handle,
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
	return &u, nil
}

func UpdateLastLogin(db *sql.DB, userID string) error {
	_, err := db.Exec(
		"UPDATE users SET last_login = now(), updated_at = now() WHERE id = $1",
		userID,
	)
	return err
}

func StoreMFASecret(db *sql.DB, userID, encryptedSecret string) error {
	_, err := db.Exec(`
		UPDATE users
		SET mfa_secret = $1,
		    updated_at = now()
		WHERE id = $2`,
		encryptedSecret, userID,
	)
	return err
}

func EnableMFA(db *sql.DB, userID, encryptedSecret string) error {
	_, err := db.Exec(`
		UPDATE users
		SET mfa_secret  = $1,
		    mfa_enabled = true,
		    updated_at  = now()
		WHERE id = $2`,
		encryptedSecret, userID,
	)
	return err
}

func DisableMFA(db *sql.DB, userID string) error {
	_, err := db.Exec(`
		UPDATE users
		SET mfa_secret  = NULL,
		    mfa_enabled = false,
		    updated_at  = now()
		WHERE id = $1`,
		userID,
	)
	return err
}

func UpdatePassword(db *sql.DB, userID, newHash string) error {
	_, err := db.Exec(
		"UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2",
		newHash, userID,
	)
	return err
}

func UpgradeUser(db *sql.DB, userID, newRole string) error {
	_, err := db.Exec(
		"UPDATE users SET role = $1, updated_at = now() WHERE id = $2",
		newRole, userID,
	)
	return err
}
