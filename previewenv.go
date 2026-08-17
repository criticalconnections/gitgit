package main

// Preview Environments: ephemeral, per-branch instances of the application
// itself — not just its files. A repository declares how to build and run
// itself in .gitgit/preview.yml; GitGit clones the branch, builds it, starts
// the process on a private loopback port, and reverse-proxies a dedicated
// subdomain to it.
//
//   https://{id}.preview.example.com  ->  127.0.0.1:{ephemeral port}
//
// Each environment gets its own origin, so cookies, localStorage, and CORS
// behave exactly as they would in production, and absolute asset paths and
// client-side routers work without rewriting.
//
// Environments are reaped on a TTL, after an idle period, and when their pull
// request merges or closes.

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

// PreviewConfig is .gitgit/preview.yml.
//
//	build:                 # optional; run once before `run`
//	  - npm ci
//	  - npm run build
//	run: npm start         # long-lived server; omit for a static preview
//	static: dist           # directory served when `run` is absent
//	health_path: /         # polled until the app answers
//	ttl_minutes: 120
//	idle_minutes: 30
//	env:
//	  NODE_ENV: production
type PreviewConfig struct {
	Build       []string          `yaml:"build"`
	Run         string            `yaml:"run"`
	Static      string            `yaml:"static"`
	HealthPath  string            `yaml:"health_path"`
	TTLMinutes  int               `yaml:"ttl_minutes"`
	IdleMinutes int               `yaml:"idle_minutes"`
	Env         map[string]string `yaml:"env"`
}

var previewConfigPaths = []string{".gitgit/preview.yml", ".gitgit/preview.yaml"}

// loadPreviewConfig reads the environment definition at a commit, or nil when
// the repository does not define one (in which case previews stay static).
func loadPreviewConfig(dir, sha string) *PreviewConfig {
	for _, p := range previewConfigPaths {
		raw := fileAtCommit(dir, sha, p)
		if raw == nil {
			continue
		}
		cfg := &PreviewConfig{}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			log.Printf("preview-env: bad config %s at %s: %v", p, short(sha), err)
			return nil
		}
		return cfg
	}
	return nil
}

func (c *PreviewConfig) ttl() time.Duration {
	if c.TTLMinutes > 0 {
		return time.Duration(c.TTLMinutes) * time.Minute
	}
	return 2 * time.Hour
}

func (c *PreviewConfig) idle() time.Duration {
	if c.IdleMinutes > 0 {
		return time.Duration(c.IdleMinutes) * time.Minute
	}
	return 30 * time.Minute
}

// ---------- state ----------

type PreviewEnv struct {
	ID         int64
	PreviewID  int64
	RepoID     int64
	Ref        string
	CommitSHA  string
	Status     string // queued | building | running | failed | stopped
	Port       int
	PID        int
	Message    string
	Log        string
	CreatedAt  int64
	StartedAt  int64
	LastUsedAt int64
	ExpiresAt  int64
}

const envCols = `id, preview_id, repo_id, ref, commit_sha, status, port, pid, message, log,
 created_at, started_at, last_used_at, expires_at`

func scanEnv(row interface{ Scan(...any) error }) *PreviewEnv {
	e := &PreviewEnv{}
	if row.Scan(&e.ID, &e.PreviewID, &e.RepoID, &e.Ref, &e.CommitSHA, &e.Status, &e.Port, &e.PID,
		&e.Message, &e.Log, &e.CreatedAt, &e.StartedAt, &e.LastUsedAt, &e.ExpiresAt) != nil {
		return nil
	}
	return e
}

func envByPreview(previewID int64) *PreviewEnv {
	return scanEnv(db.QueryRow("SELECT "+envCols+" FROM preview_envs WHERE preview_id = ?", previewID))
}

func envByID(id int64) *PreviewEnv {
	return scanEnv(db.QueryRow("SELECT "+envCols+" FROM preview_envs WHERE id = ?", id))
}

