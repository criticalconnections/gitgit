package main

// Deployments: shipping a ref to a named environment.
//
// A repository declares its environments in .gitgit/deploy.yml:
//
//	environments:
//	  staging:
//	    steps:
//	      - npm ci
//	      - npm run deploy:staging
//	    url: https://staging.example.com
//	    auto_deploy: main          # deploy every push to this branch
//	  production:
//	    steps: [ ./scripts/ship.sh ]
//	    url: https://example.com
//	    require_approval: true     # never automatic, even on push
//
// A deployment is a build plus a record: which sha is live where, who sent it,
// when, and the full log. That record is the point — "what is actually running
// in production" is the question a forge should be able to answer without
// anyone having to remember.
//
// Deploy steps get the repository's secrets, exactly as Preview Environments
// do, and the same redaction applies to their logs.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type DeployEnvConfig struct {
	Steps           []string          `yaml:"steps"`
	URL             string            `yaml:"url"`
	AutoDeploy      string            `yaml:"auto_deploy"`
	RequireApproval bool              `yaml:"require_approval"`
	TimeoutMinutes  int               `yaml:"timeout_minutes"`
	Env             map[string]string `yaml:"env"`
}

type DeployConfig struct {
	Environments map[string]*DeployEnvConfig `yaml:"environments"`
}

var deployConfigPaths = []string{".gitgit/deploy.yml", ".gitgit/deploy.yaml"}

func loadDeployConfig(dir, sha string) *DeployConfig {
	for _, p := range deployConfigPaths {
		raw := fileAtCommit(dir, sha, p)
		if raw == nil {
			continue
		}
		cfg := &DeployConfig{}
		if err := yaml.Unmarshal(raw, cfg); err != nil {
			log.Printf("deploy: bad config %s at %s: %v", p, short(sha), err)
			return nil
		}
		return cfg
	}
	return nil
}

func (c *DeployEnvConfig) timeout() time.Duration {
	if c.TimeoutMinutes > 0 {
		return time.Duration(c.TimeoutMinutes) * time.Minute
	}
	return 20 * time.Minute
}

type Deployment struct {
	ID          int64
	RepoID      int64
	Number      int64
	Environment string
	Ref         string
	CommitSHA   string
	Status      string // queued | running | success | failure
	URL         string
	Log         string
	CreatorID   int64
	CreatorName string
	CreatedAt   int64
	FinishedAt  int64
}

const deployCols = `id, repo_id, number, environment, ref, commit_sha, status, url, log,
 creator_id, created_at, finished_at`

func scanDeployment(row interface{ Scan(...any) error }) *Deployment {
	d := &Deployment{}
	if row.Scan(&d.ID, &d.RepoID, &d.Number, &d.Environment, &d.Ref, &d.CommitSHA, &d.Status,
		&d.URL, &d.Log, &d.CreatorID, &d.CreatedAt, &d.FinishedAt) != nil {
		return nil
	}
	if u, err := getUserByID(d.CreatorID); err == nil {
		d.CreatorName = u.Username
	}
	return d
}

func listDeployments(repoID int64, limit int) []*Deployment {
	rows, err := db.Query("SELECT "+deployCols+" FROM deployments WHERE repo_id = ? ORDER BY id DESC LIMIT ?", repoID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*Deployment{}
	for rows.Next() {
		if d := scanDeployment(rows); d != nil {
			out = append(out, d)
		}
	}
	return out
}

func deploymentByID(repoID, id int64) *Deployment {
	return scanDeployment(db.QueryRow("SELECT "+deployCols+" FROM deployments WHERE repo_id = ? AND id = ?", repoID, id))
}

// currentDeployments answers "what is live where": the most recent successful
// deployment per environment.
func currentDeployments(repoID int64) map[string]*Deployment {
	out := map[string]*Deployment{}
	rows, err := db.Query(`SELECT `+deployCols+` FROM deployments
		WHERE repo_id = ? AND status = 'success' ORDER BY id ASC`, repoID)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		if d := scanDeployment(rows); d != nil {
			out[d.Environment] = d // later rows overwrite, leaving the newest
		}
	}
	return out
}

func setDeployStatus(id int64, status string) {
	finished := int64(0)
	if status == "success" || status == "failure" {
		finished = now()
	}
	db.Exec("UPDATE deployments SET status = ?, finished_at = ? WHERE id = ?", status, finished, id)
}

func appendDeployLog(id int64, chunk string) {
	chunk = redactDeploySecrets(id, chunk)
	db.Exec(`UPDATE deployments SET log = substr(log || ?, max(1, length(log || ?) - 200000)) WHERE id = ?`,
		chunk, chunk, id)
}

var (
	deployMu         sync.Mutex
	deployRedactions = map[int64][]string{}
)

