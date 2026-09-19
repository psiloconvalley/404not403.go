package keychain

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	SensorID               string `json:"sensor_id"`
	OrgID                  string `json:"org_id"`
	ConfigItemID           string `json:"config_item_id"`
	ServerURL              string `json:"server_url"`
	PrivateKeyHex          string `json:"private_key_hex"`
	PublicKeyHex           string `json:"public_key_hex"`
	CheckinIntervalSeconds int    `json:"checkin_interval_seconds"`
}

func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "/tmp/404-sensor"
	}
	return filepath.Join(home, ".404-sensor")
}

func LoadConfig(dir string) (*Config, error) {
	configPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func SaveConfig(dir string, cfg *Config) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(dir, "config.json")
	return os.WriteFile(configPath, data, 0600)
}

func GenerateKeypair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ed25519 key: %w", err)
	}
	return pub, priv, nil
}

func KeypairToHex(pub ed25519.PublicKey, priv ed25519.PrivateKey) (string, string) {
	return hex.EncodeToString(pub), hex.EncodeToString(priv)
}

func PrivateKeyFromHex(privHex string) (ed25519.PrivateKey, error) {
	b, err := hex.DecodeString(privHex)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid private key length: %d", len(b))
	}
	return ed25519.PrivateKey(b), nil
}