func listEnvs(repoID int64) []*PreviewEnv {
	rows, err := db.Query("SELECT "+envCols+" FROM preview_envs WHERE repo_id = ? ORDER BY id DESC", repoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*PreviewEnv
	for rows.Next() {
		if e := scanEnv(rows); e != nil {
			out = append(out, e)
		}
	}
	return out
}

func setEnvStatus(id int64, status, message string) {
	db.Exec("UPDATE preview_envs SET status = ?, message = ? WHERE id = ?", status, message, id)
}

func appendEnvLog(id int64, chunk string) {
	// keep the tail bounded so a chatty server can't grow the row without limit
	db.Exec(`UPDATE preview_envs SET log = substr(log || ?, max(1, length(log || ?) - 60000)) WHERE id = ?`,
		chunk, chunk, id)
}

func touchEnv(id int64) {
	db.Exec("UPDATE preview_envs SET last_used_at = ? WHERE id = ?", now(), id)
}

// envLogTail returns the last n bytes of an environment's output.
func envLogTail(id int64, n int) string {
	var s string
	db.QueryRow("SELECT log FROM preview_envs WHERE id = ?", id).Scan(&s)
	if len(s) > n {
		return "…" + s[len(s)-n:]
	}
	return s
}

// ---------- lifecycle ----------

var (
	envMu      sync.Mutex
	envStarted = map[int64]bool{} // environments this process is building/running
)

const maxLiveEnvs = 4

func liveEnvCount() int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM preview_envs WHERE status IN ('queued','building','running')").Scan(&n)
	return n
}

// ensurePreviewEnv returns the environment backing a preview, starting one if
// the repository defines a `run` command and none is live yet. Returns nil
// when the preview should be served statically.
func ensurePreviewEnv(repo *Repo, p *Preview, sha string) *PreviewEnv {
	cfg := loadPreviewConfig(repo.DiskPath(), sha)
	if cfg == nil || strings.TrimSpace(cfg.Run) == "" {
		return nil // static preview
	}

	envMu.Lock()
	defer envMu.Unlock()

	e := envByPreview(p.ID)
	// a live environment for a stale commit is replaced
	if e != nil && e.CommitSHA != sha && (e.Status == "running" || e.Status == "building") {
		go stopPreviewEnv(e.ID, "superseded by a newer commit")
		e = nil
	}
	if e != nil {
		switch e.Status {
		case "running", "building", "queued":
			touchEnv(e.ID)
			return e
		case "failed":
			if e.CommitSHA == sha {
				return e // don't rebuild a known-bad commit on every request
			}
		}
		db.Exec("DELETE FROM preview_envs WHERE id = ?", e.ID)
	}

	if liveEnvCount() >= maxLiveEnvs {
		log.Printf("preview-env: at capacity (%d), not starting one for %s@%s", maxLiveEnvs, repo.FullName(), p.Ref)
		return nil
	}

	res, err := db.Exec(`INSERT INTO preview_envs
		(preview_id, repo_id, ref, commit_sha, status, created_at, last_used_at, expires_at)
		VALUES (?,?,?,?, 'queued', ?, ?, ?)`,
		p.ID, repo.ID, p.Ref, sha, now(), now(), now()+int64(cfg.ttl().Seconds()))
	if err != nil {
		log.Printf("preview-env: create: %v", err)
		return nil
	}
	id, _ := res.LastInsertId()
	go buildAndRunEnv(id, cfg)
	return envByID(id)
}

// freePort asks the kernel for an unused loopback port.
func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func envWorkspace(id int64) string {
	return filepath.Join(dataDir, "envs", fmt.Sprintf("env-%d", id))
}

