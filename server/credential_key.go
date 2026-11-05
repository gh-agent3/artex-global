package server

// At-rest credential encryption (opt-in).
//
// The master key lives in keyDir (project base dir, next to jwt.key) — NOT in
// dataDir, which is the browsable workspace root, and not next to the database
// file it protects: copying one file (the DB dump) must never be enough to
// recover plaintext credentials. The db package owns the cipher but knows
// nothing about where the key file lives, so the composition root installs it
// here. Encryption is OFF unless ARTEX_CRED_ENCRYPT=1: existing deployments
// keep plaintext until the operator opts in, and values upgrade lazily on their
// next save (enc:v1: prefix, see db/credential_crypto.go).

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"artex-global/db"
)

const credentialKeyFilename = "credential.key"

// InstallCredentialKey loads (or creates on first run) the credential master
// key and hands it to the db layer. Must run BEFORE the first profile read or
// settings write.
func InstallCredentialKey(keyDir string) error {
	key, err := loadOrCreateCredentialKey(keyDir)
	if err != nil {
		return err
	}
	db.SetCredentialKey(key)
	return nil
}

func loadOrCreateCredentialKey(keyDir string) ([]byte, error) {
	path := filepath.Join(keyDir, credentialKeyFilename)
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) >= 32 {
		return []byte(strings.TrimSpace(string(data))), nil
	}
	key, err := randomToken(32)
	if err != nil {
		return nil, fmt.Errorf("generate credential key: %w", err)
	}
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return nil, fmt.Errorf("write credential key: %w", err)
	}
	log.Printf(logA("[auth] credential master key written to %s (at-rest credential encryption ON)"), path)
	return []byte(key), nil
}

// credEncryptionEnabled reports whether the operator opted in via ARTEX_CRED_ENCRYPT=1.
func credEncryptionEnabled() bool {
	return strings.TrimSpace(os.Getenv("ARTEX_CRED_ENCRYPT")) == "1"
}
