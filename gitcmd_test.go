package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func stringsReader(s string) *strings.Reader { return strings.NewReader(s) }

func trim(b []byte) string { return strings.TrimSpace(string(b)) }

// setupTestRepo creates a bare repo with a main branch (README) and a
// feature branch with one extra commit, returning the bare repo path.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	bare := filepath.Join(tmp, "repo.git")
	if err := initBareRepo(bare, "main"); err != nil {
		t.Fatalf("init bare: %v", err)
	}
	u := &User{Username: "tester", Email: "t@example.com"}
	if err := seedInitialCommit(bare, "main", u, "repo", "test repo"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	work := filepath.Join(tmp, "work")
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=tester", "GIT_AUTHOR_EMAIL=t@example.com",
			"GIT_COMMITTER_NAME=tester", "GIT_COMMITTER_EMAIL=t@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(tmp, "clone", bare, work)
	run(work, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(work, "feature.txt"), []byte("feature work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(work, "add", "-A")
	run(work, "commit", "-m", "Add feature file")
	run(work, "push", "origin", "feature")
	return bare
}

func TestMergeStrategies(t *testing.T) {
	u := &User{Username: "tester", Email: "t@example.com"}
	for _, strategy := range []string{"merge", "squash", "rebase"} {
		t.Run(strategy, func(t *testing.T) {
			bare := setupTestRepo(t)
			pr := &Pull{Number: 1, Title: "Add feature", BaseBranch: "main", HeadBranch: "feature"}
			tip, err := mergePR(bare, pr, strategy, "Merge feature (#1)", u)
			if err != nil {
				t.Fatalf("mergePR(%s): %v", strategy, err)
			}
			mainSHA, err := resolveCommit(bare, "refs/heads/main")
			if err != nil || mainSHA != tip {
				t.Fatalf("main not moved to %s (got %s, err %v)", short(tip), short(mainSHA), err)
			}
			if fileAtCommit(bare, mainSHA, "feature.txt") == nil {
				t.Errorf("feature.txt missing from merged main")
			}
			commit, err := getCommit(bare, mainSHA)
			if err != nil {
				t.Fatal(err)
			}
			switch strategy {
			case "merge":
				if len(commit.Parents) != 2 {
					t.Errorf("merge commit should have 2 parents, got %d", len(commit.Parents))
				}
			case "squash", "rebase":
				if len(commit.Parents) != 1 {
					t.Errorf("%s tip should have 1 parent, got %d", strategy, len(commit.Parents))
				}
			}
		})
	}
}

func TestMergeConflictDetected(t *testing.T) {
	u := &User{Username: "tester", Email: "t@example.com"}
	bare := setupTestRepo(t)

	// move main by rewriting README so feature's ancestor context conflicts
	readme, _ := gitRunBytes(bare, nil, nil, "cat-file", "blob", "refs/heads/main:README.md")
	_ = readme
	blob, err := gitRunBytes(bare, stringsReader("completely different\n"), nil, "hash-object", "-w", "--stdin")
	if err != nil {
		t.Fatal(err)
	}
	tree, err := gitRunBytes(bare, stringsReader("100644 blob "+trim(blob)+"\tfeature.txt\n"), nil, "mktree")
	if err != nil {
		t.Fatal(err)
	}
	mainSHA, _ := resolveCommit(bare, "refs/heads/main")
	newMain, err := commitTree(bare, trim(tree), []string{mainSHA}, "conflicting feature.txt on main", u)
	if err != nil {
		t.Fatal(err)
	}
	if err := updateRefCAS(bare, "refs/heads/main", newMain, mainSHA); err != nil {
		t.Fatal(err)
	}

	mc, err := mergeTreeCheck(bare, newMain, mustResolve(t, bare, "refs/heads/feature"))
	if err != nil {
		t.Fatal(err)
	}
	if mc.Clean {
		t.Fatal("expected conflict, got clean merge")
	}
	if len(mc.Conflicts) == 0 {
		t.Error("expected conflict file list")
	}
}

func mustResolve(t *testing.T, dir, ref string) string {
	t.Helper()
	sha, err := resolveCommit(dir, ref)
	if err != nil {
		t.Fatalf("resolve %s: %v", ref, err)
	}
	return sha
}
