package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// repoDiskPath returns the bare repository location for owner/name.
func repoDiskPath(owner, name string) string {
	return filepath.Join(dataDir, "repos", strings.ToLower(owner), strings.ToLower(name)+".git")
}

type gitError struct {
	args   []string
	stderr string
	err    error
}

func (e *gitError) Error() string {
	msg := strings.TrimSpace(e.stderr)
	if msg == "" {
		msg = e.err.Error()
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.args, " "), msg)
}

// gitRun executes git in dir and returns trimmed stdout.
func gitRun(dir string, args ...string) (string, error) {
	out, err := gitRunBytes(dir, nil, nil, args...)
	return strings.TrimSpace(string(out)), err
}

// gitRunBytes executes git with optional stdin and extra environment.
func gitRunBytes(dir string, stdin io.Reader, env []string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), &gitError{args: args, stderr: stderr.String(), err: err}
	}
	return stdout.Bytes(), nil
}

// gitExitCode runs git and returns the exit code plus stdout (for commands
// like merge-tree where exit 1 is meaningful, not an error).
func gitExitCode(dir string, args ...string) (int, string, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			code = -1
		}
	}
	return code, stdout.String(), stderr.String()
}

// ---------- repo lifecycle ----------

func initBareRepo(path, defaultBranch string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, err := gitRun("", "init", "--bare", "--initial-branch="+defaultBranch, path)
	return err
}

func identityEnv(u *User) []string {
	name := u.DisplayName()
	email := u.Email
	if email == "" {
		email = u.Username + "@gitgit.local"
	}
	return []string{
		"GIT_AUTHOR_NAME=" + name, "GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name, "GIT_COMMITTER_EMAIL=" + email,
	}
}

// seedInitialCommit creates an initial commit containing a README on the default branch.
func seedInitialCommit(dir, branch string, u *User, repoName, desc string) error {
	readme := "# " + repoName + "\n"
	if desc != "" {
		readme += "\n" + desc + "\n"
	}
	blob, err := gitRunBytes(dir, strings.NewReader(readme), nil, "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	blobSHA := strings.TrimSpace(string(blob))
	tree, err := gitRunBytes(dir, strings.NewReader("100644 blob "+blobSHA+"\tREADME.md\n"), nil, "mktree")
	if err != nil {
		return err
	}
	treeSHA := strings.TrimSpace(string(tree))
	commit, err := gitRunBytes(dir, strings.NewReader("Initial commit"), identityEnv(u), "commit-tree", treeSHA)
	if err != nil {
		return err
	}
	commitSHA := strings.TrimSpace(string(commit))
	if _, err := gitRun(dir, "update-ref", "refs/heads/"+branch, commitSHA); err != nil {
		return err
	}
	_, err = gitRun(dir, "symbolic-ref", "HEAD", "refs/heads/"+branch)
	return err
}

func setHEADBranch(dir, branch string) {
	gitRun(dir, "symbolic-ref", "HEAD", "refs/heads/"+branch)
}

// ---------- refs ----------

type Branch struct {
	Name       string
	SHA        string
	When       int64
	Subject    string
	AuthorName string
}

func listBranches(dir string) []Branch {
	out, err := gitRun(dir, "for-each-ref", "refs/heads",
		"--sort=-committerdate", "--format=%(refname:short)%00%(objectname)%00%(committerdate:unix)%00%(contents:subject)%00%(authorname)")
	if err != nil || out == "" {
		return nil
	}
	var branches []Branch
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\x00", 5)
		if len(parts) != 5 {
			continue
		}
		when, _ := strconv.ParseInt(parts[2], 10, 64)
		branches = append(branches, Branch{Name: parts[0], SHA: parts[1], When: when, Subject: parts[3], AuthorName: parts[4]})
	}
	return branches
}

