package http

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	// jwtSecretFileName lives next to config.json so it persists on the same
	// volume and survives restarts and upgrades.
	jwtSecretFileName = "jwt_secret"
	// minJWTSecretLength is the shortest persisted secret that is accepted.
	// Generated secrets are 64 hex characters (32 random bytes).
	minJWTSecretLength = 32
	// legacyJWTSecret was the hard-coded fallback in earlier releases. It is
	// public, so it is never used implicitly.
	legacyJWTSecret = "TOBECHANGED"
)

// loadJWTSecret returns the key used to sign and verify API tokens.
//
// AGENT_JWT_SECRET always wins, so deployments that share a secret with
// another service (such as Kerberos Factory) keep working. Without it, a
// random secret is generated on first start and stored in
// <configDirectory>/data/config/jwt_secret. If that file cannot be written,
// an in-memory random secret is used, which means users must sign in again
// after a restart.
func loadJWTSecret(configDirectory string) []byte {
	if secret := os.Getenv("AGENT_JWT_SECRET"); secret != "" {
		if secret == legacyJWTSecret {
			log.WithFields(log.Fields{
				"component": "http",
				"event":     "jwt_secret_insecure",
			}).Warn("AGENT_JWT_SECRET is set to the publicly known legacy default; set a random value")
		}
		return []byte(secret)
	}

	path := filepath.Join(configDirectory, "data", "config", jwtSecretFileName)
	if data, err := os.ReadFile(path); err == nil {
		secret := strings.TrimSpace(string(data))
		if len(secret) >= minJWTSecretLength {
			return []byte(secret)
		}
		log.WithFields(log.Fields{
			"component": "http",
			"event":     "jwt_secret_invalid",
			"path":      path,
		}).Warn("Stored JWT secret is too short; generating a new one")
	} else if !errors.Is(err, os.ErrNotExist) {
		log.WithError(err).WithFields(log.Fields{
			"component": "http",
			"event":     "jwt_secret_read_failed",
			"path":      path,
		}).Warn("Could not read stored JWT secret; generating a new one")
	}

	secret, err := randomJWTSecret()
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "http",
			"event":     "jwt_secret_generation_failed",
		}).Fatal("Could not generate a JWT signing secret")
	}

	if err := writeJWTSecret(path, secret); err != nil {
		log.WithError(err).WithFields(log.Fields{
			"component": "http",
			"event":     "jwt_secret_persist_failed",
			"path":      path,
		}).Warn("Could not store the generated JWT secret; sessions will not survive a restart")
	} else {
		log.WithFields(log.Fields{
			"component": "http",
			"event":     "jwt_secret_generated",
			"path":      path,
		}).Info("Generated a new JWT signing secret")
	}
	return []byte(secret)
}

func randomJWTSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeJWTSecret(path, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(secret+"\n"), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// agentUsername is the single account allowed to use the API.
func agentUsername() string {
	if username := os.Getenv("AGENT_USERNAME"); username != "" {
		return username
	}
	return "root"
}
