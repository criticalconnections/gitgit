package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The signature must match what an independent SigV4 implementation produces;
// a wrong one fails at the far end with an opaque 403.
func TestSigV4Signature(t *testing.T) {
	c := R2Config{
		AccountID: "abc123", Bucket: "gitgit-backups",
		AccessKey: "AKIDEXAMPLE", SecretKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
	}
	req, _ := http.NewRequest(http.MethodPut,
		"https://abc123.r2.cloudflarestorage.com/gitgit-backups/gitgit-20260818-120000.tar.gz", nil)
	c.sign(req, "UNSIGNED-PAYLOAD", time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC))

	const want = "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20260818/auto/s3/aws4_request, " +
		"SignedHeaders=host;x-amz-content-sha256;x-amz-date, " +
		"Signature=f0c8489df13ecf1b727d9f08697e070c99381f391e47bf7c3010cfcf45c7b4de"
	if got := req.Header.Get("Authorization"); got != want {
		t.Errorf("Authorization =\n  %s\nwant\n  %s", got, want)
	}
}

// A full upload / list / delete round trip against an S3-compatible stand-in.
func TestR2RoundTrip(t *testing.T) {
	stored := map[string][]byte{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("request was not signed")
			w.WriteHeader(403)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, "/gitgit-backups/")
		switch {
		case r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			stored[key] = body
			w.WriteHeader(200)
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			var b strings.Builder
			b.WriteString(`<?xml version="1.0"?><ListBucketResult>`)
			for k, v := range stored {
				fmt.Fprintf(&b, `<Contents><Key>%s</Key><Size>%d</Size></Contents>`, k, len(v))
			}
			b.WriteString(`</ListBucketResult>`)
			w.Write([]byte(b.String()))
		case r.Method == http.MethodDelete:
			delete(stored, key)
			w.WriteHeader(204)
		default:
			w.WriteHeader(400)
		}
	}))
	defer srv.Close()

	c := R2Config{Bucket: "gitgit-backups", AccessKey: "k", SecretKey: "s", Endpoint: srv.URL}

	dir := t.TempDir()
	archive := filepath.Join(dir, "gitgit-20260818-120000.tar.gz")
	if err := os.WriteFile(archive, []byte("pretend archive bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := c.uploadBackup(archive, "gitgit-20260818-120000.tar.gz"); err != nil {
		t.Fatalf("upload: %v", err)
	}
	if string(stored["gitgit-20260818-120000.tar.gz"]) != "pretend archive bytes" {
		t.Fatalf("the bytes that arrived are not the bytes that were sent: %q", stored)
	}

	objects, err := c.listBackups()
	if err != nil || len(objects) != 1 {
		t.Fatalf("list = %v, %v", objects, err)
	}

	// Retention must remove the oldest and keep the newest.
	for _, name := range []string{"gitgit-20260101-000000.tar.gz", "gitgit-20260701-000000.tar.gz"} {
		p := filepath.Join(dir, name)
		os.WriteFile(p, []byte("x"), 0o600)
		if err := c.uploadBackup(p, name); err != nil {
			t.Fatal(err)
		}
	}
	c.pruneBackups(1)
	if len(stored) != 1 {
		t.Fatalf("pruning to 1 left %d objects", len(stored))
	}
	if _, ok := stored["gitgit-20260818-120000.tar.gz"]; !ok {
		t.Errorf("pruning kept the wrong archive: %v", keysOf(stored))
	}
}

func keysOf(m map[string][]byte) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Offsite must stay off unless it is fully configured: a half-set environment
// silently skipping the upload is how people end up with no backups.
func TestOffsiteRequiresFullConfig(t *testing.T) {
	for _, missing := range []string{"GITGIT_R2_BUCKET", "GITGIT_R2_ACCESS_KEY_ID", "GITGIT_R2_SECRET_ACCESS_KEY"} {
		t.Setenv("GITGIT_R2_ACCOUNT_ID", "acct")
		t.Setenv("GITGIT_R2_BUCKET", "b")
		t.Setenv("GITGIT_R2_ACCESS_KEY_ID", "k")
		t.Setenv("GITGIT_R2_SECRET_ACCESS_KEY", "s")
		t.Setenv(missing, "")
		if _, ok := r2FromEnv(); ok {
			t.Errorf("reported configured with %s unset", missing)
		}
	}
	t.Setenv("GITGIT_R2_ACCOUNT_ID", "acct")
	t.Setenv("GITGIT_R2_BUCKET", "b")
	t.Setenv("GITGIT_R2_ACCESS_KEY_ID", "k")
	t.Setenv("GITGIT_R2_SECRET_ACCESS_KEY", "s")
	if _, ok := r2FromEnv(); !ok {
		t.Error("a complete configuration was rejected")
	}
}

var _ = hex.EncodeToString
var _ = sha256.Sum256
