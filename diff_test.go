package main

import "testing"

const sampleDiff = `diff --git a/greet.sh b/greet.sh
index 1234567..89abcde 100644
--- a/greet.sh
+++ b/greet.sh
@@ -1,3 +1,4 @@
 #!/bin/bash
-echo "hello"
+echo "hello, world"
+echo "bye"
diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..e69de29
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+fresh file
diff --git a/gone.txt b/gone.txt
deleted file mode 100644
index e69de29..0000000
--- a/gone.txt
+++ /dev/null
@@ -1 +0,0 @@
-old line
diff --git a/img.png b/img.png
index 1111111..2222222 100644
Binary files a/img.png and b/img.png differ
`

func TestParseUnifiedDiff(t *testing.T) {
	files := parseUnifiedDiff(sampleDiff)
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d", len(files))
	}

	f := files[0]
	if f.NewPath != "greet.sh" || f.Status != "modified" {
		t.Errorf("file 0: got path=%q status=%q", f.NewPath, f.Status)
	}
	if f.Additions != 2 || f.Deletions != 1 {
		t.Errorf("file 0: got +%d -%d, want +2 -1", f.Additions, f.Deletions)
	}
	if len(f.Hunks) != 1 {
		t.Fatalf("file 0: want 1 hunk, got %d", len(f.Hunks))
	}
	lines := f.Hunks[0].Lines
	if lines[0].Op != ' ' || lines[0].OldNum != 1 || lines[0].NewNum != 1 {
		t.Errorf("line 0: got op=%q old=%d new=%d", lines[0].Op, lines[0].OldNum, lines[0].NewNum)
	}
	if lines[1].Op != '-' || lines[1].OldNum != 2 || lines[1].NewNum != 0 {
		t.Errorf("line 1: got op=%q old=%d new=%d", lines[1].Op, lines[1].OldNum, lines[1].NewNum)
	}
	if lines[2].Op != '+' || lines[2].NewNum != 2 {
		t.Errorf("line 2: got op=%q new=%d", lines[2].Op, lines[2].NewNum)
	}

	if files[1].Status != "added" || files[1].NewPath != "new.txt" {
		t.Errorf("file 1: got status=%q path=%q", files[1].Status, files[1].NewPath)
	}
	if files[2].Status != "deleted" || files[2].OldPath != "gone.txt" {
		t.Errorf("file 2: got status=%q path=%q", files[2].Status, files[2].OldPath)
	}
	if !files[3].IsBinary {
		t.Errorf("file 3: want binary")
	}

	st := diffStats(files)
	if st.Files != 4 || st.Additions != 3 || st.Deletions != 2 {
		t.Errorf("stats: got %+v", st)
	}
}

func TestParseHunkHeader(t *testing.T) {
	o, n := parseHunkHeader("@@ -10,4 +20,6 @@ func main() {")
	if o != 10 || n != 20 {
		t.Errorf("got old=%d new=%d, want 10, 20", o, n)
	}
}

func TestSplitGitPaths(t *testing.T) {
	a, b, ok := splitGitPaths("a/dir with space/f.txt b/dir with space/f.txt")
	if !ok || a != "dir with space/f.txt" || b != "dir with space/f.txt" {
		t.Errorf("got a=%q b=%q ok=%v", a, b, ok)
	}
}

func TestValidSlug(t *testing.T) {
	for _, good := range []string{"demo", "my-repo", "a.b_c", "X9"} {
		if !validSlug(good) {
			t.Errorf("%q should be valid", good)
		}
	}
	for _, bad := range []string{"", ".hidden", "has space", "trick.git", "a/b"} {
		if validSlug(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
