package auth

import (
	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/psiloconvalley/404not403/internal/store"
)

// PasskeyUser adapts our store.User to the webauthn.User interface.
type PasskeyUser struct {
	User     *store.User
	Passkeys []store.Passkey
}

// WebAuthnID returns the user's unique ID as bytes.
func (u *PasskeyUser) WebAuthnID() []byte {
	return []byte(u.User.ID)
}

// WebAuthnName returns the user's email (used as username).
func (u *PasskeyUser) WebAuthnName() string {
	return u.User.Email
}

// WebAuthnDisplayName returns the user's handle.
func (u *PasskeyUser) WebAuthnDisplayName() string {
	return u.User.Handle
}

// WebAuthnCredentials converts stored passkeys to webauthn credentials.
func (u *PasskeyUser) WebAuthnCredentials() []webauthn.Credential {
	creds := make([]webauthn.Credential, len(u.Passkeys))
	for i, p := range u.Passkeys {
		creds[i] = webauthn.Credential{
			ID:        p.CredentialID,
			PublicKey: p.PublicKey,
			Flags: webauthn.CredentialFlags{
				BackupEligible: true,
				BackupState:    true,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    p.AAGUID,
				SignCount: uint32(p.SignCount),
			},
		}
	}
	return creds
}

// NewWebAuthn creates a configured WebAuthn instance.
func NewWebAuthn(rpID, rpOrigin, rpName string) (*webauthn.WebAuthn, error) {
	return webauthn.New(&webauthn.Config{
		RPID:          rpID,
		RPDisplayName: rpName,
		RPOrigins:     []string{rpOrigin},
	})
}
