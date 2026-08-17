package main

// Import a repository from an uploaded .zip archive.
//
// Two shapes are accepted:
//   - a plain folder of files (the common case: "Download ZIP" from a host,
//     or a project directory) — becomes a repository with one initial commit
//   - an archive that contains a .git directory — its history is preserved
//
// Archives are hostile input, so extraction enforces: no path escaping the
// destination (zip-slip), no symlinks, no absolute paths, and caps on entry
// count and uncompressed size (zip bombs).

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	maxZipUpload     = 500 << 20 // 500 MB compressed
	maxZipEntries    = 20000
	maxZipTotalBytes = 2 << 30 // 2 GB uncompressed
	maxZipFileBytes  = 200 << 20
)

// safeExtractZip unpacks r into dest, refusing anything that would write
// outside it. Returns the number of files written.
func safeExtractZip(r *zip.Reader, dest string) (int, error) {
	if len(r.File) > maxZipEntries {
		return 0, fmt.Errorf("archive has too many entries (%d, limit %d)", len(r.File), maxZipEntries)
	}
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return 0, err
	}
	var total uint64
	written := 0

	for _, f := range r.File {
		name := f.Name
		// Reject absolute paths and Windows drive letters outright.
		if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) ||
			(len(name) > 1 && name[1] == ':') {
			return 0, fmt.Errorf("archive contains an absolute path: %q", name)
		}
		// Refuse traversal segments explicitly. Anchoring the path below would
		// already neutralize them, but silently rewriting "../../etc/passwd"
		// into a real file inside the repository hides a hostile archive
		// instead of reporting it.
		for _, seg := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
			if seg == ".." {
				return 0, fmt.Errorf("archive entry tries to escape with %q: %q", "..", name)
			}
		}
		// Normalize, then verify the result is still inside dest. This is the
		// zip-slip check: "a/../../etc/passwd" cleans to "../etc/passwd".
		target := filepath.Join(destAbs, filepath.Clean("/"+name))
		rel, err := filepath.Rel(destAbs, target)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return 0, fmt.Errorf("archive entry would escape the destination: %q", name)
		}

		info := f.FileInfo()
		switch {
		case info.IsDir():
			if err := os.MkdirAll(target, 0o755); err != nil {
				return 0, err
			}
			continue
		case info.Mode()&os.ModeSymlink != 0:
			// A symlink could point anywhere; skip rather than fail, since
			// archives of real projects often contain a few.
			continue
		case !info.Mode().IsRegular():
			continue // devices, fifos, etc.
		}

		if f.UncompressedSize64 > maxZipFileBytes {
			return 0, fmt.Errorf("%q is too large (%d bytes)", name, f.UncompressedSize64)
		}
		total += f.UncompressedSize64
		if total > maxZipTotalBytes {
			return 0, fmt.Errorf("archive expands to more than %d bytes", uint64(maxZipTotalBytes))
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return 0, err
		}
		rc, err := f.Open()
		if err != nil {
			return 0, err
		}
		mode := info.Mode().Perm()
		if mode == 0 {
			mode = 0o644
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			rc.Close()
			return 0, err
		}
		// LimitReader is belt-and-braces: the header size could lie.
		_, err = io.Copy(out, io.LimitReader(rc, maxZipFileBytes+1))
		out.Close()
		rc.Close()
		if err != nil {
			return 0, err
		}
		written++
	}
	return written, nil
}

// stripSingleRoot returns the directory to import from. Archives usually wrap
// everything in one folder ("myproject-main/"); importing that folder's
// contents is almost always what the uploader means.
func stripSingleRoot(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return dir
	}
	visible := entries[:0]
	for _, e := range entries {
		if e.Name() == "__MACOSX" || e.Name() == ".DS_Store" {
			continue
		}
		visible = append(visible, e)
	}
	if len(visible) == 1 && visible[0].IsDir() {
		return filepath.Join(dir, visible[0].Name())
	}
	return dir
}

// ZipImportRequest describes an upload.
type ZipImportRequest struct {
	Name     string
	Private  bool
	Filename string
}

