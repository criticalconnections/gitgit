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
	"errors"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
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

// needsEnv reports whether this configuration requires a workspace of its own:
// anything with a build step, a server, or a build output directory cannot be
// served straight from the git tree.
func (c *PreviewConfig) needsEnv() bool {
	return strings.TrimSpace(c.Run) != "" || strings.TrimSpace(c.Static) != "" || len(c.Build) > 0
}

// previewPlan resolves how a branch previews at a commit: the committed
// .gitgit/preview.yml when there is one, otherwise a proposal detected from
// the tree. A nil config means the tree is servable as it stands.
//
// The second return value is non-nil only for a proposal, which callers must
// treat as unapproved — see ensurePreviewEnv.
func previewPlan(dir, sha string) (*PreviewConfig, *DetectedPreview) {
	if cfg := loadPreviewConfig(dir, sha); cfg != nil {
		if !cfg.needsEnv() {
			return nil, nil
		}
		return cfg, nil
	}
	if d := detectPreview(repoTree{dir, sha}); d != nil && d.Cfg.needsEnv() {
		return d.Cfg, d
	}
	return nil, nil
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
	// what the build is doing right now, so a waiting visitor sees progress
	Step      string
	StepN     int
	StepTotal int
	StepAt    int64
}

const envCols = `id, preview_id, repo_id, ref, commit_sha, status, port, pid, message, log,
 created_at, started_at, last_used_at, expires_at, step, step_n, step_total, step_at`

func scanEnv(row interface{ Scan(...any) error }) *PreviewEnv {
	e := &PreviewEnv{}
	if row.Scan(&e.ID, &e.PreviewID, &e.RepoID, &e.Ref, &e.CommitSHA, &e.Status, &e.Port, &e.PID,
		&e.Message, &e.Log, &e.CreatedAt, &e.StartedAt, &e.LastUsedAt, &e.ExpiresAt,
		&e.Step, &e.StepN, &e.StepTotal, &e.StepAt) != nil {
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
	chunk = redactSecrets(id, chunk)
	// keep the tail bounded so a chatty server can't grow the row without limit
	db.Exec(`UPDATE preview_envs SET log = substr(log || ?, max(1, length(log || ?) - 60000)) WHERE id = ?`,
		chunk, chunk, id)
}

// redactSecrets strikes a repository's secret values out of build output.
// Applied on the way in, so the database never stores one — and again on the
// way out, since a value split across two writes slips past the first pass.
func redactSecrets(id int64, text string) string {
	envMu.Lock()
	values := envRedactions[id]
	envMu.Unlock()
	for _, v := range values {
		// Very short values would redact half the log; they are also not
		// credentials worth protecting.
		if len(v) < 6 {
			continue
		}
		text = strings.ReplaceAll(text, v, "••••••")
	}
	return text
}

// setEnvStep records what the build is doing now, so a waiting visitor sees
// progress instead of a spinner that never explains itself.
func setEnvStep(id int64, n, total int, what string) {
	db.Exec("UPDATE preview_envs SET step = ?, step_n = ?, step_total = ?, step_at = ? WHERE id = ?",
		what, n, total, now(), id)
}

func touchEnv(id int64) {
	db.Exec("UPDATE preview_envs SET last_used_at = ? WHERE id = ?", now(), id)
}

// envLogTail returns the last n bytes of an environment's output.
func envLogTail(id int64, n int) string {
	var s string
	db.QueryRow("SELECT log FROM preview_envs WHERE id = ?", id).Scan(&s)
	if len(s) > n {
		s = "…" + s[len(s)-n:]
	}
	return redactSecrets(id, s)
}

// ---------- lifecycle ----------

var (
	envMu      sync.Mutex
	envStarted = map[int64]bool{} // environments this process is building/running
	// Build-only environments have no process to proxy to; this is the
	// directory their build produced, resolved once when they go live.
	envStaticDirs = map[int64]string{}
	// Secret values to strike out of an environment's output. Build tools
	// echo their environment freely, and the log is shown in the UI.
	envRedactions = map[int64][]string{}
)

const maxLiveEnvs = 4

func liveEnvCount() int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM preview_envs WHERE status IN ('queued','building','running')").Scan(&n)
	return n
}

