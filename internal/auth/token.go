package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload for 404NOT403 sessions.
type Claims struct {
	Handle string `json:"handle"`
	Role   string `json:"role"`
	MFA    bool   `json:"mfa"`
	jwt.RegisteredClaims
}

var (
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	jwtIssuer  string
)

// InitKeys loads RSA keys from environment variables.
// Call once at startup from main().
func InitKeys() error {
	privPEM := os.Getenv("JWT_PRIVATE_KEY")
	if privPEM == "" {
		return fmt.Errorf("JWT_PRIVATE_KEY not set")
	}

	pubPEM := os.Getenv("JWT_PUBLIC_KEY")
	if pubPEM == "" {
		return fmt.Errorf("JWT_PUBLIC_KEY not set")
	}

	jwtIssuer = os.Getenv("JWT_ISSUER")
	if jwtIssuer == "" {
		jwtIssuer = "404not403.com"
	}

	// Parse private key
	privBlock, _ := pem.Decode([]byte(privPEM))
	if privBlock == nil {
		return fmt.Errorf("failed to decode JWT_PRIVATE_KEY PEM")
	}

	privParsed, err := x509.ParsePKCS8PrivateKey(privBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	var ok bool
	privateKey, ok = privParsed.(*rsa.PrivateKey)
	if !ok {
		return fmt.Errorf("JWT_PRIVATE_KEY is not RSA")
	}

	// Parse public key
	pubBlock, _ := pem.Decode([]byte(pubPEM))
	if pubBlock == nil {
		return fmt.Errorf("failed to decode JWT_PUBLIC_KEY PEM")
	}

	pubParsed, err := x509.ParsePKIXPublicKey(pubBlock.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok = pubParsed.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("JWT_PUBLIC_KEY is not RSA")
	}

	return nil
}

// GenerateToken creates a signed JWT for the given user.
// Tokens expire after 24 hours.
func GenerateToken(userID, handle, role string, mfaVerified bool) (string, error) {
	now := time.Now()

	claims := Claims{
		Handle: handle,
		Role:   role,
		MFA:    mfaVerified,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    jwtIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(privateKey)
}

// VerifyToken validates a JWT string and returns the claims.
func VerifyToken(tokenStr string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return publicKey, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}
