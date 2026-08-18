package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	_ "modernc.org/sqlite"
)

var db *sql.DB

// NullInt64 aliases sql.NullInt64 for brevity in handlers.
type NullInt64 = sql.NullInt64

const schema = `
PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
  email         TEXT NOT NULL DEFAULT '',
  full_name     TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL,
  is_admin      INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  token      TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL,
  csrf       TEXT NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS access_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL,
  name         TEXT NOT NULL,
  token_hash   TEXT NOT NULL UNIQUE,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER
);

CREATE TABLE IF NOT EXISTS repos (
  id                     INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_id               INTEGER NOT NULL,
  name                   TEXT NOT NULL COLLATE NOCASE,
  description            TEXT NOT NULL DEFAULT '',
  default_branch         TEXT NOT NULL DEFAULT 'main',
  is_private             INTEGER NOT NULL DEFAULT 0,
  next_number            INTEGER NOT NULL DEFAULT 1,
  next_run_number        INTEGER NOT NULL DEFAULT 1,
  require_ci_pass        INTEGER NOT NULL DEFAULT 0,
  require_approvals      INTEGER NOT NULL DEFAULT 0,
  allow_merge            INTEGER NOT NULL DEFAULT 1,
  allow_squash           INTEGER NOT NULL DEFAULT 1,
  allow_rebase           INTEGER NOT NULL DEFAULT 1,
  delete_branch_on_merge INTEGER NOT NULL DEFAULT 1,
  created_at             INTEGER NOT NULL,
  UNIQUE(owner_id, name)
);

CREATE TABLE IF NOT EXISTS collaborators (
  repo_id INTEGER NOT NULL,
  user_id INTEGER NOT NULL,
  role    TEXT NOT NULL DEFAULT 'write',
  PRIMARY KEY (repo_id, user_id)
);

CREATE TABLE IF NOT EXISTS pulls (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id      INTEGER NOT NULL,
  number       INTEGER NOT NULL,
  title        TEXT NOT NULL,
  body         TEXT NOT NULL DEFAULT '',
  author_id    INTEGER NOT NULL,
  base_branch  TEXT NOT NULL,
  head_branch  TEXT NOT NULL,
  state        TEXT NOT NULL DEFAULT 'open',
  merge_commit TEXT NOT NULL DEFAULT '',
  merged_by    INTEGER,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL,
  merged_at    INTEGER,
  closed_at    INTEGER,
  UNIQUE(repo_id, number)
);

CREATE TABLE IF NOT EXISTS issues (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id    INTEGER NOT NULL,
  number     INTEGER NOT NULL,
  title      TEXT NOT NULL,
  body       TEXT NOT NULL DEFAULT '',
  author_id  INTEGER NOT NULL,
  state      TEXT NOT NULL DEFAULT 'open',
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  closed_at  INTEGER,
  UNIQUE(repo_id, number)
);

CREATE TABLE IF NOT EXISTS comments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  target     TEXT NOT NULL,
  target_id  INTEGER NOT NULL,
  author_id  INTEGER NOT NULL,
  body       TEXT NOT NULL,
  system     INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS reviews (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  pull_id     INTEGER NOT NULL,
  reviewer_id INTEGER NOT NULL,
  state       TEXT NOT NULL,
  body        TEXT NOT NULL DEFAULT '',
  commit_sha  TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS review_comments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  pull_id    INTEGER NOT NULL,
  author_id  INTEGER NOT NULL,
  file       TEXT NOT NULL,
  line       INTEGER NOT NULL,
  side       TEXT NOT NULL DEFAULT 'new',
  body       TEXT NOT NULL,
  commit_sha TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS labels (
  id      INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id INTEGER NOT NULL,
  name    TEXT NOT NULL,
  color   TEXT NOT NULL DEFAULT '#1f6feb',
  UNIQUE(repo_id, name)
);

CREATE TABLE IF NOT EXISTS item_labels (
  target    TEXT NOT NULL,
  target_id INTEGER NOT NULL,
  label_id  INTEGER NOT NULL,
  PRIMARY KEY (target, target_id, label_id)
);

CREATE TABLE IF NOT EXISTS ci_runs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id     INTEGER NOT NULL,
  number      INTEGER NOT NULL,
  commit_sha  TEXT NOT NULL,
  ref         TEXT NOT NULL DEFAULT '',
  event       TEXT NOT NULL DEFAULT 'push',
  status      TEXT NOT NULL DEFAULT 'queued',
  created_at  INTEGER NOT NULL,
  started_at  INTEGER,
  finished_at INTEGER,
  UNIQUE(repo_id, number)
);

CREATE TABLE IF NOT EXISTS ci_jobs (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  run_id      INTEGER NOT NULL,
  name        TEXT NOT NULL,
  status      TEXT NOT NULL DEFAULT 'queued',
  exit_code   INTEGER NOT NULL DEFAULT 0,
  log         TEXT NOT NULL DEFAULT '',
  started_at  INTEGER,
  finished_at INTEGER
);

CREATE TABLE IF NOT EXISTS webhooks (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id    INTEGER NOT NULL,
  url        TEXT NOT NULL,
  secret     TEXT NOT NULL DEFAULT '',
  events     TEXT NOT NULL DEFAULT 'push,pull_request,issues,ci_run',
  active     INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS stars (
  user_id INTEGER NOT NULL,
  repo_id INTEGER NOT NULL,
  PRIMARY KEY (user_id, repo_id)
);

CREATE TABLE IF NOT EXISTS previews (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id    INTEGER NOT NULL,
  ref        TEXT NOT NULL,
  token      TEXT NOT NULL UNIQUE,
  created_by INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  env_ok     INTEGER NOT NULL DEFAULT 0,
  env_paused INTEGER NOT NULL DEFAULT 0
);

-- A running instance of a branch, proxied at its own subdomain.
CREATE TABLE IF NOT EXISTS preview_envs (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  preview_id   INTEGER NOT NULL,
  repo_id      INTEGER NOT NULL,
  ref          TEXT NOT NULL,
  commit_sha   TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'queued',
  port         INTEGER NOT NULL DEFAULT 0,
  pid          INTEGER NOT NULL DEFAULT 0,
  message      TEXT NOT NULL DEFAULT '',
  log          TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  started_at   INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,
  expires_at   INTEGER NOT NULL,
  step         TEXT NOT NULL DEFAULT '',
  step_n       INTEGER NOT NULL DEFAULT 0,
  step_total   INTEGER NOT NULL DEFAULT 0,
  step_at      INTEGER NOT NULL DEFAULT 0
);
-- Values injected into Preview Environments, encrypted with a key held
-- outside this database (see secrets.go).
CREATE TABLE IF NOT EXISTS repo_secrets (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_id    INTEGER NOT NULL,
  name       TEXT NOT NULL,
  sealed     TEXT NOT NULL,
  updated_by INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  UNIQUE (repo_id, name)
);

CREATE INDEX IF NOT EXISTS idx_envs_preview ON preview_envs(preview_id);
CREATE INDEX IF NOT EXISTS idx_envs_status ON preview_envs(status);

CREATE INDEX IF NOT EXISTS idx_pulls_repo_state ON pulls(repo_id, state);
CREATE INDEX IF NOT EXISTS idx_issues_repo_state ON issues(repo_id, state);
CREATE INDEX IF NOT EXISTS idx_comments_target ON comments(target, target_id);
CREATE INDEX IF NOT EXISTS idx_ci_runs_sha ON ci_runs(repo_id, commit_sha);
`

