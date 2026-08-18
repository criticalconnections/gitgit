package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// CI configuration lives in the repository at .gitgit/ci.yml (or .ci.yml):
//
//	jobs:
//	  test:
//	    steps:
//	      - name: unit tests
//	        run: go test ./...
//	    env:
//	      CGO_ENABLED: "0"
//	    timeout_minutes: 15
type CIConfig struct {
	Jobs map[string]*CIJobConfig `yaml:"jobs"`
}

type CIJobConfig struct {
	Steps          []CIStep          `yaml:"steps"`
	Env            map[string]string `yaml:"env"`
	TimeoutMinutes int               `yaml:"timeout_minutes"`
}

type CIStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

var ciConfigPaths = []string{".gitgit/ci.yml", ".gitgit/ci.yaml", ".ci.yml"}

// loadCIConfig reads the CI config at a commit; nil if the repo has none.
func loadCIConfig(dir, sha string) *CIConfig {
	for _, p := range ciConfigPaths {
		raw := fileAtCommit(dir, sha, p)
		if raw == nil {
			continue
		}
		cfg := &CIConfig{}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			log.Printf("ci: bad config %s at %s: %v", p, short(sha), err)
			return nil
		}
		if len(cfg.Jobs) == 0 {
			return nil
		}
		return cfg
	}
	return nil
}

func (c *CIConfig) jobNames() []string {
	names := make([]string, 0, len(c.Jobs))
	for n := range c.Jobs {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

var ciQueue = make(chan int64, 256)

// enqueueCI creates a run for a commit if the repo defines CI, and queues it.
func enqueueCI(repo *Repo, sha, ref, event string) *CIRun {
	cfg := loadCIConfig(repo.DiskPath(), sha)
	if cfg == nil {
		return nil
	}
	run, err := createCIRun(repo.ID, sha, ref, event, cfg.jobNames())
	if err != nil {
		log.Printf("ci: create run: %v", err)
		return nil
	}
	select {
	case ciQueue <- run.ID:
	default:
		setRunStatus(run.ID, "error")
		log.Printf("ci: queue full, dropping run %d", run.ID)
	}
	return run
}

func startCIWorkers(n int) {
	for i := 0; i < n; i++ {
		go func() {
			for id := range ciQueue {
				executeRun(id)
			}
		}()
	}
}

// requeueInterruptedRuns puts queued/running runs back on the queue after a restart.
func requeueInterruptedRuns() {
	rows, err := db.Query("SELECT id FROM ci_runs WHERE status IN ('queued','running') ORDER BY id")
	if err != nil {
		return
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		db.Exec("UPDATE ci_runs SET status='queued' WHERE id=?", id)
		select {
		case ciQueue <- id:
		default:
		}
	}
	if len(ids) > 0 {
		log.Printf("ci: requeued %d interrupted run(s)", len(ids))
	}
}

// dbLogWriter streams process output into the job's log column so the web UI
// can show logs while the job runs.
type dbLogWriter struct {
	jobID int64
	mu    sync.Mutex
	buf   strings.Builder
	done  chan struct{}
}

func newDBLogWriter(jobID int64) *dbLogWriter {
	w := &dbLogWriter{jobID: jobID, done: make(chan struct{})}
	go func() {
		t := time.NewTicker(500 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				w.flush()
			case <-w.done:
				w.flush()
				return
			}
		}
	}()
	return w
}

func (w *dbLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	w.buf.Write(p)
	w.mu.Unlock()
	return len(p), nil
}

func (w *dbLogWriter) WriteString(s string) { w.Write([]byte(s)) }

func (w *dbLogWriter) flush() {
	w.mu.Lock()
	chunk := w.buf.String()
	w.buf.Reset()
	w.mu.Unlock()
	if chunk != "" {
		appendJobLog(w.jobID, chunk)
	}
}

func (w *dbLogWriter) Close() { close(w.done) }

func executeRun(runID int64) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("ci: run %d panic: %v", runID, v)
			setRunStatus(runID, "error")
		}
	}()

	run, err := getRunByID(runID)
	if err != nil || run.Status != "queued" {
		return
	}
	repo, err := getRepoByID(run.RepoID)
	if err != nil {
		setRunStatus(runID, "error")
		return
	}
	setRunStatus(runID, "running")
	log.Printf("ci: run #%d for %s@%s started", run.Number, repo.FullName(), short(run.CommitSHA))

	cfg := loadCIConfig(repo.DiskPath(), run.CommitSHA)
	if cfg == nil {
		setRunStatus(runID, "error")
		return
	}

	ws := filepath.Join(dataDir, "ci", fmt.Sprintf("run-%d", runID))
	os.RemoveAll(ws)
	defer os.RemoveAll(ws)

	// Local clone of the bare repo, checked out at the run's commit.
	if _, err := gitRun("", "clone", "--quiet", "--no-hardlinks", repo.DiskPath(), ws); err != nil {
		markRunError(runID, "workspace clone failed: "+err.Error())
		return
	}
	if _, err := gitRun(ws, "checkout", "--quiet", run.CommitSHA); err != nil {
		markRunError(runID, "checkout failed: "+err.Error())
		return
	}

	overall := "success"
	for _, job := range runJobs(runID) {
		jc := cfg.Jobs[job.Name]
		if jc == nil {
			setJobStatus(job.ID, "error", 0)
			overall = "failure"
			continue
		}
		if !executeJob(job, jc, ws, repo, run) {
			overall = "failure"
		}
	}
	setRunStatus(runID, overall)
	log.Printf("ci: run #%d for %s finished: %s", run.Number, repo.FullName(), overall)

	fireWebhooks(repo, "ci_run", map[string]any{
		"repository": repo.FullName(),
		"run":        run.Number,
		"commit":     run.CommitSHA,
		"ref":        run.Ref,
		"status":     overall,
	})

	// Only failures are worth interrupting someone for, and only on a branch
	// with an open pull request — a red build on a scratch branch is the
	// author's business, not an inbox item.
	if overall == "failure" {
		for _, pr := range openPullsWithHead(repo.ID, strings.TrimPrefix(run.Ref, "refs/heads/")) {
			notify(pr.AuthorID, nil, repo, "pull", pr.Number,
				fmt.Sprintf("CI failed on %s", pr.Title), reasonCI)
		}
	}
}

