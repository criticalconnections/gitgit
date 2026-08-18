package main

// Operational endpoints: the two things you need to run this for somebody else.
//
// /healthz answers "is this instance actually serving", which a provisioning
// script polls to know when a new box is live, and a monitor polls to know
// when it stopped being live. It checks the database and the repository
// directory rather than just returning 200, because a process that is up but
// cannot reach its data is down as far as anyone using it is concerned.
//
// /metrics is Prometheus text format — no client library, since the entire
// output is a couple of dozen numbers and a dependency would outweigh it.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	type check struct {
		Name string `json:"name"`
		OK   bool   `json:"ok"`
		Note string `json:"note,omitempty"`
	}
	checks := []check{}
	healthy := true
	add := func(name string, ok bool, note string) {
		checks = append(checks, check{name, ok, note})
		if !ok {
			healthy = false
		}
	}

	var users int
	err := db.QueryRow("SELECT COUNT(*) FROM users").Scan(&users)
	add("database", err == nil, errText(err))

	repoRoot := filepath.Join(dataDir, "repos")
	_, statErr := os.Stat(repoRoot)
	add("repositories", statErr == nil, errText(statErr))

	// A writable data directory: SQLite fails in confusing ways on a full or
	// read-only disk, so name it here instead.
	probe := filepath.Join(dataDir, ".healthz")
	writeErr := os.WriteFile(probe, []byte("ok"), 0o600)
	os.Remove(probe)
	add("writable", writeErr == nil, errText(writeErr))

	status := http.StatusOK
	if !healthy {
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"ok":      healthy,
		"version": version,
		"uptime":  int64(time.Since(startTme).Seconds()),
		"checks":  checks,
	})
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// handleMetrics exposes the numbers worth alerting on across a fleet.
func handleMetrics(w http.ResponseWriter, r *http.Request) {
	count := func(q string) int64 {
		var n int64
		db.QueryRow(q).Scan(&n)
		return n
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	m := func(name, help, typ string, value any) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %v\n", name, help, name, typ, name, value)
	}
	m("gitgit_up", "Always 1 when the instance is serving.", "gauge", 1)
	m("gitgit_uptime_seconds", "Seconds since start.", "gauge", int64(time.Since(startTme).Seconds()))
	m("gitgit_users", "Registered accounts, organizations included.", "gauge", count("SELECT COUNT(*) FROM users"))
	m("gitgit_repositories", "Repositories.", "gauge", count("SELECT COUNT(*) FROM repos"))
	m("gitgit_pulls_open", "Open pull requests.", "gauge", count("SELECT COUNT(*) FROM pulls WHERE state = 'open'"))
	m("gitgit_issues_open", "Open issues.", "gauge", count("SELECT COUNT(*) FROM issues WHERE state = 'open'"))
	m("gitgit_ci_runs_active", "CI runs queued or running.", "gauge",
		count("SELECT COUNT(*) FROM ci_runs WHERE status IN ('queued','running')"))
	m("gitgit_preview_envs_live", "Preview Environments building or running.", "gauge",
		count("SELECT COUNT(*) FROM preview_envs WHERE status IN ('queued','building','running')"))
	m("gitgit_goroutines", "Goroutines.", "gauge", runtime.NumGoroutine())
	m("gitgit_memory_bytes", "Heap in use.", "gauge", mem.HeapInuse)

	// Disk headroom on the data volume: the failure that takes an instance
	// down quietly, since git and SQLite both just start erroring.
	if free, total, err := diskUsage(dataDir); err == nil {
		m("gitgit_disk_free_bytes", "Free space on the data volume.", "gauge", free)
		m("gitgit_disk_total_bytes", "Size of the data volume.", "gauge", total)
	}
	if info, err := os.Stat(filepath.Join(dataDir, "gitgit.db")); err == nil {
		m("gitgit_database_bytes", "Size of the SQLite database.", "gauge", info.Size())
	}
}
