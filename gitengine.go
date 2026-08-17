package main

// Object-read engine adapted from Gitea's modules/git cat-file batch design
// (gitea-main/modules/git/catfile_batch*.go, MIT licensed — see NOTICE).
//
// Instead of spawning one `git cat-file` process per object read, each bare
// repository keeps a single long-lived `git cat-file --batch-command` child.
// Requests are serialized per repo; any protocol hiccup kills the child and
// the caller transparently falls back to one-shot execution.

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

type catFileBatch struct {
	mu  sync.Mutex
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

var (
	batchesMu sync.Mutex
	batches   = map[string]*catFileBatch{}
)

func repoBatch(dir string) *catFileBatch {
	batchesMu.Lock()
	defer batchesMu.Unlock()
	if b, ok := batches[dir]; ok {
		return b
	}
	b := &catFileBatch{}
	batches[dir] = b
	return b
}

// closeRepoBatch shuts down the batch process for a repo (called on delete).
func closeRepoBatch(dir string) {
	batchesMu.Lock()
	b, ok := batches[dir]
	delete(batches, dir)
	batchesMu.Unlock()
	if ok {
		b.mu.Lock()
		b.killLocked()
		b.mu.Unlock()
	}
}

func (b *catFileBatch) startLocked(dir string) error {
	cmd := exec.Command("git", "cat-file", "--batch-command")
	cmd.Dir = dir
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		in.Close()
		return err
	}
	if err := cmd.Start(); err != nil {
		in.Close()
		return err
	}
	go cmd.Wait() // reap on exit; state is reset lazily on next use
	b.cmd, b.in, b.out = cmd, in, bufio.NewReaderSize(out, 32*1024)
	return nil
}

func (b *catFileBatch) killLocked() {
	if b.cmd != nil && b.cmd.Process != nil {
		b.in.Close()
		b.cmd.Process.Kill()
	}
	b.cmd, b.in, b.out = nil, nil, nil
}

var errObjectMissing = fmt.Errorf("object missing")

// contents fetches an object's type and content via the batch process.
// obj may be any revision git understands (sha, ref, "sha:path", …).
func (b *catFileBatch) contents(dir, obj string) (string, []byte, error) {
	if strings.ContainsAny(obj, "\n\r") || strings.HasPrefix(obj, "-") {
		return "", nil, fmt.Errorf("invalid object spec")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil {
		if err := b.startLocked(dir); err != nil {
			return "", nil, err
		}
	}
	typ, data, err := b.contentsLocked(obj)
	if err != nil && err != errObjectMissing {
		// protocol desync or dead child: kill and retry once on a fresh process
		b.killLocked()
		if err2 := b.startLocked(dir); err2 != nil {
			return "", nil, err
		}
		typ, data, err = b.contentsLocked(obj)
		if err != nil && err != errObjectMissing {
			b.killLocked()
		}
	}
	return typ, data, err
}

func (b *catFileBatch) contentsLocked(obj string) (string, []byte, error) {
	if _, err := fmt.Fprintf(b.in, "contents %s\n", obj); err != nil {
		return "", nil, err
	}
	header, err := b.out.ReadString('\n')
	if err != nil {
		return "", nil, err
	}
	header = strings.TrimSuffix(header, "\n")
	fields := strings.Fields(header)
	if len(fields) == 2 && (fields[1] == "missing" || fields[1] == "ambiguous") {
		return "", nil, errObjectMissing
	}
	if len(fields) != 3 {
		return "", nil, fmt.Errorf("unexpected cat-file header %q", header)
	}
	size, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return "", nil, fmt.Errorf("bad size in cat-file header %q", header)
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(b.out, data); err != nil {
		return "", nil, err
	}
	// each object is followed by a newline
	if _, err := b.out.Discard(1); err != nil {
		return "", nil, err
	}
	return fields[1], data, nil
}

// info fetches an object's type/size without content.
func (b *catFileBatch) info(dir, obj string) (string, int64, error) {
	if strings.ContainsAny(obj, "\n\r") || strings.HasPrefix(obj, "-") {
		return "", 0, fmt.Errorf("invalid object spec")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil {
		if err := b.startLocked(dir); err != nil {
			return "", 0, err
		}
	}
	if _, err := fmt.Fprintf(b.in, "info %s\n", obj); err != nil {
		b.killLocked()
		return "", 0, err
	}
	header, err := b.out.ReadString('\n')
	if err != nil {
		b.killLocked()
		return "", 0, err
	}
	fields := strings.Fields(strings.TrimSuffix(header, "\n"))
	if len(fields) == 2 && (fields[1] == "missing" || fields[1] == "ambiguous") {
		return "", 0, errObjectMissing
	}
	if len(fields) != 3 {
		b.killLocked()
		return "", 0, fmt.Errorf("unexpected cat-file header %q", header)
	}
	size, _ := strconv.ParseInt(fields[2], 10, 64)
	return fields[1], size, nil
}

// readObject reads an object through the batch engine, falling back to
// one-shot `git cat-file` if the engine is unavailable.
func readObject(dir, spec string) (string, []byte, error) {
	typ, data, err := repoBatch(dir).contents(dir, spec)
	if err == nil || err == errObjectMissing {
		if err == errObjectMissing {
			return "", nil, fmt.Errorf("object not found: %s", spec)
		}
		return typ, data, nil
	}
	log.Printf("catfile batch fallback for %s (%v)", dir, err)
	out, err := gitRunBytes(dir, nil, nil, "cat-file", "blob", spec)
	if err != nil {
		return "", nil, err
	}
	return "blob", out, nil
}

// ---------- tags (modeled on Gitea's repo_tag listing) ----------

type TagInfo struct {
	Name    string
	SHA     string // commit the tag points at (peeled for annotated tags)
	When    int64
	Subject string
}

func listTags(dir string) []TagInfo {
	out, err := gitRun(dir, "for-each-ref", "refs/tags", "--sort=-creatordate",
		"--format=%(refname:short)%00%(objectname)%00%(*objectname)%00%(creatordate:unix)%00%(contents:subject)")
	if err != nil || out == "" {
		return nil
	}
	var tags []TagInfo
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, "\x00", 5)
		if len(parts) != 5 {
			continue
		}
		sha := parts[1]
		if parts[2] != "" {
			sha = parts[2] // annotated tag: use the peeled commit
		}
		when, _ := strconv.ParseInt(parts[3], 10, 64)
		tags = append(tags, TagInfo{Name: parts[0], SHA: sha, When: when, Subject: parts[4]})
	}
	return tags
}
