// gitgit — a self-hostable software forge: git hosting, pull requests,
// stacked PRs, code review, issues, and built-in CI in a single binary.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	addr            string
	dataDir         string
	baseURL         string // optional external URL override, e.g. https://git.example.com
	devMode         bool
	ciSlots         int
	openReg         bool
	previewDomain   string // wildcard base for Preview Environments, e.g. "preview.example.com"
	previewInsecure bool   // build preview URLs with http:// (local development)
	sshAddr         string // listen address for git over SSH ("" disables it)
	sshHostName     string // hostname shown in SSH clone URLs
	backupEvery     time.Duration
	backupKeep      int
	startTme        = time.Now()
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	flag.StringVar(&addr, "addr", envOr("GITGIT_ADDR", ":3000"), "listen address")
	flag.StringVar(&dataDir, "data", envOr("GITGIT_DATA", "./data"), "data directory (repos, database, CI workspaces)")
	flag.StringVar(&baseURL, "base-url", envOr("GITGIT_BASE_URL", ""), "external base URL used in clone instructions (default: derived from request)")
	flag.IntVar(&ciSlots, "ci-workers", 2, "number of concurrent CI job runners")
	flag.BoolVar(&openReg, "open-registration", envOr("GITGIT_OPEN_REGISTRATION", "true") != "false",
		"allow anyone to register; set false for internet-facing deployments (the first account can still be created to bootstrap the admin)")
	flag.StringVar(&previewDomain, "preview-domain", envOr("GITGIT_PREVIEW_DOMAIN", ""),
		"wildcard base domain for Preview Environments, e.g. preview.example.com (requires *.<domain> DNS and TLS)")
	flag.BoolVar(&previewInsecure, "preview-insecure", envOr("GITGIT_PREVIEW_INSECURE", "") == "true",
		"build preview URLs with http:// instead of https:// (local development)")
	flag.StringVar(&sshAddr, "ssh-addr", envOr("GITGIT_SSH_ADDR", ":2222"),
		"listen address for git over SSH; empty to disable")
	flag.StringVar(&sshHostName, "ssh-host", envOr("GITGIT_SSH_HOST", ""),
		"hostname shown in SSH clone URLs (default: the request's host)")
	flag.DurationVar(&backupEvery, "backup-every", 0,
		"take a backup on this interval, e.g. 24h (0 disables scheduled backups)")
	flag.IntVar(&backupKeep, "backup-keep", backupKeepDefault, "number of scheduled backups to retain")
	flag.BoolVar(&devMode, "dev", false, "dev mode: serve templates/static from disk instead of the embedded copies")
	flag.Parse()

	var err error
	dataDir, err = filepath.Abs(dataDir)
	if err != nil {
		log.Fatalf("resolve data dir: %v", err)
	}
	for _, d := range []string{dataDir, filepath.Join(dataDir, "repos"), filepath.Join(dataDir, "ci")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			log.Fatalf("create %s: %v", d, err)
		}
	}

	if err := openDB(filepath.Join(dataDir, "gitgit.db")); err != nil {
		log.Fatalf("open database: %v", err)
	}
	startCIWorkers(ciSlots)
	maybeStartSSH(sshAddr)
	scheduleBackups(backupEvery, backupKeep)
	resetPreviewEnvsOnBoot()
	go reapPreviewEnvs()
	requeueInterruptedRuns()

	srv := &http.Server{
		Addr:              addr,
		Handler:           buildHandler(),
		ReadHeaderTimeout: 30 * time.Second,
	}
	log.Printf("gitgit serving on %s (data: %s)", addr, dataDir)
	log.Fatal(srv.ListenAndServe())
}
