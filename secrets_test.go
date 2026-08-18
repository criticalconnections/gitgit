package main

import (
	"os"
	"strings"
	"testing"
)

func TestSealOpenRoundTrip(t *testing.T) {
	t.Setenv("GITGIT_SECRET_KEY", strings.Repeat("ab", 32)) // 32 bytes of hex
	resetSecretKey()

	sealed, err := sealSecret(7, "DATABASE_URL", "postgres://u:p@host/db")
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(sealed, "postgres") {
		t.Fatal("the plaintext survived into the stored value")
	}
	got, err := openSecret(7, "DATABASE_URL", sealed)
	if err != nil || got != "postgres://u:p@host/db" {
		t.Fatalf("open = %q, %v", got, err)
	}

	// The ciphertext is bound to its repository and name, so a row copied to
	// another repo — or renamed over a different variable — will not open.
	if _, err := openSecret(8, "DATABASE_URL", sealed); err == nil {
		t.Error("a secret opened under the wrong repository")
	}
	if _, err := openSecret(7, "OTHER_NAME", sealed); err == nil {
		t.Error("a secret opened under the wrong name")
	}
}

func TestSealIsNotDeterministic(t *testing.T) {
	t.Setenv("GITGIT_SECRET_KEY", strings.Repeat("cd", 32))
	resetSecretKey()
	a, _ := sealSecret(1, "K", "same value")
	b, _ := sealSecret(1, "K", "same value")
	if a == b {
		t.Fatal("identical plaintexts sealed identically — the nonce is not random")
	}
}

func TestKeyMustBe32Bytes(t *testing.T) {
	t.Setenv("GITGIT_SECRET_KEY", "tooshort")
	resetSecretKey()
	if _, err := repoSecretKey(); err == nil {
		t.Fatal("accepted a short key")
	}
}

func TestGeneratedKeyIsNotWorldReadable(t *testing.T) {
	os.Unsetenv("GITGIT_SECRET_KEY")
	dir := t.TempDir()
	old := dataDir
	dataDir = dir
	defer func() { dataDir = old }()
	resetSecretKey()

	if _, err := repoSecretKey(); err != nil {
		t.Fatalf("generating a key: %v", err)
	}
	st, err := os.Stat(dir + "/secret.key")
	if err != nil {
		t.Fatalf("no key file written: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode is %v, want 0600", st.Mode().Perm())
	}
}

func TestParseDotenv(t *testing.T) {
	env, bad := parseDotenv(`
# a comment
DATABASE_URL=postgres://user:pw@host:5432/db
export SUPABASE_ANON_KEY=eyJhbGciOi
QUOTED="hello world"
SINGLE='keeps # hash'
ESCAPED="line\none"
TRAILING=value # inline comment
EMPTY=
9INVALID=x
not a pair
`)
	want := map[string]string{
		"DATABASE_URL":      "postgres://user:pw@host:5432/db",
		"SUPABASE_ANON_KEY": "eyJhbGciOi",
		"QUOTED":            "hello world",
		"SINGLE":            "keeps # hash",
		"ESCAPED":           "line\none",
		"TRAILING":          "value",
		"EMPTY":             "",
	}
	for k, v := range want {
		if env[k] != v {
			t.Errorf("%s = %q, want %q", k, env[k], v)
		}
	}
	if len(env) != len(want) {
		t.Errorf("parsed %d entries, want %d: %v", len(env), len(want), env)
	}
	// A name a shell cannot export, and a line that is not a pair at all,
	// are reported rather than silently dropped.
	if len(bad) != 2 {
		t.Errorf("expected 2 rejected lines, got %v", bad)
	}
}

func TestValidSecretName(t *testing.T) {
	for _, ok := range []string{"A", "DATABASE_URL", "_private", "KEY2"} {
		if !validSecretName(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "2LEADING", "has-dash", "has space", "has$dollar", "PATH;rm -rf /"} {
		if validSecretName(bad) {
			t.Errorf("%q should be rejected", bad)
		}
	}
}