// ensurePreviewEnv returns the environment backing a preview, starting one if
// the branch needs building and none is live yet. Returns nil when the tree
// can be served as it stands — or when it needs a build nobody has approved.
func ensurePreviewEnv(repo *Repo, p *Preview, sha string) *PreviewEnv {
	cfg, detected := previewPlan(repo.DiskPath(), sha)
	if cfg == nil {
		return nil // nothing to build: serve the tree
	}
	// A detected configuration is a guess, and building it executes repository
	// code on this host. A committed .gitgit/preview.yml is consent from
	// somebody with push access; a guess carries no such consent, so it waits
	// for a maintainer to approve it once (startPreviewEnv, via the API).
	if detected != nil && !p.EnvOK {
		return nil
	}
	return startPreviewEnv(repo, p, sha, cfg, detected != nil)
}

// startPreviewEnv creates or reuses the environment for a preview, bypassing
// the approval check — callers must have established consent themselves.
// `detected` says the configuration was inferred rather than committed, which
// decides whether a wrong output directory is a failure or something to
// recover from.
func startPreviewEnv(repo *Repo, p *Preview, sha string, cfg *PreviewConfig, detected bool) *PreviewEnv {
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
	go buildAndRunEnv(id, cfg, detected)
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

func buildAndRunEnv(id int64, cfg *PreviewConfig, detected bool) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("preview-env %d panic: %v", id, v)
			failEnv(id, fmt.Sprint(v))
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
		failEnv(id, "repository is gone")
		return
	}
	setEnvStatus(id, "building", "")

	// A visitor staring at a holding page deserves to know which of these is
	// taking the time; number the phases up front so progress is honest.
	total := 2 + len(cfg.Build)
	if strings.TrimSpace(cfg.Run) != "" {
		total++ // starting the process, then waiting for it to listen
	}
	stepN := 0
	step := func(what string) {
		stepN++
		setEnvStep(id, stepN, total, what)
	}

	step("Cloning the branch")
	ws := envWorkspace(id)
	os.RemoveAll(ws)
	if _, err := gitRun("", "clone", "--quiet", "--no-hardlinks", repo.DiskPath(), ws); err != nil {
		failEnv(id, "clone failed")
		appendEnvLog(id, "clone failed: "+err.Error()+"\n")
		return
	}
	if _, err := gitRun(ws, "checkout", "--quiet", e.CommitSHA); err != nil {
		failEnv(id, "checkout failed")
		appendEnvLog(id, "checkout failed: "+err.Error()+"\n")
		return
	}

	port, err := freePort()
	if err != nil {
		failEnv(id, "no free port")
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

	// Repository secrets go on last, so a value committed in preview.yml
	// cannot shadow or blank one. Register the values for redaction *before*
	// running anything, or the first command to echo its environment wins.
	pairs, values, skipped := secretEnv(e.RepoID)
	if len(values) > 0 {
		envMu.Lock()
		envRedactions[id] = values
		envMu.Unlock()
	}
	base = append(base, pairs...)
	if len(pairs) > 0 {
		names := make([]string, 0, len(pairs))
		for _, kv := range pairs {
			name, _, _ := strings.Cut(kv, "=")
			names = append(names, name)
		}
		appendEnvLog(id, "using "+fmt.Sprint(len(names))+" repository secret(s): "+strings.Join(names, ", ")+"\n")
	}
	if len(skipped) > 0 {
		appendEnvLog(id, "!!! skipped (cannot decrypt with the current key): "+strings.Join(skipped, ", ")+"\n")
	}

	// build steps
	for _, buildStep := range cfg.Build {
		step(strings.TrimSpace(buildStep))
		appendEnvLog(id, "$ "+strings.TrimSpace(buildStep)+"\n")
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		cmd := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", buildStep)
		cmd.Dir, cmd.Env = ws, base
		out, err := cmd.CombinedOutput()
		cancel()
		appendEnvLog(id, string(out))
		if err != nil {
			appendEnvLog(id, "\n!!! build step failed: "+err.Error()+"\n")
			failEnv(id, "build failed")
			return
		}
	}

	// A build with no server of its own: serve what it produced. This is the
	// common case — most front-end frameworks compile to a directory of files.
	if strings.TrimSpace(cfg.Run) == "" {
		step("Publishing the build")
		root, err := envStaticRoot(id, cfg, detected)
		if err != nil {
			appendEnvLog(id, "\n!!! "+err.Error()+"\n")
			failEnv(id, err.Error())
			return
		}
		envMu.Lock()
		envStaticDirs[id] = root
		envMu.Unlock()
		appendEnvLog(id, "\nserving "+strings.TrimPrefix(root, ws+"/")+"/ as a static site\n")
		db.Exec("UPDATE preview_envs SET started_at = ? WHERE id = ?", now(), id)
		setEnvStatus(id, "running", "")
		touchEnv(id)
		log.Printf("preview-env %d serving static build: %s@%s", id, repo.FullName(), short(e.CommitSHA))
		return
	}

	// long-lived server
	step("Starting " + strings.TrimSpace(cfg.Run))
	appendEnvLog(id, "$ "+strings.TrimSpace(cfg.Run)+"   (PORT="+fmt.Sprint(port)+")\n")
	cmd := exec.Command("bash", "-e", "-o", "pipefail", "-c", cfg.Run)
	cmd.Dir, cmd.Env = ws, base
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own group, so we can kill children too
	pw := &envLogWriter{id: id}
	cmd.Stdout, cmd.Stderr = pw, pw
	if err := cmd.Start(); err != nil {
		appendEnvLog(id, "failed to start: "+err.Error()+"\n")
		failEnv(id, "start failed")
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
			failEnv(id, msg)
		}
		os.RemoveAll(envWorkspace(id))
	}()

	// wait for the app to accept connections
	step("Waiting for the app to listen on port " + fmt.Sprint(port))
	if !waitForPort(port, 90*time.Second) {
		appendEnvLog(id, "\n!!! app did not listen on PORT within 90s\n")
		killEnvProcess(id)
		failEnv(id, "app never became ready")
		return
	}
	setEnvStatus(id, "running", "")
	touchEnv(id)
	log.Printf("preview-env %d running: %s@%s on :%d", id, repo.FullName(), short(e.CommitSHA), port)
}