func openDB(path string) error {
	var err error
	db, err = sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return err
	}
	// WAL allows concurrent readers; busy_timeout resolves writer contention.
	// More than one connection is required because some code paths query
	// while iterating another statement's rows.
	db.SetMaxOpenConns(8)
	if _, err = db.Exec(schema); err != nil {
		return err
	}
	// CREATE TABLE IF NOT EXISTS leaves an existing table alone, so columns
	// added after a release have to be applied to it explicitly.
	ensureColumn("previews", "env_ok", "INTEGER NOT NULL DEFAULT 0")
	ensureColumn("previews", "env_paused", "INTEGER NOT NULL DEFAULT 0")
	for _, c := range []struct{ col, ddl string }{
		{"step", "TEXT NOT NULL DEFAULT ''"},
		{"step_n", "INTEGER NOT NULL DEFAULT 0"},
		{"step_total", "INTEGER NOT NULL DEFAULT 0"},
		{"step_at", "INTEGER NOT NULL DEFAULT 0"},
	} {
		ensureColumn("preview_envs", c.col, c.ddl)
	}
	return nil
}

// ensureColumn adds a column to an existing table; SQLite has no
// ADD COLUMN IF NOT EXISTS, so check the table's shape first.
func ensureColumn(table, col, ddl string) {
	var n int
	if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, col).Scan(&n); err != nil || n > 0 {
		return
	}
	if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + ddl); err != nil {
		log.Printf("db: adding %s.%s: %v", table, col, err)
	}
}

func now() int64 { return time.Now().Unix() }

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

// ---------- users ----------

type User struct {
	ID           int64
	Username     string
	Email        string
	FullName     string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    int64
}