func branchExists(dir, name string) bool {
	_, err := gitRun(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// resolveCommit resolves any ref/sha expression to a full commit sha.
func resolveCommit(dir, ref string) (string, error) {
	if strings.HasPrefix(ref, "-") {
		return "", errors.New("invalid ref")
	}
	return gitRun(dir, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

func isEmptyRepo(dir string) bool {
	out, err := gitRun(dir, "for-each-ref", "refs/heads", "--count=1")
	return err != nil || out == ""
}

// refsSnapshot maps refname -> sha for all branches (used to detect pushes).
func refsSnapshot(dir string) map[string]string {
	out, _ := gitRun(dir, "for-each-ref", "refs/heads", "--format=%(refname)%00%(objectname)")
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if parts := strings.SplitN(line, "\x00", 2); len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	return m
}

func updateRefCAS(dir, ref, newSHA, oldSHA string) error {
	if oldSHA == "" {
		_, err := gitRun(dir, "update-ref", ref, newSHA)
		return err
	}
	_, err := gitRun(dir, "update-ref", ref, newSHA, oldSHA)
	return err
}

func deleteBranch(dir, branch string) error {
	_, err := gitRun(dir, "update-ref", "-d", "refs/heads/"+branch)
	return err
}

func createBranchAt(dir, branch, sha string) error {
	_, err := gitRun(dir, "update-ref", "refs/heads/"+branch, sha, strings.Repeat("0", 40))
	return err
}

// ---------- trees and blobs ----------

type TreeEntry struct {
	Mode string
	Type string // blob | tree | commit (submodule)
	SHA  string
	Size int64
	Name string
	Path string
}

func lsTree(dir, commitSHA, path string) ([]TreeEntry, error) {
	spec := commitSHA
	if path != "" {
		spec = commitSHA + ":" + path
	}
	out, err := gitRunBytes(dir, nil, nil, "ls-tree", "-z", "--long", spec)
	if err != nil {
		return nil, err
	}
	var entries []TreeEntry
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		// format: <mode> <type> <sha> <size>\t<name>
		tab := strings.IndexByte(rec, '\t')
		if tab < 0 {
			continue
		}
		meta := strings.Fields(rec[:tab])
		if len(meta) < 4 {
			continue
		}
		size, _ := strconv.ParseInt(meta[3], 10, 64)
		name := rec[tab+1:]
		p := name
		if path != "" {
			p = path + "/" + name
		}
		entries = append(entries, TreeEntry{Mode: meta[0], Type: meta[1], SHA: meta[2], Size: size, Name: name, Path: p})
	}
	// directories first, then files, alphabetical
	sortTreeEntries(entries)
	return entries, nil
}

func sortTreeEntries(entries []TreeEntry) {
	rank := func(e TreeEntry) int {
		if e.Type == "tree" {
			return 0
		}
		return 1
	}
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0; j-- {
			a, b := entries[j-1], entries[j]
			if rank(a) > rank(b) || (rank(a) == rank(b) && strings.ToLower(a.Name) > strings.ToLower(b.Name)) {
				entries[j-1], entries[j] = b, a
			} else {
				break
			}
		}
	}
}

// pathKind reports whether path at commit is a tree, blob, or missing.
func pathKind(dir, commitSHA, path string) string {
	if path == "" {
		return "tree"
	}
	if typ, _, err := repoBatch(dir).info(dir, commitSHA+":"+path); err == nil {
		return typ
	} else if err == errObjectMissing {
		return ""
	}
	out, err := gitRun(dir, "cat-file", "-t", commitSHA+":"+path)
	if err != nil {
		return ""
	}
	return out
}

func readBlob(dir, commitSHA, path string, limit int64) ([]byte, bool, error) {
	typ, out, err := readObject(dir, commitSHA+":"+path)
	if err != nil {
		return nil, false, err
	}
	if typ != "blob" {
		return nil, false, errors.New("not a file: " + path)
	}
	truncated := false
	if limit > 0 && int64(len(out)) > limit {
		out = out[:limit]
		truncated = true
	}
	return out, truncated, nil
}

// ---------- commits ----------

type CommitInfo struct {
	SHA         string
	ShortSHA    string
	AuthorName  string
	AuthorEmail string
	When        int64
	Subject     string
	Body        string
	Parents     []string
}

const logFormat = "%H%x00%an%x00%ae%x00%at%x00%s%x00%b%x00%P%x1e"

func parseCommits(raw string) []*CommitInfo {
	var out []*CommitInfo
	for _, rec := range strings.Split(raw, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if strings.TrimSpace(rec) == "" {
			continue
		}
		parts := strings.SplitN(rec, "\x00", 7)
		if len(parts) < 7 {
			continue
		}
		when, _ := strconv.ParseInt(parts[3], 10, 64)
		c := &CommitInfo{SHA: parts[0], ShortSHA: parts[0][:min(10, len(parts[0]))],
			AuthorName: parts[1], AuthorEmail: parts[2], When: when, Subject: parts[4], Body: strings.TrimSpace(parts[5])}
		if p := strings.TrimSpace(parts[6]); p != "" {
			c.Parents = strings.Fields(p)
		}
		out = append(out, c)
	}
	return out
}

func listCommits(dir, ref, path string, limit, skip int) ([]*CommitInfo, error) {
	args := []string{"log", "--format=" + logFormat, "-n", strconv.Itoa(limit), "--skip=" + strconv.Itoa(skip), ref}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := gitRunBytes(dir, nil, nil, args...)
	if err != nil {
		return nil, err
	}
	return parseCommits(string(out)), nil
}

func commitRange(dir, base, head string) ([]*CommitInfo, error) {
	out, err := gitRunBytes(dir, nil, nil, "log", "--format="+logFormat, "--reverse", base+".."+head)
	if err != nil {
		return nil, err
	}
	return parseCommits(string(out)), nil
}

func getCommit(dir, sha string) (*CommitInfo, error) {
	out, err := gitRunBytes(dir, nil, nil, "log", "--format="+logFormat, "-n", "1", sha)
	if err != nil {
		return nil, err
	}
	commits := parseCommits(string(out))
	if len(commits) == 0 {
		return nil, errors.New("commit not found")
	}
	return commits[0], nil
}

// commitPatch returns the unified diff a commit introduces (vs first parent).
func commitPatch(dir, sha string) (string, error) {
	out, err := gitRunBytes(dir, nil, nil, "show", "--format=", "--patch", "--stat-width=1", sha)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// diffRange produces a unified diff. threeDot diffs from the merge-base
// (what a PR would introduce); twoDot compares tips directly.
func diffRange(dir, base, head string, threeDot bool) (string, error) {
	sep := ".."
	if threeDot {
		sep = "..."
	}
	out, err := gitRunBytes(dir, nil, nil, "diff", base+sep+head)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func mergeBase(dir, a, b string) (string, error) {
	return gitRun(dir, "merge-base", a, b)
}

// aheadBehind returns how many commits head is ahead of / behind base.
func aheadBehind(dir, base, head string) (ahead, behind int) {
	out, err := gitRun(dir, "rev-list", "--left-right", "--count", base+"..."+head)
	if err != nil {
		return 0, 0
	}
	fields := strings.Fields(out)
	if len(fields) == 2 {
		behind, _ = strconv.Atoi(fields[0])
		ahead, _ = strconv.Atoi(fields[1])
	}
	return
}

// ---------- merging (all in the bare repo via merge-tree) ----------

type MergeCheck struct {
	Clean     bool
	TreeSHA   string
	Conflicts []string
}

// mergeTreeCheck computes the merged tree of base+head without touching refs.
func mergeTreeCheck(dir, base, head string) (*MergeCheck, error) {
	code, stdout, stderr := gitExitCode(dir, "merge-tree", "--write-tree", "--name-only", base, head)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	switch code {
	case 0:
		if len(lines) == 0 || lines[0] == "" {
			return nil, errors.New("merge-tree returned no tree")
		}
		return &MergeCheck{Clean: true, TreeSHA: strings.TrimSpace(lines[0])}, nil
	case 1:
		mc := &MergeCheck{Clean: false}
		if len(lines) > 0 {
			mc.TreeSHA = strings.TrimSpace(lines[0])
			for _, l := range lines[1:] {
				if l = strings.TrimSpace(l); l != "" {
					mc.Conflicts = append(mc.Conflicts, l)
				}
			}
		}
		return mc, nil
	default:
		return nil, fmt.Errorf("merge-tree failed: %s", strings.TrimSpace(stderr))
	}
}

func commitTree(dir, tree string, parents []string, message string, u *User) (string, error) {
	args := []string{"commit-tree", tree}
	for _, p := range parents {
		args = append(args, "-p", p)
	}
	out, err := gitRunBytes(dir, strings.NewReader(message), identityEnv(u), args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// mergePR merges head into base with the given strategy: "merge", "squash", or "rebase".
// Returns the new tip of the base branch.
func mergePR(dir string, pr *Pull, strategy, message string, u *User) (string, error) {
	baseSHA, err := resolveCommit(dir, "refs/heads/"+pr.BaseBranch)
	if err != nil {
		return "", fmt.Errorf("base branch %q not found", pr.BaseBranch)
	}
	headSHA, err := resolveCommit(dir, "refs/heads/"+pr.HeadBranch)
	if err != nil {
		return "", fmt.Errorf("head branch %q not found", pr.HeadBranch)
	}

	var newTip string
	switch strategy {
	case "merge":
		mc, err := mergeTreeCheck(dir, baseSHA, headSHA)
		if err != nil {
			return "", err
		}
		if !mc.Clean {
			return "", fmt.Errorf("merge conflicts in: %s", strings.Join(mc.Conflicts, ", "))
		}
		newTip, err = commitTree(dir, mc.TreeSHA, []string{baseSHA, headSHA}, message, u)
		if err != nil {
			return "", err
		}
	case "squash":
		mc, err := mergeTreeCheck(dir, baseSHA, headSHA)
		if err != nil {
			return "", err
		}
		if !mc.Clean {
			return "", fmt.Errorf("merge conflicts in: %s", strings.Join(mc.Conflicts, ", "))
		}
		newTip, err = commitTree(dir, mc.TreeSHA, []string{baseSHA}, message, u)
		if err != nil {
			return "", err
		}
	case "rebase":
		newTip, err = rebaseCommits(dir, baseSHA, headSHA, u)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unknown merge strategy %q", strategy)
	}

	if err := updateRefCAS(dir, "refs/heads/"+pr.BaseBranch, newTip, baseSHA); err != nil {
		return "", fmt.Errorf("base branch moved during merge, try again: %w", err)
	}
	return newTip, nil
}

// rebaseCommits replays base..head onto base one commit at a time using
// merge-tree cherry-picks, dropping commits that become empty. Returns the
// new tip (not yet written to any ref).
func rebaseCommits(dir, baseSHA, headSHA string, u *User) (string, error) {
	commits, err := commitRange(dir, baseSHA, headSHA)
	if err != nil {
		return "", err
	}
	if len(commits) == 0 {
		return "", errors.New("nothing to rebase")
	}
	target := baseSHA
	for _, c := range commits {
		if len(c.Parents) > 1 {
			return "", fmt.Errorf("cannot rebase merge commit %s; use a merge commit instead", c.ShortSHA)
		}
		parent := strings.Repeat("0", 40)
		if len(c.Parents) == 1 {
			parent = c.Parents[0]
		}
		// cherry-pick c onto target: 3-way merge with base = c's parent
		code, stdout, stderr := gitExitCode(dir, "merge-tree", "--write-tree", "--merge-base="+parent, target, c.SHA)
		if code != 0 {
			if code == 1 {
				return "", fmt.Errorf("rebase would conflict at commit %s (%s)", c.ShortSHA, c.Subject)
			}
			return "", fmt.Errorf("merge-tree failed: %s", strings.TrimSpace(stderr))
		}
		tree := strings.TrimSpace(strings.Split(stdout, "\n")[0])
		// drop commits that become empty (already applied upstream),
		// matching `git rebase --empty=drop`
		if targetTree, err := gitRun(dir, "rev-parse", target+"^{tree}"); err == nil && targetTree == tree {
			continue
		}
		msg := c.Subject
		if c.Body != "" {
			msg += "\n\n" + c.Body
		}
		target, err = commitTree(dir, tree, []string{target}, msg, u)
		if err != nil {
			return "", err
		}
	}
	if target == baseSHA {
		return "", errors.New("nothing to rebase: every commit is already in the base branch")
	}
	return target, nil
}

// fileAtCommit reads a file's content at a commit; returns nil if absent.
func fileAtCommit(dir, sha, path string) []byte {
	typ, out, err := readObject(dir, sha+":"+path)
	if err != nil || typ != "blob" {
		return nil
	}
	return out
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