func markRunError(runID int64, msg string) {
	for _, j := range runJobs(runID) {
		appendJobLog(j.ID, msg+"\n")
		setJobStatus(j.ID, "error", -1)
	}
	setRunStatus(runID, "error")
}

// executeJob runs each step of a job with bash in the workspace; returns success.
func executeJob(job *CIJob, jc *CIJobConfig, ws string, repo *Repo, run *CIRun) bool {
	setJobStatus(job.ID, "running", 0)
	logw := newDBLogWriter(job.ID)
	defer logw.Close()

	timeout := 10 * time.Minute
	if jc.TimeoutMinutes > 0 {
		timeout = time.Duration(jc.TimeoutMinutes) * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	env := os.Environ()
	env = append(env,
		"CI=true", "GITGIT=true",
		"GITGIT_REPO="+repo.FullName(),
		"GITGIT_SHA="+run.CommitSHA,
		"GITGIT_REF="+run.Ref,
		"GITGIT_EVENT="+run.Event,
		"GITGIT_RUN_NUMBER="+fmt.Sprint(run.Number),
	)
	for k, v := range jc.Env {
		env = append(env, k+"="+v)
	}

	for i, step := range jc.Steps {
		name := step.Name
		if name == "" {
			name = fmt.Sprintf("step %d", i+1)
		}
		logw.WriteString(fmt.Sprintf("=== %s ===\n$ %s\n", name, strings.TrimSpace(step.Run)))
		cmd := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", step.Run)
		cmd.Dir = ws
		cmd.Env = env
		cmd.Stdout = logw
		cmd.Stderr = logw
		start := time.Now()
		err := cmd.Run()
		dur := time.Since(start).Round(time.Millisecond)
		if err != nil {
			exit := -1
			if ee, ok := err.(*exec.ExitError); ok {
				exit = ee.ExitCode()
			}
			status := "failure"
			if ctx.Err() == context.DeadlineExceeded {
				status = "timeout"
				logw.WriteString(fmt.Sprintf("\n!!! job timed out after %s\n", timeout))
			} else {
				logw.WriteString(fmt.Sprintf("\n!!! step failed (exit %d) after %s\n", exit, dur))
			}
			setJobStatus(job.ID, status, int64(exit))
			return false
		}
		logw.WriteString(fmt.Sprintf("(ok, %s)\n\n", dur))
	}
	setJobStatus(job.ID, "success", 0)
	return true
}

// ciStatusForSHA summarizes the latest run for a commit ("", queued, running,
// success, failure, error).
func ciStatusForSHA(repoID int64, sha string) (string, *CIRun) {
	run := latestRunForSHA(repoID, sha)
	if run == nil {
		return "", nil
	}
	return run.Status, run
}
