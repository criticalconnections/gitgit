package main

import (
	"strconv"
	"strings"
)

type DiffLine struct {
	Op     byte // ' ', '+', '-', '\\'
	OldNum int  // 0 when not present on that side
	NewNum int
	Text   string
}

type DiffHunk struct {
	Header string
	Lines  []DiffLine
}

type DiffFile struct {
	OldPath   string
	NewPath   string
	Status    string // modified | added | deleted | renamed
	IsBinary  bool
	Additions int
	Deletions int
	Hunks     []*DiffHunk
	Truncated bool
}

func (f *DiffFile) DisplayPath() string {
	if f.Status == "renamed" && f.OldPath != f.NewPath {
		return f.OldPath + " → " + f.NewPath
	}
	if f.NewPath != "" && f.NewPath != "/dev/null" {
		return f.NewPath
	}
	return f.OldPath
}

// Anchor is a stable id used for fragment links and review comment targets.
func (f *DiffFile) Anchor() string {
	p := f.DisplayPath()
	var b strings.Builder
	for _, c := range p {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			b.WriteRune(c)
		} else {
			b.WriteByte('-')
		}
	}
	return "d-" + b.String()
}

const maxDiffLinesPerFile = 3000

// parseUnifiedDiff parses `git diff` output into structured files.
func parseUnifiedDiff(raw string) []*DiffFile {
	var files []*DiffFile
	var cur *DiffFile
	var hunk *DiffHunk
	oldN, newN := 0, 0

	flushFile := func() {
		if cur != nil {
			files = append(files, cur)
		}
		cur, hunk = nil, nil
	}

	lines := strings.Split(raw, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			cur = &DiffFile{Status: "modified"}
			// paths parsed from ---/+++/rename lines below; fall back to header
			rest := strings.TrimPrefix(line, "diff --git ")
			if a, b, ok := splitGitPaths(rest); ok {
				cur.OldPath, cur.NewPath = a, b
			}
		case cur == nil:
			continue
		case strings.HasPrefix(line, "new file mode"):
			cur.Status = "added"
		case strings.HasPrefix(line, "deleted file mode"):
			cur.Status = "deleted"
		case strings.HasPrefix(line, "rename from "):
			cur.Status = "renamed"
			cur.OldPath = strings.TrimPrefix(line, "rename from ")
		case strings.HasPrefix(line, "rename to "):
			cur.Status = "renamed"
			cur.NewPath = strings.TrimPrefix(line, "rename to ")
		case strings.HasPrefix(line, "Binary files "), strings.HasPrefix(line, "GIT binary patch"):
			cur.IsBinary = true
		case strings.HasPrefix(line, "--- "):
			p := strings.TrimPrefix(line, "--- ")
			if p != "/dev/null" {
				cur.OldPath = strings.TrimPrefix(p, "a/")
			}
		case strings.HasPrefix(line, "+++ "):
			p := strings.TrimPrefix(line, "+++ ")
			if p != "/dev/null" {
				cur.NewPath = strings.TrimPrefix(p, "b/")
			}
		case strings.HasPrefix(line, "@@"):
			hunk = &DiffHunk{Header: line}
			oldN, newN = parseHunkHeader(line)
			cur.Hunks = append(cur.Hunks, hunk)
		case hunk != nil && len(line) > 0:
			if countHunkLines(cur) > maxDiffLinesPerFile {
				cur.Truncated = true
				hunk = nil
				continue
			}
			dl := DiffLine{Op: line[0], Text: line[1:]}
			switch line[0] {
			case '+':
				dl.NewNum = newN
				newN++
				cur.Additions++
			case '-':
				dl.OldNum = oldN
				oldN++
				cur.Deletions++
			case ' ':
				dl.OldNum, dl.NewNum = oldN, newN
				oldN++
				newN++
			case '\\': // "\ No newline at end of file"
			default:
				continue
			}
			hunk.Lines = append(hunk.Lines, dl)
		case hunk != nil && line == "":
			// context line that is empty (leading space stripped by Split edge)
			dl := DiffLine{Op: ' ', OldNum: oldN, NewNum: newN, Text: ""}
			oldN++
			newN++
			hunk.Lines = append(hunk.Lines, dl)
		}
	}
	flushFile()
	return files
}

func countHunkLines(f *DiffFile) int {
	n := 0
	for _, h := range f.Hunks {
		n += len(h.Lines)
	}
	return n
}

// parseHunkHeader extracts starting line numbers from "@@ -a,b +c,d @@".
func parseHunkHeader(h string) (oldStart, newStart int) {
	oldStart, newStart = 1, 1
	parts := strings.Fields(h)
	for _, p := range parts {
		if strings.HasPrefix(p, "-") {
			nums := strings.SplitN(p[1:], ",", 2)
			if n, err := strconv.Atoi(nums[0]); err == nil {
				oldStart = n
			}
		} else if strings.HasPrefix(p, "+") {
			nums := strings.SplitN(p[1:], ",", 2)
			if n, err := strconv.Atoi(nums[0]); err == nil {
				newStart = n
			}
		}
	}
	return
}

// splitGitPaths splits `a/foo b/foo` (best effort; quoted paths handled crudely).
func splitGitPaths(rest string) (string, string, bool) {
	if strings.HasPrefix(rest, "\"") {
		return "", "", false // exotic quoted paths: rely on ---/+++ lines
	}
	// try the midpoint split: "a/X b/X" where X may contain spaces
	if idx := strings.Index(rest, " b/"); idx >= 0 {
		a := strings.TrimPrefix(rest[:idx], "a/")
		b := rest[idx+3:]
		return a, b, true
	}
	return "", "", false
}

type DiffStat struct {
	Files     int
	Additions int
	Deletions int
}

func diffStats(files []*DiffFile) DiffStat {
	s := DiffStat{Files: len(files)}
	for _, f := range files {
		s.Additions += f.Additions
		s.Deletions += f.Deletions
	}
	return s
}