func redactDeploySecrets(id int64, text string) string {
	deployMu.Lock()
	values := deployRedactions[id]
	deployMu.Unlock()
	for _, v := range values {
		if len(v) < 6 {
			continue
		}
		text = strings.ReplaceAll(text, v, "••••••")
	}
	return text
}

// startDeployment records a deployment and runs it in the background.
func startDeployment(repo *Repo, u *User, environment, ref string) (*Deployment, error) {
	dir := repo.DiskPath()
	sha, err := resolveCommit(dir, "refs/heads/"+ref)
	if err != nil {
		if sha, err = resolveCommit(dir, ref); err != nil {
			return nil, fmt.Errorf("no such ref: %s", ref)
		}
	}
	cfg := loadDeployConfig(dir, sha)
	if cfg == nil || cfg.Environments[environment] == nil {
		return nil, fmt.Errorf("%s does not define an environment called %q in .gitgit/deploy.yml", ref, environment)
	}
	env := cfg.Environments[environment]
	if len(env.Steps) == 0 {
		return nil, fmt.Errorf("environment %q has no steps to run", environment)
	}

	number, err := nextDeployNumber(repo.ID)
	if err != nil {
		return nil, err
	}
	var creatorID int64
	if u != nil {
		creatorID = u.ID
	}
	res, err := db.Exec(`INSERT INTO deployments
		(repo_id, number, environment, ref, commit_sha, status, url, log, creator_id, created_at, finished_at)
		VALUES (?,?,?,?,?, 'queued', ?, '', ?, ?, 0)`,
		repo.ID, number, environment, ref, sha, env.URL, creatorID, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	go runDeployment(id, repo.ID, env)
	return deploymentByID(repo.ID, id), nil
}

func nextDeployNumber(repoID int64) (int64, error) {
	var n int64
	err := db.QueryRow("SELECT COALESCE(MAX(number), 0) + 1 FROM deployments WHERE repo_id = ?", repoID).Scan(&n)
	return n, err
}

func runDeployment(id, repoID int64, cfg *DeployEnvConfig) {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("deploy %d panic: %v", id, v)
			setDeployStatus(id, "failure")
		}
		deployMu.Lock()
		delete(deployRedactions, id)
		deployMu.Unlock()
	}()

	d := deploymentByID(repoID, id)
	repo, err := getRepoByID(repoID)
	if err != nil || d == nil {
		setDeployStatus(id, "failure")
		return
	}
	setDeployStatus(id, "running")

	ws := filepath.Join(dataDir, "deploys", fmt.Sprintf("deploy-%d", id))
	os.RemoveAll(ws)
	defer os.RemoveAll(ws)
	if _, err := gitRun("", "clone", "--quiet", "--no-hardlinks", repo.DiskPath(), ws); err != nil {
		appendDeployLog(id, "clone failed: "+err.Error()+"\n")
		setDeployStatus(id, "failure")
		return
	}
	if _, err := gitRun(ws, "checkout", "--quiet", d.CommitSHA); err != nil {
		appendDeployLog(id, "checkout failed: "+err.Error()+"\n")
		setDeployStatus(id, "failure")
		return
	}

	base := append(os.Environ(),
		"CI=true", "GITGIT=true", "GITGIT_DEPLOY=true",
		"GITGIT_REPO="+repo.FullName(),
		"GITGIT_REF="+d.Ref,
		"GITGIT_SHA="+d.CommitSHA,
		"GITGIT_ENVIRONMENT="+d.Environment,
		"GITGIT_DEPLOY_NUMBER="+fmt.Sprint(d.Number),
	)
	for k, v := range cfg.Env {
		base = append(base, k+"="+v)
	}
	// Secrets last, so a value committed in deploy.yml cannot shadow one, and
	// registered for redaction before anything runs.
	pairs, values, skipped := secretEnv(repoID)
	if len(values) > 0 {
		deployMu.Lock()
		deployRedactions[id] = values
		deployMu.Unlock()
	}
	base = append(base, pairs...)
	if len(pairs) > 0 {
		names := make([]string, 0, len(pairs))
		for _, kv := range pairs {
			name, _, _ := strings.Cut(kv, "=")
			names = append(names, name)
		}
		appendDeployLog(id, "using "+fmt.Sprint(len(names))+" repository secret(s): "+strings.Join(names, ", ")+"\n")
	}
	if len(skipped) > 0 {
		appendDeployLog(id, "!!! skipped (cannot decrypt with the current key): "+strings.Join(skipped, ", ")+"\n")
	}

	for i, step := range cfg.Steps {
		appendDeployLog(id, fmt.Sprintf("\n=== step %d/%d ===\n$ %s\n", i+1, len(cfg.Steps), strings.TrimSpace(step)))
		ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout())
		cmd := exec.CommandContext(ctx, "bash", "-e", "-o", "pipefail", "-c", step)
		cmd.Dir, cmd.Env = ws, base
		out, err := cmd.CombinedOutput()
		cancel()
		appendDeployLog(id, string(out))
		if err != nil {
			appendDeployLog(id, "\n!!! step failed: "+err.Error()+"\n")
			setDeployStatus(id, "failure")
			fireWebhooks(repo, "deployment", deployPayload(repo, deploymentByID(repoID, id)))
			return
		}
	}
	appendDeployLog(id, "\ndeployed "+short(d.CommitSHA)+" to "+d.Environment+"\n")
	setDeployStatus(id, "success")
	fireWebhooks(repo, "deployment", deployPayload(repo, deploymentByID(repoID, id)))
	log.Printf("deploy: %s %s -> %s (%s)", repo.FullName(), short(d.CommitSHA), d.Environment, "success")
}