func buildAndRunEnv(id int64, cfg *PreviewConfig) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("preview-env %d panic: %v", id, v)
			setEnvStatus(id, "failed", fmt.Sprint(v))
		}
	}()

	envMu.Lock()
	envStarted[id] = true
	envMu.Unlock()

	e := envByID(id)
	if e == nil {
		return
	}
	repo, err := getRepoByID(e.RepoID)
	if err != nil {
		setEnvStatus(id, "failed", "repository is gone")
		return
	}
	setEnvStatus(id, "building", "")

	ws := envWorkspace(id)
	os.RemoveAll(ws)
	if _, err := gitRun("", "clone", "--quiet", "--no-hardlinks", repo.DiskPath(), ws); err != nil {
		setEnvStatus(id, "failed", "clone failed")
		appendEnvLog(id, "clone failed: "+err.Error()+"\n")
		return
	}
	if _, err := gitRun(ws, "checkout", "--quiet", e.CommitSHA); err != nil {
		setEnvStatus(id, "failed", "checkout failed")
		appendEnvLog(id, "checkout failed: "+err.Error()+"\n")
		return
	}

	port, err := freePort()
	if err != nil {
		setEnvStatus(id, "failed", "no free port")
		return
	}

	base := append(os.Environ(),
		"CI=true", "GITGIT=true", "PORT="+fmt.Sprint(port),
		"GITGIT_REPO="+repo.FullName(), "GITGIT_REF="+e.Ref, "GITGIT_SHA="+e.CommitSHA,
		"GITGIT_PREVIEW_URL="+previewOrigin(previewByID(e.PreviewID)),
	)
	for k, v := range cfg.Env {
		base = append(base, k+"="+v)
	}

	// build steps
	for _, step := range cfg.Build {
		appendEnvLog(id, "$ "+strings.TrimSpace(step)+"\n")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		cmd := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", step)
		cmd.Dir, cmd.Env = ws, base
		out, err := cmd.CombinedOutput()
		cancel()
		appendEnvLog(id, string(out))
		if err != nil {
			appendEnvLog(id, "\n!!! build step failed: "+err.Error()+"\n")
			setEnvStatus(id, "failed", "build failed")
			return
		}
	}

	// long-lived server
	appendEnvLog(id, "$ "+strings.TrimSpace(cfg.Run)+"   (PORT="+fmt.Sprint(port)+")\n")
	cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", cfg.Run)
	cmd.Dir, cmd.Env = ws, base
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own group, so we can kill children too
	pw := &envLogWriter{id: id}
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		appendEnvLog(id, "failed to start: "+err.Error()+"\n")
		setEnvStatus(id, "failed", "start failed")
		return
	}
	db.Exec("UPDATE preview_envs SET port = ?, pid = ?, started_at = ? WHERE id = ?", port, cmd.Process.Pid, now(), id)

	// reap the process if it exits on its own
	go func() {
		err := cmd.Wait()
		cur := envByID(id)
		if cur != nil && cur.Status != "stopped" {
			msg := "process exited"
			if err != nil {
				msg = "process exited: " + err.Error()
			}
			appendEnvLog(id, "\n!!! "+msg+"\n")
			setEnvStatus(id, "failed", msg)
		}
		os.RemoveAll(envWorkspace(id))
	}()

	// wait for the app to accept connections
	if !waitForPort(port, 90*time.Second) {
		appendEnvLog(id, "\n!!! app did not listen on PORT within 90s\n")
		setEnvStatus(id, "failed", "app never became ready")
		killEnvProcess(id)
		return
	}
	setEnvStatus(id, "running", "")
	touchEnv(id)
	log.Printf("preview-env %d running: %s@%s on :%d", id, repo.FullName(), short(e.CommitSHA), port)
}

type envLogWriter struct{ id int64 }

func (w *envLogWriter) Write(p []byte) (int, error) {
	appendEnvLog(w.id, string(p))
	return len(p), nil
}

func waitForPort(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
		if err == nil {
			c.Close()
			return true
		}
		time.Sleep(400 * time.Millisecond)
	}
	return false
}

func killEnvProcess(id int64) {
	e := envByID(id)
	if e == nil || e.PID == 0 {
		return
	}
	// negative pid == the whole process group started with Setpgid
	syscall.Kill(-e.PID, syscall.SIGTERM)
	time.AfterFunc(5*time.Second, func() { syscall.Kill(-e.PID, syscall.SIGKILL) })
}

func stopPreviewEnv(id int64, reason string) {
	killEnvProcess(id)
	setEnvStatus(id, "stopped", reason)
	os.RemoveAll(envWorkspace(id))
	envMu.Lock()
	delete(envStarted, id)
	envMu.Unlock()
	log.Printf("preview-env %d stopped: %s", id, reason)
}

// stopEnvsForRef tears down environments for a branch (used when a PR merges
// or closes, and when a branch is deleted).
func stopEnvsForRef(repoID int64, ref string) {
	rows, err := db.Query("SELECT id FROM preview_envs WHERE repo_id = ? AND ref = ? AND status IN ('queued','building','running')", repoID, ref)
	if err != nil {
		return
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		stopPreviewEnv(id, "branch no longer has an open pull request")
	}
}

