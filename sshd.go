package main

// Git over SSH.
//
// `git clone git@host:owner/repo.git` opens an SSH session and asks to exec
// one command: `git-upload-pack 'owner/repo.git'` (or receive-pack to push).
// So the whole server is: authenticate a public key to a user, parse that one
// command, and wire the process to the session's stdio.
//
// Two things are load-bearing for safety. Only those two commands are ever
// executed, never the client's string — an SSH server that runs what it is
// asked to run is a remote shell. And permissions come from the same canRead
// and canWrite the HTTP path uses, so access cannot drift between protocols.
//
// Pushes are detected exactly as over HTTP: snapshot the refs either side and
// hand the difference to processPush, so CI, pull request updates and webhooks
// fire identically whichever protocol was used.

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type SSHKey struct {
	ID          int64
	UserID      int64
	Title       string
	Fingerprint string
	PublicKey   string
	CreatedAt   int64
	LastUsedAt  int64
}

// addSSHKey stores a key in authorized_keys form, keyed by fingerprint so the
// same key cannot be registered to two accounts — which would make "who
// pushed this" ambiguous.
func addSSHKey(userID int64, title, authorized string) (*SSHKey, error) {
	authorized = strings.TrimSpace(authorized)
	pub, comment, _, _, err := ssh.ParseAuthorizedKey([]byte(authorized))
	if err != nil {
		return nil, errors.New("that does not look like an SSH public key (expected something like 'ssh-ed25519 AAAA… you@host')")
	}
	fp := ssh.FingerprintSHA256(pub)
	if title = strings.TrimSpace(title); title == "" {
		title = comment
	}
	if title == "" {
		title = "key"
	}
	var owner int64
	if db.QueryRow("SELECT user_id FROM ssh_keys WHERE fingerprint = ?", fp).Scan(&owner) == nil {
		if owner == userID {
			return nil, errors.New("you have already added that key")
		}
		return nil, errors.New("that key is already registered to another account")
	}
	normalized := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(pub)))
	res, err := db.Exec(`INSERT INTO ssh_keys (user_id, title, fingerprint, public_key, created_at)
		VALUES (?,?,?,?,?)`, userID, title, fp, normalized, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return &SSHKey{ID: id, UserID: userID, Title: title, Fingerprint: fp, PublicKey: normalized, CreatedAt: now()}, nil
}