func deployPayload(repo *Repo, d *Deployment) map[string]any {
	if d == nil {
		return map[string]any{"repository": repo.FullName()}
	}
	return map[string]any{
		"repository": repo.FullName(), "environment": d.Environment, "ref": d.Ref,
		"commit": d.CommitSHA, "status": d.Status, "url": d.URL, "number": d.Number,
	}
}

// autoDeployOnPush ships environments configured to follow the branch that was
// just pushed. require_approval always wins, so production cannot be reached
// by a push even if somebody also set auto_deploy on it.
func autoDeployOnPush(repo *Repo, pusher *User, branch, sha string) {
	cfg := loadDeployConfig(repo.DiskPath(), sha)
	if cfg == nil {
		return
	}
	for name, env := range cfg.Environments {
		if env == nil || env.RequireApproval || env.AutoDeploy != branch {
			continue
		}
		if _, err := startDeployment(repo, pusher, name, branch); err != nil {
			log.Printf("deploy: auto-deploy %s: %v", name, err)
		}
	}
}

// deployEnvironments lists what a ref declares, so the UI can offer them.
func deployEnvironments(repo *Repo, ref string) (map[string]*DeployEnvConfig, error) {
	dir := repo.DiskPath()
	sha, err := resolveCommit(dir, "refs/heads/"+ref)
	if err != nil {
		if sha, err = resolveCommit(dir, ref); err != nil {
			return nil, errors.New("no such ref")
		}
	}
	cfg := loadDeployConfig(dir, sha)
	if cfg == nil {
		return map[string]*DeployEnvConfig{}, nil
	}
	return cfg.Environments, nil
}

// ---------- API ----------

func deploymentJSON(d *Deployment) map[string]any {
	return map[string]any{
		"id": d.ID, "number": d.Number, "environment": d.Environment, "ref": d.Ref,
		"commit": d.CommitSHA, "status": d.Status, "url": d.URL,
		"creator": d.CreatorName, "created_at": d.CreatedAt, "finished_at": d.FinishedAt,
	}
}

func apiDeployments(c *apiCtx, repo *Repo, rest []string) {
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		history := []map[string]any{}
		for _, d := range listDeployments(repo.ID, 50) {
			history = append(history, deploymentJSON(d))
		}
		// what each environment declares, from the default branch
		declared := []map[string]any{}
		envs, _ := deployEnvironments(repo, repo.DefaultBranch)
		live := currentDeployments(repo.ID)
		names := make([]string, 0, len(envs))
		for name := range envs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			row := map[string]any{
				"name": name, "url": envs[name].URL,
				"auto_deploy": envs[name].AutoDeploy, "require_approval": envs[name].RequireApproval,
			}
			if d := live[name]; d != nil {
				row["current"] = deploymentJSON(d)
			}
			declared = append(declared, row)
		}
		c.out(200, map[string]any{"environments": declared, "deployments": history})

	case len(rest) == 0 && c.r.Method == http.MethodPost:
		if c.u == nil || !canWrite(c.u, repo) {
			c.err(403, "write access required")
			return
		}
		var req struct{ Environment, Ref string }
		if !c.decode(&req) {
			return
		}
		ref := strings.TrimSpace(req.Ref)
		if ref == "" {
			ref = repo.DefaultBranch
		}
		d, err := startDeployment(repo, c.u, strings.TrimSpace(req.Environment), ref)
		if err != nil {
			c.err(422, err.Error())
			return
		}
		c.out(201, deploymentJSON(d))

	case len(rest) == 1 && c.r.Method == http.MethodGet:
		id, _ := strconv.ParseInt(rest[0], 10, 64)
		d := deploymentByID(repo.ID, id)
		if d == nil {
			c.err(404, "deployment not found")
			return
		}
		m := deploymentJSON(d)
		m["log"] = d.Log
		c.out(200, m)

	default:
		c.err(404, "unknown endpoint")
	}
}