// reapPreviewEnvs enforces TTL and idle timeouts.
func reapPreviewEnvs() {
	for {
		time.Sleep(30 * time.Second)
		rows, err := db.Query(`SELECT id, last_used_at, expires_at, repo_id, commit_sha FROM preview_envs
			WHERE status IN ('queued','building','running')`)
		if err != nil {
			continue
		}
		type cand struct {
			id, lastUsed, expires, repoID int64
			sha                           string
		}
		var cands []cand
		for rows.Next() {
			var c cand
			if rows.Scan(&c.id, &c.lastUsed, &c.expires, &c.repoID, &c.sha) == nil {
				cands = append(cands, c)
			}
		}
		rows.Close()

		for _, c := range cands {
			if now() > c.expires {
				stopPreviewEnv(c.id, "time to live reached")
				continue
			}
			idle := 30 * time.Minute
			if repo, err := getRepoByID(c.repoID); err == nil {
				if cfg := loadPreviewConfig(repo.DiskPath(), c.sha); cfg != nil {
					idle = cfg.idle()
				}
			}
			if c.lastUsed > 0 && now()-c.lastUsed > int64(idle.Seconds()) {
				stopPreviewEnv(c.id, "idle")
			}
		}
	}
}

// resetPreviewEnvsOnBoot clears environments left over from a previous run of
// the server: their processes died with it (or are orphans we should kill).
func resetPreviewEnvsOnBoot() {
	rows, err := db.Query("SELECT id, pid FROM preview_envs WHERE status IN ('queued','building','running')")
	if err != nil {
		return
	}
	type row struct{ id, pid int64 }
	var rs []row
	for rows.Next() {
		var r row
		if rows.Scan(&r.id, &r.pid) == nil {
			rs = append(rs, r)
		}
	}
	rows.Close()
	for _, r := range rs {
		if r.pid > 0 {
			syscall.Kill(-int(r.pid), syscall.SIGKILL) // orphan from a previous process
		}
		setEnvStatus(r.id, "stopped", "server restarted")
		os.RemoveAll(envWorkspace(r.id))
	}
	if len(rs) > 0 {
		log.Printf("preview-env: cleared %d environment(s) from a previous run", len(rs))
	}
}

// ---------- proxying ----------

// proxyToEnv forwards a request to the environment's process. The request
// arrives on the environment's own subdomain, so the path is passed through
// untouched and the app can use absolute URLs normally.
func proxyToEnv(w http.ResponseWriter, r *http.Request, e *PreviewEnv) {
	target, err := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", e.Port))
	if err != nil {
		http.Error(w, "bad environment", http.StatusBadGateway)
		return
	}
	touchEnv(e.ID)

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		log.Printf("preview-env %d proxy error: %v", e.ID, err)
		writeEnvStatusPage(w, e, "The preview environment stopped responding.")
	}
	proxy.ModifyResponse = func(resp *http.Response) error {
		// The environment is on its own origin, so the app's own cookies and
		// storage are already isolated from GitGit. Still refuse framing and
		// MIME sniffing.
		resp.Header.Set("X-Content-Type-Options", "nosniff")
		resp.Header.Del("Strict-Transport-Security")
		return nil
	}
	// strip hop-by-hop identity of the forge; the app sees a normal request
	r.Header.Del("Cookie") // never forward GitGit session cookies into a preview
	proxy.ServeHTTP(w, r)
}

// writeEnvStatusPage renders a small holding page while an environment builds
// or after it fails, so a visitor sees progress instead of a blank error.
func writeEnvStatusPage(w http.ResponseWriter, e *PreviewEnv, note string) {
	status := http.StatusServiceUnavailable
	title, body, refresh := "Starting preview environment…", "Building this branch. This page refreshes automatically.", true
	switch e.Status {
	case "failed":
		title, body, refresh = "Preview environment failed", "The build or the app exited. Check the environment log in GitGit.", false
	case "stopped":
		title, body, refresh = "Preview environment stopped", "It was idle or reached its time limit. Reopen it from the pull request.", false
	}
	if note != "" {
		body = note
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if refresh {
		w.Header().Set("Refresh", "5")
	}
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1">
<title>%s</title><body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0;color:#232b36;background:#f5f8f7">
<div style="max-width:32rem;padding:2rem;text-align:center">
<div style="font-size:2rem;font-weight:800;letter-spacing:-.02em"><span style="color:#f08a24">&lt;</span> Git<span style="color:#1fa06a">Git</span> <span style="color:#1fa06a">&gt;</span></div>
<h1 style="font-size:1.15rem;margin:1.25rem 0 .5rem">%s</h1>
<p style="color:#5c6672;font-size:.95rem;line-height:1.5">%s</p>
<p style="color:#8a929c;font-size:.8rem;margin-top:1.5rem">%s @ %s</p>
</div>`, title, title, body, e.Ref, short(e.CommitSHA))
}