func (u *User) DisplayName() string {
	if u == nil {
		return "ghost"
	}
	if u.FullName != "" {
		return u.FullName
	}
	return u.Username
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

const userCols = "id, username, email, full_name, password_hash, is_admin, created_at"

func createUser(username, email, password string) (*User, error) {
	if !validSlug(username) {
		return nil, errors.New("username may only contain letters, digits, '-', '_', '.', and must not start with '.'")
	}
	if reservedNames[strings.ToLower(username)] {
		return nil, errors.New("that username is reserved")
	}
	if len(password) < 6 {
		return nil, errors.New("password must be at least 6 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	var cnt int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&cnt)
	res, err := db.Exec("INSERT INTO users (username, email, full_name, password_hash, is_admin, created_at) VALUES (?,?,?,?,?,?)",
		username, email, "", string(hash), cnt == 0, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("username already taken")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return getUserByID(id)
}

// userCount reports how many accounts exist (used to allow bootstrapping the
// first admin even when registration is closed).
func userCount() int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&n)
	return n
}

func getUserByID(id int64) (*User, error) {
	return scanUser(db.QueryRow("SELECT "+userCols+" FROM users WHERE id = ?", id))
}

func getUserByName(name string) (*User, error) {
	return scanUser(db.QueryRow("SELECT "+userCols+" FROM users WHERE username = ? COLLATE NOCASE", name))
}

func checkPassword(u *User, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil
}

func setPassword(userID int64, password string) error {
	if len(password) < 6 {
		return errors.New("password must be at least 6 characters")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(hash), userID)
	return err
}

func updateProfile(userID int64, email, fullName string) error {
	_, err := db.Exec("UPDATE users SET email = ?, full_name = ? WHERE id = ?", email, fullName, userID)
	return err
}

func listUsers() []*User {
	rows, err := db.Query("SELECT " + userCols + " FROM users ORDER BY username")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*User
	for rows.Next() {
		if u, err := scanUser(rows); err == nil {
			out = append(out, u)
		}
	}
	return out
}

// ---------- sessions ----------

type Session struct {
	Token     string
	UserID    int64
	CSRF      string
	ExpiresAt int64
}

func createSession(userID int64) (*Session, error) {
	s := &Session{Token: randHex(24), UserID: userID, CSRF: randHex(16), ExpiresAt: now() + 30*24*3600}
	_, err := db.Exec("INSERT INTO sessions (token, user_id, csrf, expires_at) VALUES (?,?,?,?)",
		s.Token, s.UserID, s.CSRF, s.ExpiresAt)
	return s, err
}

func getSession(token string) *Session {
	s := &Session{}
	err := db.QueryRow("SELECT token, user_id, csrf, expires_at FROM sessions WHERE token = ?", token).
		Scan(&s.Token, &s.UserID, &s.CSRF, &s.ExpiresAt)
	if err != nil || s.ExpiresAt < now() {
		return nil
	}
	return s
}

func deleteSession(token string) {
	db.Exec("DELETE FROM sessions WHERE token = ?", token)
}

// ---------- access tokens ----------

type AccessToken struct {
	ID         int64
	UserID     int64
	Name       string
	CreatedAt  int64
	LastUsedAt sql.NullInt64
}

func createAccessToken(userID int64, name string) (string, error) {
	plain := "gg_" + randHex(20)
	_, err := db.Exec("INSERT INTO access_tokens (user_id, name, token_hash, created_at) VALUES (?,?,?,?)",
		userID, name, hashToken(plain), now())
	return plain, err
}

func listAccessTokens(userID int64) []*AccessToken {
	rows, err := db.Query("SELECT id, user_id, name, created_at, last_used_at FROM access_tokens WHERE user_id = ? ORDER BY id DESC", userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*AccessToken
	for rows.Next() {
		t := &AccessToken{}
		if rows.Scan(&t.ID, &t.UserID, &t.Name, &t.CreatedAt, &t.LastUsedAt) == nil {
			out = append(out, t)
		}
	}
	return out
}

func deleteAccessToken(id, userID int64) {
	db.Exec("DELETE FROM access_tokens WHERE id = ? AND user_id = ?", id, userID)
}

func userByToken(plain string) *User {
	var uid int64
	err := db.QueryRow("SELECT user_id FROM access_tokens WHERE token_hash = ?", hashToken(plain)).Scan(&uid)
	if err != nil {
		return nil
	}
	db.Exec("UPDATE access_tokens SET last_used_at = ? WHERE token_hash = ?", now(), hashToken(plain))
	u, err := getUserByID(uid)
	if err != nil {
		return nil
	}
	return u
}

// ---------- repos ----------

type Repo struct {
	ID                  int64
	OwnerID             int64
	OwnerName           string
	Name                string
	Description         string
	DefaultBranch       string
	IsPrivate           bool
	NextNumber          int64
	NextRunNumber       int64
	RequireCIPass       bool
	RequireApprovals    int64
	AllowMerge          bool
	AllowSquash         bool
	AllowRebase         bool
	DeleteBranchOnMerge bool
	CreatedAt           int64
}

func (r *Repo) FullName() string { return r.OwnerName + "/" + r.Name }
func (r *Repo) DiskPath() string { return repoDiskPath(r.OwnerName, r.Name) }

const repoCols = `r.id, r.owner_id, u.username, r.name, r.description, r.default_branch, r.is_private,
 r.next_number, r.next_run_number, r.require_ci_pass, r.require_approvals,
 r.allow_merge, r.allow_squash, r.allow_rebase, r.delete_branch_on_merge, r.created_at`

func scanRepo(row interface{ Scan(...any) error }) (*Repo, error) {
	r := &Repo{}
	err := row.Scan(&r.ID, &r.OwnerID, &r.OwnerName, &r.Name, &r.Description, &r.DefaultBranch, &r.IsPrivate,
		&r.NextNumber, &r.NextRunNumber, &r.RequireCIPass, &r.RequireApprovals,
		&r.AllowMerge, &r.AllowSquash, &r.AllowRebase, &r.DeleteBranchOnMerge, &r.CreatedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func getRepo(owner, name string) (*Repo, error) {
	name = strings.TrimSuffix(name, ".git")
	return scanRepo(db.QueryRow(`SELECT `+repoCols+` FROM repos r JOIN users u ON u.id = r.owner_id
		WHERE u.username = ? COLLATE NOCASE AND r.name = ? COLLATE NOCASE`, owner, name))
}

func getRepoByID(id int64) (*Repo, error) {
	return scanRepo(db.QueryRow(`SELECT `+repoCols+` FROM repos r JOIN users u ON u.id = r.owner_id WHERE r.id = ?`, id))
}

func insertRepo(ownerID int64, name, desc string, private bool) (*Repo, error) {
	if !validSlug(name) {
		return nil, errors.New("repository name may only contain letters, digits, '-', '_', '.', and must not start with '.'")
	}
	res, err := db.Exec("INSERT INTO repos (owner_id, name, description, is_private, created_at) VALUES (?,?,?,?,?)",
		ownerID, name, desc, private, now())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, errors.New("a repository with that name already exists")
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return getRepoByID(id)
}

func repoQuery(where string, args ...any) []*Repo {
	rows, err := db.Query(`SELECT `+repoCols+` FROM repos r JOIN users u ON u.id = r.owner_id `+where, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Repo
	for rows.Next() {
		if r, err := scanRepo(rows); err == nil {
			out = append(out, r)
		}
	}
	return out
}

// listVisibleRepos returns repos viewable by u (all public plus private ones u can read).
func listVisibleRepos(u *User) []*Repo {
	if u == nil {
		return repoQuery("WHERE r.is_private = 0 ORDER BY r.created_at DESC")
	}
	if u.IsAdmin {
		return repoQuery("ORDER BY r.created_at DESC")
	}
	return repoQuery(`WHERE r.is_private = 0 OR r.owner_id = ?
		OR EXISTS (SELECT 1 FROM collaborators c WHERE c.repo_id = r.id AND c.user_id = ?)
		ORDER BY r.created_at DESC`, u.ID, u.ID)
}

func listReposForOwner(ownerID int64, includePrivate bool) []*Repo {
	if includePrivate {
		return repoQuery("WHERE r.owner_id = ? ORDER BY r.name", ownerID)
	}
	return repoQuery("WHERE r.owner_id = ? AND r.is_private = 0 ORDER BY r.name", ownerID)
}

func updateRepoMeta(r *Repo) error {
	_, err := db.Exec(`UPDATE repos SET description=?, default_branch=?, is_private=?,
		require_ci_pass=?, require_approvals=?, allow_merge=?, allow_squash=?, allow_rebase=?, delete_branch_on_merge=?
		WHERE id=?`,
		r.Description, r.DefaultBranch, r.IsPrivate, r.RequireCIPass, r.RequireApprovals,
		r.AllowMerge, r.AllowSquash, r.AllowRebase, r.DeleteBranchOnMerge, r.ID)
	return err
}

func deleteRepoRows(repoID int64) {
	for _, q := range []string{
		"DELETE FROM comments WHERE target='pull' AND target_id IN (SELECT id FROM pulls WHERE repo_id=?)",
		"DELETE FROM comments WHERE target='issue' AND target_id IN (SELECT id FROM issues WHERE repo_id=?)",
		"DELETE FROM reviews WHERE pull_id IN (SELECT id FROM pulls WHERE repo_id=?)",
		"DELETE FROM review_comments WHERE pull_id IN (SELECT id FROM pulls WHERE repo_id=?)",
		"DELETE FROM item_labels WHERE label_id IN (SELECT id FROM labels WHERE repo_id=?)",
		"DELETE FROM labels WHERE repo_id=?",
		"DELETE FROM pulls WHERE repo_id=?",
		"DELETE FROM issues WHERE repo_id=?",
		"DELETE FROM ci_jobs WHERE run_id IN (SELECT id FROM ci_runs WHERE repo_id=?)",
		"DELETE FROM ci_runs WHERE repo_id=?",
		"DELETE FROM webhooks WHERE repo_id=?",
		"DELETE FROM collaborators WHERE repo_id=?",
		"DELETE FROM stars WHERE repo_id=?",
		"DELETE FROM repos WHERE id=?",
	} {
		db.Exec(q, repoID)
	}
}

// nextItemNumber allocates the next shared issue/PR number for a repo.
func nextItemNumber(repoID int64) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var n int64
	if err := tx.QueryRow("SELECT next_number FROM repos WHERE id = ?", repoID).Scan(&n); err != nil {
		return 0, err
	}
	if _, err := tx.Exec("UPDATE repos SET next_number = next_number + 1 WHERE id = ?", repoID); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

func nextRunNumber(repoID int64) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var n int64
	if err := tx.QueryRow("SELECT next_run_number FROM repos WHERE id = ?", repoID).Scan(&n); err != nil {
		return 0, err
	}
	if _, err := tx.Exec("UPDATE repos SET next_run_number = next_run_number + 1 WHERE id = ?", repoID); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}

// ---------- permissions ----------

func collabRole(repoID, userID int64) string {
	var role string
	if db.QueryRow("SELECT role FROM collaborators WHERE repo_id = ? AND user_id = ?", repoID, userID).Scan(&role) != nil {
		return ""
	}
	return role
}

func canRead(u *User, r *Repo) bool {
	if !r.IsPrivate {
		return true
	}
	if u == nil {
		return false
	}
	return u.IsAdmin || u.ID == r.OwnerID || collabRole(r.ID, u.ID) != ""
}

func canWrite(u *User, r *Repo) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin || u.ID == r.OwnerID {
		return true
	}
	role := collabRole(r.ID, u.ID)
	return role == "write" || role == "admin"
}

func canAdmin(u *User, r *Repo) bool {
	if u == nil {
		return false
	}
	if u.IsAdmin || u.ID == r.OwnerID {
		return true
	}
	return collabRole(r.ID, u.ID) == "admin"
}

type Collaborator struct {
	User *User
	Role string
}

func listCollaborators(repoID int64) []*Collaborator {
	rows, err := db.Query(`SELECT `+userCols+`, c.role FROM collaborators c JOIN users ON users.id = c.user_id WHERE c.repo_id = ? ORDER BY users.username`, repoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Collaborator
	for rows.Next() {
		u := &User{}
		var role string
		if rows.Scan(&u.ID, &u.Username, &u.Email, &u.FullName, &u.PasswordHash, &u.IsAdmin, &u.CreatedAt, &role) == nil {
			out = append(out, &Collaborator{User: u, Role: role})
		}
	}
	return out
}

func addCollaborator(repoID, userID int64, role string) error {
	if role != "read" && role != "write" && role != "admin" {
		role = "write"
	}
	_, err := db.Exec("INSERT OR REPLACE INTO collaborators (repo_id, user_id, role) VALUES (?,?,?)", repoID, userID, role)
	return err
}

func removeCollaborator(repoID, userID int64) {
	db.Exec("DELETE FROM collaborators WHERE repo_id = ? AND user_id = ?", repoID, userID)
}

// ---------- stars ----------

func starRepo(userID, repoID int64, on bool) {
	if on {
		db.Exec("INSERT OR IGNORE INTO stars (user_id, repo_id) VALUES (?,?)", userID, repoID)
	} else {
		db.Exec("DELETE FROM stars WHERE user_id = ? AND repo_id = ?", userID, repoID)
	}
}

func isStarred(userID, repoID int64) bool {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM stars WHERE user_id = ? AND repo_id = ?", userID, repoID).Scan(&n)
	return n > 0
}

func starCount(repoID int64) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM stars WHERE repo_id = ?", repoID).Scan(&n)
	return n
}

// ---------- pulls ----------

type Pull struct {
	ID          int64
	RepoID      int64
	Number      int64
	Title       string
	Body        string
	AuthorID    int64
	BaseBranch  string
	HeadBranch  string
	State       string // open | merged | closed
	MergeCommit string
	MergedBy    sql.NullInt64
	CreatedAt   int64
	UpdatedAt   int64
	MergedAt    sql.NullInt64
	ClosedAt    sql.NullInt64

	Author *User // populated by loadPullAuthors
}

const pullCols = "id, repo_id, number, title, body, author_id, base_branch, head_branch, state, merge_commit, merged_by, created_at, updated_at, merged_at, closed_at"

func scanPull(row interface{ Scan(...any) error }) (*Pull, error) {
	p := &Pull{}
	err := row.Scan(&p.ID, &p.RepoID, &p.Number, &p.Title, &p.Body, &p.AuthorID, &p.BaseBranch, &p.HeadBranch,
		&p.State, &p.MergeCommit, &p.MergedBy, &p.CreatedAt, &p.UpdatedAt, &p.MergedAt, &p.ClosedAt)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func createPull(repoID, authorID int64, title, body, base, head string) (*Pull, error) {
	num, err := nextItemNumber(repoID)
	if err != nil {
		return nil, err
	}
	t := now()
	res, err := db.Exec(`INSERT INTO pulls (repo_id, number, title, body, author_id, base_branch, head_branch, state, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,'open',?,?)`, repoID, num, title, body, authorID, base, head, t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return getPullByID(id)
}

func getPullByID(id int64) (*Pull, error) {
	return scanPull(db.QueryRow("SELECT "+pullCols+" FROM pulls WHERE id = ?", id))
}

func getPull(repoID, number int64) (*Pull, error) {
	return scanPull(db.QueryRow("SELECT "+pullCols+" FROM pulls WHERE repo_id = ? AND number = ?", repoID, number))
}

func pullQuery(where string, args ...any) []*Pull {
	rows, err := db.Query("SELECT "+pullCols+" FROM pulls "+where, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Pull
	for rows.Next() {
		if p, err := scanPull(rows); err == nil {
			out = append(out, p)
		}
	}
	return out
}

func listPulls(repoID int64, state string) []*Pull {
	if state == "all" {
		return pullQuery("WHERE repo_id = ? ORDER BY number DESC", repoID)
	}
	if state == "closed" {
		// closed tab includes merged, like GitHub
		return pullQuery("WHERE repo_id = ? AND state != 'open' ORDER BY number DESC", repoID)
	}
	return pullQuery("WHERE repo_id = ? AND state = ? ORDER BY number DESC", repoID, state)
}

func openPullsWithHead(repoID int64, head string) []*Pull {
	return pullQuery("WHERE repo_id = ? AND state = 'open' AND head_branch = ?", repoID, head)
}

func openPullsWithBase(repoID int64, base string) []*Pull {
	return pullQuery("WHERE repo_id = ? AND state = 'open' AND base_branch = ? ORDER BY number", repoID, base)
}

func countPulls(repoID int64, state string) int {
	var n int
	if state == "closed" {
		db.QueryRow("SELECT COUNT(*) FROM pulls WHERE repo_id = ? AND state != 'open'", repoID).Scan(&n)
	} else {
		db.QueryRow("SELECT COUNT(*) FROM pulls WHERE repo_id = ? AND state = ?", repoID, state).Scan(&n)
	}
	return n
}

func touchPull(id int64) {
	db.Exec("UPDATE pulls SET updated_at = ? WHERE id = ?", now(), id)
}

func savePull(p *Pull) error {
	_, err := db.Exec(`UPDATE pulls SET title=?, body=?, base_branch=?, head_branch=?, state=?, merge_commit=?,
		merged_by=?, updated_at=?, merged_at=?, closed_at=? WHERE id=?`,
		p.Title, p.Body, p.BaseBranch, p.HeadBranch, p.State, p.MergeCommit, p.MergedBy, now(), p.MergedAt, p.ClosedAt, p.ID)
	return err
}

func loadPullAuthors(pulls []*Pull) {
	cache := map[int64]*User{}
	for _, p := range pulls {
		if u, ok := cache[p.AuthorID]; ok {
			p.Author = u
			continue
		}
		u, err := getUserByID(p.AuthorID)
		if err != nil {
			u = &User{Username: "ghost"}
		}
		cache[p.AuthorID] = u
		p.Author = u
	}
}

// ---------- issues ----------

type Issue struct {
	ID        int64
	RepoID    int64
	Number    int64
	Title     string
	Body      string
	AuthorID  int64
	State     string
	CreatedAt int64
	UpdatedAt int64
	ClosedAt  sql.NullInt64

	Author *User
	Labels []*Label
}

const issueCols = "id, repo_id, number, title, body, author_id, state, created_at, updated_at, closed_at"

func scanIssue(row interface{ Scan(...any) error }) (*Issue, error) {
	i := &Issue{}
	err := row.Scan(&i.ID, &i.RepoID, &i.Number, &i.Title, &i.Body, &i.AuthorID, &i.State, &i.CreatedAt, &i.UpdatedAt, &i.ClosedAt)
	if err != nil {
		return nil, err
	}
	return i, nil
}

func createIssue(repoID, authorID int64, title, body string) (*Issue, error) {
	num, err := nextItemNumber(repoID)
	if err != nil {
		return nil, err
	}
	t := now()
	res, err := db.Exec(`INSERT INTO issues (repo_id, number, title, body, author_id, state, created_at, updated_at)
		VALUES (?,?,?,?,?,'open',?,?)`, repoID, num, title, body, authorID, t, t)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return scanIssue(db.QueryRow("SELECT "+issueCols+" FROM issues WHERE id = ?", id))
}

func getIssue(repoID, number int64) (*Issue, error) {
	return scanIssue(db.QueryRow("SELECT "+issueCols+" FROM issues WHERE repo_id = ? AND number = ?", repoID, number))
}

func listIssues(repoID int64, state string) []*Issue {
	q := "WHERE repo_id = ? AND state = ? ORDER BY number DESC"
	args := []any{repoID, state}
	if state == "all" {
		q = "WHERE repo_id = ? ORDER BY number DESC"
		args = []any{repoID}
	}
	rows, err := db.Query("SELECT "+issueCols+" FROM issues "+q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Issue
	for rows.Next() {
		if i, err := scanIssue(rows); err == nil {
			out = append(out, i)
		}
	}
	return out
}

func countIssues(repoID int64, state string) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM issues WHERE repo_id = ? AND state = ?", repoID, state).Scan(&n)
	return n
}

func saveIssue(i *Issue) error {
	_, err := db.Exec("UPDATE issues SET title=?, body=?, state=?, updated_at=?, closed_at=? WHERE id=?",
		i.Title, i.Body, i.State, now(), i.ClosedAt, i.ID)
	return err
}

// ---------- comments ----------

type Comment struct {
	ID        int64
	Target    string
	TargetID  int64
	AuthorID  int64
	Body      string
	System    bool
	CreatedAt int64

	Author *User
}

func addComment(target string, targetID, authorID int64, body string, system bool) (*Comment, error) {
	res, err := db.Exec("INSERT INTO comments (target, target_id, author_id, body, system, created_at) VALUES (?,?,?,?,?,?)",
		target, targetID, authorID, body, system, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	c := &Comment{ID: id, Target: target, TargetID: targetID, AuthorID: authorID, Body: body, System: system, CreatedAt: now()}
	return c, nil
}

func listComments(target string, targetID int64) []*Comment {
	rows, err := db.Query("SELECT id, target, target_id, author_id, body, system, created_at FROM comments WHERE target = ? AND target_id = ? ORDER BY id", target, targetID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Comment
	for rows.Next() {
		c := &Comment{}
		if rows.Scan(&c.ID, &c.Target, &c.TargetID, &c.AuthorID, &c.Body, &c.System, &c.CreatedAt) == nil {
			c.Author, _ = getUserByID(c.AuthorID)
			out = append(out, c)
		}
	}
	return out
}

func countComments(target string, targetID int64) int {
	var n int
	db.QueryRow("SELECT COUNT(*) FROM comments WHERE target = ? AND target_id = ? AND system = 0", target, targetID).Scan(&n)
	return n
}

// ---------- reviews ----------

type Review struct {
	ID         int64
	PullID     int64
	ReviewerID int64
	State      string // approved | changes_requested | commented
	Body       string
	CommitSHA  string
	CreatedAt  int64

	Reviewer *User
}

func addReview(pullID, reviewerID int64, state, body, sha string) (*Review, error) {
	res, err := db.Exec("INSERT INTO reviews (pull_id, reviewer_id, state, body, commit_sha, created_at) VALUES (?,?,?,?,?,?)",
		pullID, reviewerID, state, body, sha, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &Review{ID: id, PullID: pullID, ReviewerID: reviewerID, State: state, Body: body, CommitSHA: sha, CreatedAt: now()}, nil
}

func listReviews(pullID int64) []*Review {
	rows, err := db.Query("SELECT id, pull_id, reviewer_id, state, body, commit_sha, created_at FROM reviews WHERE pull_id = ? ORDER BY id", pullID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Review
	for rows.Next() {
		r := &Review{}
		if rows.Scan(&r.ID, &r.PullID, &r.ReviewerID, &r.State, &r.Body, &r.CommitSHA, &r.CreatedAt) == nil {
			r.Reviewer, _ = getUserByID(r.ReviewerID)
			out = append(out, r)
		}
	}
	return out
}

// reviewVerdicts returns each reviewer's latest non-comment review state.
func reviewVerdicts(pullID int64) map[int64]string {
	out := map[int64]string{}
	for _, r := range listReviews(pullID) {
		if r.State == "approved" || r.State == "changes_requested" {
			out[r.ReviewerID] = r.State
		}
	}
	return out
}

func approvalCount(pullID int64) (approvals int, changesRequested int) {
	for _, s := range reviewVerdicts(pullID) {
		switch s {
		case "approved":
			approvals++
		case "changes_requested":
			changesRequested++
		}
	}
	return
}

type ReviewComment struct {
	ID        int64
	PullID    int64
	AuthorID  int64
	File      string
	Line      int64
	Side      string
	Body      string
	CommitSHA string
	CreatedAt int64

	Author *User
}

func addReviewComment(pullID, authorID int64, file string, line int64, side, body, sha string) error {
	_, err := db.Exec("INSERT INTO review_comments (pull_id, author_id, file, line, side, body, commit_sha, created_at) VALUES (?,?,?,?,?,?,?,?)",
		pullID, authorID, file, line, side, body, sha, now())
	return err
}

func listReviewComments(pullID int64) []*ReviewComment {
	rows, err := db.Query("SELECT id, pull_id, author_id, file, line, side, body, commit_sha, created_at FROM review_comments WHERE pull_id = ? ORDER BY id", pullID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*ReviewComment
	for rows.Next() {
		c := &ReviewComment{}
		if rows.Scan(&c.ID, &c.PullID, &c.AuthorID, &c.File, &c.Line, &c.Side, &c.Body, &c.CommitSHA, &c.CreatedAt) == nil {
			c.Author, _ = getUserByID(c.AuthorID)
			out = append(out, c)
		}
	}
	return out
}

// ---------- labels ----------

type Label struct {
	ID     int64
	RepoID int64
	Name   string
	Color  string
}

func createLabel(repoID int64, name, color string) error {
	if name == "" {
		return errors.New("label name required")
	}
	if color == "" {
		color = "#1f6feb"
	}
	_, err := db.Exec("INSERT OR IGNORE INTO labels (repo_id, name, color) VALUES (?,?,?)", repoID, name, color)
	return err
}

func deleteLabel(repoID, labelID int64) {
	db.Exec("DELETE FROM item_labels WHERE label_id = ?", labelID)
	db.Exec("DELETE FROM labels WHERE id = ? AND repo_id = ?", labelID, repoID)
}

func listLabels(repoID int64) []*Label {
	rows, err := db.Query("SELECT id, repo_id, name, color FROM labels WHERE repo_id = ? ORDER BY name", repoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Label
	for rows.Next() {
		l := &Label{}
		if rows.Scan(&l.ID, &l.RepoID, &l.Name, &l.Color) == nil {
			out = append(out, l)
		}
	}
	return out
}

func setItemLabel(target string, targetID, labelID int64, on bool) {
	if on {
		db.Exec("INSERT OR IGNORE INTO item_labels (target, target_id, label_id) VALUES (?,?,?)", target, targetID, labelID)
	} else {
		db.Exec("DELETE FROM item_labels WHERE target = ? AND target_id = ? AND label_id = ?", target, targetID, labelID)
	}
}

func itemLabels(target string, targetID int64) []*Label {
	rows, err := db.Query(`SELECT l.id, l.repo_id, l.name, l.color FROM item_labels il
		JOIN labels l ON l.id = il.label_id WHERE il.target = ? AND il.target_id = ? ORDER BY l.name`, target, targetID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Label
	for rows.Next() {
		l := &Label{}
		if rows.Scan(&l.ID, &l.RepoID, &l.Name, &l.Color) == nil {
			out = append(out, l)
		}
	}
	return out
}

// ---------- CI ----------

type CIRun struct {
	ID         int64
	RepoID     int64
	Number     int64
	CommitSHA  string
	Ref        string
	Event      string
	Status     string // queued | running | success | failure | error
	CreatedAt  int64
	StartedAt  sql.NullInt64
	FinishedAt sql.NullInt64

	Jobs []*CIJob
}

type CIJob struct {
	ID         int64
	RunID      int64
	Name       string
	Status     string
	ExitCode   int64
	Log        string
	StartedAt  sql.NullInt64
	FinishedAt sql.NullInt64
}

const runCols = "id, repo_id, number, commit_sha, ref, event, status, created_at, started_at, finished_at"

func scanRun(row interface{ Scan(...any) error }) (*CIRun, error) {
	r := &CIRun{}
	err := row.Scan(&r.ID, &r.RepoID, &r.Number, &r.CommitSHA, &r.Ref, &r.Event, &r.Status, &r.CreatedAt, &r.StartedAt, &r.FinishedAt)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func createCIRun(repoID int64, sha, ref, event string, jobNames []string) (*CIRun, error) {
	num, err := nextRunNumber(repoID)
	if err != nil {
		return nil, err
	}
	res, err := db.Exec("INSERT INTO ci_runs (repo_id, number, commit_sha, ref, event, status, created_at) VALUES (?,?,?,?,?,'queued',?)",
		repoID, num, sha, ref, event, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	for _, name := range jobNames {
		db.Exec("INSERT INTO ci_jobs (run_id, name, status) VALUES (?,?,'queued')", id, name)
	}
	return getRunByID(id)
}

func getRunByID(id int64) (*CIRun, error) {
	return scanRun(db.QueryRow("SELECT "+runCols+" FROM ci_runs WHERE id = ?", id))
}

func getRun(repoID, number int64) (*CIRun, error) {
	return scanRun(db.QueryRow("SELECT "+runCols+" FROM ci_runs WHERE repo_id = ? AND number = ?", repoID, number))
}

func listRuns(repoID int64, limit int) []*CIRun {
	rows, err := db.Query("SELECT "+runCols+" FROM ci_runs WHERE repo_id = ? ORDER BY id DESC LIMIT ?", repoID, limit)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*CIRun
	for rows.Next() {
		if r, err := scanRun(rows); err == nil {
			out = append(out, r)
		}
	}
	return out
}

// latestRunForSHA returns the newest CI run for a commit, or nil.
func latestRunForSHA(repoID int64, sha string) *CIRun {
	r, err := scanRun(db.QueryRow("SELECT "+runCols+" FROM ci_runs WHERE repo_id = ? AND commit_sha = ? ORDER BY id DESC LIMIT 1", repoID, sha))
	if err != nil {
		return nil
	}
	return r
}

func runJobs(runID int64) []*CIJob {
	rows, err := db.Query("SELECT id, run_id, name, status, exit_code, log, started_at, finished_at FROM ci_jobs WHERE run_id = ? ORDER BY id", runID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*CIJob
	for rows.Next() {
		j := &CIJob{}
		if rows.Scan(&j.ID, &j.RunID, &j.Name, &j.Status, &j.ExitCode, &j.Log, &j.StartedAt, &j.FinishedAt) == nil {
			out = append(out, j)
		}
	}
	return out
}

func setRunStatus(runID int64, status string) {
	switch status {
	case "running":
		db.Exec("UPDATE ci_runs SET status = ?, started_at = ? WHERE id = ?", status, now(), runID)
	case "success", "failure", "error":
		db.Exec("UPDATE ci_runs SET status = ?, finished_at = ? WHERE id = ?", status, now(), runID)
	default:
		db.Exec("UPDATE ci_runs SET status = ? WHERE id = ?", status, runID)
	}
}

func setJobStatus(jobID int64, status string, exitCode int64) {
	switch status {
	case "running":
		db.Exec("UPDATE ci_jobs SET status = ?, started_at = ? WHERE id = ?", status, now(), jobID)
	case "success", "failure", "error", "timeout":
		db.Exec("UPDATE ci_jobs SET status = ?, exit_code = ?, finished_at = ? WHERE id = ?", status, exitCode, now(), jobID)
	default:
		db.Exec("UPDATE ci_jobs SET status = ? WHERE id = ?", status, jobID)
	}
}

func appendJobLog(jobID int64, chunk string) {
	db.Exec("UPDATE ci_jobs SET log = log || ? WHERE id = ?", chunk, jobID)
}

// ---------- webhooks ----------

type Webhook struct {
	ID        int64
	RepoID    int64
	URL       string
	Secret    string
	Events    string
	Active    bool
	CreatedAt int64
}

func (w *Webhook) HasEvent(ev string) bool {
	for _, e := range strings.Split(w.Events, ",") {
		if strings.TrimSpace(e) == ev {
			return true
		}
	}
	return false
}

func createWebhook(repoID int64, url, secret, events string) error {
	if url == "" {
		return errors.New("webhook URL required")
	}
	if events == "" {
		events = "push,pull_request,issues,ci_run"
	}
	_, err := db.Exec("INSERT INTO webhooks (repo_id, url, secret, events, active, created_at) VALUES (?,?,?,?,1,?)",
		repoID, url, secret, events, now())
	return err
}

func deleteWebhook(repoID, id int64) {
	db.Exec("DELETE FROM webhooks WHERE id = ? AND repo_id = ?", id, repoID)
}

func listWebhooks(repoID int64) []*Webhook {
	rows, err := db.Query("SELECT id, repo_id, url, secret, events, active, created_at FROM webhooks WHERE repo_id = ?", repoID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []*Webhook
	for rows.Next() {
		w := &Webhook{}
		if rows.Scan(&w.ID, &w.RepoID, &w.URL, &w.Secret, &w.Events, &w.Active, &w.CreatedAt) == nil {
			out = append(out, w)
		}
	}
	return out
}

// ---------- misc ----------

var reservedNames = map[string]bool{
	"login": true, "logout": true, "register": true, "settings": true, "new": true,
	"api": true, "static": true, "admin": true, "explore": true, "avatar": true,
	"assets": true, "help": true, "search": true, "stars": true, "notifications": true,
	"p": true, "preview": true, "previews": true, "dashboard": true,
}

func validSlug(s string) bool {
	if s == "" || len(s) > 100 || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".git") {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func fmtErr(format string, a ...any) error { return fmt.Errorf(format, a...) }
