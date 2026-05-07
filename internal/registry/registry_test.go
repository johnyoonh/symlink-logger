package registry

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanFindsTopLevelSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "old")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "regular"), 0o755); err != nil {
		t.Fatal(err)
	}

	candidates, err := Scan(root, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	if candidates[0].OldPath != filepath.Join(root, "old") {
		t.Fatalf("OldPath = %q", candidates[0].OldPath)
	}
	if candidates[0].TargetPath != target {
		t.Fatalf("TargetPath = %q", candidates[0].TargetPath)
	}
}

func TestRecursiveScanFindsNestedSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(nested, "old")); err != nil {
		t.Fatal(err)
	}

	topLevel, err := ScanWithOptions(root, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(topLevel) != 0 {
		t.Fatalf("top-level len = %d, want 0", len(topLevel))
	}

	recursive, err := ScanWithOptions(root, time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(recursive) != 1 {
		t.Fatalf("recursive len = %d, want 1", len(recursive))
	}
	if recursive[0].Source != "recursive-symlink" {
		t.Fatalf("Source = %q", recursive[0].Source)
	}
}

func TestTSVRoundTripAndFind(t *testing.T) {
	candidates := []Candidate{{
		OldPath:      "/old",
		TargetPath:   "/target",
		DiscoveredOn: "2026-05-07",
		Source:       "test",
	}}
	var buf bytes.Buffer
	if err := WriteTSV(&buf, candidates); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "candidates.tsv")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadTSV(path)
	if err != nil {
		t.Fatal(err)
	}
	candidate, ok := Find(got, "/old")
	if !ok {
		t.Fatal("Find did not find /old")
	}
	if candidate.TargetPath != "/target" {
		t.Fatalf("TargetPath = %q", candidate.TargetPath)
	}
}

func TestRestoreReplacesEmptyDirectoryWithSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	old := filepath.Join(root, "old")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(old, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := Restore(Candidate{OldPath: old, TargetPath: target}); err != nil {
		t.Fatal(err)
	}
	got, err := os.Readlink(old)
	if err != nil {
		t.Fatal(err)
	}
	if got != target {
		t.Fatalf("Readlink = %q, want %q", got, target)
	}
}

func TestReplaceReplacesSymlinkWithMountpointDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	old := filepath.Join(root, "old")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, old); err != nil {
		t.Fatal(err)
	}

	if err := Replace(Candidate{OldPath: old, TargetPath: target}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(old)
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory after replace", old)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s is still a symlink after replace", old)
	}
}
