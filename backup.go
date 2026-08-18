package main

// Backups: one archive containing everything needed to rebuild this instance.
//
// The database is snapshotted with `VACUUM INTO`, not copied. Copying a live
// SQLite file in WAL mode can capture a torn page or miss committed data
// sitting in the -wal, producing an archive that restores into a corrupt
// database — and you would not find out until the day you needed it.
// VACUUM INTO takes a read lock and writes a consistent, already-compacted
// database, which is exactly what a backup should be.
//
// Repositories are bare, so they are copied as they are. A repository being
// pushed to *during* a backup may be captured mid-update; git's own atomicity
// means the worst case is a ref pointing at an object that arrived after the
// pack was read, which `git fsck` reports and a re-push fixes. Stopping the
// world to avoid that would make backups something people switch off.
//
// The secret key is deliberately NOT included: an archive holding both the
// encrypted secrets and the key that opens them protects nothing. Back the
// key up separately, and the restore instructions say so.

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const backupKeepDefault = 7

var backupMu sync.Mutex // one backup at a time; they are IO-bound

type BackupFile struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

func backupDir() string { return filepath.Join(dataDir, "backups") }

// createBackup writes a gzipped tar of the database snapshot and every bare
// repository, and returns its filename.
func createBackup() (string, error) {
	backupMu.Lock()
	defer backupMu.Unlock()

	if err := os.MkdirAll(backupDir(), 0o700); err != nil {
		return "", err
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	name := "gitgit-" + stamp + ".tar.gz"
	path := filepath.Join(backupDir(), name)

	// A consistent database snapshot, written outside the archive first so a
	// failure here never produces a half-written backup.
	snapshot := filepath.Join(backupDir(), ".snapshot-"+stamp+".db")
	defer os.Remove(snapshot)
	if _, err := db.Exec("VACUUM INTO ?", snapshot); err != nil {
		return "", fmt.Errorf("snapshot the database: %w", err)
	}

	tmp := path + ".partial"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	fail := func(err error) (string, error) {
		tw.Close()
		gz.Close()
		f.Close()
		os.Remove(tmp)
		return "", err
	}

	if err := addFileToTar(tw, snapshot, "gitgit.db"); err != nil {
		return fail(err)
	}
	repoRoot := filepath.Join(dataDir, "repos")
	if err := addTreeToTar(tw, repoRoot, "repos"); err != nil {
		return fail(err)
	}
	if err := tw.Close(); err != nil {
		return fail(err)
	}
	if err := gz.Close(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		return fail(err)
	}
	// Rename last: a reader can never observe a partial archive under its
	// final name.
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	log.Printf("backup: wrote %s", name)
	return name, nil
}

func addFileToTar(tw *tar.Writer, path, name string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	hdr, err := tar.FileInfoHeader(st, "")
	if err != nil {
		return err
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(tw, src)
	return err
}

func addTreeToTar(tw *tar.Writer, root, prefix string) error {
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := filepath.ToSlash(filepath.Join(prefix, rel))
		// Symlinks are stored as links, never followed: following one would
		// copy whatever it points at into the archive.
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			hdr, err := tar.FileInfoHeader(info, target)
			if err != nil {
				return err
			}
			hdr.Name = name
			return tw.WriteHeader(hdr)
		}
		if info.IsDir() {
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = name + "/"
			return tw.WriteHeader(hdr)
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		return addFileToTar(tw, path, name)
	})
}

func listBackups() []BackupFile {
	entries, err := os.ReadDir(backupDir())
	if err != nil {
		return []BackupFile{}
	}
	out := []BackupFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".tar.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupFile{Name: e.Name(), Size: info.Size(), CreatedAt: info.ModTime().Unix()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name > out[j].Name })
	return out
}

// pruneBackups keeps the newest `keep` archives.
func pruneBackups(keep int) {
	if keep <= 0 {
		return
	}
	all := listBackups()
	for i := keep; i < len(all); i++ {
		if err := os.Remove(filepath.Join(backupDir(), all[i].Name)); err == nil {
			log.Printf("backup: pruned %s", all[i].Name)
		}
	}
}

// safeBackupName refuses anything that is not a plain archive filename, so a
// download or delete cannot be pointed at another file on disk.
func safeBackupName(name string) (string, bool) {
	if name == "" || name != filepath.Base(name) || strings.Contains(name, "..") {
		return "", false
	}
	if !strings.HasPrefix(name, "gitgit-") || !strings.HasSuffix(name, ".tar.gz") {
		return "", false
	}
	return filepath.Join(backupDir(), name), true
}

// scheduleBackups runs a backup every interval, keeping `keep` of them.
func scheduleBackups(every time.Duration, keep int) {
	if every <= 0 {
		return
	}
	go func() {
		for {
			time.Sleep(every)
			if _, err := createBackup(); err != nil {
				log.Printf("backup: scheduled run failed: %v", err)
				continue
			}
			pruneBackups(keep)
		}
	}()
	log.Printf("backup: scheduled every %s, keeping %d", every, keep)
}

// ---------- API ----------

// handleAPIBackups is site-admin only: an archive contains every private
// repository and every user row on the instance.
func handleAPIBackups(c *apiCtx, rest []string) {
	if c.u == nil || !c.u.IsAdmin {
		c.err(403, "site admin access required")
		return
	}
	switch {
	case len(rest) == 0 && c.r.Method == http.MethodGet:
		c.out(200, map[string]any{"backups": listBackups(), "directory": backupDir()})

	case len(rest) == 0 && c.r.Method == http.MethodPost:
		name, err := createBackup()
		if err != nil {
			c.err(500, err.Error())
			return
		}
		pruneBackups(backupKeep)
		c.out(201, map[string]any{"name": name})

	case len(rest) == 1 && c.r.Method == http.MethodDelete:
		path, ok := safeBackupName(rest[0])
		if !ok {
			c.err(422, "not a backup filename")
			return
		}
		os.Remove(path)
		c.out(200, map[string]bool{"ok": true})

	case len(rest) == 2 && rest[1] == "download" && c.r.Method == http.MethodGet:
		path, ok := safeBackupName(rest[0])
		if !ok {
			c.err(422, "not a backup filename")
			return
		}
		f, err := os.Open(path)
		if err != nil {
			c.err(404, "no such backup")
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			c.err(500, "cannot read that backup")
			return
		}
		c.w.Header().Set("Content-Type", "application/gzip")
		c.w.Header().Set("Content-Length", fmt.Sprint(st.Size()))
		c.w.Header().Set("Content-Disposition", `attachment; filename="`+rest[0]+`"`)
		io.Copy(c.w, f)

	default:
		c.err(404, "unknown endpoint")
	}
}