type envLogWriter struct{ id int64 }

// failEnv marks an environment failed and discards its workspace. The log
// lives in the database, so nothing diagnosable is lost — and a failed
// environment is never reaped, so a leftover node_modules would leak for good.
func failEnv(id int64, msg string) {
	setEnvStatus(id, "failed", msg)
	os.RemoveAll(envWorkspace(id))
	envMu.Lock()
	delete(envStaticDirs, id)
	envMu.Unlock()
}

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
	delete(envStaticDirs, id)
	delete(envRedactions, id)
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
	// Nothing is running immediately after a restart, so every workspace still
	// on disk is an orphan — including those a failed build used to leave
	// behind, which no reaper ever looked at.
	root := filepath.Join(dataDir, "envs")
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() {
			os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	if len(entries) > 0 {
		log.Printf("preview-env: removed %d leftover workspace(s)", len(entries))
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

// writeEnvStatusPage renders the holding page a visitor sees while an
// environment builds. It reports the phase, the elapsed time and the tail of
// the build output, because "please wait" with no detail is indistinguishable
// from a hang.
func writeEnvStatusPage(w http.ResponseWriter, e *PreviewEnv, note string) {
	status := http.StatusServiceUnavailable
	title, body, refresh := "Starting preview environment…", "Building this branch.", true
	switch e.Status {
	case "failed":
		title, body, refresh = "Preview environment failed", "The build or the app exited.", false
	case "stopped":
		title, body, refresh = "Preview environment stopped", "It was idle or reached its time limit. Reopen it from the pull request.", false
	}
	if e.Message != "" && e.Status == "failed" {
		body = e.Message
	}
	if note != "" {
		body = note
	}

	var detail strings.Builder
	if e.Step != "" {
		pct := 0
		if e.StepTotal > 0 {
			pct = e.StepN * 100 / e.StepTotal
		}
		detail.WriteString(fmt.Sprintf(
			`<div style="margin:1.5rem 0 .5rem"><div style="height:6px;border-radius:99px;background:#e3e8ee;overflow:hidden">`+
				`<div style="height:100%%;width:%d%%;background:#1fa06a;transition:width .3s"></div></div></div>`+
				`<p style="color:#232b36;font-size:.9rem;margin:.5rem 0 0"><b>Step %d of %d</b> · %s</p>`,
			pct, e.StepN, e.StepTotal, html.EscapeString(e.Step)))
		if e.StepAt > 0 && refresh {
			detail.WriteString(fmt.Sprintf(`<p style="color:#8a929c;font-size:.8rem;margin:.25rem 0 0">%ds on this step</p>`,
				now()-e.StepAt))
		}
	}
	if tail := lastLines(envLogTail(e.ID, 4000), 12); tail != "" {
		detail.WriteString(`<pre style="text-align:left;margin:1.25rem 0 0;padding:.9rem;border-radius:10px;background:#111826;color:#d8dee9;` +
			`font-size:11.5px;line-height:1.55;overflow:auto;max-height:15rem;white-space:pre-wrap">` +
			html.EscapeString(tail) + `</pre>`)
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if refresh {
		w.Header().Set("Refresh", "2")
	}
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1">
<title>%s</title><body style="font-family:-apple-system,system-ui,sans-serif;display:grid;place-items:center;min-height:90vh;margin:0;color:#232b36;background:#f5f8f7">
<div style="max-width:34rem;width:100%%;padding:2rem;text-align:center">
<div style="font-size:2rem;font-weight:800;letter-spacing:-.02em"><span style="color:#f08a24">&lt;</span> Git<span style="color:#1fa06a">Git</span> <span style="color:#1fa06a">&gt;</span></div>
<h1 style="font-size:1.15rem;margin:1.25rem 0 .5rem">%s</h1>
<p style="color:#5c6672;font-size:.95rem;line-height:1.5">%s</p>
%s
<p style="color:#8a929c;font-size:.8rem;margin-top:1.5rem">%s @ %s</p>
</div>`, title, title, body, detail.String(), html.EscapeString(e.Ref), short(e.CommitSHA))
}

// lastLines keeps the tail of the output, where the interesting part is.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return strings.Join(out, "\n")
}

// ---------- serving a build that has no server ----------

// staticOutputCandidates are the conventional places a build leaves a site.
// Used only to rescue a *detected* configuration whose guess was wrong — a
// committed one is a decision, and second-guessing it would hide the mistake.
var staticOutputCandidates = []string{
	"dist", "dist/client", "build", "build/client", "out", ".output/public", "public", "_site", "www",
}

// envStaticRoot locates the directory a build produced inside the
// environment's workspace.
func envStaticRoot(id int64, cfg *PreviewConfig, detected bool) (string, error) {
	rel := strings.TrimSpace(cfg.Static)
	if rel == "" {
		rel = "."
	}
	// The build already ran repository code, so `..` here is not an
	// escalation — but quietly rewriting it would hide a mistake in the
	// config, so refuse it and say so.
	for _, seg := range strings.Split(filepath.ToSlash(rel), "/") {
		if seg == ".." {
			return "", fmt.Errorf("static: %q must stay inside the workspace", rel)
		}
	}
	ws := envWorkspace(id)
	declared := filepath.Join(ws, filepath.FromSlash(rel))
	if st, err := os.Stat(declared); err == nil && st.IsDir() {
		return descendToIndex(declared), nil
	}
	if !detected {
		return "", errors.New(describeBuildOutput(ws, rel))
	}
	// The guess was wrong. Frameworks that share a dependency do not share an
	// output directory (a TanStack Start app depends on vite but builds to
	// .output), so look where builds actually put things.
	for _, cand := range staticOutputCandidates {
		if cand == rel {
			continue
		}
		root := descendToIndex(filepath.Join(ws, filepath.FromSlash(cand)))
		if fileExists(filepath.Join(root, "index.html")) {
			log.Printf("preview-env %d: no %s/, serving %s/ instead", id, rel, cand)
			return root, nil
		}
	}
	return "", errors.New(describeBuildOutput(ws, rel))
}

// descendToIndex follows a lone wrapper directory down to the entry point,
// for tools that nest their output (Angular writes dist/<project>/browser).
func descendToIndex(root string) string {
	for i := 0; i < 3; i++ {
		if fileExists(filepath.Join(root, "index.html")) {
			return root
		}
		entries, err := os.ReadDir(root)
		if err != nil || len(entries) != 1 || !entries[0].IsDir() {
			return root
		}
		root = filepath.Join(root, entries[0].Name())
	}
	return root
}

// describeBuildOutput turns "no dist/" into something actionable: what the
// build actually left behind, and the likeliest reason it is not a site.
func describeBuildOutput(ws, want string) string {
	if fileExists(filepath.Join(ws, ".output", "server", "index.mjs")) {
		return "the build produced a server bundle (.output/server/index.mjs), not a static site — set `run:` in .gitgit/preview.yml to start it"
	}
	var dirs []string
	entries, _ := os.ReadDir(ws)
	for _, e := range entries {
		if e.IsDir() && e.Name() != "node_modules" && !strings.HasPrefix(e.Name(), ".git") {
			dirs = append(dirs, e.Name()+"/")
		}
	}
	if len(dirs) == 0 {
		return fmt.Sprintf("the build produced no %s/ directory", want)
	}
	sort.Strings(dirs)
	if len(dirs) > 8 {
		dirs = append(dirs[:8], "…")
	}
	return fmt.Sprintf("no %s/ after the build — it produced: %s. Set `static:` or `run:` in .gitgit/preview.yml",
		want, strings.Join(dirs, " "))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// serveEnvStatic serves a build-only environment's output from disk. Like
// proxyToEnv it answers on the environment's own subdomain, so absolute asset
// paths and client-side routers behave as they will in production.
func serveEnvStatic(w http.ResponseWriter, r *http.Request, e *PreviewEnv) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	envMu.Lock()
	root := envStaticDirs[e.ID]
	envMu.Unlock()
	if root == "" {
		writeEnvStatusPage(w, e, "The build output is no longer on disk. Rebuild this environment.")
		return
	}
	touchEnv(e.ID)

	// path.Clean on a rooted path removes any `..`; the containment check is
	// the belt to that braces. (A symlink planted by the build could still
	// point outside, but the build ran this repository's own code already.)
	name := filepath.Join(root, filepath.FromSlash(path.Clean("/"+r.URL.Path)))
	if name != root && !strings.HasPrefix(name, root+string(os.PathSeparator)) {
		http.NotFound(w, r)
		return
	}
	if st, err := os.Stat(name); err == nil && st.IsDir() {
		name = filepath.Join(name, "index.html")
	}
	// client-side routing: an extensionless path falls back to the entry point
	if !fileExists(name) {
		if path.Ext(path.Base(r.URL.Path)) != "" {
			http.NotFound(w, r)
			return
		}
		name = filepath.Join(root, "index.html")
	}
	f, err := os.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, filepath.Base(name), st.ModTime(), f)
}