func listSSHKeys(userID int64) []*SSHKey {
	rows, err := db.Query(`SELECT id, user_id, title, fingerprint, public_key, created_at, last_used_at
		FROM ssh_keys WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	out := []*SSHKey{}
	for rows.Next() {
		k := &SSHKey{}
		if rows.Scan(&k.ID, &k.UserID, &k.Title, &k.Fingerprint, &k.PublicKey, &k.CreatedAt, &k.LastUsedAt) == nil {
			out = append(out, k)
		}
	}
	return out
}

func deleteSSHKey(userID, id int64) {
	db.Exec("DELETE FROM ssh_keys WHERE id = ? AND user_id = ?", id, userID)
}

func userForSSHKey(pub ssh.PublicKey) *User {
	fp := ssh.FingerprintSHA256(pub)
	var userID int64
	if db.QueryRow("SELECT user_id FROM ssh_keys WHERE fingerprint = ?", fp).Scan(&userID) != nil {
		return nil
	}
	db.Exec("UPDATE ssh_keys SET last_used_at = ? WHERE fingerprint = ?", now(), fp)
	u, err := getUserByID(userID)
	if err != nil || u.IsOrg {
		return nil
	}
	return u
}

// ---------- host key ----------

// hostKey loads the server's identity, generating one on first run. Clients
// pin this on first connect, so it must be stable across restarts — which is
// why it lives in the data directory rather than being made up each boot.
func hostKey() (ssh.Signer, error) {
	path := filepath.Join(dataDir, "ssh_host_ed25519_key")
	if raw, err := os.ReadFile(path); err == nil {
		return ssh.ParsePrivateKey(raw)
	}
	pem, err := generateHostKeyPEM()
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, pem, 0o600); err != nil {
		return nil, err
	}
	log.Printf("ssh: generated a host key at %s", path)
	return ssh.ParsePrivateKey(pem)
}

// ---------- server ----------

func startSSHServer(addr string) {
	signer, err := hostKey()
	if err != nil {
		log.Printf("ssh: disabled (%v)", err)
		return
	}
	cfg := &ssh.ServerConfig{
		PublicKeyCallback: func(conn ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			u := userForSSHKey(key)
			if u == nil {
				return nil, errors.New("unrecognized key")
			}
			// The SSH username is conventionally "git" and carries no
			// authority; the key is the identity.
			return &ssh.Permissions{Extensions: map[string]string{"user_id": fmt.Sprint(u.ID)}}, nil
		},
		ServerVersion: "SSH-2.0-GitGit",
	}
	cfg.AddHostKey(signer)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("ssh: cannot listen on %s: %v", addr, err)
		return
	}
	log.Printf("ssh: listening on %s (fingerprint %s)", addr, ssh.FingerprintSHA256(signer.PublicKey()))
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("ssh: accept: %v", err)
			return
		}
		go handleSSHConn(conn, cfg)
	}
}

func handleSSHConn(nConn net.Conn, cfg *ssh.ServerConfig) {
	defer nConn.Close()
	// A handshake that never completes must not hold a goroutine forever.
	nConn.SetDeadline(time.Now().Add(30 * time.Second))
	sc, chans, reqs, err := ssh.NewServerConn(nConn, cfg)
	if err != nil {
		return // bad key, port scan, or a client that changed its mind
	}
	defer sc.Close()
	nConn.SetDeadline(time.Time{}) // a clone of a large repository takes as long as it takes
	go ssh.DiscardRequests(reqs)

	var userID int64
	fmt.Sscan(sc.Permissions.Extensions["user_id"], &userID)
	u, err := getUserByID(userID)
	if err != nil {
		return
	}

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			newChan.Reject(ssh.UnknownChannelType, "only session channels are supported")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			continue
		}
		go handleSSHSession(ch, chReqs, u)
	}
}

func handleSSHSession(ch ssh.Channel, reqs <-chan *ssh.Request, u *User) {
	defer ch.Close()
	for req := range reqs {
		if req.Type != "exec" {
			// No shell, no pty, no port forwarding: this endpoint speaks git
			// and nothing else.
			req.Reply(false, nil)
			continue
		}
		var payload struct{ Command string }
		if err := ssh.Unmarshal(req.Payload, &payload); err != nil {
			req.Reply(false, nil)
			return
		}
		req.Reply(true, nil)
		status := runGitOverSSH(ch, u, payload.Command)
		ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
		return
	}
}

func sshFail(ch ssh.Channel, msg string) uint32 {
	fmt.Fprintf(ch.Stderr(), "GitGit: %s\r\n", msg)
	return 1
}

// runGitOverSSH parses the single command a git client sends and serves it.
func runGitOverSSH(ch ssh.Channel, u *User, command string) uint32 {
	service, path, err := parseGitSSHCommand(command)
	if err != nil {
		return sshFail(ch, err.Error())
	}
	owner, name, ok := splitRepoPath(path)
	if !ok {
		return sshFail(ch, "expected a path like owner/repo.git")
	}
	repo, err := getRepo(owner, name)
	if err != nil {
		// Same answer for "missing" and "not yours", so SSH cannot be used to
		// enumerate private repositories.
		return sshFail(ch, "repository not found")
	}
	push := service == "git-receive-pack"
	if push && !canWrite(u, repo) {
		return sshFail(ch, "you do not have write access to "+repo.FullName())
	}
	if !push && !canRead(u, repo) {
		return sshFail(ch, "repository not found")
	}

	dir := repo.DiskPath()
	var before map[string]string
	if push {
		before = refsSnapshot(dir)
	}

	cmd := exec.Command("git", strings.TrimPrefix(service, "git-"), ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_PROTOCOL=version=2")
	cmd.Stdin = ch
	cmd.Stdout = ch
	cmd.Stderr = ch.Stderr()
	if err := cmd.Run(); err != nil {
		log.Printf("ssh: %s %s: %v", service, repo.FullName(), err)
		return 1
	}
	if push {
		go processPush(repo, u, before, refsSnapshot(dir))
	}
	return 0
}

// parseGitSSHCommand accepts only the two commands a git client sends, and
// returns the repository path. Anything else is refused rather than run.
func parseGitSSHCommand(command string) (service, path string, err error) {
	command = strings.TrimSpace(command)
	for _, s := range []string{"git-upload-pack", "git-receive-pack", "git-upload-archive"} {
		if rest, ok := strings.CutPrefix(command, s+" "); ok {
			if s == "git-upload-archive" {
				return "", "", errors.New("git archive over SSH is not supported; use the web UI or HTTPS")
			}
			return s, unquoteGitPath(rest), nil
		}
	}
	return "", "", errors.New("this server only serves git; interactive shells are not available")
}

// unquoteGitPath strips the quoting git puts around the repository path.
func unquoteGitPath(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"') && s[len(s)-1] == s[0] {
		s = s[1 : len(s)-1]
	}
	return s
}

// splitRepoPath turns "/owner/repo.git" into its parts, rejecting anything
// that is not exactly two segments — no traversal, no nesting.
func splitRepoPath(p string) (owner, name string, ok bool) {
	p = strings.Trim(p, "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	if strings.Contains(p, "..") {
		return "", "", false
	}
	return parts[0], parts[1], true
}

var sshOnce sync.Once

// maybeStartSSH starts the git SSH server unless it is switched off.
func maybeStartSSH(addr string) {
	if strings.TrimSpace(addr) == "" {
		return
	}
	sshOnce.Do(func() { go startSSHServer(addr) })
}

// sshCloneURL builds the git@host:owner/repo.git address, or "" when SSH is
// switched off. The port is only shown when it is not 22, since scp-style
// syntax cannot express a port and needs ssh:// form instead.
func sshCloneURL(r *http.Request, repo *Repo) string {
	if strings.TrimSpace(sshAddr) == "" {
		return ""
	}
	host := sshHostName
	if host == "" {
		host, _, _ = strings.Cut(r.Host, ":")
	}
	if host == "" {
		return ""
	}
	_, port, _ := net.SplitHostPort(sshAddr)
	if port == "" || port == "22" {
		return "git@" + host + ":" + repo.FullName() + ".git"
	}
	return "ssh://git@" + host + ":" + port + "/" + repo.FullName() + ".git"
}