// startZipImport extracts an uploaded archive into a new repository. It runs
// synchronously: the upload has already been received, and extraction plus a
// single commit is fast relative to a network clone.
func startZipImport(u *User, r io.ReaderAt, size int64, req ZipImportRequest) (*ImportJob, *Repo, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		// derive from the filename: "my-project-main.zip" -> "my-project-main"
		base := filepath.Base(req.Filename)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	name = strings.TrimSpace(name)
	if !validSlug(name) {
		return nil, nil, fmt.Errorf("%q is not a valid repository name", name)
	}
	if existing, err := getRepo(u.Username, name); err == nil && existing != nil {
		return nil, nil, fmt.Errorf("you already have a repository named %q", name)
	}

	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, nil, fmt.Errorf("not a readable .zip archive")
	}

	job := newImportJob(u.ID, req.Filename)
	work, err := os.MkdirTemp(dataDir, "zipimport-")
	if err != nil {
		return nil, nil, err
	}
	defer os.RemoveAll(work)

	job.log("Extracting %s", req.Filename)
	n, err := safeExtractZip(zr, work)
	if err != nil {
		job.finish("failed", err.Error(), 0)
		return job, nil, err
	}
	if n == 0 {
		err := fmt.Errorf("the archive contains no files")
		job.finish("failed", err.Error(), 0)
		return job, nil, err
	}
	job.log("Extracted %d file(s)", n)

	src := stripSingleRoot(work)
	repo, err := insertRepo(u.ID, name, "Imported from "+filepath.Base(req.Filename), req.Private)
	if err != nil {
		job.finish("failed", err.Error(), 0)
		return job, nil, err
	}
	dest := repo.DiskPath()

	// An archive of an actual repository keeps its history.
	if fi, err := os.Stat(filepath.Join(src, ".git")); err == nil && fi.IsDir() {
		job.log("Found a .git directory — preserving history")
		os.RemoveAll(dest)
		if _, err := gitRun("", "clone", "--mirror", src, dest); err != nil {
			deleteRepoRows(repo.ID)
			os.RemoveAll(dest)
			job.finish("failed", "could not read the repository inside the archive", 0)
			return job, nil, fmt.Errorf("could not read the repository inside the archive")
		}
		gitRun(dest, "remote", "remove", "origin")
		if head, err := gitRun(dest, "symbolic-ref", "--short", "HEAD"); err == nil && branchExists(dest, head) {
			repo.DefaultBranch = head
			updateRepoMeta(repo)
		}
	} else {
		job.log("Creating the initial commit")
		if err := initBareRepo(dest, repo.DefaultBranch); err != nil {
			deleteRepoRows(repo.ID)
			job.finish("failed", err.Error(), 0)
			return job, nil, err
		}
		if err := commitDirectory(src, dest, repo.DefaultBranch, u, "Initial commit"); err != nil {
			deleteRepoRows(repo.ID)
			os.RemoveAll(dest)
			job.finish("failed", err.Error(), 0)
			return job, nil, err
		}
	}

	branches := listBranches(dest)
	if sha, err := resolveCommit(dest, "refs/heads/"+repo.DefaultBranch); err == nil {
		enqueueCI(repo, sha, repo.DefaultBranch, "push")
	}
	job.log("Done.")
	job.finish("done", fmt.Sprintf("imported %d file(s) into %d branch(es)", n, len(branches)), repo.ID)
	return job, repo, nil
}

// commitDirectory turns a plain directory into the first commit of a bare
// repository, using a temporary work tree and index.
func commitDirectory(src, bare, branch string, u *User, message string) error {
	if _, err := gitRun(src, "init", "--quiet", "--initial-branch="+branch); err != nil {
		return fmt.Errorf("could not initialize a repository: %w", err)
	}
	// A stray .gitmodules with no submodules present would break `add`.
	if _, err := gitRunBytes(src, nil, identityEnv(u), "add", "-A", "--", "."); err != nil {
		return fmt.Errorf("could not stage the files: %w", err)
	}
	env := append(identityEnv(u), "GIT_COMMITTER_DATE=", "GIT_AUTHOR_DATE=")
	if _, err := gitRunBytes(src, nil, env, "commit", "--quiet", "-m", message, "--allow-empty"); err != nil {
		return fmt.Errorf("could not create the initial commit: %w", err)
	}
	if _, err := gitRun(src, "push", "--quiet", bare, branch+":refs/heads/"+branch); err != nil {
		return fmt.Errorf("could not write the commit into the repository: %w", err)
	}
	os.RemoveAll(filepath.Join(src, ".git"))
	return nil
}
